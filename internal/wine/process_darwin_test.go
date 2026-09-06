package wine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParsePS(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "ps.txt"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	got := parsePS(string(data))
	if len(got) != 3 {
		t.Fatalf("parsePS returned %d entries, want 3 windows paths: %+v", len(got), got)
	}
	if got[1].pid != 88170 || got[1].winPath != `T:\DemoGame\InnoSetup\Compil32.exe` {
		t.Fatalf("entry = %+v", got[1])
	}
	if got[1].createdAt.IsZero() {
		t.Fatal("createdAt is zero")
	}
	// Путь с пробелом и аргументами: аргументы в путь попадать не должны.
	if got[2].winPath != `T:\Some Game\bin\game.exe` {
		t.Fatalf("winPath = %q, want %q", got[2].winPath, `T:\Some Game\bin\game.exe`)
	}
}

func TestParsePSIgnoresNativeAndHeader(t *testing.T) {
	got := parsePS("  PID                  STARTED COMMAND\n 1 Sun Sep  6 07:29:02 2026 /bin/zsh\n")
	if len(got) != 0 {
		t.Fatalf("parsePS = %+v, want none", got)
	}
}

func TestProcessesFiltersByBottle(t *testing.T) {
	m, _, _ := newTestManager(t)
	games := t.TempDir()
	dest := filepath.Join(games, "DemoGame")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	b, err := m.Ensure(dest, games)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	// Буква у бутыля выбирается свободной, поэтому фикстуру приводим к ней.
	data, err := os.ReadFile(filepath.Join("testdata", "ps.txt"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	m.psOutput = func() (string, error) { return retarget(string(data), b.Drive), nil }

	got, err := m.Processes(t.Context(), b)
	if err != nil {
		t.Fatalf("Processes: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Processes = %+v, want only the DemoGame entry", got)
	}
	if got[0].PID != 88170 {
		t.Fatalf("PID = %d, want 88170", got[0].PID)
	}
	want := filepath.Join(dest, "InnoSetup", "Compil32.exe")
	if got[0].Path != want {
		t.Fatalf("Path = %q, want %q", got[0].Path, want)
	}
}

func TestAllProcessesCoversEveryBottle(t *testing.T) {
	m, _, _ := newTestManager(t)
	games := t.TempDir()
	first := filepath.Join(games, "DemoGame")
	second := filepath.Join(games, "Some Game")
	for _, dir := range []string{first, second} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
	}
	a, err := m.Ensure(first, games)
	if err != nil {
		t.Fatalf("Ensure first: %v", err)
	}
	if _, err := m.Ensure(second, games); err != nil {
		t.Fatalf("Ensure second: %v", err)
	}
	data, err := os.ReadFile(filepath.Join("testdata", "ps.txt"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	// Обе игры видны через одну и ту же букву: папка игр у них общая.
	m.psOutput = func() (string, error) { return retarget(string(data), a.Drive), nil }

	got, err := m.AllProcesses(t.Context())
	if err != nil {
		t.Fatalf("AllProcesses: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("AllProcesses = %+v, want both games", got)
	}
}

func TestKillUsesBottlePrefix(t *testing.T) {
	m, bottles, log := newTestManager(t)
	b := Bottle{Name: "B", Path: filepath.Join(bottles, "B"), Drive: "t"}
	script := "#!/bin/sh\necho \"wineserver $@ prefix=$WINEPREFIX\" >> " + log + "\nexit 0\n"
	if err := os.WriteFile(m.rt.WineServer, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := m.Kill(b); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("ReadFile log: %v", err)
	}
	if !contains(string(data), "wineserver -k") {
		t.Fatalf("log %q missing 'wineserver -k'", string(data))
	}
	if !contains(string(data), "prefix="+b.Path) {
		t.Fatalf("log %q missing WINEPREFIX=%s", string(data), b.Path)
	}
}
