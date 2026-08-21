package download

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestNewManagerAtRejectsEmptyDir(t *testing.T) {
	if _, err := newManagerAt("", nil); err == nil {
		t.Fatal("empty dir must not produce a manager")
	}
}

func TestCorruptDownloadsFailStartupAndKeepFile(t *testing.T) {
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
			path := filepath.Join(dir, "downloads.json")
			if err := os.WriteFile(path, []byte(tc.raw), 0o600); err != nil {
				t.Fatal(err)
			}
			m := mustManagerAt(t, dir)
			if err := m.ServiceStartup(context.Background(), application.ServiceOptions{}); err == nil {
				t.Fatal("corrupt downloads must not start the manager")
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

func TestMissingDownloadsStartEmptyAndSave(t *testing.T) {
	dir := t.TempDir()
	s := newStore(dir)
	records, err := s.load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if records != nil {
		t.Fatalf("records = %+v, want nil", records)
	}
	if err := s.save([]record{{ID: "a", InfoHash: "aaaa"}}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := os.Stat(s.listPath()); err != nil {
		t.Fatalf("downloads not saved: %v", err)
	}
}
