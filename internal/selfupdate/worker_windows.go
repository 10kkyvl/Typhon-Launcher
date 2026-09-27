//go:build windows

package selfupdate

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"

	"typhon/internal/uierr"
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

// RunWorker is the entry point of the separate --selfupdate-worker process:
// it is started right before the launcher quits, so there is no caller ctx to
// thread through (invariant 20 permits context.Background only in main, and
// this is the worker's equivalent of main).
func RunWorker(specPath string) error {
	return runWorker(specPath, func(title, detail string) stageReporter {
		return newProgressUI(title, detail)
	})
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
		return uierr.New("selfupdate.relaunch_path_empty", "selfupdate: relaunch path is empty")
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
