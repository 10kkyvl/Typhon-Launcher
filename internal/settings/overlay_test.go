package settings

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/uierr"
)

func TestOverlayDefaults(t *testing.T) {
	d := Defaults()
	if !d.OverlayEnabled || d.OverlayHotkey != OverlayHotkeyAltBacktick {
		t.Fatalf("overlay defaults = %v %q", d.OverlayEnabled, d.OverlayHotkey)
	}
	if _, err := sanitize(d); err != nil {
		t.Fatalf("defaults must pass validation: %v", err)
	}
}

func TestOldConfigWithoutOverlayFieldsLoadsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"theme":"dark"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got := mustServiceAt(t, path).GetSettings()
	if !got.OverlayEnabled || got.OverlayHotkey != OverlayHotkeyAltBacktick {
		t.Fatalf("overlay fields of an old config = %v %q", got.OverlayEnabled, got.OverlayHotkey)
	}
}

func TestStoredOverlayDisabledSurvivesLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"overlayEnabled":false,"overlayHotkey":"Shift+F2"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got := mustServiceAt(t, path).GetSettings()
	if got.OverlayEnabled || got.OverlayHotkey != OverlayHotkeyShiftF2 {
		t.Fatalf("stored overlay fields = %v %q", got.OverlayEnabled, got.OverlayHotkey)
	}
}

func TestOverlayHotkeyValidation(t *testing.T) {
	cases := []struct {
		name string
		key  string
		ok   bool
	}{
		{"alt backtick", "Alt+`", true},
		{"shift f1", "Shift+F1", true},
		{"shift f2", "Shift+F2", true},
		{"ctrl shift o", "Ctrl+Shift+O", true},
		{"empty", "", false},
		{"unknown combination", "Alt+Q", false},
		{"lower case", "alt+`", false},
		{"padded", " Alt+` ", false},
		{"bare function key", "F1", false},
		{"modifiers reordered", "Shift+Ctrl+O", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			svc := mustServiceAt(t, path)
			called := false
			if err := svc.AddApplier(func(prev, next Settings) error { called = true; return nil }); err != nil {
				t.Fatal(err)
			}
			next := Defaults()
			next.OverlayHotkey = c.key
			err := svc.SaveSettings(next)
			if c.ok {
				if err != nil {
					t.Fatalf("SaveSettings: %v", err)
				}
				if got := mustServiceAt(t, path).GetSettings().OverlayHotkey; got != c.key {
					t.Fatalf("reloaded hotkey %q, want %q", got, c.key)
				}
				return
			}
			if !errors.Is(err, ErrOverlayHotkeyInvalid) {
				t.Fatalf("error = %v, want ErrOverlayHotkeyInvalid", err)
			}
			if uierr.Code(err) != "settings.overlay_hotkey_invalid" {
				t.Fatalf("code = %q", uierr.Code(err))
			}
			if called {
				t.Fatal("an applier ran for settings that failed validation")
			}
			if got := svc.GetSettings().OverlayHotkey; got != OverlayHotkeyAltBacktick {
				t.Fatalf("a rejected hotkey changed the stored one to %q", got)
			}
			if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("a rejected save wrote the settings file: %v", statErr)
			}
		})
	}
}

func TestLoadRefusesUnknownOverlayHotkey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"overlayHotkey":"Alt+Q"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	svc, err := NewServiceAt(path)
	if err == nil {
		t.Fatalf("a file with an unknown overlay hotkey must not be replaced by the default, got %+v", svc.GetSettings())
	}
	if !errors.Is(err, ErrOverlayHotkeyInvalid) {
		t.Fatalf("error = %v, want ErrOverlayHotkeyInvalid", err)
	}
}

func TestOverlayFieldsAreLocal(t *testing.T) {
	names := jsonNames(Portable{})
	for _, n := range []string{"overlayEnabled", "overlayHotkey"} {
		for _, p := range names {
			if p == n {
				t.Fatalf("%q must not travel with the account: the key may be taken by another program on this machine", n)
			}
		}
	}
}
