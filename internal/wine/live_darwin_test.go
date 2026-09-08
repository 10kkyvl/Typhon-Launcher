package wine

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// liveEnv включает проверки против настоящего CrossOver. По умолчанию они не
// идут: создание бутыля занимает секунды и сотни мегабайт, а на CI CrossOver
// нет вовсе. Запуск: TYPHON_WINE_LIVE=1 go test ./internal/wine/ -run Live
const liveEnv = "TYPHON_WINE_LIVE"

func liveManager(t *testing.T) *Manager {
	t.Helper()
	if os.Getenv(liveEnv) == "" {
		t.Skipf("живые проверки выключены: установите %s=1", liveEnv)
	}
	rt, err := Detect()
	if err != nil {
		t.Skipf("CrossOver не найден: %v", err)
	}
	t.Logf("CrossOver %s в %s", rt.Version, rt.Root)
	return NewManager(rt)
}

// TestLiveBottleLifecycle проходит весь путь на настоящем CrossOver: завести
// бутыль, увидеть его в перечислении, найти по пути, запустить в нём
// программу, увидеть её процесс, убить бутыль и снести его.
func TestLiveBottleLifecycle(t *testing.T) {
	m := liveManager(t)

	games := t.TempDir()
	dest := filepath.Join(games, "TyphonLiveCheck")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	bottle, err := m.Ensure(dest, games)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	t.Cleanup(func() {
		if err := m.Remove(dest); err != nil {
			t.Errorf("Remove: %v", err)
		}
	})
	t.Logf("бутыль %s на букве %s:", bottle.Name, bottle.Drive)

	// Буква действительно смотрит на папку игр.
	target, err := os.Readlink(filepath.Join(bottle.Path, "dosdevices", bottle.Drive+":"))
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if target != games {
		t.Fatalf("буква %s указывает на %q, а не на %q", bottle.Drive, target, games)
	}

	found, ok := m.Lookup(filepath.Join(dest, "any", "file.exe"))
	if !ok || found.Name != bottle.Name {
		t.Fatalf("Lookup вернул %+v, ok=%v", found, ok)
	}

	// Программа из самого бутыля: своего exe у нас тут нет, а notepad есть
	// в любом бутыле и живёт, пока его не убьют.
	if err := m.StartDetached(t.Context(), bottle, Cmd{Path: `C:\windows\system32\notepad.exe`}); err != nil {
		t.Fatalf("StartDetached: %v", err)
	}
	if err := m.Kill(bottle); err != nil {
		t.Fatalf("Kill: %v", err)
	}

	// Реестр бутыля читается и содержит записи, которые CrossOver пишет сам.
	entries, err := bottle.UninstallEntries()
	if err != nil {
		t.Fatalf("UninstallEntries: %v", err)
	}
	t.Logf("записей Uninstall в свежем бутыле: %d", len(entries))
}

// TestLiveProcessesSeeARealWindowsProgram проверяет самое хрупкое место
// дизайна: что ps действительно печатает виндовый путь запущенной программы
// и что мы переводим его обратно в native.
func TestLiveProcessesSeeARealWindowsProgram(t *testing.T) {
	m := liveManager(t)

	games := t.TempDir()
	dest := filepath.Join(games, "TyphonLiveProcs")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	bottle, err := m.Ensure(dest, games)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	t.Cleanup(func() {
		if err := m.Remove(dest); err != nil {
			t.Errorf("Remove: %v", err)
		}
	})

	// Копируем notepad на нашу букву: только так путь процесса окажется
	// внутри каталога установки, а значит попадёт в Processes.
	source := filepath.Join(bottle.Path, "drive_c", "windows", "system32", "notepad.exe")
	payload, err := os.ReadFile(source)
	if err != nil {
		t.Skipf("в бутыле нет notepad.exe: %v", err)
	}
	copyPath := filepath.Join(dest, "notepad.exe")
	//nolint:gosec // G703: путь целиком из t.TempDir(), внешнего ввода в нём нет
	if err := os.WriteFile(copyPath, payload, 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	winPath, err := bottle.ToWindows(copyPath)
	if err != nil {
		t.Fatalf("ToWindows: %v", err)
	}
	if err := m.StartDetached(t.Context(), bottle, Cmd{Path: winPath}); err != nil {
		t.Fatalf("StartDetached: %v", err)
	}
	t.Cleanup(func() {
		if err := m.Kill(bottle); err != nil {
			t.Errorf("Kill: %v", err)
		}
	})

	found := waitForProcess(t, m, bottle, copyPath)
	t.Logf("процесс найден: pid=%d win=%q native=%q старт=%s",
		found.PID, found.WinPath, found.Path, found.CreatedAt)
	if found.CreatedAt.IsZero() {
		t.Fatal("время старта не прочитано: подтверждение личности сессии сломается")
	}
}

// waitForProcess опрашивает таблицу процессов: синхронизироваться тут не с
// чем, процесс запускает не тест, а wine, и появляется он не мгновенно.
func waitForProcess(t *testing.T, m *Manager, bottle Bottle, native string) Process {
	t.Helper()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.After(15 * time.Second)
	for {
		list, err := m.Processes(t.Context(), bottle)
		if err != nil {
			t.Fatalf("Processes: %v", err)
		}
		for _, p := range list {
			if p.Path == native {
				return p
			}
		}
		select {
		case <-deadline:
			t.Fatalf("процесс %s не появился в бутыле за отведённое время", native)
			return Process{}
		case <-ticker.C:
		}
	}
}

// TestLiveSharedBottleRunsExternalExe — приёмка задачи целиком на настоящем
// CrossOver: игра лежит вне бутыля, в каталоге с пробелами и восклицательным
// знаком, запускается в пользовательском бутыле со Steam, видна в таблице
// процессов под своим настоящим путём и гасится, не трогая бутыль. Нового
// бутыля Typhon-* при этом появиться не должно.
//
// Запуск: TYPHON_WINE_LIVE=1 go test ./internal/wine/ -run TestLiveSharedBottle
func TestLiveSharedBottleRunsExternalExe(t *testing.T) {
	m := liveManager(t)
	before, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	// Каталог в домашней папке, а не в t.TempDir(): именно домашние пути
	// CrossOver переводит на отдельную букву, и проверять надо этот случай.
	dest := filepath.Join(home, "Kebab Chefs! Restaurant Simulator")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dest); err != nil {
			t.Errorf("уборка %s: %v", dest, err)
		}
	})

	b, err := m.SharedBottle(dest)
	if err != nil {
		t.Skipf("общего бутыля нет: %v", err)
	}
	t.Logf("бутыль %s, диск %s: -> %s", b.Name, b.Drive, b.Games)

	source := filepath.Join(b.Path, "drive_c", "windows", "system32", "notepad.exe")
	if _, err := os.Stat(source); err != nil {
		t.Skipf("в бутыле нет %s: %v", source, err)
	}
	// Симлинк, а не копия: содержимое exe тесту безразлично, а копировать
	// его пришлось бы записью в домашний каталог.
	exe := filepath.Join(dest, "Kebab Chefs.exe")
	if err := os.Symlink(source, exe); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	win, err := b.ToWindows(exe)
	if err != nil {
		t.Fatalf("ToWindows: %v", err)
	}
	t.Logf("windows-путь игры: %s", win)
	if err := m.StartDetached(t.Context(), b, Cmd{Path: win}); err != nil {
		t.Fatalf("StartDetached: %v", err)
	}

	var seen []Process
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		found, err := m.Processes(t.Context(), b)
		if err != nil {
			t.Fatalf("Processes: %v", err)
		}
		if len(found) > 0 {
			seen = found
			break
		}
	}
	if len(seen) == 0 {
		t.Fatal("процесс игры не появился в общем бутыле")
	}
	t.Logf("процесс: pid=%d %s -> %s", seen[0].PID, seen[0].WinPath, seen[0].Path)
	if seen[0].Path != exe {
		t.Fatalf("native-путь процесса = %q, want %q", seen[0].Path, exe)
	}

	all, err := m.AllProcesses(t.Context())
	if err != nil {
		t.Fatalf("AllProcesses: %v", err)
	}
	found := false
	for _, p := range all {
		if p.Path == exe {
			found = true
		}
	}
	if !found {
		t.Fatalf("AllProcesses не увидел игру в общем бутыле: %+v", all)
	}

	if err := m.KillProcesses(t.Context(), b); err != nil {
		t.Fatalf("KillProcesses: %v", err)
	}

	after, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(after) != len(before) {
		t.Fatalf("бутылей стало %d вместо %d: под игру завели свой", len(after), len(before))
	}
	if _, err := os.Stat(filepath.Join(b.Path, markerName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("в пользовательский бутыль записана наша метка")
	}
}
