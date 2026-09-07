//go:build darwin && !devmock

package install

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	"typhon/internal/uierr"
	"typhon/internal/wine"
)

var errWineMissing = uierr.New("wine.not_installed", "для установки игр на macOS нужен CrossOver")

// wineRunner ставит игру в её собственном бутыле. Повышения прав здесь нет:
// UAC на macOS не существует, поэтому весь протокол воркера, state- и
// cancel-файлов остаётся Windows-только.
type wineRunner struct {
	gamesPath func() string
	detect    func() (wine.Runtime, error)
	// runCmd подменяется в тестах: настоящий cxstart на машине сборки может
	// отсутствовать, а проверять надо собранную команду.
	runCmd func(ctx context.Context, b wine.Bottle, c wine.Cmd) (int, error)
}

func newRunner(gamesPath func() string) runner {
	return wineRunner{gamesPath: gamesPath, detect: wine.Detect}
}

func (r wineRunner) run(ctx context.Context, spec runSpec) (int, error) {
	bottle, err := r.bottle(spec)
	if err != nil {
		return 0, err
	}

	outcome, err := attemptDiscovery(ctx, spec.discovery())
	if err != nil {
		return 0, err
	}
	if outcome.reason != "" {
		slog.Warn("component discovery skipped", "path", spec.Path, "reason", outcome.reason)
	}
	prepared, err := applyDiscoveredComponents(spec, outcome.components)
	if err != nil {
		return 0, err
	}
	return r.runPrepared(ctx, prepared, bottle)
}

// bottle заводит бутыль под каталог установки. Для удаления Destination
// пустой, и каталогом установки служит папка самого деинсталлятора.
func (r wineRunner) bottle(spec runSpec) (wine.Bottle, error) {
	dest := spec.Destination
	if dest == "" {
		dest = filepath.Dir(spec.Path)
	}
	if dest == "" || dest == "." || dest == string(filepath.Separator) {
		return wine.Bottle{}, errEmptyDestination
	}
	rt, err := r.detect()
	if err != nil {
		return wine.Bottle{}, errWineMissing
	}
	games := ""
	if r.gamesPath != nil {
		games = r.gamesPath()
	}
	bottle, err := wine.NewManager(rt).Ensure(dest, games)
	if err != nil {
		return wine.Bottle{}, uierr.Wrap("wine.bottle_create_failed", err)
	}
	return bottle, nil
}

func (r wineRunner) runPrepared(ctx context.Context, spec runSpec, bottle wine.Bottle) (int, error) {
	winPath, err := bottle.ToWindows(spec.Path)
	if err != nil {
		return 0, fmt.Errorf("путь установщика: %w", err)
	}
	// WaitChildren: установщик распаковывает себя во временный каталог и
	// работает уже оттуда, поэтому ждать надо всё дерево, а не загрузчик.
	cmd := wine.Cmd{Path: winPath, Args: spec.Args, Log: spec.LogPath, WaitChildren: true}
	if spec.Dir != "" {
		if dir, dirErr := bottle.ToWindows(spec.Dir); dirErr == nil {
			cmd.WorkDir = dir
		}
	}
	if r.runCmd != nil {
		return r.runCmd(ctx, bottle, cmd)
	}
	rt, err := r.detect()
	if err != nil {
		return 0, errWineMissing
	}
	return wine.NewManager(rt).Run(ctx, bottle, cmd)
}
