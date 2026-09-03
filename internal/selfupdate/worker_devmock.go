//go:build devmock && !windows

package selfupdate

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"typhon/internal/settings"
	"typhon/internal/storage"
)

const (
	workerLogName = "worker.log"
	progressName  = "progress.json"
)

type workerProgress struct {
	Title  string    `json:"title"`
	Detail string    `json:"detail"`
	Failed bool      `json:"failed"`
	At     time.Time `json:"at"`
}

// logReporter stands in for the Windows progress window: every stage lands
// in the worker log and in progress.json next to it, which is what a
// developer tails while the launcher is gone.
type logReporter struct {
	path string
}

func newLogReporter(path, title, detail string) *logReporter {
	r := &logReporter{path: path}
	r.write(title, detail, false)
	return r
}

func (r *logReporter) setStage(title, detail string) { r.write(title, detail, false) }
func (r *logReporter) fail(title, detail string)     { r.write(title, detail, true) }
func (r *logReporter) wait()                         {}
func (r *logReporter) close()                        {}

func (r *logReporter) write(title, detail string, failed bool) {
	if failed {
		slog.Error("selfupdate worker stage failed", "title", title, "detail", detail)
	} else {
		slog.Info("selfupdate worker stage", "title", title, "detail", detail)
	}
	data, err := json.Marshal(workerProgress{Title: title, Detail: detail, Failed: failed, At: time.Now()})
	if err != nil {
		slog.Warn("encode selfupdate worker progress", "error", err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		slog.Warn("create selfupdate worker dir", "path", r.path, "error", err)
		return
	}
	if err := storage.WriteAtomic(r.path, data); err != nil {
		slog.Warn("write selfupdate worker progress", "path", r.path, "error", err)
	}
}

func workerDirForCurrentUser() (string, error) {
	dir, err := settings.ConfigDir()
	if err != nil {
		return "", fmt.Errorf("selfupdate worker: resolve config dir: %w", err)
	}
	return WorkerDir(dir)
}

// RunWorker is the entry point of the separate --selfupdate-worker process:
// it is started right before the launcher quits, so there is no caller ctx to
// thread through (invariant 20 permits context.Background only in main, and
// this is the worker's equivalent of main).
func RunWorker(specPath string) error {
	workerDir, err := workerDirForCurrentUser()
	if err != nil {
		return err
	}
	progressPath := filepath.Join(workerDir, progressName)
	return runWorker(specPath, func(title, detail string) stageReporter {
		return newLogReporter(progressPath, title, detail)
	})
}

// detachedProcAttr puts the child in its own session so it survives the
// launcher's exit: without Setsid the worker shares the launcher's process
// group and dies with it on the terminal's SIGHUP.
func detachedProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}

// workerProcessAlive probes with signal 0: ESRCH is the only "gone" answer,
// EPERM means the pid exists but belongs to another user, so it is alive.
func workerProcessAlive(pid int) (bool, error) {
	if pid <= 0 {
		return false, nil
	}
	err := syscall.Kill(pid, 0)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, syscall.ESRCH):
		return false, nil
	case errors.Is(err, syscall.EPERM):
		return true, nil
	default:
		return false, fmt.Errorf("probe parent process %d: %w", pid, err)
	}
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
	workerDir, err := workerDirForCurrentUser()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(workerDir, 0o755); err != nil {
		return fmt.Errorf("create selfupdate worker dir: %w", err)
	}
	logPath := filepath.Join(workerDir, workerLogName)
	logFile, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open selfupdate worker log: %w", err)
	}

	//nolint:gosec // G204: workerPath is our own copy of the launcher binary, specPath is our own generated file (invariant 33)
	cmd := exec.Command(workerPath, "--selfupdate-worker", specPath)
	cmd.SysProcAttr = detachedProcAttr()
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	startErr := cmd.Start()
	if cerr := logFile.Close(); cerr != nil && startErr == nil {
		startErr = fmt.Errorf("close selfupdate worker log: %w", cerr)
	}
	if startErr != nil {
		return fmt.Errorf("start selfupdate worker: %w", startErr)
	}
	return nil
}
