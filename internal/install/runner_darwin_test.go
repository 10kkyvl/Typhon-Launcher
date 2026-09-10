//go:build darwin && !devmock

package install

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"typhon/internal/wine"
)

func failingDetect() (wine.Runtime, error) { return wine.Runtime{}, wine.ErrNotInstalled }

func testBottle(games, dest string) wine.Bottle {
	return wine.Bottle{Key: dest, Name: "Typhon-Demo-test", Path: filepath.Join(games, ".bottle"), Drive: "t", Games: games}
}

// fakeBottlesDir подсовывает поддельный $HOME с пустым каталогом бутылей
// CrossOver: wine.NewManager вычисляет BottlesDir из $HOME сам, а
// wineRunner.bottle заводит менеджер заново на каждый вызов, так что другого
// способа подставить тестовый каталог бутылей нет.
func fakeBottlesDir(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, "Library", "Application Support", "CrossOver", "Bottles")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll bottles dir: %v", err)
	}
	return dir
}

// fakeCrossOverRuntime готовит Runtime с игрушечными cxbottle/cxstart,
// пишущими вызовы в лог, — тем же приёмом, что и scriptRuntime в
// internal/wine (сюда его не импортировать: он unexported в тестах другого
// пакета). Настоящий CrossOver для этих тестов не нужен.
func fakeCrossOverRuntime(t *testing.T, bottlesDir string) (rt wine.Runtime, log string) {
	t.Helper()
	dir := t.TempDir()
	log = filepath.Join(dir, "calls.log")

	cxbottle := filepath.Join(dir, "cxbottle")
	body := `#!/bin/sh
echo "cxbottle $@" >> ` + log + `
name=""
while [ $# -gt 0 ]; do
  case "$1" in
    --bottle) name="$2"; shift 2;;
    *) shift;;
  esac
done
b="` + bottlesDir + `/$name"
mkdir -p "$b/dosdevices" "$b/drive_c/users/crossover/AppData/Roaming"
ln -sfn ../drive_c "$b/dosdevices/c:"
exit 0
`
	if err := os.WriteFile(cxbottle, []byte(body), 0o755); err != nil {
		t.Fatalf("WriteFile cxbottle: %v", err)
	}
	for _, name := range []string{"cxstart", "wineserver"} {
		path := filepath.Join(dir, name)
		script := "#!/bin/sh\necho \"" + name + " $@\" >> " + log + "\nexit 0\n"
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}
	rt = wine.Runtime{
		Root:       dir,
		CxBottle:   cxbottle,
		CxStart:    filepath.Join(dir, "cxstart"),
		WineServer: filepath.Join(dir, "wineserver"),
		Version:    "26.3",
	}
	return rt, log
}

// makeSharedBottleDir создаёт каталог общего бутыля так, чтобы SharedBottle
// его нашёл: dosdevices с одной буквой, направленной на ancestor — предка
// каталога установки, как z: у настоящего CrossOver указывает на корень ФС,
// только не на сам корень (там у generic-хелпера underKey своя особенность
// с завершающим разделителем, не имеющая отношения к этой задаче).
func makeSharedBottleDir(t *testing.T, bottlesDir, name, ancestor string) {
	t.Helper()
	path := filepath.Join(bottlesDir, name)
	if err := os.MkdirAll(filepath.Join(path, "dosdevices"), 0o755); err != nil {
		t.Fatalf("MkdirAll dosdevices: %v", err)
	}
	if err := os.Symlink(ancestor, filepath.Join(path, "dosdevices", "z:")); err != nil {
		t.Fatalf("Symlink z: %v", err)
	}
}

func TestWineRunnerBottlePrefersSharedBottle(t *testing.T) {
	bottlesDir := fakeBottlesDir(t)
	rt, log := fakeCrossOverRuntime(t, bottlesDir)
	games := t.TempDir()
	dest := filepath.Join(games, "Demo")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	makeSharedBottleDir(t, bottlesDir, wine.SharedBottleName(), games)

	r := wineRunner{
		gamesPath: func() string { return games },
		detect:    func() (wine.Runtime, error) { return rt, nil },
	}

	bottle, err := r.bottle(runSpec{Destination: dest})
	if err != nil {
		t.Fatalf("bottle: %v", err)
	}
	if !bottle.Shared {
		t.Fatal("bottle.Shared = false, want true: a shared bottle covers the destination")
	}
	if bottle.Name != wine.SharedBottleName() {
		t.Fatalf("bottle.Name = %q, want %q", bottle.Name, wine.SharedBottleName())
	}
	data, err := os.ReadFile(log)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("ReadFile log: %v", err)
	}
	if strings.Contains(string(data), "cxbottle") {
		t.Fatalf("cxbottle was invoked, want no calls when a shared bottle is available: %s", data)
	}
}

func TestWineRunnerBottleFallsBackWithoutSharedBottle(t *testing.T) {
	bottlesDir := fakeBottlesDir(t)
	rt, log := fakeCrossOverRuntime(t, bottlesDir)
	games := t.TempDir()
	dest := filepath.Join(games, "Demo")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	r := wineRunner{
		gamesPath: func() string { return games },
		detect:    func() (wine.Runtime, error) { return rt, nil },
	}

	bottle, err := r.bottle(runSpec{Destination: dest})
	if err != nil {
		t.Fatalf("bottle: %v", err)
	}
	if bottle.Shared {
		t.Fatal("bottle.Shared = true, want false: no shared bottle exists on this machine")
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("ReadFile log: %v", err)
	}
	if got := strings.Count(string(data), "cxbottle "); got != 1 {
		t.Fatalf("cxbottle calls = %d, want exactly 1: %s", got, data)
	}
}

func TestWineRunnerBottleSharedNameFromEnv(t *testing.T) {
	bottlesDir := fakeBottlesDir(t)
	t.Setenv(wine.SharedBottleEnv, "MyPlayStore")
	rt, _ := fakeCrossOverRuntime(t, bottlesDir)
	games := t.TempDir()
	dest := filepath.Join(games, "Demo")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	makeSharedBottleDir(t, bottlesDir, "MyPlayStore", games)

	r := wineRunner{
		gamesPath: func() string { return games },
		detect:    func() (wine.Runtime, error) { return rt, nil },
	}

	bottle, err := r.bottle(runSpec{Destination: dest})
	if err != nil {
		t.Fatalf("bottle: %v", err)
	}
	if !bottle.Shared || bottle.Name != "MyPlayStore" {
		t.Fatalf("bottle = %+v, want the shared bottle named by %s", bottle, wine.SharedBottleEnv)
	}
}

// TestWineRunnerBottleSharedHandlesSpecialPath проверяет перевод пути
// установки с пробелами и восклицательным знаком в windows-путь через
// подобранный общий бутыль — без какой-либо экранизации: ToWindows работает
// со строками, а не с шеллом.
func TestWineRunnerBottleSharedHandlesSpecialPath(t *testing.T) {
	bottlesDir := fakeBottlesDir(t)
	rt, _ := fakeCrossOverRuntime(t, bottlesDir)
	games := t.TempDir()
	dest := filepath.Join(games, "Kebab Chefs! Restaurant Simulator")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	makeSharedBottleDir(t, bottlesDir, wine.SharedBottleName(), games)

	r := wineRunner{
		gamesPath: func() string { return games },
		detect:    func() (wine.Runtime, error) { return rt, nil },
	}

	bottle, err := r.bottle(runSpec{Destination: dest})
	if err != nil {
		t.Fatalf("bottle: %v", err)
	}
	if !bottle.Shared {
		t.Fatal("bottle.Shared = false, want true")
	}
	winPath, err := bottle.ToWindows(filepath.Join(dest, "setup.exe"))
	if err != nil {
		t.Fatalf("ToWindows: %v", err)
	}
	want := strings.ToUpper(bottle.Drive) + `:\Kebab Chefs! Restaurant Simulator\setup.exe`
	if winPath != want {
		t.Fatalf("ToWindows = %q, want %q", winPath, want)
	}
}

func TestWineRunnerFailsWithoutCrossOver(t *testing.T) {
	games := t.TempDir()
	r := wineRunner{gamesPath: func() string { return games }, detect: failingDetect}

	_, err := r.run(context.Background(), runSpec{Path: filepath.Join(games, "Demo", "setup.exe"), Destination: filepath.Join(games, "Demo")})
	if err == nil {
		t.Fatal("run without CrossOver: want error")
	}
}

func TestWineRunnerNeedsDestination(t *testing.T) {
	games := t.TempDir()
	r := wineRunner{gamesPath: func() string { return games }, detect: failingDetect}

	if _, err := r.run(context.Background(), runSpec{}); err == nil {
		t.Fatal("run without a destination: want error")
	}
}

func TestWineRunnerInstallerArgs(t *testing.T) {
	games := t.TempDir()
	dest := filepath.Join(games, "Demo")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	installer := filepath.Join(dest, "setup.exe")
	if err := os.WriteFile(installer, []byte("MZ"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var recorded wine.Cmd
	r := wineRunner{
		gamesPath: func() string { return games },
		detect:    failingDetect,
		runCmd: func(_ context.Context, _ wine.Bottle, c wine.Cmd) (int, error) {
			recorded = c
			return 0, nil
		},
	}

	code, err := r.runPrepared(context.Background(), runSpec{
		Path: installer, Destination: dest, Dir: dest, Args: []string{"/VERYSILENT"}, LogPath: "/tmp/x.log",
	}, testBottle(games, dest))
	if err != nil {
		t.Fatalf("runPrepared: %v", err)
	}
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if recorded.Path != `T:\Demo\setup.exe` {
		t.Fatalf("Path = %q, want %q", recorded.Path, `T:\Demo\setup.exe`)
	}
	if recorded.WorkDir != `T:\Demo` {
		t.Fatalf("WorkDir = %q, want %q", recorded.WorkDir, `T:\Demo`)
	}
	if !recorded.WaitChildren {
		t.Fatal("WaitChildren = false: установщик распаковывает себя и работает уже оттуда")
	}
	if strings.Join(recorded.Args, " ") != "/VERYSILENT" {
		t.Fatalf("Args = %v", recorded.Args)
	}
	if recorded.Log != "/tmp/x.log.wine.log" {
		t.Fatalf("Log = %q", recorded.Log)
	}
}

// TestRunPreparedReportsInstallerNotConfirmedStopped закрывает КРИТ-находку:
// отмена во время установки должна приводить к тому же классу ошибки, что и
// на Windows (errInstallerNotConfirmedStopped), иначе discardSilent не
// узнает, что писатель мог остаться жив, и удалит каталог назначения гонкой.
func TestRunPreparedReportsInstallerNotConfirmedStopped(t *testing.T) {
	games := t.TempDir()
	dest := filepath.Join(games, "Demo")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	installer := filepath.Join(dest, "setup.exe")
	if err := os.WriteFile(installer, []byte("MZ"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	r := wineRunner{
		gamesPath: func() string { return games },
		detect:    failingDetect,
		runCmd: func(_ context.Context, _ wine.Bottle, _ wine.Cmd) (int, error) {
			return 0, fmt.Errorf("cxstart: %w", wine.ErrTreeNotStopped)
		},
	}

	_, err := r.runPrepared(context.Background(), runSpec{
		Path: installer, Destination: dest, Dir: dest,
	}, testBottle(games, dest))
	if err == nil {
		t.Fatal("runPrepared with an unstopped tree: want error")
	}
	if !errors.Is(err, errInstallerNotConfirmedStopped) {
		t.Fatalf("err = %v, want it to wrap errInstallerNotConfirmedStopped", err)
	}
}

func TestWineRunnerRejectsInstallerOutsideGames(t *testing.T) {
	games := t.TempDir()
	dest := filepath.Join(games, "Demo")
	r := wineRunner{gamesPath: func() string { return games }, detect: failingDetect}

	_, err := r.runPrepared(context.Background(), runSpec{
		Path: filepath.Join(t.TempDir(), "setup.exe"), Destination: dest,
	}, testBottle(games, dest))
	if err == nil {
		t.Fatal("installer outside the games folder: want error")
	}
}

func TestWineRunnerInstallerInDownloads(t *testing.T) {
	root := t.TempDir()
	games := filepath.Join(root, "Games")
	downloads := filepath.Join(root, "Downloads", "9-Bit Armies")
	prefix := filepath.Join(root, "Bottle")
	dos := filepath.Join(prefix, "dosdevices")
	for _, dir := range []string{games, downloads, dos} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(root, filepath.Join(dos, "z:")); err != nil {
		t.Fatal(err)
	}
	installer := filepath.Join(downloads, "setup.exe")
	if err := os.WriteFile(installer, []byte("fixture"), 0644); err != nil {
		t.Fatal(err)
	}
	b := wine.Bottle{Path: prefix, Games: games, Drive: "t"}
	called := false
	r := wineRunner{runCmd: func(ctx context.Context, b wine.Bottle, c wine.Cmd) (int, error) {
		called = true
		if len(c.Args) != 2 || c.Args[0] != `/DIR=T:\9-Bit Armies` || c.Args[1] != `/LOG=Z:\install.log` {
			t.Fatalf("native paths leaked into args: %+v", c.Args)
		}
		if c.Path != `Z:\Downloads\9-Bit Armies\setup.exe` || c.WorkDir != `Z:\Downloads\9-Bit Armies` {
			t.Fatalf("wrong mapped command: %+v", c)
		}
		return 0, nil
	}}
	if _, err := r.runPrepared(context.Background(), runSpec{Path: installer, Dir: downloads, Args: []string{"/DIR=" + filepath.Join(games, "9-Bit Armies"), "/LOG=" + filepath.Join(root, "install.log")}}, b); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("installer not launched")
	}
}

func TestWineFitGirlAudioIsScopedToInstaller(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "setup.exe")
	if got := wineInstallerDLLOverrides(EngineInno, path); got != "" {
		t.Fatal(got)
	}
	if err := os.WriteFile(filepath.Join(root, "fg-01.bin"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if got := wineInstallerDLLOverrides(EngineInno, path); got != "dsound=" {
		t.Fatalf("music override=%q", got)
	}
	for _, tc := range []struct {
		engine Engine
		path   string
	}{{EngineNsis, path}, {EngineInno, filepath.Join(root, "game.exe")}, {EngineInno, filepath.Join(root, "unins000.exe")}} {
		if got := wineInstallerDLLOverrides(tc.engine, tc.path); got != "" {
			t.Errorf("audio changed outside profile: %q", got)
		}
	}
}
