//go:build darwin && !devmock

package install

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"typhon/internal/wine"
)

var (
	discoveryPollInterval = 200 * time.Millisecond
	discoveryTimeout      = 60 * time.Second
)

// attemptDiscovery повторяет windows-логику: гоняем установщик с /SAVEINF и
// обрываем, как только файл разведки появился. Отличий два — запуск идёт
// через бутыль, и повышения прав на macOS не бывает, поэтому elevate здесь
// не выставляется никогда.
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

	rt, err := wine.Detect()
	if err != nil {
		return discoveryOutcome{reason: "CrossOver не найден"}, nil
	}
	manager := wine.NewManager(rt)
	bottle, ok := manager.Lookup(in.Destination)
	if !ok {
		return discoveryOutcome{reason: "бутыль установки ещё не заведён"}, nil
	}
	winInstaller, err := bottle.ToWindows(in.InstallerPath)
	if err != nil {
		return discoveryOutcome{reason: fmt.Sprintf("путь установщика: %v", err)}, nil
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Код возврата разведки не важен: прогон обрывается намеренно, как
		// только появится INF. А вот ошибка запуска — важна: без неё падение
		// разведки выглядело бы как «установщик просто не создал файл».
		if _, runErr := manager.Run(runCtx, bottle, wine.Cmd{
			Path: winInstaller, Args: plan.Args, WaitChildren: true,
		}); runErr != nil && !errors.Is(runErr, context.Canceled) {
			slog.Debug("discovery run", "installer", winInstaller, "error", runErr)
		}
	}()

	reason := awaitDiscoveryFile(ctx, in.InfPath, done)
	cancel()
	<-done
	if reason != "" {
		return discoveryOutcome{reason: reason}, nil
	}

	components, readReason, err := readDiscoveredComponents(in.InfPath, in.Options)
	if err != nil {
		return discoveryOutcome{}, err
	}
	return discoveryOutcome{components: components, reason: readReason}, nil
}

// awaitDiscoveryFile ждёт появления INF, а не завершения установщика:
// разведка — не установка, доводить прогон до конца нельзя.
func awaitDiscoveryFile(ctx context.Context, infPath string, done <-chan struct{}) string {
	ticker := time.NewTicker(discoveryPollInterval)
	defer ticker.Stop()
	deadline := time.After(discoveryTimeout)
	for {
		if _, err := os.Stat(infPath); err == nil {
			return ""
		}
		select {
		case <-ctx.Done():
			return "разведка прервана"
		case <-done:
			if _, err := os.Stat(infPath); err == nil {
				return ""
			}
			return "установщик завершился, не создав файл разведки"
		case <-deadline:
			return "разведка не уложилась в отведённое время"
		case <-ticker.C:
		}
	}
}
