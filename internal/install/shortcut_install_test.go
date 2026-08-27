package install

import (
	"errors"
	"path/filepath"
	"testing"
)

func setDesktopShortcuts(t *testing.T, s *Service, enabled bool) {
	t.Helper()
	cfg := s.settings.GetSettings()
	cfg.DesktopShortcuts = enabled
	if err := s.settings.SaveSettings(cfg); err != nil {
		t.Fatalf("save settings: %v", err)
	}
}

func (f *fakeRegistrar) shortcutsCreated() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.shortcuts...)
}

func installOneGame(t *testing.T, s *Service, downloads *fakeDownloads) {
	t.Helper()
	root := t.TempDir()
	portableSource(t, root, "Game")
	downloads.add("d1", "Game", root)
	dest := filepath.Join(t.TempDir(), "Games", "Game")
	item, err := s.Start("d1", StartOptions{Destination: dest, Mode: ModeCopy})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	s.waitStatus(t, item.ID, StatusCompleted)
}

func TestInstallCreatesDesktopShortcut(t *testing.T) {
	s, downloads, registrar := newTestService(t)
	setDesktopShortcuts(t, s, true)

	installOneGame(t, s, downloads)

	created := registrar.shortcutsCreated()
	games := registrar.registered()
	if len(games) != 1 {
		t.Fatalf("registered = %+v", games)
	}
	if len(created) != 1 {
		t.Fatalf("shortcuts created = %v, want one", created)
	}
}

func TestInstallSkipsShortcutWhenSettingOff(t *testing.T) {
	s, downloads, registrar := newTestService(t)
	setDesktopShortcuts(t, s, false)

	installOneGame(t, s, downloads)

	if created := registrar.shortcutsCreated(); len(created) != 0 {
		t.Fatalf("shortcuts created with the setting off: %v", created)
	}
}

func TestInstallSucceedsWhenShortcutFails(t *testing.T) {
	s, downloads, registrar := newTestService(t)
	setDesktopShortcuts(t, s, true)
	registrar.mu.Lock()
	registrar.shortcutErr = errors.New("desktop unavailable")
	registrar.mu.Unlock()

	installOneGame(t, s, downloads)

	if games := registrar.registered(); len(games) != 1 {
		t.Fatalf("a failed shortcut must not undo the install: %+v", games)
	}
}
