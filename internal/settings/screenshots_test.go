package settings

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"typhon/internal/uierr"
)

func TestScreenshotDefaults(t *testing.T) {
	d := Defaults()
	if !d.ScreenshotsEnabled || d.ScreenshotHotkey != ScreenshotHotkeyCtrlF12 {
		t.Fatalf("screenshot defaults = %v %q", d.ScreenshotsEnabled, d.ScreenshotHotkey)
	}
	if _, err := sanitize(d); err != nil {
		t.Fatalf("defaults must pass validation: %v", err)
	}
}

func TestOldConfigWithoutScreenshotFieldsLoadsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"theme":"dark","overlayHotkey":"Shift+F2"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got := mustServiceAt(t, path).GetSettings()
	if !got.ScreenshotsEnabled || got.ScreenshotHotkey != ScreenshotHotkeyCtrlF12 {
		t.Fatalf("screenshot fields of an old config = %v %q", got.ScreenshotsEnabled, got.ScreenshotHotkey)
	}
}

func TestStoredScreenshotsDisabledSurvivesLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"screenshotsEnabled":false,"screenshotHotkey":"Ctrl+F12"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got := mustServiceAt(t, path).GetSettings()
	if got.ScreenshotsEnabled || got.ScreenshotHotkey != ScreenshotHotkeyCtrlF12 {
		t.Fatalf("stored screenshot fields = %v %q", got.ScreenshotsEnabled, got.ScreenshotHotkey)
	}
}

func TestScreenshotHotkeyValidation(t *testing.T) {
	cases := []struct {
		name string
		key  string
		ok   bool
	}{
		{"f12", "F12", true},
		{"f11", "F11", true},
		{"shift f12", "Shift+F12", true},
		{"ctrl f12", "Ctrl+F12", true},
		{"empty", "", false},
		{"unknown", "F10", false},
		{"lower case", "f12", false},
		{"padded", " F12", false},
		{"overlay key", "Alt+`", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			svc := mustServiceAt(t, path)
			next := Defaults()
			next.ScreenshotHotkey = c.key
			err := svc.SaveSettings(next)
			if c.ok {
				if err != nil {
					t.Fatalf("SaveSettings: %v", err)
				}
				if got := mustServiceAt(t, path).GetSettings().ScreenshotHotkey; got != c.key {
					t.Fatalf("reloaded hotkey %q, want %q", got, c.key)
				}
				return
			}
			if !errors.Is(err, ErrScreenshotHotkeyInvalid) {
				t.Fatalf("error = %v, want ErrScreenshotHotkeyInvalid", err)
			}
			if uierr.Code(err) != "settings.screenshot_hotkey_invalid" {
				t.Fatalf("code = %q", uierr.Code(err))
			}
			if got := svc.GetSettings().ScreenshotHotkey; got != ScreenshotHotkeyCtrlF12 {
				t.Fatalf("a rejected hotkey changed the stored one to %q", got)
			}
		})
	}
}

func TestScreenshotHotkeysNeverCollideWithOverlay(t *testing.T) {
	for _, key := range ScreenshotHotkeys() {
		if slices.Contains(OverlayHotkeys(), key) {
			t.Errorf("%q is offered for both the overlay and screenshots", key)
		}
	}
}

func TestScreenshotFieldsAreLocal(t *testing.T) {
	names := jsonNames(Portable{})
	for _, n := range []string{"screenshotsEnabled", "screenshotHotkey"} {
		if slices.Contains(names, n) {
			t.Fatalf("%q must not travel with the account: the key may be taken by another program on this machine", n)
		}
	}
}
