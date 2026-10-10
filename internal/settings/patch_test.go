package settings

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func rawPatch(t *testing.T, fields map[string]string) map[string]json.RawMessage {
	t.Helper()
	out := make(map[string]json.RawMessage, len(fields))
	for key, value := range fields {
		out[key] = json.RawMessage(value)
	}
	return out
}

func TestSaveSettingsPatchChangesOnlyTheNamedFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	svc := mustServiceAt(t, path)
	seeded := svc.GetSettings()
	seeded.Theme = "light"
	seeded.UIScale = 1.25
	seeded.MaxActiveDownloads = 4
	seeded.MinimizeToTray = false
	if err := svc.SaveSettings(seeded); err != nil {
		t.Fatalf("seed: %v", err)
	}
	before := svc.GetSettings()

	cases := []struct {
		name  string
		patch map[string]string
		want  func(*Settings)
	}{
		{"one number", map[string]string{"maxActiveDownloads": "6"}, func(s *Settings) { s.MaxActiveDownloads = 6 }},
		{"one switch", map[string]string{"launchOnStartup": "false", "minimizeToTray": "true"}, func(s *Settings) { s.MinimizeToTray = true }},
		{"a string and a float", map[string]string{"theme": `"dark"`, "uiScale": "0.9"}, func(s *Settings) { s.Theme = "dark"; s.UIScale = 0.9 }},
		{"a large rate limit", map[string]string{"downloadRateLimit": "9007199254740993"}, func(s *Settings) { s.DownloadRateLimit = 9007199254740993 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := mustServiceAt(t, path)
			if err := svc.SaveSettings(before); err != nil {
				t.Fatalf("reset: %v", err)
			}
			want := before
			tc.want(&want)

			got, err := svc.SaveSettingsPatch(rawPatch(t, tc.patch))
			if err != nil {
				t.Fatalf("SaveSettingsPatch: %v", err)
			}
			if got != want {
				t.Fatalf("returned %+v, want %+v", got, want)
			}
			if mem := svc.GetSettings(); mem != want {
				t.Fatalf("memory %+v, want %+v", mem, want)
			}
			if disk := mustServiceAt(t, path).GetSettings(); disk != want {
				t.Fatalf("disk %+v, want %+v", disk, want)
			}
		})
	}
}

func TestSaveSettingsPatchRefusesWhatItCannotApply(t *testing.T) {
	cases := []struct {
		name    string
		patch   map[string]string
		wantErr error
		inText  string
	}{
		{"unknown key", map[string]string{"colour": `"red"`}, ErrPatchUnknownField, "colour"},
		{"unknown key beside a good one", map[string]string{"theme": `"light"`, "nope": "1"}, ErrPatchUnknownField, "nope"},
		{"key with the Go field name", map[string]string{"Theme": `"light"`}, ErrPatchUnknownField, "Theme"},
		{"games path derived from the library", map[string]string{"gamesPath": `"D:\\x"`}, ErrPatchDerivedField, "gamesPath"},
		{"downloads path derived from the library", map[string]string{"downloadsPath": `"D:\\x"`}, ErrPatchDerivedField, "downloadsPath"},
		{"screenshots path derived from the library", map[string]string{"screenshotsPath": `"D:\\x"`}, ErrPatchDerivedField, "screenshotsPath"},
		{"string for a number", map[string]string{"maxActiveDownloads": `"five"`}, ErrPatchInvalidValue, "maxActiveDownloads"},
		{"number for a string", map[string]string{"theme": "3"}, ErrPatchInvalidValue, "theme"},
		{"bool for a float", map[string]string{"uiScale": "true"}, ErrPatchInvalidValue, "uiScale"},
		{"fraction for an integer", map[string]string{"maxActiveDownloads": "1.5"}, ErrPatchInvalidValue, "maxActiveDownloads"},
		{"object for a switch", map[string]string{"tintLogo": `{}`}, ErrPatchInvalidValue, "tintLogo"},
		{"null", map[string]string{"theme": "null"}, ErrPatchNullValue, "theme"},
		{"null with spaces", map[string]string{"maxActiveDownloads": " null "}, ErrPatchNullValue, "maxActiveDownloads"},
		{"not JSON", map[string]string{"theme": `{`}, ErrPatchInvalidValue, "theme"},
		{"empty value", map[string]string{"theme": ``}, ErrPatchInvalidValue, "theme"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			svc := mustServiceAt(t, path)
			before := svc.GetSettings()

			_, err := svc.SaveSettingsPatch(rawPatch(t, tc.patch))
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error %v, want %v", err, tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.inText) {
				t.Fatalf("error %q does not name %q", err, tc.inText)
			}
			if got := svc.GetSettings(); got != before {
				t.Fatalf("memory changed to %+v", got)
			}
			if _, statErr := os.Stat(path); !errors.Is(statErr, fs.ErrNotExist) {
				t.Fatalf("a refused patch touched the file: %v", statErr)
			}
		})
	}
}

func TestSaveSettingsPatchAppliesNothingWhenOneKeyIsRefused(t *testing.T) {
	svc := mustServiceAt(t, filepath.Join(t.TempDir(), "settings.json"))
	before := svc.GetSettings()

	_, err := svc.SaveSettingsPatch(rawPatch(t, map[string]string{"theme": `"light"`, "maxActiveDownloads": `"x"`}))
	if err == nil {
		t.Fatal("expected an error")
	}
	if got := svc.GetSettings(); got != before {
		t.Fatalf("a half of the patch was applied: %+v", got)
	}
}

func TestSaveSettingsPatchWithNothingToChangeWritesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	svc := mustServiceAt(t, path)
	before := svc.GetSettings()

	for name, patch := range map[string]map[string]json.RawMessage{"nil": nil, "empty": {}} {
		got, err := svc.SaveSettingsPatch(patch)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got != before {
			t.Fatalf("%s: returned %+v, want the stored settings", name, got)
		}
	}
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("an empty patch wrote the file: %v", err)
	}
}

func TestSaveSettingsPatchFailedWriteKeepsMemory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	svc := mustServiceAt(t, path)
	before := svc.GetSettings()
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}

	got, err := svc.SaveSettingsPatch(rawPatch(t, map[string]string{"theme": `"light"`}))
	if err == nil {
		t.Fatal("expected the write to fail")
	}
	if got != (Settings{}) {
		t.Fatalf("a failed save returned %+v", got)
	}
	if mem := svc.GetSettings(); mem != before {
		t.Fatalf("a failed write changed the stored settings: %+v", mem)
	}
}

func TestSaveSettingsPatchKeepsASettingSavedWhileItWasInFlight(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	svc := mustServiceAt(t, path)
	reached, release := gateFirst(t, svc, func(_, next Settings) bool { return next.Theme == "light" })

	type result struct {
		saved Settings
		err   error
	}
	done := make(chan result, 1)
	go func() {
		saved, err := svc.SaveSettingsPatch(rawPatch(t, map[string]string{"theme": `"light"`}))
		done <- result{saved, err}
	}()
	<-reached

	other := svc.GetSettings()
	other.MaxActiveDownloads = 7
	other.MinimizeToTray = false
	if err := svc.SaveSettings(other); err != nil {
		t.Fatalf("concurrent save: %v", err)
	}
	release()
	res := <-done
	if res.err != nil {
		t.Fatalf("SaveSettingsPatch: %v", res.err)
	}

	for name, got := range map[string]Settings{
		"returned": res.saved,
		"memory":   svc.GetSettings(),
		"disk":     mustServiceAt(t, path).GetSettings(),
	} {
		if got.Theme != "light" {
			t.Errorf("%s: theme %q, the patched field was lost", name, got.Theme)
		}
		if got.MaxActiveDownloads != 7 || got.MinimizeToTray {
			t.Errorf("%s: downloads %d, tray %v: the patch wrote a stale copy over a newer save", name, got.MaxActiveDownloads, got.MinimizeToTray)
		}
	}
}
