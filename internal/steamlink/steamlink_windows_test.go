//go:build windows

package steamlink

import (
	"strings"
	"testing"

	"golang.org/x/sys/windows/registry"
)

func tempKey(t *testing.T) (string, registry.Key) {
	t.Helper()
	path := `Software\TyphonTest\steamlink-` + strings.ReplaceAll(t.Name(), "/", "-")
	k, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := k.Close(); err != nil {
			t.Errorf("close key: %v", err)
		}
		if err := registry.DeleteKey(registry.CURRENT_USER, path); err != nil {
			t.Errorf("delete key: %v", err)
		}
		if err := registry.DeleteKey(registry.CURRENT_USER, `Software\TyphonTest`); err != nil {
			t.Logf("delete parent key: %v", err)
		}
	})
	return path, k
}

func TestActivePID(t *testing.T) {
	t.Run("value present", func(t *testing.T) {
		path, k := tempKey(t)
		if err := k.SetDWordValue("pid", 4242); err != nil {
			t.Fatal(err)
		}
		got, err := activePID(path)
		if err != nil || got != 4242 {
			t.Fatalf("activePID = %d, %v", got, err)
		}
	})

	t.Run("zero means closed", func(t *testing.T) {
		path, k := tempKey(t)
		if err := k.SetDWordValue("pid", 0); err != nil {
			t.Fatal(err)
		}
		got, err := activePID(path)
		if err != nil || got != 0 {
			t.Fatalf("activePID = %d, %v", got, err)
		}
	})

	t.Run("missing value", func(t *testing.T) {
		path, _ := tempKey(t)
		got, err := activePID(path)
		if err != nil || got != 0 {
			t.Fatalf("activePID = %d, %v", got, err)
		}
	})

	t.Run("missing key", func(t *testing.T) {
		got, err := activePID(`Software\TyphonTest\steamlink-absent`)
		if err != nil || got != 0 {
			t.Fatalf("activePID = %d, %v", got, err)
		}
	})

	t.Run("wrong value type is an error", func(t *testing.T) {
		path, k := tempKey(t)
		if err := k.SetStringValue("pid", "12"); err != nil {
			t.Fatal(err)
		}
		if _, err := activePID(path); err == nil {
			t.Fatal("string pid accepted")
		}
	})
}

func TestJoinArgsEscapes(t *testing.T) {
	got := JoinArgs([]string{"-a", "two words", `say "hi"`, ""})
	want := `-a "two words" "say \"hi\"" ""`
	if got != want {
		t.Fatalf("JoinArgs = %s, want %s", got, want)
	}
}
