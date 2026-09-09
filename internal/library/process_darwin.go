//go:build darwin && !devmock

package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"typhon/internal/wine"
)

var (
	errNoBottle     = errors.New("для этой игры нет бутыля CrossOver")
	errNoRuntime    = errors.New("для запуска игр на macOS нужен CrossOver")
	errGameNotSeen  = errors.New("процесс игры не появился в бутыле")
	errNoContext    = errors.New("сервис библиотеки ещё не запущен")
	errNoSharedStop = errors.New("остановка игры в общем бутыле недоступна")
)

const (
	gameSettleInterval = 500 * time.Millisecond
	gameAppearTimeout  = 15 * time.Second
)

// wineStarter собран из функций, а не из *wine.Manager, чтобы тест мог
// подставить каждую по отдельности: настоящий CrossOver на машине сборки
// может отсутствовать.
type wineStarter struct {
	lookup func(path string) (wine.Bottle, bool)
	// shared отдаёт общий бутыль со Steam, нацеленный на каталог установки.
	shared     func(destDir string) (wine.Bottle, error)
	steam      func(ctx context.Context, b wine.Bottle) (bool, error)
	overrides  func(executable, workDir string) (string, error)
	launch     func(ctx context.Context, b wine.Bottle, c wine.Cmd) error
	poll       func(ctx context.Context, b wine.Bottle) ([]wine.Process, error)
	stop       func(b wine.Bottle) error
	stopShared func(ctx context.Context, b wine.Bottle) error
	settle     time.Duration
	timeout    time.Duration
}

func newGameStarter() gameStarter {
	rt, err := wine.Detect()
	if err != nil {
		return func(context.Context, launch) (gameProcess, error) { return nil, errNoRuntime }
	}
	manager := wine.NewManager(rt)
	s := wineStarter{
		lookup:     manager.Lookup,
		shared:     manager.SharedBottle,
		steam:      manager.EnsureSteam,
		overrides:  proxyDLLOverrides,
		launch:     manager.StartDetached,
		poll:       manager.Processes,
		stop:       manager.Kill,
		stopShared: manager.KillProcesses,
		settle:     gameSettleInterval,
		timeout:    gameAppearTimeout,
	}
	return s.start
}

// start блокируется до появления процесса в бутыле: PlayGame читает pid сразу
// после старта, а личность сессии подтверждается парой pid + время старта,
// поэтому вернуть handle без настоящего pid нельзя.
func (s wineStarter) start(ctx context.Context, req launch) (gameProcess, error) {
	// Не всё в библиотеке приходит из каталога: пользователь может добавить
	// уже стоящую игру, и на macOS она бывает нативной. Windows-программе
	// нужен бутыль, нативной — обычный запуск.
	if !isWindowsExecutable(req.executable) {
		return execStarter(ctx, req)
	}
	if ctx == nil {
		return nil, errNoContext
	}
	bottle, err := s.bottleFor(req)
	if err != nil {
		return nil, err
	}
	steamFix, hasSteamFix := detectSteamFix(req.executable, req.workDir)
	if hasSteamFix {
		slog.Info("steam fix detected",
			"kind", steamFix.Kind, "config", steamFix.Path,
			"realAppID", steamFix.RealAppID, "fakeAppID", steamFix.FakeAppID)
	}
	winExe, err := bottle.ToWindows(req.executable)
	if err != nil {
		return nil, fmt.Errorf("путь игры: %w", err)
	}
	dllOverrides := ""
	if s.overrides != nil {
		dllOverrides, err = s.overrides(req.executable, req.workDir)
		if err != nil {
			return nil, fmt.Errorf("проверка локальных DLL: %w", err)
		}
	}
	cmd := wine.Cmd{
		Path:         winExe,
		Args:         req.args,
		DLLOverrides: dllOverrides,
	}
	if req.workDir != "" {
		if winDir, dirErr := bottle.ToWindows(req.workDir); dirErr == nil {
			cmd.WorkDir = winDir
		}
	}
	slog.Info("launching game in bottle",
		"bottle", bottle.Name, "shared", bottle.Shared,
		"executable", req.executable, "winPath", winExe, "winWorkDir", cmd.WorkDir,
		"dllOverrides", cmd.DLLOverrides)
	if bottle.Shared {
		// Steam поднимается до игры, а не после: игра со Steam API
		// проверяет живого клиента в первые же секунды и без него молча
		// закрывается. Неудача при этом не отменяет запуск — пользователю
		// полезнее увидеть ошибку самой игры, чем отказ лаунчера.
		if started, steamErr := s.steam(ctx, bottle); steamErr != nil {
			slog.Warn("ensure steam", "bottle", bottle.Name, "started", started, "error", steamErr)
		}
	}
	var logCursor steamLogCursor
	if hasSteamFix {
		logCursor = steamGameProcessLogCursor(bottle)
	}
	if err := s.launch(ctx, bottle, cmd); err != nil {
		return nil, err
	}
	proc, err := s.await(ctx, bottle, req.executable)
	if err != nil {
		return nil, err
	}
	if hasSteamFix {
		go monitorSteamAppID(ctx, bottle, winExe, steamFix, logCursor)
	}
	return proc, nil
}

// proxyDLLOverrides включает app-local proxy DLL раньше встроенной Wine DLL.
// Без override Wine предпочитает builtin даже когда игра положила рядом с EXE
// winmm.dll/version.dll/winhttp.dll для загрузки модулей или Steam-fix. Правило
// действует только на один запуск: реестр общего Steam-бутыля не меняется.
func proxyDLLOverrides(executable, workDir string) (string, error) {
	candidates := []string{"winmm.dll", "version.dll", "winhttp.dll"}
	dirs := []string{filepath.Dir(executable)}
	if workDir != "" && !strings.EqualFold(filepath.Clean(workDir), filepath.Clean(dirs[0])) {
		dirs = append(dirs, workDir)
	}

	found := make(map[string]bool, len(candidates))
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return "", fmt.Errorf("чтение %s: %w", dir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			for _, candidate := range candidates {
				if strings.EqualFold(entry.Name(), candidate) {
					found[strings.TrimSuffix(candidate, filepath.Ext(candidate))] = true
				}
			}
		}
	}

	var overrides []string
	for _, candidate := range candidates {
		name := strings.TrimSuffix(candidate, filepath.Ext(candidate))
		if found[name] {
			overrides = append(overrides, name+"=n,b")
		}
	}
	return strings.Join(overrides, ";"), nil
}

// bottleFor выбирает, где игре жить. Общий бутыль предпочтительнее: рядом с
// ним крутится windows Steam, и только в одном с ним префиксе у игры
// работают Steam API, оверлей и достижения. Собственный бутыль остаётся
// запасным путём — для игр, которым Steam запретили явно, и для машин, где
// общего бутыля просто нет.
func (s wineStarter) bottleFor(req launch) (wine.Bottle, error) {
	key := req.installDir
	if key == "" {
		key = filepath.Dir(req.executable)
	}
	if req.shared && s.shared != nil {
		bottle, err := s.shared(key)
		if err == nil {
			return bottle, nil
		}
		if !wine.SharedBottleUnavailable(err) {
			return wine.Bottle{}, fmt.Errorf("общий бутыль Steam: %w", err)
		}
		// Общего бутыля нет или он не видит путь игры — это ожидаемое
		// состояние машины, а не поломка: откатываемся на свой бутыль.
		slog.Info("shared bottle unavailable, falling back", "installDir", key, "error", err)
	}
	bottle, ok := s.lookup(req.executable)
	if !ok {
		return wine.Bottle{}, errNoBottle
	}
	return bottle, nil
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
					poll: s.poll, stop: s.stop, stopShared: s.stopShared,
					settle: s.settle,
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
	bottle     wine.Bottle
	id         int
	ctx        context.Context
	poll       func(ctx context.Context, b wine.Bottle) ([]wine.Process, error)
	stop       func(b wine.Bottle) error
	stopShared func(ctx context.Context, b wine.Bottle) error
	settle     time.Duration
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

// kill валит собственный бутыль игры целиком: он заведён под одну игру,
// поэтому чужого в нём нет, а репаки нередко запускают игру не тем
// процессом, который стартовал.
//
// Общий бутыль так валить нельзя: вместе с игрой умерли бы Steam и все
// остальные игры того же префикса. Там гасятся только процессы, чей путь
// лежит внутри каталога этой установки.
func (p *wineGameProcess) kill() error {
	if p.bottle.Shared {
		if p.stopShared == nil {
			return errNoSharedStop
		}
		return p.stopShared(p.ctx, p.bottle)
	}
	return p.stop(p.bottle)
}

func isWindowsExecutable(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".exe", ".bat", ".cmd", ".com", ".msi":
		return true
	default:
		return false
	}
}
