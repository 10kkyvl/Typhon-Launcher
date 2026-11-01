//go:build windows

package protocolreg

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows/registry"
)

func deleteTree(t *testing.T, base string) {
	t.Helper()
	for _, key := range []string{`\shell\open\command`, `\shell\open`, `\shell`, `\DefaultIcon`, ``} {
		err := registry.DeleteKey(registry.CURRENT_USER, base+key)
		if err != nil && !errors.Is(err, registry.ErrNotExist) {
			t.Errorf("delete %s: %v", base+key, err)
		}
	}
}

func TestRunsFromIgnoresCase(t *testing.T) {
	dir := t.TempDir()
	exe := touch(t, filepath.Join(dir, "typhon.exe"))
	got, err := RunsFrom(exe, strings.ToUpper(dir))
	if err != nil || !got {
		t.Fatalf("RunsFrom = %v, %v", got, err)
	}
}

func TestRegistryStoreRoundTrip(t *testing.T) {
	base := `Software\TyphonProtocolRegTest`
	deleteTree(t, base)
	t.Cleanup(func() { deleteTree(t, base) })
	exe := filepath.Join(t.TempDir(), "typhon.exe")
	s := regStore{base: base}

	if _, found, err := s.get(`shell\open\command`, ""); err != nil || found {
		t.Fatalf("missing key: found=%v err=%v", found, err)
	}
	if err := apply(s, exe); err != nil {
		t.Fatal(err)
	}
	command, found, err := s.get(`shell\open\command`, "")
	if err != nil || !found || command != `"`+exe+`" "%1"` {
		t.Fatalf("command = %q, %v, %v", command, found, err)
	}
	marker, found, err := s.get("", "URL Protocol")
	if err != nil || !found || marker != "" {
		t.Fatalf("URL Protocol = %q, %v, %v", marker, found, err)
	}
}

func TestRegistryValueOfAnotherTypeIsReplaced(t *testing.T) {
	base := `Software\TyphonProtocolRegTypeTest`
	deleteTree(t, base)
	t.Cleanup(func() { deleteTree(t, base) })
	k, _, err := registry.CreateKey(registry.CURRENT_USER, base, registry.SET_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	if err := k.SetDWordValue("URL Protocol", 1); err != nil {
		t.Fatal(err)
	}
	if err := k.Close(); err != nil {
		t.Fatal(err)
	}
	s := regStore{base: base}
	if err := apply(s, filepath.Join(t.TempDir(), "typhon.exe")); err != nil {
		t.Fatal(err)
	}
	if data, found, err := s.get("", "URL Protocol"); err != nil || !found || data != "" {
		t.Fatalf("URL Protocol = %q, %v, %v", data, found, err)
	}
}
