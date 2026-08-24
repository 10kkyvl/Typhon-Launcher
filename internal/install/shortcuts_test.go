package install

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeShortcut(t *testing.T, path, target string) {
	t.Helper()
	data := append([]byte("L\x00\x00\x00"), utf16Bytes(target)...)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestReferencesPath(t *testing.T) {
	target := `C:\Games\GTA SA`
	cases := []struct {
		name string
		data []byte
		want bool
	}{
		{name: "ansi", data: []byte(`X` + target + `\gta_sa.exe`), want: true},
		{name: "utf16", data: append([]byte{0x4c, 0}, utf16Bytes(target+`\gta_sa.exe`)...), want: true},
		{name: "url с прямыми слешами", data: []byte("[InternetShortcut]\r\nURL=file:///C:/Games/GTA SA/gta_sa.exe\r\n"), want: true},
		{name: "другой регистр", data: []byte(strings.ToUpper(target)), want: true},
		{name: "чужой путь", data: []byte(`C:\Games\Other\game.exe`), want: false},
		{name: "пусто", data: nil, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := referencesPath(tc.data, target); got != tc.want {
				t.Fatalf("referencesPath = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestReferencesPathEmptyTarget(t *testing.T) {
	if referencesPath([]byte("anything"), "") {
		t.Fatal("пустая цель не должна совпадать ни с чем")
	}
}

func TestCleanShellShortcuts(t *testing.T) {
	desktop := t.TempDir()
	programs := t.TempDir()
	dest := filepath.Join(t.TempDir(), "GTA SA")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}

	mine := filepath.Join(desktop, "Моя папка.lnk")
	writeShortcut(t, mine, filepath.Join(dest, "gta_sa.exe"))
	oldGroup := filepath.Join(programs, "Старая группа")
	if err := os.MkdirAll(oldGroup, 0o755); err != nil {
		t.Fatalf("mkdir group: %v", err)
	}

	ctx := context.Background()
	before, err := takeShellSnapshot(ctx, []string{desktop, programs})
	if err != nil {
		t.Fatalf("takeShellSnapshot error = %v", err)
	}

	game := filepath.Join(desktop, "GTA SA.lnk")
	writeShortcut(t, game, filepath.Join(dest, "gta_sa.exe"))
	foreign := filepath.Join(desktop, "Браузер.lnk")
	writeShortcut(t, foreign, `C:\Program Files\Browser\browser.exe`)
	notShortcut := filepath.Join(desktop, "заметка.txt")
	if err := os.WriteFile(notShortcut, []byte(dest), 0o600); err != nil {
		t.Fatalf("write note: %v", err)
	}
	group := filepath.Join(programs, "GTA SA")
	if err := os.MkdirAll(group, 0o755); err != nil {
		t.Fatalf("mkdir new group: %v", err)
	}
	writeShortcut(t, filepath.Join(group, "Играть.lnk"), filepath.Join(dest, "gta_sa.exe"))
	writeShortcut(t, filepath.Join(oldGroup, "Играть.lnk"), filepath.Join(dest, "gta_sa.exe"))

	removed, err := cleanShellShortcuts(ctx, before, dest)
	if err != nil {
		t.Fatalf("cleanShellShortcuts error = %v", err)
	}
	if len(removed) != 4 {
		t.Fatalf("removed = %v, want 4 записи", removed)
	}
	for _, path := range []string{game, group, notShortcut} {
		if _, err := os.Stat(path); (err == nil) != (path == notShortcut) {
			t.Fatalf("stat %s = %v", path, err)
		}
	}
	if _, err := os.Stat(mine); err != nil {
		t.Fatalf("ярлык, существовавший до установки, удалён: %v", err)
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatalf("чужой ярлык удалён: %v", err)
	}
	if _, err := os.Stat(oldGroup); err != nil {
		t.Fatalf("существовавшая группа удалена: %v", err)
	}
}

func TestCleanShellShortcutsWithoutBaseline(t *testing.T) {
	desktop := t.TempDir()
	dest := t.TempDir()
	writeShortcut(t, filepath.Join(desktop, "GTA SA.lnk"), filepath.Join(dest, "gta_sa.exe"))

	removed, err := cleanShellShortcuts(context.Background(), shellSnapshot{}, dest)
	if err != nil {
		t.Fatalf("cleanShellShortcuts error = %v", err)
	}
	if len(removed) != 0 {
		t.Fatalf("без снимка до установки удалять нечего, removed = %v", removed)
	}
	if _, err := os.Stat(filepath.Join(desktop, "GTA SA.lnk")); err != nil {
		t.Fatalf("ярлык удалён без снимка: %v", err)
	}
}

func TestCleanShellShortcutsCancelled(t *testing.T) {
	desktop := t.TempDir()
	dest := t.TempDir()
	before, err := takeShellSnapshot(context.Background(), []string{desktop})
	if err != nil {
		t.Fatalf("takeShellSnapshot error = %v", err)
	}
	writeShortcut(t, filepath.Join(desktop, "GTA SA.lnk"), filepath.Join(dest, "gta_sa.exe"))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := cleanShellShortcuts(ctx, before, dest); err == nil {
		t.Fatal("отменённый ctx должен вернуть ошибку")
	}
	if _, err := os.Stat(filepath.Join(desktop, "GTA SA.lnk")); err != nil {
		t.Fatalf("ярлык удалён после отмены: %v", err)
	}
}

func TestTakeShellSnapshotMissingRoot(t *testing.T) {
	snap, err := takeShellSnapshot(context.Background(), []string{filepath.Join(t.TempDir(), "нет-такой-папки")})
	if err != nil {
		t.Fatalf("отсутствующий каталог не ошибка: %v", err)
	}
	if !snap.taken || len(snap.entries) != 0 {
		t.Fatalf("snapshot = %+v, want пустой но взятый", snap)
	}
}
