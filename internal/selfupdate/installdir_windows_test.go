//go:build windows

package selfupdate

import (
	"errors"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows/registry"
)

func installDirTestKey(t *testing.T, name string) string {
	t.Helper()
	keyPath := `Software\Typhon-test-` + name
	restore := installDirKey
	installDirKey = keyPath
	t.Cleanup(func() {
		installDirKey = restore
		if err := registry.DeleteKey(registry.CURRENT_USER, keyPath); err != nil && !errors.Is(err, registry.ErrNotExist) {
			t.Errorf("DeleteKey: %v", err)
		}
	})
	return keyPath
}

func installedDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, uninstallerName), []byte("uninstaller"))
	return dir
}

// TestRecordInstallDirFor covers the migration for installations made before
// the installer wrote the key: nothing else knows where they live, so a
// manually started installer would land on the compiled default and leave them
// behind.
func TestRecordInstallDirFor(t *testing.T) {
	keyPath := installDirTestKey(t, "record")
	dir := installedDir(t)

	if err := recordInstallDirFor(dir); err != nil {
		t.Fatalf("recordInstallDirFor() error = %v, want nil", err)
	}
	if got := readInstallDir(t, keyPath); got != dir {
		t.Fatalf("recorded install dir = %q, want %q", got, dir)
	}

	if err := recordInstallDirFor(dir); err != nil {
		t.Fatalf("second recordInstallDirFor() error = %v, want nil", err)
	}
	if got := readInstallDir(t, keyPath); got != dir {
		t.Fatalf("recorded install dir after a repeat run = %q, want %q", got, dir)
	}
}

func TestRecordInstallDirForOverwritesAStaleValue(t *testing.T) {
	keyPath := installDirTestKey(t, "stale")
	dir := installedDir(t)

	key, _, err := registry.CreateKey(registry.CURRENT_USER, keyPath, registry.SET_VALUE)
	if err != nil {
		t.Fatalf("CreateKey: %v", err)
	}
	if err := key.SetStringValue(installDirValue, `C:\gone`); err != nil {
		t.Fatalf("SetStringValue: %v", err)
	}
	if err := key.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if err := recordInstallDirFor(dir); err != nil {
		t.Fatalf("recordInstallDirFor() error = %v, want nil", err)
	}
	if got := readInstallDir(t, keyPath); got != dir {
		t.Fatalf("recorded install dir = %q, want %q", got, dir)
	}
}

// A development build or an unpacked copy has no uninstaller beside it. Letting
// it record itself would send the next installer run into that directory.
func TestRecordInstallDirForSkipsACopyWithoutAnUninstaller(t *testing.T) {
	keyPath := installDirTestKey(t, "nouninstaller")

	if err := recordInstallDirFor(t.TempDir()); err != nil {
		t.Fatalf("recordInstallDirFor() error = %v, want nil", err)
	}
	if _, err := registry.OpenKey(registry.CURRENT_USER, keyPath, registry.QUERY_VALUE); !errors.Is(err, registry.ErrNotExist) {
		t.Fatalf("OpenKey(%q) error = %v, want ErrNotExist: nothing may be recorded for a copy", keyPath, err)
	}
}

func TestRecordInstallDirForRejectsABadDir(t *testing.T) {
	installDirTestKey(t, "baddir")

	if err := recordInstallDirFor(""); !errors.Is(err, errInstallDirEmpty) {
		t.Fatalf("recordInstallDirFor(\"\") error = %v, want errInstallDirEmpty", err)
	}
}

func readInstallDir(t *testing.T, keyPath string) string {
	t.Helper()
	key, err := registry.OpenKey(registry.CURRENT_USER, keyPath, registry.QUERY_VALUE)
	if err != nil {
		t.Fatalf("OpenKey: %v", err)
	}
	defer func() {
		if cerr := key.Close(); cerr != nil {
			t.Errorf("Close: %v", cerr)
		}
	}()
	got, _, err := key.GetStringValue(installDirValue)
	if err != nil {
		t.Fatalf("GetStringValue: %v", err)
	}
	return got
}
