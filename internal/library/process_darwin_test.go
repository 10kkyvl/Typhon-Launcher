//go:build darwin && !devmock

package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"typhon/internal/wine"
)

// demoLaunch собирает запрос на запуск демо-игры; опции меняют в нём ровно
// то, что проверяет конкретный тест.
func demoLaunch(opts ...func(*launch)) launch {
	req := launch{
		installDir: "/Users/x/Games/Demo",
		executable: "/Users/x/Games/Demo/game.exe",
		workDir:    "/Users/x/Games/Demo",
	}
	for _, opt := range opts {
		opt(&req)
	}
	return req
}

func demoBottle() wine.Bottle {
	return wine.Bottle{Key: "/Users/x/Games/Demo", Name: "B", Path: "/bottles/B", Drive: "t", Games: "/Users/x/Games"}
}

func TestWineStarterFailsWithoutBottle(t *testing.T) {
	starter := wineStarter{lookup: func(string) (wine.Bottle, bool) { return wine.Bottle{}, false }}

	if _, err := starter.start(t.Context(), demoLaunch()); err == nil {
		t.Fatal("start without a bottle: want error")
	}
}

func TestWineStarterWaitsForAppearance(t *testing.T) {
	calls := 0
	var launched wine.Cmd
	starter := wineStarter{
		lookup: func(string) (wine.Bottle, bool) { return demoBottle(), true },
		launch: func(_ context.Context, _ wine.Bottle, c wine.Cmd) error { launched = c; return nil },
		poll: func(context.Context, wine.Bottle) ([]wine.Process, error) {
			calls++
			if calls < 2 {
				return nil, nil
			}
			return []wine.Process{{PID: 4242, Path: "/Users/x/Games/Demo/game.exe", CreatedAt: time.Unix(1, 0)}}, nil
		},
		stop:    func(wine.Bottle) error { return nil },
		settle:  time.Millisecond,
		timeout: time.Second,
	}

	proc, err := starter.start(t.Context(), demoLaunch(func(r *launch) { r.args = []string{"-windowed"} }))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if proc.pid() != 4242 {
		t.Fatalf("pid = %d, want 4242", proc.pid())
	}
	if launched.Path != `T:\Demo\game.exe` {
		t.Fatalf("launched Path = %q", launched.Path)
	}
	if launched.WorkDir != `T:\Demo` {
		t.Fatalf("launched WorkDir = %q", launched.WorkDir)
	}
	if len(launched.Args) != 1 || launched.Args[0] != "-windowed" {
		t.Fatalf("launched Args = %v", launched.Args)
	}
}

func TestWineStarterTimesOut(t *testing.T) {
	starter := wineStarter{
		lookup:  func(string) (wine.Bottle, bool) { return demoBottle(), true },
		launch:  func(context.Context, wine.Bottle, wine.Cmd) error { return nil },
		poll:    func(context.Context, wine.Bottle) ([]wine.Process, error) { return nil, nil },
		stop:    func(wine.Bottle) error { return nil },
		settle:  time.Millisecond,
		timeout: 20 * time.Millisecond,
	}

	_, err := starter.start(t.Context(), demoLaunch())
	if !errors.Is(err, errGameNotSeen) {
		t.Fatalf("err = %v, want errGameNotSeen", err)
	}
}

func TestWineStarterReportsLaunchFailure(t *testing.T) {
	boom := errors.New("cxstart failed")
	starter := wineStarter{
		lookup:  func(string) (wine.Bottle, bool) { return demoBottle(), true },
		launch:  func(context.Context, wine.Bottle, wine.Cmd) error { return boom },
		poll:    func(context.Context, wine.Bottle) ([]wine.Process, error) { return nil, nil },
		stop:    func(wine.Bottle) error { return nil },
		settle:  time.Millisecond,
		timeout: time.Second,
	}

	if _, err := starter.start(t.Context(), demoLaunch(func(r *launch) { r.workDir = "" })); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the launch error", err)
	}
}

func TestWineProcessKillStopsBottle(t *testing.T) {
	stopped := false
	proc := &wineGameProcess{
		bottle: demoBottle(),
		id:     4242,
		ctx:    t.Context(),
		poll:   func(context.Context, wine.Bottle) ([]wine.Process, error) { return nil, nil },
		stop:   func(wine.Bottle) error { stopped = true; return nil },
		settle: time.Millisecond,
	}

	if err := proc.kill(); err != nil {
		t.Fatalf("kill: %v", err)
	}
	if !stopped {
		t.Fatal("kill did not stop the bottle")
	}
}

func TestWineProcessWaitReturnsWhenGone(t *testing.T) {
	proc := &wineGameProcess{
		bottle: demoBottle(),
		id:     4242,
		ctx:    t.Context(),
		poll:   func(context.Context, wine.Bottle) ([]wine.Process, error) { return nil, nil },
		stop:   func(wine.Bottle) error { return nil },
		settle: time.Millisecond,
	}

	done := make(chan error, 1)
	go func() { done <- proc.wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("wait: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("wait did not return once the process was gone")
	}
}

// Нативный исполняемый файл бутыля не требует: на macOS в библиотеке может
// лежать и обычная маковая программа, добавленная руками.
func TestWineStarterRunsNativeExecutableDirectly(t *testing.T) {
	looked := false
	starter := wineStarter{
		lookup:  func(string) (wine.Bottle, bool) { looked = true; return wine.Bottle{}, false },
		settle:  time.Millisecond,
		timeout: time.Second,
	}

	proc, err := starter.start(t.Context(), demoLaunch(func(r *launch) { r.executable = "/usr/bin/true"; r.workDir = "" }))
	if err != nil {
		t.Fatalf("start native executable: %v", err)
	}
	if looked {
		t.Fatal("native executable must not be looked up among bottles")
	}
	if err := proc.wait(); err != nil {
		t.Fatalf("wait: %v", err)
	}
}

func TestIsWindowsExecutable(t *testing.T) {
	for _, path := range []string{"/games/Demo/game.exe", "/games/Demo/Setup.EXE", "/games/Demo/run.bat"} {
		if !isWindowsExecutable(path) {
			t.Fatalf("isWindowsExecutable(%q) = false, want true", path)
		}
	}
	for _, path := range []string{"/usr/bin/yes", "/Applications/Game.app/Contents/MacOS/Game"} {
		if isWindowsExecutable(path) {
			t.Fatalf("isWindowsExecutable(%q) = true, want false", path)
		}
	}
}

func TestProxyDLLOverrides(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "WINMM.DLL"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "winhttp.dll"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "version.dll"), 0o700); err != nil {
		t.Fatal(err)
	}

	got, err := proxyDLLOverrides(filepath.Join(dir, "game.exe"), "")
	if err != nil {
		t.Fatalf("proxyDLLOverrides: %v", err)
	}
	if got != "winmm=n,b;winhttp=n,b" {
		t.Fatalf("proxyDLLOverrides = %q, want winmm and winhttp", got)
	}
}

func TestProxyDLLOverridesChecksWorkDir(t *testing.T) {
	exeDir := t.TempDir()
	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "version.dll"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.Mkdir(filepath.Join(exeDir, "bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := proxyDLLOverrides(filepath.Join(exeDir, "bin", "game.exe"), workDir)
	if err != nil {
		t.Fatalf("proxyDLLOverrides: %v", err)
	}
	if got != "version=n,b" {
		t.Fatalf("proxyDLLOverrides = %q, want version=n,b", got)
	}
}

func TestProxyDLLOverridesEmptyWithoutProxy(t *testing.T) {
	dir := t.TempDir()
	got, err := proxyDLLOverrides(filepath.Join(dir, "game.exe"), dir)
	if err != nil {
		t.Fatalf("proxyDLLOverrides: %v", err)
	}
	if got != "" {
		t.Fatalf("proxyDLLOverrides = %q, want empty", got)
	}
}

func TestWineStarterReportsProxyDirectoryFailure(t *testing.T) {
	starter := wineStarter{
		lookup:    func(string) (wine.Bottle, bool) { return demoBottle(), true },
		overrides: proxyDLLOverrides,
		launch: func(context.Context, wine.Bottle, wine.Cmd) error {
			t.Fatal("launch called after proxy scan failed")
			return nil
		},
		poll:    func(context.Context, wine.Bottle) ([]wine.Process, error) { return nil, nil },
		stop:    func(wine.Bottle) error { return nil },
		settle:  time.Millisecond,
		timeout: time.Second,
	}
	req := demoLaunch(func(r *launch) { r.workDir = filepath.Join(t.TempDir(), "missing") })
	if _, err := starter.start(t.Context(), req); err == nil {
		t.Fatal("start with an unreadable proxy directory: want error")
	}
}

// prepareRuntime не должен трогать бутыли ради нативной программы: заводить
// под неё 300 МБ окружения незачем.
func TestPrepareRuntimeSkipsNativeExecutable(t *testing.T) {
	if err := prepareRuntime(t.Context(), demoLaunch(func(r *launch) { r.executable = "/Users/x/Games/Demo/game" })); err != nil {
		t.Fatalf("prepareRuntime for a native executable: %v", err)
	}
}

func TestPrepareRuntimeSkipsEmptyInstallDir(t *testing.T) {
	if err := prepareRuntime(t.Context(), demoLaunch(func(r *launch) { r.installDir = "" })); err != nil {
		t.Fatalf("prepareRuntime without an install dir: %v", err)
	}
}

func sharedBottle() wine.Bottle {
	return wine.Bottle{
		Key: "/Users/x/Games/Kebab Chefs! Restaurant Simulator", Name: "Steam",
		Path: "/bottles/Steam", Drive: "y", Games: "/Users/x", Shared: true,
	}
}

func sharedLaunch() launch {
	return launch{
		installDir: "/Users/x/Games/Kebab Chefs! Restaurant Simulator",
		executable: "/Users/x/Games/Kebab Chefs! Restaurant Simulator/Kebab Chefs.exe",
		workDir:    "/Users/x/Games/Kebab Chefs! Restaurant Simulator",
		shared:     true,
	}
}

// Главный сценарий задачи: игре, которой разрешён общий бутыль, собственный
// не заводится вовсе — она едет туда, где уже крутится Steam.
func TestWineStarterPrefersSharedBottle(t *testing.T) {
	looked, steamed := false, false
	var launched wine.Cmd
	var launchedIn wine.Bottle
	starter := wineStarter{
		lookup: func(string) (wine.Bottle, bool) { looked = true; return demoBottle(), true },
		shared: func(dest string) (wine.Bottle, error) {
			if dest != "/Users/x/Games/Kebab Chefs! Restaurant Simulator" {
				t.Errorf("shared bottle asked for %q", dest)
			}
			return sharedBottle(), nil
		},
		steam: func(context.Context, wine.Bottle) (bool, error) { steamed = true; return true, nil },
		launch: func(_ context.Context, b wine.Bottle, c wine.Cmd) error {
			launchedIn, launched = b, c
			return nil
		},
		poll: func(context.Context, wine.Bottle) ([]wine.Process, error) {
			return []wine.Process{{
				PID:  4242,
				Path: "/Users/x/Games/Kebab Chefs! Restaurant Simulator/Kebab Chefs.exe",
			}}, nil
		},
		stop:       func(wine.Bottle) error { return nil },
		stopShared: func(context.Context, wine.Bottle) error { return nil },
		settle:     time.Millisecond,
		timeout:    time.Second,
	}

	proc, err := starter.start(t.Context(), sharedLaunch())
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if looked {
		t.Fatal("собственный бутыль искали, хотя игре разрешён общий")
	}
	if !steamed {
		t.Fatal("Steam не подняли перед запуском игры в общем бутыле")
	}
	if launchedIn.Name != "Steam" || !launchedIn.Shared {
		t.Fatalf("игра ушла в бутыль %+v, want общий Steam", launchedIn)
	}
	if launched.Path != `Y:\Games\Kebab Chefs! Restaurant Simulator\Kebab Chefs.exe` {
		t.Fatalf("launched Path = %q", launched.Path)
	}
	if launched.WorkDir != `Y:\Games\Kebab Chefs! Restaurant Simulator` {
		t.Fatalf("launched WorkDir = %q", launched.WorkDir)
	}
	if proc.pid() != 4242 {
		t.Fatalf("pid = %d, want 4242", proc.pid())
	}
}

// Общего бутыля на машине может не быть: тогда игра обязана поехать в
// собственный, а не отказать в запуске.
func TestWineStarterFallsBackToOwnBottle(t *testing.T) {
	var launchedIn wine.Bottle
	starter := wineStarter{
		lookup: func(string) (wine.Bottle, bool) { return demoBottle(), true },
		shared: func(string) (wine.Bottle, error) { return wine.Bottle{}, wine.ErrNoSharedBottle },
		steam: func(context.Context, wine.Bottle) (bool, error) {
			t.Error("Steam поднимали для собственного бутыля")
			return false, nil
		},
		launch: func(_ context.Context, b wine.Bottle, _ wine.Cmd) error { launchedIn = b; return nil },
		poll: func(context.Context, wine.Bottle) ([]wine.Process, error) {
			return []wine.Process{{PID: 7, Path: "/Users/x/Games/Demo/game.exe"}}, nil
		},
		stop:    func(wine.Bottle) error { return nil },
		settle:  time.Millisecond,
		timeout: time.Second,
	}

	req := demoLaunch(func(r *launch) { r.shared = true })
	if _, err := starter.start(t.Context(), req); err != nil {
		t.Fatalf("start: %v", err)
	}
	if launchedIn.Shared || launchedIn.Name != "B" {
		t.Fatalf("игра ушла в %+v, want собственный бутыль", launchedIn)
	}
}

func TestWineStarterDoesNotHideSharedBottleFailure(t *testing.T) {
	starter := wineStarter{
		lookup: func(string) (wine.Bottle, bool) {
			t.Fatal("own bottle lookup called after an unexpected shared bottle failure")
			return wine.Bottle{}, false
		},
		shared: func(string) (wine.Bottle, error) { return wine.Bottle{}, os.ErrPermission },
	}
	req := demoLaunch(func(r *launch) { r.shared = true })
	if _, err := starter.bottleFor(req); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("bottleFor = %v, want permission error", err)
	}
}

// Не поднявшийся Steam не отменяет запуск: пользователю полезнее увидеть
// ошибку самой игры, чем отказ лаунчера.
func TestWineStarterLaunchesWhenSteamFails(t *testing.T) {
	launched := false
	starter := wineStarter{
		shared: func(string) (wine.Bottle, error) { return sharedBottle(), nil },
		steam: func(context.Context, wine.Bottle) (bool, error) {
			return false, errors.New("steam.exe не найден")
		},
		launch: func(context.Context, wine.Bottle, wine.Cmd) error { launched = true; return nil },
		poll: func(context.Context, wine.Bottle) ([]wine.Process, error) {
			return []wine.Process{{
				PID:  4242,
				Path: "/Users/x/Games/Kebab Chefs! Restaurant Simulator/Kebab Chefs.exe",
			}}, nil
		},
		stopShared: func(context.Context, wine.Bottle) error { return nil },
		settle:     time.Millisecond,
		timeout:    time.Second,
	}

	if _, err := starter.start(t.Context(), sharedLaunch()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if !launched {
		t.Fatal("игру не запустили из-за неудачи со Steam")
	}
}

// «Стоп» в общем бутыле не имеет права звать wineserver -k: вместе с игрой
// умерли бы Steam и все соседние игры того же префикса.
func TestWineProcessKillSharedSparesBottle(t *testing.T) {
	killedBottle, killedGame := false, false
	proc := &wineGameProcess{
		bottle:     sharedBottle(),
		id:         4242,
		ctx:        t.Context(),
		poll:       func(context.Context, wine.Bottle) ([]wine.Process, error) { return nil, nil },
		stop:       func(wine.Bottle) error { killedBottle = true; return nil },
		stopShared: func(context.Context, wine.Bottle) error { killedGame = true; return nil },
		settle:     time.Millisecond,
	}

	if err := proc.kill(); err != nil {
		t.Fatalf("kill: %v", err)
	}
	if killedBottle {
		t.Fatal("общий бутыль свалили целиком")
	}
	if !killedGame {
		t.Fatal("процессы игры не остановлены")
	}
}
