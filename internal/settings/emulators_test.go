package settings

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestEmulatorPathsCannotBePatched(t *testing.T) {
	svc := mustServiceAt(t, filepath.Join(t.TempDir(), "settings.json"))
	_, err := svc.SaveSettingsPatch(rawPatch(t, map[string]string{"emulatorPaths": `{"ps1":"C:\\evil.exe"}`}))
	if !errors.Is(err, ErrPatchEmulatorPath) {
		t.Fatalf("SaveSettingsPatch = %v", err)
	}
	if got := svc.GetSettings().EmulatorPaths; len(got) != 0 {
		t.Fatalf("an unchecked program reached the settings: %v", got)
	}
}

func TestEmulatorPathsSurviveARestartAndAnOtherPatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	svc := mustServiceAt(t, path)
	if _, err := svc.Update(func(next *Settings) error {
		next.EmulatorPaths = map[string]string{"ps1": `C:\e.exe`}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SaveSettingsPatch(rawPatch(t, map[string]string{"theme": `"light"`})); err != nil {
		t.Fatal(err)
	}
	if got := mustServiceAt(t, path).GetSettings().EmulatorPaths["ps1"]; got != `C:\e.exe` {
		t.Fatalf("path after restart: %q", got)
	}
}

func TestOlderSettingsFileGetsAnEmptyEmulatorMap(t *testing.T) {
	for _, body := range []string{`{"theme":"dark"}`, `{"emulatorPaths":null}`} {
		path := filepath.Join(t.TempDir(), "settings.json")
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := mustServiceAt(t, path).GetSettings().EmulatorPaths; got == nil || len(got) != 0 {
			t.Fatalf("%s: %v", body, got)
		}
	}
}
