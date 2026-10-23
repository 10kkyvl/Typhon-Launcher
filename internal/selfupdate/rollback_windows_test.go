//go:build windows

package selfupdate

import (
	"errors"
	"testing"

	"golang.org/x/sys/windows/registry"
)

const testUninstallKey = `Software\Typhon-test-suite-uninstall`

func useTestUninstallKey(t *testing.T) {
	t.Helper()
	restore := uninstallKeyPath
	uninstallKeyPath = testUninstallKey
	t.Cleanup(func() {
		uninstallKeyPath = restore
		if err := registry.DeleteKey(registry.CURRENT_USER, testUninstallKey); err != nil && !errors.Is(err, registry.ErrNotExist) {
			t.Errorf("delete test key: %v", err)
		}
	})
}

func createTestUninstallKey(t *testing.T, displayVersion string) {
	t.Helper()
	key, _, err := registry.CreateKey(registry.CURRENT_USER, testUninstallKey, registry.SET_VALUE)
	if err != nil {
		t.Fatalf("create key: %v", err)
	}
	defer func() {
		if err := key.Close(); err != nil {
			t.Errorf("close key: %v", err)
		}
	}()
	if displayVersion != "" {
		if err := key.SetStringValue(displayVersionValue, displayVersion); err != nil {
			t.Fatalf("seed DisplayVersion: %v", err)
		}
	}
}

func readTestDisplayVersion(t *testing.T) (string, bool) {
	t.Helper()
	key, err := registry.OpenKey(registry.CURRENT_USER, testUninstallKey, registry.QUERY_VALUE)
	if err != nil {
		t.Fatalf("open key: %v", err)
	}
	defer func() {
		if err := key.Close(); err != nil {
			t.Errorf("close key: %v", err)
		}
	}()
	value, _, err := key.GetStringValue(displayVersionValue)
	if errors.Is(err, registry.ErrNotExist) {
		return "", false
	}
	if err != nil {
		t.Fatalf("read DisplayVersion: %v", err)
	}
	return value, true
}

func TestSetDisplayVersionWritesAndReverts(t *testing.T) {
	useTestUninstallKey(t)
	createTestUninstallKey(t, "1.3.0")

	revert, err := setDisplayVersion("1.2.0")
	if err != nil {
		t.Fatalf("setDisplayVersion: %v", err)
	}
	if got, _ := readTestDisplayVersion(t); got != "1.2.0" {
		t.Fatalf("DisplayVersion = %q, want 1.2.0", got)
	}
	if err := revert(); err != nil {
		t.Fatalf("revert: %v", err)
	}
	if got, _ := readTestDisplayVersion(t); got != "1.3.0" {
		t.Fatalf("DisplayVersion after the revert = %q, want 1.3.0", got)
	}
}

func TestSetDisplayVersionWithoutAnEntryIsNotAnError(t *testing.T) {
	useTestUninstallKey(t)

	revert, err := setDisplayVersion("1.2.0")
	if err != nil {
		t.Fatalf("setDisplayVersion without a key: %v", err)
	}
	if err := revert(); err != nil {
		t.Fatalf("revert: %v", err)
	}
	if _, err := registry.OpenKey(registry.CURRENT_USER, testUninstallKey, registry.QUERY_VALUE); !errors.Is(err, registry.ErrNotExist) {
		t.Fatalf("the missing entry was created: %v", err)
	}
}

func TestSetDisplayVersionRevertRemovesAValueThatWasNotThere(t *testing.T) {
	useTestUninstallKey(t)
	createTestUninstallKey(t, "")

	revert, err := setDisplayVersion("1.2.0")
	if err != nil {
		t.Fatalf("setDisplayVersion: %v", err)
	}
	if got, ok := readTestDisplayVersion(t); !ok || got != "1.2.0" {
		t.Fatalf("DisplayVersion = %q (%v)", got, ok)
	}
	if err := revert(); err != nil {
		t.Fatalf("revert: %v", err)
	}
	if got, ok := readTestDisplayVersion(t); ok {
		t.Fatalf("DisplayVersion = %q after the revert, want the value gone", got)
	}
}
