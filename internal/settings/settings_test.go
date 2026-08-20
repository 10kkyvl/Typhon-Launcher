package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := newServiceAt(path)

	next := s.GetSettings()
	next.Theme = "dark"
	next.UIScale = 1.1
	next.GamesPath = `D:\Games`
	next.MinimizeToTray = false
	if err := s.SaveSettings(next); err != nil {
		t.Fatal(err)
	}

	reloaded := newServiceAt(path).GetSettings()
	if reloaded != next {
		t.Fatalf("got %+v, want %+v", reloaded, next)
	}
}

func TestInvalidScaleFallsBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := newServiceAt(path)
	next := s.GetSettings()
	next.UIScale = 3
	if err := s.SaveSettings(next); err != nil {
		t.Fatal(err)
	}
	if got := s.GetSettings().UIScale; got != 1 {
		t.Fatalf("ui scale = %v, want 1", got)
	}
}

func TestCorruptFileFallsBackToDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := newServiceAt(path).GetSettings()
	if got != Defaults() {
		t.Fatalf("got %+v, want defaults", got)
	}
}
