package install

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestFailedLoadNeverOverwritesInstallations(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"truncated", `[{"id":"a","name":"Game"`},
		{"garbage", `not json at all`},
		{"scalar root", `42`},
		{"empty file", ``},
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
				t.Fatal("unreadable installations must not start the service")
			}

			if err := s.ServiceShutdown(); err != nil {
				t.Fatalf("ServiceShutdown after a failed startup = %v, the load error was already reported by startup", err)
			}
			got, err := os.ReadFile(filepath.Clean(path))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.raw {
				t.Fatalf("installations.json after the failed startup and shutdown = %q, want it untouched %q", got, tc.raw)
			}

			s.mu.Lock()
			persistErr := s.persistNowLocked()
			s.mu.Unlock()
			if persistErr == nil {
				t.Fatal("a service whose load failed accepted a persist")
			}
			again, err := os.ReadFile(filepath.Clean(path))
			if err != nil {
				t.Fatal(err)
			}
			if string(again) != tc.raw {
				t.Fatalf("installations.json after a refused persist = %q, want it untouched %q", again, tc.raw)
			}
		})
	}
}

func TestFailedLoadKeepsDirectoryInPlaceOfInstallations(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "installations.json")
	marker := filepath.Join(path, "keep.txt")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := mustServiceAt(t, dir)
	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err == nil {
		t.Fatal("a directory in place of installations.json must not start the service")
	}
	if err := s.ServiceShutdown(); err != nil {
		t.Fatalf("ServiceShutdown after a failed startup = %v, the load error was already reported by startup", err)
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		t.Fatalf("installations.json after shutdown: info %v err %v, want the directory", info, err)
	}
	if got, err := os.ReadFile(marker); err != nil || string(got) != "keep" {
		t.Fatalf("file inside the directory = %q, %v", got, err)
	}
}

func TestSuccessfulLoadStillPersistsOnShutdown(t *testing.T) {
	dir := t.TempDir()
	st := newStore(dir)
	if err := st.save([]Installation{{ID: "a", Name: "Game", Status: StatusCompleted}}); err != nil {
		t.Fatal(err)
	}
	s := mustServiceAt(t, dir)
	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.findLocked("a").Name = "Renamed"
	s.mu.Unlock()
	if err := s.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
	items, err := newStore(dir).load()
	if err != nil || len(items) != 1 || items[0].Name != "Renamed" {
		t.Fatalf("after a clean shutdown: %+v, %v", items, err)
	}
}
