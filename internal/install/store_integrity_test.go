package install

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestNewServiceAtRejectsEmptyDir(t *testing.T) {
	if _, err := newServiceAt("", nil); err == nil {
		t.Fatal("empty dir must not produce a service")
	}
}

func TestCorruptInstallationsFailStartupAndKeepFile(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"truncated", `[{"id":"a"`},
		{"garbage", `not json at all`},
		{"scalar root", `42`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "installations.json")
			if err := os.WriteFile(path, []byte(tc.raw), 0o600); err != nil {
				t.Fatal(err)
			}
			s := mustServiceAt(t, dir)
			if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err == nil {
				t.Fatal("corrupt installations must not start the service")
			}
			got, err := os.ReadFile(filepath.Clean(path))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.raw {
				t.Fatalf("file rewritten: %q", got)
			}
		})
	}
}

func TestMissingInstallationsStartEmptyAndSave(t *testing.T) {
	dir := t.TempDir()
	st := newStore(dir)
	items, err := st.load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if items != nil {
		t.Fatalf("items = %+v, want nil", items)
	}
	if err := st.save([]Installation{{ID: "a"}}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := os.Stat(st.listPath()); err != nil {
		t.Fatalf("installations not saved: %v", err)
	}
}

func TestRegisterRejectsEmptyDestination(t *testing.T) {
	s := mustServiceAt(t, t.TempDir())
	s.library = &fakeRegistrar{}
	if _, err := s.register(Installation{ID: "a", Name: "Game"}, "1.0", "manual"); err == nil {
		t.Fatal("empty destination must be rejected")
	}
}
