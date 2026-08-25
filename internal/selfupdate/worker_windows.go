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

	"typhon/internal/settings"

	"golang.org/x/sys/windows"
)

const stillActive = 259

// detachedProcAttr starts a process fully independent of this one: the worker
// must keep running after ApplyUpdate calls application.Get().Quit(), and the
// relaunched launcher must not be tied to the worker's lifetime either.
//
// HideWindow must stay off. It fills STARTUPINFO with STARTF_USESHOWWINDOW and
// SW_HIDE, and Windows applies that to the child's first show of a top-level
// window instead of what the child asked for. Wails creates the launcher window
// with WS_VISIBLE, whose implicit show is that first one, so the relaunched
// launcher used to run with no window at all: the update installed, the process
// started, and the user saw nothing come back. DETACHED_PROCESS already keeps a
// console from appearing, which is all this flag was here for.
func detachedProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
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

// stageReporter is what the user watches while the launcher is gone. Tests
// swap in silentReporter so a unit run neither pops a window nor blocks on
// one being closed.
type stageReporter interface {
	setStage(title, detail string)
	fail(title, detail string)
	wait()
	close()
}

type silentReporter struct{}

func (silentReporter) setStage(string, string) {}
func (silentReporter) fail(string, string)     {}
func (silentReporter) wait()                   {}
func (silentReporter) close()                  {}

// RunWorker is the entry point of the separate --selfupdate-worker process:
// it is started right before the launcher quits, so there is no caller ctx to
// thread through (invariant 20 permits context.Background only in main, and
// this is the worker's equivalent of main).
func RunWorker(specPath string) error {
	return runWorker(specPath, func(title, detail string) stageReporter {
		return newProgressUI(title, detail)
	})
}

func runWorker(specPath string, newReporter func(title, detail string) stageReporter) error {
	spec, err := readUpdateSpec(specPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := os.Remove(specPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("remove selfupdate spec", "path", specPath, "error", err)
		}
	}()

	dir, err := settings.ConfigDir()
	if err != nil {
		return fmt.Errorf("selfupdate worker: resolve config dir: %w", err)
	}
	outcomePath, err := OutcomePath(dir)
	if err != nil {
		return err
	}

	//nolint:forbidigo // RunWorker is a separate process's entry point, the worker's equivalent of main: there is no caller ctx (invariant 20 allows Background only in main). This base feeds both waitCtx (parent-exit wait) and applyCtx (installer run) below, each with its own independent timeout.
	waitCtx, applyCtx, cancel := newWorkerContexts(context.Background())
	defer cancel()

	ui := newReporter(updateTitle(spec.Version), "Ожидание закрытия лаунчера…")
	applyErr := runUpdate(waitCtx, applyCtx, spec, ui)

	outcome := Outcome{Version: spec.Version, OK: applyErr == nil, FinishedAt: time.Now()}
	if applyErr != nil {
		outcome.Error = applyErr.Error()
		slog.Error("selfupdate worker: apply failed", "version", spec.Version, "error", applyErr)
	}
	if err := writeOutcome(outcomePath, outcome); err != nil {
		applyErr = errors.Join(applyErr, err)
		slog.Error("selfupdate worker: record outcome", "error", err)
	}

	// A launcher that never quit is still on screen: relaunching would leave
	// the user with two of them.
	if errors.Is(applyErr, errParentStillRunning) {
		ui.fail("Не удалось обновить Typhon", "Лаунчер не закрылся, обновление отменено.")
		ui.wait()
		return applyErr
	}

	if applyErr != nil {
		ui.setStage("Не удалось обновить Typhon", "Возвращаем прежнюю версию. Подробности — в лаунчере.")
	} else {
		ui.setStage(updateTitle(spec.Version), "Обновление установлено, запускаем Typhon…")
	}

	if err := relaunch(spec.RelaunchPath); err != nil {
		slog.Error("selfupdate worker: relaunch failed", "path", spec.RelaunchPath, "error", err)
		ui.fail("Не удалось запустить Typhon", "Лаунчер не запустился автоматически. Откройте Typhon из меню Пуск.")
		ui.wait()
		return errors.Join(applyErr, err)
	}
	ui.close()
	return applyErr
}

func updateTitle(version string) string {
	if version == "" {
		return "Обновление Typhon"
	}
	return "Обновление Typhon до " + version
}

func runUpdate(waitCtx, applyCtx context.Context, spec updateSpec, ui stageReporter) error {
	if err := waitForParentExit(waitCtx, spec.ParentPID); err != nil {
		return err
	}

	ui.setStage(updateTitle(spec.Version), "Устанавливаем новую версию, лаунчер запустится сам.")

	before, err := fileDigest(applyCtx, spec.RelaunchPath)
	if err != nil {
		return err
	}
	if err := Apply(applyCtx, spec.InstallerPath); err != nil {
		return err
	}
	after, err := fileDigest(applyCtx, spec.RelaunchPath)
	if err != nil {
		return err
	}

	return ensureReplaced(before, after, spec.RelaunchPath)
}

// ensureReplaced is what tells a real install from a no-op. The NSIS installer
// exits 0 even when it could not open the launcher binary for writing, which is
// exactly what happened while the worker still ran from that binary: the file
// staying byte-identical is the only signal that the install did nothing.
func ensureReplaced(before, after, path string) error {
	if after == "" {
		return fmt.Errorf("%w: %s", ErrNotReplaced, path)
	}
	if before != "" && after == before {
		return fmt.Errorf("%w: %s", ErrNotReplaced, path)
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

func startUpdateWorker(workerPath, specPath string) error {
	//nolint:gosec // G204: workerPath is our own copy of the launcher binary, specPath is our own generated file (invariant 33)
	cmd := exec.Command(workerPath, "--selfupdate-worker", specPath)
	cmd.SysProcAttr = detachedProcAttr()
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start selfupdate worker: %w", err)
	}
	return nil
}
