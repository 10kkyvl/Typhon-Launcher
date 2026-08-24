package library

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func gameDir(t *testing.T, root, name string) (string, string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "game.exe")
	if err := os.WriteFile(exe, []byte("stub"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir, exe
}

func TestApplyDiscoveredCreatesRecord(t *testing.T) {
	root := t.TempDir()
	dir, exe := gameDir(t, root, "Portal")
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))

	game, outcome, err := s.ApplyDiscovered(Discovered{Title: "Portal", Executable: exe, InstallDir: dir, SizeBytes: 4})
	if err != nil {
		t.Fatalf("apply discovered: %v", err)
	}
	if outcome != OutcomeCreated {
		t.Fatalf("outcome = %q, want %q", outcome, OutcomeCreated)
	}
	if game.Source != SourceDiscovered {
		t.Fatalf("source = %q, want %q", game.Source, SourceDiscovered)
	}
	if game.InstallDir != dir || game.Executable != exe {
		t.Fatalf("game = %+v, want install dir %q and executable %q", game, dir, exe)
	}
}

func TestApplyDiscoveredIsIdempotent(t *testing.T) {
	root := t.TempDir()
	dir, exe := gameDir(t, root, "Portal")
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	d := Discovered{Title: "Portal", Executable: exe, InstallDir: dir, SizeBytes: 4}

	if _, _, err := s.ApplyDiscovered(d); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	game, outcome, err := s.ApplyDiscovered(d)
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if outcome != OutcomeUnchanged {
		t.Fatalf("outcome = %q, want %q", outcome, OutcomeUnchanged)
	}
	if games := s.GetInstalledGames(); len(games) != 1 {
		t.Fatalf("games = %d, want 1", len(games))
	}
	if game.Title != "Portal" {
		t.Fatalf("title = %q, want Portal", game.Title)
	}
}

func TestApplyDiscoveredKeepsOnePerPath(t *testing.T) {
	root := t.TempDir()
	dir, exe := gameDir(t, root, "Portal")
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))

	if _, _, err := s.ApplyDiscovered(Discovered{Title: "Portal", Executable: exe, InstallDir: dir, SizeBytes: 4}); err != nil {
		t.Fatalf("apply discovered: %v", err)
	}
	// тот же каталог, записанный иначе: одна установка — одна запись
	noisy := filepath.Join(root, ".", "Portal")
	if _, outcome, err := s.ApplyDiscovered(Discovered{Title: "Portal 2", InstallDir: noisy, SizeBytes: 4}); err != nil {
		t.Fatalf("apply discovered: %v", err)
	} else if outcome == OutcomeCreated {
		t.Fatal("тот же физический путь не должен создавать вторую запись")
	}
	if games := s.GetInstalledGames(); len(games) != 1 {
		t.Fatalf("games = %d, want 1", len(games))
	}
}

func TestApplyDiscoveredMovesExistingRecord(t *testing.T) {
	root := t.TempDir()
	oldDir, oldExe := gameDir(t, root, "Portal")
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	created, _, err := s.ApplyDiscovered(Discovered{Title: "Portal", Executable: oldExe, InstallDir: oldDir, SizeBytes: 4})
	if err != nil {
		t.Fatalf("apply discovered: %v", err)
	}

	newDir := filepath.Join(root, "Portal Renamed")
	if err := os.Rename(oldDir, newDir); err != nil {
		t.Fatal(err)
	}
	newExe := filepath.Join(newDir, "game.exe")

	moved, outcome, err := s.ApplyDiscovered(Discovered{
		GameID: created.ID, Title: "Portal", Executable: newExe, InstallDir: newDir, SizeBytes: 4,
	})
	if err != nil {
		t.Fatalf("apply moved: %v", err)
	}
	if outcome != OutcomeUpdated {
		t.Fatalf("outcome = %q, want %q", outcome, OutcomeUpdated)
	}
	if moved.ID != created.ID {
		t.Fatalf("id = %q, want %q", moved.ID, created.ID)
	}
	if moved.InstallDir != newDir || moved.Executable != newExe {
		t.Fatalf("moved = %+v, want %q / %q", moved, newDir, newExe)
	}
	if games := s.GetInstalledGames(); len(games) != 1 {
		t.Fatalf("games = %d, want 1", len(games))
	}
}

func TestApplyDiscoveredAcceptsInstallWithoutExecutable(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Ambiguous")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))

	game, outcome, err := s.ApplyDiscovered(Discovered{Title: "Ambiguous", InstallDir: dir})
	if err != nil {
		t.Fatalf("apply discovered: %v", err)
	}
	if outcome != OutcomeCreated {
		t.Fatalf("outcome = %q, want %q", outcome, OutcomeCreated)
	}
	if game.Executable != "" {
		t.Fatalf("executable = %q, want empty", game.Executable)
	}
}

func TestApplyDiscoveredRejectsBadInput(t *testing.T) {
	root := t.TempDir()
	dir, exe := gameDir(t, root, "Portal")
	outside, _ := gameDir(t, root, "Other")

	cases := []struct {
		name string
		in   Discovered
	}{
		{"пустой каталог", Discovered{Title: "Portal", Executable: exe}},
		{"нет исполняемого файла", Discovered{Title: "Portal", InstallDir: dir, Executable: filepath.Join(dir, "missing.exe")}},
		{"исполняемый файл — каталог", Discovered{Title: "Portal", InstallDir: dir, Executable: dir}},
		{"файл вне каталога установки", Discovered{Title: "Portal", InstallDir: dir, Executable: filepath.Join(outside, "game.exe")}},
		{"нет названия", Discovered{InstallDir: dir}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
			if _, _, err := s.ApplyDiscovered(tc.in); err == nil {
				t.Fatal("некорректный ввод должен отклоняться")
			}
			if games := s.GetInstalledGames(); len(games) != 0 {
				t.Fatalf("games = %+v, want none", games)
			}
		})
	}
}

func TestApplyDiscoveredRollsBackOnPersistFailure(t *testing.T) {
	root := t.TempDir()
	dir, exe := gameDir(t, root, "Portal")
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)

	if err := os.MkdirAll(filepath.Join(path, "busy"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ApplyDiscovered(Discovered{Title: "Portal", Executable: exe, InstallDir: dir}); err == nil {
		t.Fatal("ошибка записи должна дойти до вызывающего")
	}
	if games := s.GetInstalledGames(); len(games) != 0 {
		t.Fatalf("games = %+v, want none after failed save", games)
	}
}

func TestSetExecutable(t *testing.T) {
	root := t.TempDir()
	dir, exe := gameDir(t, root, "Portal")
	outside, outsideExe := gameDir(t, root, "Other")
	_ = outside
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	game, _, err := s.ApplyDiscovered(Discovered{Title: "Portal", InstallDir: dir})
	if err != nil {
		t.Fatalf("apply discovered: %v", err)
	}

	if _, err := s.SetExecutable(game.ID, outsideExe); err == nil {
		t.Fatal("файл вне каталога установки должен отклоняться")
	}
	if _, err := s.SetExecutable(game.ID, filepath.Join(dir, "missing.exe")); err == nil {
		t.Fatal("несуществующий файл должен отклоняться")
	}
	if _, err := s.SetExecutable("unknown", exe); err == nil {
		t.Fatal("неизвестная игра должна отклоняться")
	}

	updated, err := s.SetExecutable(game.ID, exe)
	if err != nil {
		t.Fatalf("set executable: %v", err)
	}
	if !strings.EqualFold(updated.Executable, exe) {
		t.Fatalf("executable = %q, want %q", updated.Executable, exe)
	}

	reloaded := mustServiceAt(t, s.path)
	games := reloaded.GetInstalledGames()
	if len(games) != 1 || !strings.EqualFold(games[0].Executable, exe) {
		t.Fatalf("games = %+v, want saved executable %q", games, exe)
	}
}
