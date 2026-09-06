//go:build darwin && !devmock

package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"typhon/internal/wine"
)

var (
	errNoBottle    = errors.New("для этой игры нет бутыля CrossOver")
	errNoRuntime   = errors.New("для запуска игр на macOS нужен CrossOver")
	errGameNotSeen = errors.New("процесс игры не появился в бутыле")
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
	launch  func(b wine.Bottle, c wine.Cmd) error
	poll    func(b wine.Bottle) ([]wine.Process, error)
	stop    func(b wine.Bottle) error
	settle  time.Duration
	timeout time.Duration
}

func newGameStarter() gameStarter {
	rt, err := wine.Detect()
	if err != nil {
		return func(string, []string, string) (gameProcess, error) { return nil, errNoRuntime }
	}
	manager := wine.NewManager(rt)
	s := wineStarter{
		lookup: manager.Lookup,
		launch: func(b wine.Bottle, c wine.Cmd) error {
			return manager.StartDetached(context.Background(), b, c)
		},
		poll:    func(b wine.Bottle) ([]wine.Process, error) { return manager.Processes(context.Background(), b) },
		stop:    manager.Kill,
		settle:  gameSettleInterval,
		timeout: gameAppearTimeout,
	}
	return s.start
}

// start блокируется до появления процесса в бутыле: PlayGame читает pid сразу
// после старта, а личность сессии подтверждается парой pid + время старта,
// поэтому вернуть handle без настоящего pid нельзя.
func (s wineStarter) start(executable string, args []string, dir string) (gameProcess, error) {
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
	if err := s.launch(bottle, cmd); err != nil {
		return nil, err
	}

	deadline := time.Now().Add(s.timeout)
	for {
		found, err := s.poll(bottle)
		if err != nil {
			slog.Warn("poll bottle processes", "bottle", bottle.Name, "error", err)
		}
		for _, p := range found {
			if p.Path == executable {
				return &wineGameProcess{
					bottle: bottle, id: p.PID,
					poll: s.poll, stop: s.stop, settle: s.settle,
				}, nil
			}
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("%w: %s", errGameNotSeen, executable)
		}
		time.Sleep(s.settle)
	}
}

// wineGameProcess: настоящего дочернего процесса у лаунчера нет — cxstart
// вернулся сразу после старта, а игра живёт внутри бутыля. Поэтому и
// ожидание, и остановка идут через бутыль, а не через os/exec.
type wineGameProcess struct {
	bottle wine.Bottle
	id     int
	poll   func(b wine.Bottle) ([]wine.Process, error)
	stop   func(b wine.Bottle) error
	settle time.Duration
}

func (p *wineGameProcess) pid() int { return p.id }

func (p *wineGameProcess) wait() error {
	for {
		found, err := p.poll(p.bottle)
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
		time.Sleep(p.settle)
	}
}

// kill валит бутыль целиком: он заведён под одну игру, поэтому чужого в нём
// нет, а репаки нередко запускают игру не тем процессом, который стартовал.
func (p *wineGameProcess) kill() error { return p.stop(p.bottle) }
