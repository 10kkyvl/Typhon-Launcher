package library

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"typhon/internal/shortcut"
)

type fakeShortcuts struct {
	desktop     string
	created     map[string]shortcut.Link
	createErr   error
	removeErr   error
	removed     []string
	nameErr     error
	desktopErr  error
	unsupported bool
}

func newFakeShortcuts(desktop string) *fakeShortcuts {
	return &fakeShortcuts{desktop: desktop, created: map[string]shortcut.Link{}}
}

func (f *fakeShortcuts) Supported() bool { return !f.unsupported }

func (f *fakeShortcuts) DesktopDir() (string, error) {
	if f.desktopErr != nil {
		return "", f.desktopErr
	}
	return f.desktop, nil
}

func (f *fakeShortcuts) FileName(title string) (string, error) {
	if f.nameErr != nil {
		return "", f.nameErr
	}
	return shortcut.FileName(title)
}

func (f *fakeShortcuts) Create(path string, link shortcut.Link) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.created[path] = link
	return nil
}

func (f *fakeShortcuts) Remove(path string) error {
	if f.removeErr != nil {
		return f.removeErr
	}
	f.removed = append(f.removed, path)
	delete(f.created, path)
	return nil
}

func shortcutService(t *testing.T) (*Service, *fakeShortcuts, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	fake := newFakeShortcuts(t.TempDir())
	s.shortcuts = fake
	s.launcherPath = func() (string, error) {
		return filepath.Join(t.TempDir(), "typhon.exe"), nil
	}
	return s, fake, path
}

func addShortcutGame(t *testing.T, s *Service) Game {
	t.Helper()
	game, err := s.AddGame(tempGameExe(t), "Half-Life 2")
	if err != nil {
		t.Fatalf("add game: %v", err)
	}
	return game
}

func TestCreateShortcutWritesLinkAndRemembersPath(t *testing.T) {
	s, fake, path := shortcutService(t)
	game := addShortcutGame(t, s)

	if err := s.CreateShortcut(game.ID); err != nil {
		t.Fatalf("create shortcut: %v", err)
	}

	want := filepath.Join(fake.desktop, "Half-Life 2.lnk")
	link, ok := fake.created[want]
	if !ok {
		t.Fatalf("shortcut not created at %s, got %v", want, fake.created)
	}
	if link.Args != "--play "+game.ID {
		t.Fatalf("args = %q", link.Args)
	}
	if link.Icon != game.Executable {
		t.Fatalf("icon = %q, want the game executable %q", link.Icon, game.Executable)
	}
	if !strings.HasSuffix(strings.ToLower(link.Target), "typhon.exe") {
		t.Fatalf("target = %q, want the launcher", link.Target)
	}

	stored, err := s.Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ShortcutPath != want {
		t.Fatalf("shortcut path = %q", stored.ShortcutPath)
	}
	reloaded := mustServiceAt(t, path)
	persisted, err := reloaded.Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.ShortcutPath != want {
		t.Fatalf("shortcut path not persisted: %q", persisted.ShortcutPath)
	}
}

func TestCreateShortcutRejectsGameWithoutExecutable(t *testing.T) {
	s, fake, _ := shortcutService(t)
	game := addShortcutGame(t, s)
	if err := os.Remove(game.Executable); err != nil {
		t.Fatal(err)
	}

	if err := s.CreateShortcut(game.ID); err == nil {
		t.Fatal("expected an error for a missing executable")
	}
	if len(fake.created) != 0 {
		t.Fatalf("shortcut created anyway: %v", fake.created)
	}
	stored, err := s.Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ShortcutPath != "" {
		t.Fatalf("shortcut path recorded without a shortcut: %q", stored.ShortcutPath)
	}
}

func TestCreateShortcutReportsBackendFailure(t *testing.T) {
	s, fake, _ := shortcutService(t)
	game := addShortcutGame(t, s)
	fake.createErr = errors.New("COM refused")

	err := s.CreateShortcut(game.ID)
	if err == nil {
		t.Fatal("expected the backend failure to reach the caller")
	}
	stored, findErr := s.Find(game.ID)
	if findErr != nil {
		t.Fatal(findErr)
	}
	if stored.ShortcutPath != "" {
		t.Fatalf("failed creation still recorded a path: %q", stored.ShortcutPath)
	}
}

func TestCreateShortcutRollsBackWhenPersistFails(t *testing.T) {
	s, fake, _ := shortcutService(t)
	game := addShortcutGame(t, s)

	// Каталог состояния подменяется на файл: persist гарантированно падает,
	// а созданный ярлык не должен пережить откат.
	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	s.path = filepath.Join(blocked, "library.json")

	if err := s.CreateShortcut(game.ID); err == nil {
		t.Fatal("expected persist to fail")
	}
	if len(fake.created) != 0 {
		t.Fatalf("shortcut left behind after rollback: %v", fake.created)
	}
	stored, err := s.Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ShortcutPath != "" {
		t.Fatalf("in-memory path not rolled back: %q", stored.ShortcutPath)
	}
}

func TestCreateShortcutReplacesRenamedLink(t *testing.T) {
	s, fake, _ := shortcutService(t)
	game := addShortcutGame(t, s)
	if err := s.CreateShortcut(game.ID); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(fake.desktop, "Half-Life 2.lnk")

	s.mu.Lock()
	s.findLocked(game.ID).Title = "Half-Life 3"
	s.mu.Unlock()

	if err := s.CreateShortcut(game.ID); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(fake.desktop, "Half-Life 3.lnk")
	if _, ok := fake.created[want]; !ok {
		t.Fatalf("renamed shortcut not created: %v", fake.created)
	}
	if _, ok := fake.created[old]; ok {
		t.Fatal("stale shortcut left on the desktop")
	}
}

func TestRemoveShortcutClearsPath(t *testing.T) {
	s, fake, _ := shortcutService(t)
	game := addShortcutGame(t, s)
	if err := s.CreateShortcut(game.ID); err != nil {
		t.Fatal(err)
	}

	if err := s.RemoveShortcut(game.ID); err != nil {
		t.Fatalf("remove shortcut: %v", err)
	}
	if len(fake.created) != 0 {
		t.Fatalf("shortcut file still there: %v", fake.created)
	}
	stored, err := s.Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ShortcutPath != "" {
		t.Fatalf("shortcut path = %q", stored.ShortcutPath)
	}
}

func TestRemoveShortcutWithoutOneIsNoOp(t *testing.T) {
	s, fake, _ := shortcutService(t)
	game := addShortcutGame(t, s)

	if err := s.RemoveShortcut(game.ID); err != nil {
		t.Fatalf("remove shortcut: %v", err)
	}
	if len(fake.removed) != 0 {
		t.Fatalf("removed something: %v", fake.removed)
	}
}

func TestRemoveShortcutKeepsPathWhenFileCannotBeRemoved(t *testing.T) {
	s, fake, _ := shortcutService(t)
	game := addShortcutGame(t, s)
	if err := s.CreateShortcut(game.ID); err != nil {
		t.Fatal(err)
	}
	fake.removeErr = errors.New("file locked")

	if err := s.RemoveShortcut(game.ID); err == nil {
		t.Fatal("expected the locked file to be reported")
	}
	stored, err := s.Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ShortcutPath == "" {
		t.Fatal("path forgotten even though the file is still on disk")
	}
}

func TestRemoveGameDropsShortcut(t *testing.T) {
	s, fake, _ := shortcutService(t)
	game := addShortcutGame(t, s)
	if err := s.CreateShortcut(game.ID); err != nil {
		t.Fatal(err)
	}

	if err := s.RemoveGame(game.ID); err != nil {
		t.Fatalf("remove game: %v", err)
	}
	if len(fake.created) != 0 {
		t.Fatalf("shortcut outlived the game: %v", fake.created)
	}
}

func TestMarkUninstalledDropsShortcut(t *testing.T) {
	s, fake, _ := shortcutService(t)
	game := addShortcutGame(t, s)
	if err := s.CreateShortcut(game.ID); err != nil {
		t.Fatal(err)
	}

	if err := s.MarkUninstalled(game.ID); err != nil {
		t.Fatalf("mark uninstalled: %v", err)
	}
	if len(fake.created) != 0 {
		t.Fatalf("shortcut outlived the installation: %v", fake.created)
	}
	stored, err := s.Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ShortcutPath != "" {
		t.Fatalf("shortcut path = %q", stored.ShortcutPath)
	}
}

func TestCreateShortcutRefusesUninstalledGame(t *testing.T) {
	s, fake, _ := shortcutService(t)
	game := addShortcutGame(t, s)
	if err := s.MarkUninstalled(game.ID); err != nil {
		t.Fatal(err)
	}

	if err := s.CreateShortcut(game.ID); !errors.Is(err, errShortcutUninstalled) {
		t.Fatalf("err = %v, want errShortcutUninstalled", err)
	}
	if len(fake.created) != 0 {
		t.Fatalf("shortcut created for an uninstalled game: %v", fake.created)
	}
}

func TestCreateShortcutUnsupportedPlatform(t *testing.T) {
	s, fake, _ := shortcutService(t)
	game := addShortcutGame(t, s)
	fake.unsupported = true

	if err := s.CreateShortcut(game.ID); !errors.Is(err, errShortcutUnsupported) {
		t.Fatalf("err = %v, want errShortcutUnsupported", err)
	}
}

func TestSafeCommandLineID(t *testing.T) {
	cases := []struct {
		id   string
		want bool
	}{
		{"a1b2c3d4", true},
		{"with-dash_and_underscore", true},
		{"", false},
		{"has space", false},
		{`quote"inside`, false},
		{"--play", false},
		{"semi;colon", false},
		{"кириллица", false},
	}
	for _, tc := range cases {
		if got := safeCommandLineID(tc.id); got != tc.want {
			t.Errorf("safeCommandLineID(%q) = %v, want %v", tc.id, got, tc.want)
		}
	}
}
