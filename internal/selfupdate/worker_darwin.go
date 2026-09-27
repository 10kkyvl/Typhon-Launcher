//go:build darwin && !devmock

package selfupdate

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"syscall"

	"typhon/internal/uierr"
)

// RunWorker — точка входа отдельного процесса --selfupdate-worker. Окна
// прогресса на macOS нет: показать его мог бы только полноценный бандл, а
// воркер живёт секунды и запускается уже после того, как окно лаунчера
// закрылось. Стадии остаются в логе воркера.
func RunWorker(specPath string) error {
	return runWorker(specPath, func(string, string) stageReporter { return silentReporter{} })
}

// startUpdateWorker запускает копию лаунчера воркером и отвязывает её:
// родитель сейчас начнёт выходить, и переживший его процесс не должен
// оказаться в той же группе.
func startUpdateWorker(workerPath, specPath string) error {
	//nolint:gosec // G204: workerPath — наша же копия лаунчера, specPath мы сами и записали (инвариант 33)
	cmd := exec.Command(workerPath, "--selfupdate-worker", specPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start selfupdate worker: %w", err)
	}
	// Ждать нечего, но и держать зомби незачем: родитель вот-вот выйдет.
	go func() {
		if err := cmd.Wait(); err != nil {
			slog.Debug("selfupdate worker exited", "error", err)
		}
	}()
	return nil
}

// workerProcessAlive: сигнал 0 не доставляется, но проверяет, что процесс с
// таким pid существует и мы вправе ему сигналить.
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
		// Процесс есть, но чужой. Для ожидания выхода родителя это «жив».
		return true, nil
	default:
		return false, fmt.Errorf("selfupdate: проверка процесса %d: %w", pid, err)
	}
}

// relaunch поднимает лаунчер обратно. Через open, а не запуском бинаря
// напрямую: только так macOS считает приложение запущенным по-настоящему —
// с иконкой в Dock, активацией и единственным экземпляром.
func relaunch(path string) error {
	if path == "" {
		return uierr.New("selfupdate.relaunch_path_empty", "selfupdate: relaunch path is empty")
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("stat relaunch target: %w", err)
	}
	target := path
	if bundle, err := bundleOf(path); err == nil {
		target = bundle
	}
	//nolint:gosec // G204: target — собственный бандл лаунчера, не внешний ввод (инвариант 33)
	cmd := exec.Command("/usr/bin/open", "-a", target)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("relaunch: %w", err)
	}
	go func() {
		if err := cmd.Wait(); err != nil {
			slog.Debug("relaunch helper exited", "error", err)
		}
	}()
	return nil
}
