//go:build windows

package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

const stillActive = 259

// detachedProcAttr starts a process fully independent of this one: the worker
// must keep running after ApplyUpdate calls application.Get().Quit(), and the
// relaunched launcher must not be tied to the worker's lifetime either.
func detachedProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
	}
}

// newWorkerContexts derives two independent timeout budgets from a single
// base context: waiting for the parent launcher to exit and running the
// installer must not share one clock. Sharing it meant that whenever
// waitForParentExit ate most of parentExitTimeout (antivirus holding a
// handle, a slow webview teardown), Apply/exec.CommandContext inherited
// whatever was left and could kill the installer mid-write over the
// launcher's own files, with no recovery path on the next start.
func newWorkerContexts(base context.Context) (waitCtx, applyCtx context.Context, cancel func()) {
	waitCtx, waitCancel := context.WithTimeout(base, parentExitTimeout)
	applyCtx, applyCancel := context.WithTimeout(base, applyTimeout)
	return waitCtx, applyCtx, func() {
		waitCancel()
		applyCancel()
	}
}

// RunWorker is the entry point of the separate --selfupdate-worker process:
// it is started right before the launcher quits, so there is no caller ctx to
// thread through (invariant 20 permits context.Background only in main, and
// this is the worker's equivalent of main).
func RunWorker(specPath string) error {
	spec, err := readUpdateSpec(specPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := os.Remove(specPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("remove selfupdate spec", "path", specPath, "error", err)
		}
	}()

	//nolint:forbidigo // RunWorker is a separate process's entry point, the worker's equivalent of main: there is no caller ctx (invariant 20 allows Background only in main). This base feeds both waitCtx (parent-exit wait) and applyCtx (installer run) below, each with its own independent timeout.
	waitCtx, applyCtx, cancel := newWorkerContexts(context.Background())
	defer cancel()

	if err := waitForParentExit(waitCtx, spec.ParentPID); err != nil {
		slog.Error("selfupdate worker: parent still running", "error", err)
		return err
	}

	if err := Apply(applyCtx, spec.InstallerPath); err != nil {
		slog.Error("selfupdate worker: apply failed", "error", err)
		return err
	}

	if err := relaunch(spec.RelaunchPath); err != nil {
		slog.Error("selfupdate worker: relaunch failed", "error", err)
		return err
	}
	return nil
}

func waitForParentExit(ctx context.Context, pid int) error {
	ticker := time.NewTicker(parentPollInterval)
	defer ticker.Stop()
	for {
		alive, err := workerProcessAlive(pid)
		if err != nil {
			return err
		}
		if !alive {
			return nil
		}
		select {
		case <-ctx.Done():
			return errParentStillRunning
		case <-ticker.C:
		}
	}
}

// workerProcessAlive answers "is this pid alive" the way ServiceStartup's
// stale-worker recovery needs: os.FindProcess always succeeds on Windows
// regardless of whether the process is alive, so OpenProcess +
// GetExitCodeProcess is the only reliable way. This mirrors
// internal/install/runner_windows.go's workerProcessAlive; that function is
// unexported there, so this package keeps its own copy.
func workerProcessAlive(pid int) (bool, error) {
	if pid <= 0 {
		return false, nil
	}
	//nolint:gosec // G115: pid was read back from our own spec file, always fits uint32
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return false, nil
		}
		return false, fmt.Errorf("open parent process %d: %w", pid, err)
	}
	defer func() {
		if cerr := windows.CloseHandle(handle); cerr != nil {
			slog.Warn("close parent process handle", "pid", pid, "error", cerr)
		}
	}()
	var code uint32
	if err := windows.GetExitCodeProcess(handle, &code); err != nil {
		return false, fmt.Errorf("parent process %d exit code: %w", pid, err)
	}
	return code == stillActive, nil
}

func relaunch(path string) error {
	if path == "" {
		return errors.New("selfupdate: relaunch path is empty")
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("stat relaunch target: %w", err)
	}
	//nolint:gosec // G204: path is the launcher's own os.Executable(), not external input (invariant 33)
	cmd := exec.Command(path)
	cmd.SysProcAttr = detachedProcAttr()
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("relaunch: %w", err)
	}
	return nil
}

func startUpdateWorker(exePath, specPath string) error {
	//nolint:gosec // G204: exePath is the launcher's own os.Executable(), specPath is our own generated file (invariant 33)
	cmd := exec.Command(exePath, "--selfupdate-worker", specPath)
	cmd.SysProcAttr = detachedProcAttr()
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start selfupdate worker: %w", err)
	}
	return nil
}
