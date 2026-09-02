//go:build windows

package install

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"time"

	"golang.org/x/sys/windows"
)

var (
	discoveryPollInterval = 200 * time.Millisecond
	discoveryTimeout      = 60 * time.Second
)

// attemptDiscovery — единственное место, которое умеет проводить разведку
// компонентов Inno: пользуются им и воркер (уже с правами администратора), и
// обычный неэлевированный путь запуска (processRunner.run в
// runner_windows.go), которому без разведки установщик без UAC ставил бы
// DirectX/vcredist ровно как до появления воркера (инвариант 28 — один
// источник правды на понятие).
func attemptDiscovery(ctx context.Context, in discoverySpec) (discoveryOutcome, error) {
	if !shouldDiscoverComponents(in) {
		return discoveryOutcome{}, nil
	}

	plan, ok, err := discoverPlan(in.Engine, in.InstallerPath, in.Destination, in.InfPath)
	if err != nil {
		return discoveryOutcome{}, fmt.Errorf("построение плана разведки: %w", err)
	}
	if !ok {
		return discoveryOutcome{reason: "движок не поддерживает разведку компонентов"}, nil
	}
	if err := os.Remove(in.InfPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return discoveryOutcome{}, fmt.Errorf("удаление старого файла разведки: %w", err)
	}

	rs := runSpec{Path: in.InstallerPath, Args: plan.Args, CmdLine: plan.CmdLine, Dir: in.WorkingDir, Hidden: true, Background: true}
	cmd, group, startErr := startDiscoveryRun(rs)
	if startErr != nil {
		if needsElevation(startErr) {
			return discoveryOutcome{elevate: true}, nil
		}
		return discoveryOutcome{reason: fmt.Sprintf("запуск установщика для разведки: %v", startErr)}, nil
	}

	reason, err := awaitDiscoveryIni(ctx, cmd, group, in.InfPath, in.InstallerPath)
	if err != nil {
		return discoveryOutcome{}, err
	}
	if reason != "" {
		return discoveryOutcome{reason: reason}, nil
	}

	components, readReason, err := readDiscoveredComponents(in.InfPath, in.Options)
	if err != nil {
		return discoveryOutcome{}, err
	}
	return discoveryOutcome{components: components, reason: readReason}, nil
}

// startDiscoveryRun запускает установщик с /SAVEINF и заводит для него
// отдельный job-объект: разведку нужно оборвать до того, как установщик
// закончит настоящую установку, а без job-объекта оборвать дерево процессов
// Inno нечем.
func startDiscoveryRun(rs runSpec) (*exec.Cmd, windows.Handle, error) {
	//nolint:gosec // G204: воркер запускает установщик, путь которого уже проверен в Inspect/Start (инвариант 32); переменный путь — сама суть install-сервиса
	cmd := exec.Command(rs.Path, rs.Args...)
	cmd.Dir = rs.Dir
	cmd.SysProcAttr = startupAttr(rs)
	if err := cmd.Start(); err != nil {
		return nil, 0, err
	}

	group, groupErr := groupProcess(cmd.Process.Pid, true)
	if groupErr != nil {
		if killErr := cmd.Process.Kill(); killErr != nil {
			slog.Warn("kill discovery installer", "path", rs.Path, "error", killErr)
		}
		return nil, 0, fmt.Errorf("job object разведки: %w", groupErr)
	}
	return cmd, group, nil
}

// awaitDiscoveryIni обрывает уже запущенный разведывательный прогон сразу,
// как только появляется файл разведки, или по истечении потолка ожидания:
// доводить этот прогон до конца не нужно, разведка — не установка.
func awaitDiscoveryIni(ctx context.Context, cmd *exec.Cmd, group windows.Handle, infPath, installerPath string) (string, error) {
	defer closeGroup(group, installerPath)

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	ticker := time.NewTicker(discoveryPollInterval)
	defer ticker.Stop()
	deadline := time.After(discoveryTimeout)

	reason := ""
	exited := false
wait:
	for {
		if _, err := os.Stat(infPath); err == nil {
			break wait
		}
		select {
		case <-done:
			exited = true
			reason = "установщик завершился раньше, чем появился файл разведки"
			break wait
		case <-ctx.Done():
			terminateGroup(group, cmd, installerPath)
			<-done
			return "", ctx.Err()
		case <-deadline:
			reason = "файл разведки не появился за отведённое время"
			break wait
		case <-ticker.C:
		}
	}

	terminateGroup(group, cmd, installerPath)
	// done уже осушён веткой case <-done: выше — второй приём на том же
	// буферизованном канале никогда не разблокируется, потому что cmd.Wait()
	// отправляет в него ровно один раз.
	if !exited {
		<-done
	}
	return reason, nil
}

func readDiscoveredComponents(infPath string, opts installOptions) ([]string, string, error) {
	data, err := os.ReadFile(infPath)
	if err != nil {
		return nil, fmt.Sprintf("чтение файла разведки: %v", err), nil
	}
	list, ok := infComponents(data)
	if !ok {
		return nil, "секция Components не найдена в файле разведки", nil
	}
	filtered, changed := filterComponents(list, opts)
	if !changed {
		return nil, "", nil
	}
	return filtered, "", nil
}
