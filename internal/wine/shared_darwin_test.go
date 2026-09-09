package wine

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeSharedBottle раскладывает пользовательский бутыль так, как это делает
// CrossOver: dosdevices с c: на drive_c, z: на корень и y: на домашний
// каталог. Метку typhon-bottle.json сюда не пишем сознательно — общий бутыль
// не наш, и List/Lookup/Remove не должны его видеть.
func fakeSharedBottle(t *testing.T, bottles, name, home string) string {
	t.Helper()
	path := filepath.Join(bottles, name)
	dosdevices := filepath.Join(path, "dosdevices")
	if err := os.MkdirAll(filepath.Join(path, "drive_c"), 0o755); err != nil {
		t.Fatalf("MkdirAll drive_c: %v", err)
	}
	if err := os.MkdirAll(dosdevices, 0o755); err != nil {
		t.Fatalf("MkdirAll dosdevices: %v", err)
	}
	links := map[string]string{"c:": "../drive_c", "z:": "/", "y:": home}
	for letter, target := range links {
		if err := os.Symlink(target, filepath.Join(dosdevices, letter)); err != nil {
			t.Fatalf("Symlink %s: %v", letter, err)
		}
	}
	// Устройство: такие записи в dosdevices есть всегда и путями не являются.
	if err := os.Symlink("/dev/rdisk1s1", filepath.Join(dosdevices, "y::")); err != nil {
		t.Fatalf("Symlink device: %v", err)
	}
	return path
}

func TestSharedBottlePicksLongestDrive(t *testing.T) {
	m, bottles, _ := newTestManager(t)
	home := t.TempDir()
	fakeSharedBottle(t, bottles, "Steam", home)

	dest := filepath.Join(home, "Games", "Kebab Chefs! Restaurant Simulator")
	b, err := m.SharedBottle(dest)
	if err != nil {
		t.Fatalf("SharedBottle: %v", err)
	}
	if !b.Shared {
		t.Fatal("SharedBottle: бутыль не помечен общим")
	}
	if b.Name != "Steam" {
		t.Fatalf("Name = %q, want Steam", b.Name)
	}
	// z: тоже покрывает путь, но y: покрывает его точнее — и именно так же
	// путь переводит сам wine, что проверено на живом CrossOver.
	if b.Drive != "y" {
		t.Fatalf("Drive = %q, want y (самая длинная подходящая цель)", b.Drive)
	}
	if b.Key != dest {
		t.Fatalf("Key = %q, want %q", b.Key, dest)
	}
}

func TestSharedBottleWindowsPathKeepsSpaces(t *testing.T) {
	m, bottles, _ := newTestManager(t)
	home := t.TempDir()
	fakeSharedBottle(t, bottles, "Steam", home)

	dest := filepath.Join(home, "Games", "Kebab Chefs! Restaurant Simulator")
	b, err := m.SharedBottle(dest)
	if err != nil {
		t.Fatalf("SharedBottle: %v", err)
	}
	exe := filepath.Join(dest, "Kebab Chefs.exe")
	win, err := b.ToWindows(exe)
	if err != nil {
		t.Fatalf("ToWindows: %v", err)
	}
	want := `Y:\Games\Kebab Chefs! Restaurant Simulator\Kebab Chefs.exe`
	if win != want {
		t.Fatalf("ToWindows = %q, want %q", win, want)
	}
	back, err := b.ToNative(win)
	if err != nil {
		t.Fatalf("ToNative: %v", err)
	}
	if back != exe {
		t.Fatalf("ToNative = %q, want %q", back, exe)
	}
}

func TestSharedBottleMissing(t *testing.T) {
	m, _, _ := newTestManager(t)
	if _, err := m.SharedBottle(t.TempDir()); !errors.Is(err, ErrNoSharedBottle) {
		t.Fatalf("SharedBottle без бутыля = %v, want ErrNoSharedBottle", err)
	}
}

func TestSharedBottleNameFromEnv(t *testing.T) {
	m, bottles, _ := newTestManager(t)
	home := t.TempDir()
	fakeSharedBottle(t, bottles, "Steam Games", home)
	t.Setenv(SharedBottleEnv, "Steam Games")

	b, err := m.SharedBottle(filepath.Join(home, "Demo"))
	if err != nil {
		t.Fatalf("SharedBottle: %v", err)
	}
	if b.Name != "Steam Games" {
		t.Fatalf("Name = %q, want Steam Games", b.Name)
	}
}

func TestSharedBottleWithoutCoveringDrive(t *testing.T) {
	m, bottles, _ := newTestManager(t)
	path := filepath.Join(bottles, "Steam")
	if err := os.MkdirAll(filepath.Join(path, "dosdevices"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Symlink("../drive_c", filepath.Join(path, "dosdevices", "c:")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	if _, err := m.SharedBottle(t.TempDir()); !errors.Is(err, ErrNoDriveForPath) {
		t.Fatalf("SharedBottle = %v, want ErrNoDriveForPath", err)
	}
}

func TestSharedBottleReportsBrokenDriveEntry(t *testing.T) {
	m, bottles, _ := newTestManager(t)
	path := filepath.Join(bottles, "Steam", "dosdevices")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(path, "z:"), nil, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := m.SharedBottle(t.TempDir())
	if err == nil || errors.Is(err, ErrNoDriveForPath) || errors.Is(err, ErrNoSharedBottle) {
		t.Fatalf("SharedBottle with a broken drive entry = %v, want the read error", err)
	}
}

// TestSharedBottleStaysInvisibleToOwnAPI — самая важная гарантия задачи:
// пользовательский бутыль не должен попадать ни в перечисление наших, ни под
// удаление. Иначе снос игры унёс бы вместе с ней Steam.
func TestSharedBottleStaysInvisibleToOwnAPI(t *testing.T) {
	m, bottles, log := newTestManager(t)
	home := t.TempDir()
	path := fakeSharedBottle(t, bottles, "Steam", home)

	list, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("List = %+v, want пусто: общий бутыль не наш", list)
	}
	dest := filepath.Join(home, "Demo")
	if _, ok := m.Lookup(dest); ok {
		t.Fatal("Lookup нашёл общий бутыль")
	}
	if err := m.Remove(dest); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("общий бутыль исчез после Remove: %v", err)
	}
	if data, readErr := os.ReadFile(log); readErr == nil && contains(string(data), "--delete") {
		t.Fatalf("cxbottle звали на удаление: %s", data)
	}
	if _, err := os.Stat(filepath.Join(path, markerName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("в общий бутыль записана наша метка")
	}
}

func TestAllProcessesSeesSharedBottle(t *testing.T) {
	m, bottles, _ := newTestManager(t)
	home := t.TempDir()
	fakeSharedBottle(t, bottles, "Steam", home)

	m.psOutput = func() (string, error) {
		return strings.Join([]string{
			"  PID                  STARTED COMMAND",
			`87451 Sun Sep  6 07:31:35 2026 C:\Program Files (x86)\Steam\steam.exe -silent`,
			`88170 Sun Sep  6 07:33:37 2026 Y:\Games\Kebab Chefs! Restaurant Simulator\game.exe`,
		}, "\n"), nil
	}

	got, err := m.AllProcesses(t.Context())
	if err != nil {
		t.Fatalf("AllProcesses: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("AllProcesses = %+v, want только игру: c: — внутренности бутыля", got)
	}
	want := filepath.Join(home, "Games", "Kebab Chefs! Restaurant Simulator", "game.exe")
	if got[0].Path != want {
		t.Fatalf("Path = %q, want %q", got[0].Path, want)
	}
}

func TestSteamRunning(t *testing.T) {
	m, _, _ := newTestManager(t)
	m.psOutput = func() (string, error) {
		return "  PID                  STARTED COMMAND\n" +
			`87451 Sun Sep  6 07:31:35 2026 C:\windows\system32\notepad.exe` + "\n", nil
	}
	m.processEnv = func(int) (string, error) { return "CX_BOTTLE=Steam ", nil }
	b := Bottle{Name: "Steam"}
	running, err := m.SteamRunning(b)
	if err != nil {
		t.Fatalf("SteamRunning: %v", err)
	}
	if running {
		t.Fatal("SteamRunning = true без Steam в таблице процессов")
	}

	m.psOutput = func() (string, error) {
		return "  PID                  STARTED COMMAND\n" +
			`87451 Sun Sep  6 07:31:35 2026 C:\Program Files (x86)\Steam\steam.exe -silent` + "\n", nil
	}
	running, err = m.SteamRunning(b)
	if err != nil {
		t.Fatalf("SteamRunning: %v", err)
	}
	if !running {
		t.Fatal("SteamRunning = false при живом Steam")
	}
}

func TestSteamRunningIgnoresAnotherBottle(t *testing.T) {
	m, _, _ := newTestManager(t)
	m.psOutput = func() (string, error) {
		return "  PID                  STARTED COMMAND\n" +
			`87451 Sun Sep  6 07:31:35 2026 C:\Program Files (x86)\Steam\steam.exe` + "\n", nil
	}
	m.processEnv = func(pid int) (string, error) {
		if pid != 87451 {
			t.Fatalf("processEnv pid = %d, want 87451", pid)
		}
		return "CX_BOTTLE=Other WINEPREFIX=/bottles/Other ", nil
	}

	running, err := m.SteamRunning(Bottle{Name: "Steam"})
	if err != nil {
		t.Fatalf("SteamRunning: %v", err)
	}
	if running {
		t.Fatal("SteamRunning accepted Steam from another bottle")
	}
}

func TestProcessRunsInBottleSupportsSpaces(t *testing.T) {
	env := "PATH=/bin CX_BOTTLE=Steam Games WINEPREFIX=/bottles/Steam Games"
	if !processRunsInBottle(env, "Steam Games") {
		t.Fatal("processRunsInBottle did not match a bottle name containing spaces")
	}
	if processRunsInBottle(env, "Steam") {
		t.Fatal("processRunsInBottle matched a bottle-name prefix")
	}
}

func TestEnsureSteamSkipsRunning(t *testing.T) {
	m, bottles, log := newTestManager(t)
	home := t.TempDir()
	fakeSharedBottle(t, bottles, "Steam", home)
	m.psOutput = func() (string, error) {
		return "  PID                  STARTED COMMAND\n" +
			`87451 Sun Sep  6 07:31:35 2026 C:\Program Files (x86)\Steam\steam.exe` + "\n", nil
	}
	m.processEnv = func(int) (string, error) { return "CX_BOTTLE=Steam ", nil }
	b, err := m.SharedBottle(filepath.Join(home, "Demo"))
	if err != nil {
		t.Fatalf("SharedBottle: %v", err)
	}
	started, err := m.EnsureSteam(t.Context(), b)
	if err != nil {
		t.Fatalf("EnsureSteam: %v", err)
	}
	if started {
		t.Fatal("EnsureSteam запустил второй Steam поверх живого")
	}
	if data, readErr := os.ReadFile(log); readErr == nil && contains(string(data), "cxstart") {
		t.Fatalf("cxstart звали при уже запущенном Steam: %s", data)
	}
}

func TestEnsureSteamStartsAndWaits(t *testing.T) {
	m, bottles, log := newTestManager(t)
	home := t.TempDir()
	path := fakeSharedBottle(t, bottles, "Steam", home)
	exe := filepath.Join(path, "drive_c", "Program Files (x86)", "Steam", "steam.exe")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(exe, []byte("MZ"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	calls := 0
	m.psOutput = func() (string, error) {
		calls++
		head := "  PID                  STARTED COMMAND\n"
		if calls == 1 {
			return head, nil
		}
		return head + `87451 Sun Sep  6 07:31:35 2026 C:\Program Files (x86)\Steam\steam.exe -silent` + "\n", nil
	}
	m.processEnv = func(int) (string, error) { return "CX_BOTTLE=Steam ", nil }

	b, err := m.SharedBottle(filepath.Join(home, "Demo"))
	if err != nil {
		t.Fatalf("SharedBottle: %v", err)
	}
	started, err := m.EnsureSteam(t.Context(), b)
	if err != nil {
		t.Fatalf("EnsureSteam: %v", err)
	}
	if !started {
		t.Fatal("EnsureSteam не поднял Steam")
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	for _, want := range []string{"--bottle Steam", `C:\Program Files (x86)\Steam\steam.exe`, "-silent", "--no-wait"} {
		if !contains(string(data), want) {
			t.Fatalf("в вызове cxstart нет %q: %s", want, data)
		}
	}
}

func TestSteamExeNotInstalled(t *testing.T) {
	m, bottles, _ := newTestManager(t)
	home := t.TempDir()
	fakeSharedBottle(t, bottles, "Steam", home)
	b, err := m.SharedBottle(filepath.Join(home, "Demo"))
	if err != nil {
		t.Fatalf("SharedBottle: %v", err)
	}
	if _, err := b.SteamExe(); err == nil {
		t.Fatal("SteamExe нашёл steam.exe в пустом бутыле")
	}
}

// TestKillProcessesSparesTheBottle: остановка игры в общем бутыле не имеет
// права звать wineserver -k — он свалил бы Steam и все чужие игры префикса.
func TestKillProcessesSparesTheBottle(t *testing.T) {
	m, bottles, log := newTestManager(t)
	home := t.TempDir()
	fakeSharedBottle(t, bottles, "Steam", home)
	m.psOutput = func() (string, error) {
		return "  PID                  STARTED COMMAND\n" +
			`87451 Sun Sep  6 07:31:35 2026 C:\Program Files (x86)\Steam\steam.exe` + "\n", nil
	}
	b, err := m.SharedBottle(filepath.Join(home, "Demo"))
	if err != nil {
		t.Fatalf("SharedBottle: %v", err)
	}
	if err := m.KillProcesses(t.Context(), b); err != nil {
		t.Fatalf("KillProcesses: %v", err)
	}
	if data, readErr := os.ReadFile(log); readErr == nil && contains(string(data), "wineserver") {
		t.Fatalf("wineserver -k в общем бутыле: %s", data)
	}
}

// У общего бутыля путей больше, чем букв в Bottle: игра лежит на своей,
// записи реестра — на C:, сейвы — на третьей. Перевод обязан идти по всей
// таблице dosdevices, иначе деинсталлятор игры из общего бутыля не найдётся.
func TestSharedBottleToNativeUsesWholeDriveTable(t *testing.T) {
	m, bottles, _ := newTestManager(t)
	home := t.TempDir()
	path := fakeSharedBottle(t, bottles, "Steam", home)

	b, ok := m.SharedBottleAny()
	if !ok {
		t.Fatal("SharedBottleAny не нашёл общий бутыль")
	}
	if b.Name != "Steam" || !b.Shared {
		t.Fatalf("SharedBottleAny = %+v", b)
	}
	cases := map[string]string{
		`C:\Program Files (x86)\Steam\steamapps`:     filepath.Join(path, "drive_c", "Program Files (x86)", "Steam", "steamapps"),
		`Y:\Games\Kebab Chefs! Restaurant Simulator`: filepath.Join(home, "Games", "Kebab Chefs! Restaurant Simulator"),
	}
	for win, want := range cases {
		got, err := b.ToNative(win)
		if err != nil {
			t.Fatalf("ToNative(%q): %v", win, err)
		}
		if got != want {
			t.Fatalf("ToNative(%q) = %q, want %q", win, got, want)
		}
	}
	if _, err := b.ToNative(`Q:\Nowhere`); err == nil {
		t.Fatal("ToNative принял путь на букве, которой в бутыле нет")
	}
}

// Оборванная установка в общем бутыле обязана унести с собой установщик.
// По каталогу установки он не опознаётся — лежит в папке загрузок, — поэтому
// проверяется именно опознание по запущенному windows-пути.
func TestStopBottleKillsLaunchedInstaller(t *testing.T) {
	m, bottles, log := newTestManager(t)
	home := t.TempDir()
	fakeSharedBottle(t, bottles, "Steam", home)
	b, err := m.SharedBottle(filepath.Join(home, "Games", "Demo"))
	if err != nil {
		t.Fatalf("SharedBottle: %v", err)
	}

	victim := exec.Command("/bin/sleep", "30")
	if err := victim.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- victim.Wait() }()

	installer := `Z:\Users\x\Downloads\setup.exe`
	m.psOutput = func() (string, error) {
		return "  PID                  STARTED COMMAND\n" +
			strconv.Itoa(victim.Process.Pid) + " Sun Sep  6 07:31:35 2026 " + installer + "\n" +
			`87451 Sun Sep  6 07:31:35 2026 C:\Program Files (x86)\Steam\steam.exe` + "\n", nil
	}

	if err := m.stopBottle(b, installer); err != nil {
		t.Fatalf("stopBottle: %v", err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("установщик завершился сам, а не был убит")
		}
	case <-time.After(5 * time.Second):
		if killErr := victim.Process.Kill(); killErr != nil {
			t.Errorf("уборка: %v", killErr)
		}
		t.Fatal("установщик пережил остановку")
	}
	if data, readErr := os.ReadFile(log); readErr == nil && contains(string(data), "wineserver") {
		t.Fatalf("wineserver -k в общем бутыле: %s", data)
	}
}
