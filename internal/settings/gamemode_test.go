package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGameModeDefaultsOff(t *testing.T) {
	if got := Defaults().GameMode; got != (GameMode{}) {
		t.Fatalf("game mode defaults = %+v, want everything off", got)
	}
}

func TestGameModeSurvivesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := mustServiceAt(t, path)
	next := s.GetSettings()
	next.GameMode = GameMode{RaisePriority: true, HideLauncher: true}
	if err := s.SaveSettings(next); err != nil {
		t.Fatalf("save: %v", err)
	}
	want := GameMode{RaisePriority: true, HideLauncher: true}
	if got := mustServiceAt(t, path).GetSettings().GameMode; got != want {
		t.Fatalf("game mode after reload = %+v, want %+v", got, want)
	}
}

func TestGameModeMissingInOldConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"minimizeToTray":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := mustServiceAt(t, path).GetSettings().GameMode; got != (GameMode{}) {
		t.Fatalf("legacy config game mode = %+v, want everything off", got)
	}
}
