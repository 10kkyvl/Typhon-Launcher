//go:build !devmock

package settings

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func isolatedUserConfigDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"APPDATA", "XDG_CONFIG_HOME", "HOME"} {
		t.Setenv(name, root)
	}
	base, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("user config dir: %v", err)
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	migrateConfigDirOnce = sync.Once{}
	t.Cleanup(func() { migrateConfigDirOnce = sync.Once{} })
	return base
}

func TestConfigDirMovesTheAuroraFolderAndTheServiceReadsIt(t *testing.T) {
	base := isolatedUserConfigDir(t)
	legacy := filepath.Join(base, "Aurora")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "settings.json"), []byte(`{"theme":"light"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	dir, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir: %v", err)
	}
	if want := filepath.Join(base, "Typhon"); dir != want {
		t.Fatalf("ConfigDir = %q, want %q", dir, want)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("the Aurora folder is still there: %v", err)
	}

	svc, err := NewService()
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if got := svc.GetSettings().Theme; got != "light" {
		t.Fatalf("theme = %q, want the one saved under the old name", got)
	}
}

func TestConfigDirLeavesAuroraAloneWhenTyphonExists(t *testing.T) {
	base := isolatedUserConfigDir(t)
	legacy := filepath.Join(base, "Aurora")
	current := filepath.Join(base, "Typhon")
	for _, dir := range []string{legacy, current} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(legacy, "settings.json"), []byte(`{"theme":"light"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(current, "settings.json"), []byte(`{"theme":"dark"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	dir, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir: %v", err)
	}
	if dir != current {
		t.Fatalf("ConfigDir = %q, want %q", dir, current)
	}
	if got := readBytes(t, filepath.Join(legacy, "settings.json")); got != `{"theme":"light"}` {
		t.Fatalf("the old folder was touched: %q", got)
	}
	if got := readBytes(t, filepath.Join(current, "settings.json")); got != `{"theme":"dark"}` {
		t.Fatalf("the current folder was overwritten: %q", got)
	}
}

func TestConfigDirCreatesNothingOnItsOwn(t *testing.T) {
	base := isolatedUserConfigDir(t)

	dir, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir: %v", err)
	}
	if want := filepath.Join(base, "Typhon"); dir != want {
		t.Fatalf("ConfigDir = %q, want %q", dir, want)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("resolving the path created the folder: %v", err)
	}
	svc, err := NewService()
	if err != nil {
		t.Fatalf("NewService on a fresh machine: %v", err)
	}
	if svc.GetSettings() != Defaults() {
		t.Fatalf("a fresh machine did not start from the defaults: %+v", svc.GetSettings())
	}
}
