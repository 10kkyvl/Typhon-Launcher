package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPersonalAccentPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := mustServiceAt(t, path)
	next := s.GetSettings()
	next.AccentColor, next.TintLogo = "#ffFf00", true
	if err := s.SaveSettings(next); err != nil {
		t.Fatal(err)
	}
	loaded := mustServiceAt(t, path).GetSettings()
	if loaded.AccentColor != "#FFFF00" || !loaded.TintLogo {
		t.Fatalf("lost accent: %+v", loaded)
	}
	loaded.Theme = "light"
	if err := s.SaveSettings(loaded); err != nil {
		t.Fatal(err)
	}
	if got := mustServiceAt(t, path).GetSettings(); got.AccentColor != "#FFFF00" {
		t.Fatal("theme erased accent")
	}
	for _, color := range []string{"#fff", "#12345", "#12345678", "#GG0000"} {
		invalid := loaded
		invalid.AccentColor = color
		if err := s.SaveSettings(invalid); err == nil {
			t.Fatalf("accepted %q", color)
		}
		if s.GetSettings() != loaded {
			t.Fatal("invalid save changed memory")
		}
	}
	loaded.AccentColor, loaded.TintLogo = "", false
	if err := s.SaveSettings(loaded); err != nil {
		t.Fatal(err)
	}
	if got := mustServiceAt(t, path).GetSettings(); got.AccentColor != "" || got.TintLogo {
		t.Fatal("reset not persisted")
	}
}

func TestPersonalAccentFailedSaveKeepsMemory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := mustServiceAt(t, path)
	before := s.GetSettings()
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	next := before
	next.AccentColor, next.TintLogo = "#123456", true
	if err := s.SaveSettings(next); err == nil {
		t.Fatal("expected write failure")
	}
	if s.GetSettings() != before {
		t.Fatal("failed persistence changed saved accent")
	}
}
