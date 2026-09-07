//go:build darwin && !devmock

package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"typhon/internal/wine"
)

var (
	errNoBottle    = errors.New("для этой игры нет бутыля CrossOver")
	errNoRuntime   = errors.New("для запуска игр на macOS нужен CrossOver")
	errGameNotSeen = errors.New("процесс игры не появился в бутыле")
	errNoContext   = errors.New("сервис библиотеки ещё не запущен")
)

const (
	gameSettleInterval = 500 * time.Millisecond
	gameAppearTimeout  = 15 * time.Second
)

// wineStarter собран из функций, а не из *wine.Manager, чтобы тест мог
// подставить каждую по отдельности: настоящий CrossOver на машине сборки
// может отсутствовать.
type wineStarter struct {
	lookup  func(path string) (wine.Bottle, bool)
	launch  func(ctx context.Context, b wine.Bottle, c wine.Cmd) error
	poll    func(ctx context.Context, b wine.Bottle) ([]wine.Process, error)
	stop    func(b wine.Bottle) error
	settle  time.Duration
	timeout time.Duration
}

func newGameStarter() gameStarter {
	rt, err := wine.Detect()
	if err != nil {
		return func(context.Context, string, []string, string) (gameProcess, error) { return nil, errNoRuntime }
	}
	manager := wine.NewManager(rt)
	s := wineStarter{
		lookup:  manager.Lookup,
		launch:  manager.StartDetached,
		poll:    manager.Processes,
		stop:    manager.Kill,
		settle:  gameSettleInterval,
		timeout: gameAppearTimeout,
	}
	return s.start
}

// start блокируется до появления процесса в бутыле: PlayGame читает pid сразу
// после старта, а личность сессии подтверждается парой pid + время старта,
// поэтому вернуть handle без настоящего pid нельзя.
func (s wineStarter) start(ctx context.Context, executable string, args []string, dir string) (gameProcess, error) {
	// Не всё в библиотеке приходит из каталога: пользователь может добавить
	// уже стоящую игру, и на macOS она бывает нативной. Windows-программе
	// нужен бутыль, нативной — обычный запуск.
	if !isWindowsExecutable(executable) {
		return execStarter(ctx, executable, args, dir)
	}
	if ctx == nil {
		return nil, errNoContext
	}
	bottle, ok := s.lookup(executable)
	if !ok {
		return nil, errNoBottle
	}
	winExe, err := bottle.ToWindows(executable)
	if err != nil {
		return nil, fmt.Errorf("путь игры: %w", err)
	}
	cmd := wine.Cmd{Path: winExe, Args: args}
	if dir != "" {
		if winDir, dirErr := bottle.ToWindows(dir); dirErr == nil {
			cmd.WorkDir = winDir
		}
	}
	if err := s.launch(ctx, bottle, cmd); err != nil {
		return nil, err
	}
	return s.await(ctx, bottle, executable)
}

func (s wineStarter) await(ctx context.Context, bottle wine.Bottle, executable string) (gameProcess, error) {
	ticker := time.NewTicker(s.settle)
	defer ticker.Stop()
	deadline := time.After(s.timeout)
	for {
		found, err := s.poll(ctx, bottle)
		if err != nil {
			slog.Warn("poll bottle processes", "bottle", bottle.Name, "error", err)
		}
		for _, p := range found {
			if p.Path == executable {
				return &wineGameProcess{
					bottle: bottle, id: p.PID, ctx: ctx,
					poll: s.poll, stop: s.stop, settle: s.settle,
				}, nil
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline:
			return nil, fmt.Errorf("%w: %s", errGameNotSeen, executable)
		case <-ticker.C:
		}
	}
}

// wineGameProcess: настоящего дочернего процесса у лаунчера нет — cxstart
// вернулся сразу после старта, а игра живёт внутри бутыля. Поэтому и
// ожидание, и остановка идут через бутыль, а не через os/exec.
type wineGameProcess struct {
	bottle wine.Bottle
	id     int
	ctx    context.Context
	poll   func(ctx context.Context, b wine.Bottle) ([]wine.Process, error)
	stop   func(b wine.Bottle) error
	settle time.Duration
}

func (p *wineGameProcess) pid() int { return p.id }

func (p *wineGameProcess) wait() error {
	ticker := time.NewTicker(p.settle)
	defer ticker.Stop()
	for {
		found, err := p.poll(p.ctx, p.bottle)
		if err != nil {
			return err
		}
		alive := false
		for _, entry := range found {
			if entry.PID == p.id {
				alive = true
				break
			}
		}
		if !alive {
			return nil
		}
		select {
		case <-p.ctx.Done():
			return p.ctx.Err()
		case <-ticker.C:
		}
	}
}

// kill валит бутыль целиком: он заведён под одну игру, поэтому чужого в нём
// нет, а репаки нередко запускают игру не тем процессом, который стартовал.
func (p *wineGameProcess) kill() error { return p.stop(p.bottle) }

func isWindowsExecutable(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".exe", ".bat", ".cmd", ".com", ".msi":
		return true
	default:
		return false
	}
}
