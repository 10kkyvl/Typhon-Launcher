package download

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestShutdownAfterAFailedStartupNeverRewritesTheStateFile(t *testing.T) {
	cases := []struct {
		name  string
		raw   string
		isDir bool
	}{
		{name: "truncated json", raw: `[{"id":"a","name":"Game","status":"paused"`},
		{name: "garbage", raw: "not json at all"},
		{name: "scalar root", raw: "42"},
		{name: "a directory in place of the file", isDir: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "downloads.json")
			if c.isDir {
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(path, []byte(c.raw), 0o600); err != nil {
				t.Fatal(err)
			}
			m := mustManagerAt(t, dir)
			if err := m.ServiceStartup(t.Context(), application.ServiceOptions{}); err == nil {
				t.Fatal("an unreadable state must stop the start")
			}

			err := m.ServiceShutdown()

			if !errors.Is(err, errStateUnread) {
				t.Errorf("shutdown error = %v, want errStateUnread: the refused save must be reported", err)
			}
			if c.isDir {
				st, statErr := os.Stat(path)
				if statErr != nil || !st.IsDir() {
					t.Fatalf("the directory at the state path was replaced: %v", statErr)
				}
			} else {
				got, readErr := os.ReadFile(filepath.Clean(path))
				if readErr != nil {
					t.Fatal(readErr)
				}
				if string(got) != c.raw {
					t.Fatalf("state file = %q, want it byte for byte as it was: %q", got, c.raw)
				}
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range entries {
				if strings.HasPrefix(e.Name(), ".tmp") {
					t.Fatalf("a temporary file was left beside the state: %s", e.Name())
				}
			}
		})
	}
}

func TestEveryPersistIsRefusedAfterAFailedLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "downloads.json")
	const raw = `[{"id":"a"`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	m := mustManagerAt(t, dir)
	m.mu.Lock()
	loadErr := m.loadLocked()
	m.mu.Unlock()
	if loadErr == nil {
		t.Fatal("the truncated state was loaded")
	}

	m.mu.Lock()
	err := m.persistLocked()
	m.mu.Unlock()

	if !errors.Is(err, errStateUnread) {
		t.Fatalf("persist error = %v, want errStateUnread", err)
	}
	if st := m.degradedStatus(); !st.Degraded {
		t.Fatalf("degraded = %+v: the refused save must be surfaced", st)
	}
	got, readErr := os.ReadFile(filepath.Clean(path))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != raw {
		t.Fatalf("state file = %q, want %q", got, raw)
	}
}

func TestAMissingStateFileLoadsEmptyAndSaves(t *testing.T) {
	m := mustManagerAt(t, t.TempDir())
	m.mu.Lock()
	err := m.loadLocked()
	m.mu.Unlock()
	if err != nil {
		t.Fatalf("a first run has no state yet, which is not an error: %v", err)
	}

	m.addTestItem("a", StatusPaused)

	if got := persistedStatuses(t, m); got["a"] != StatusPaused {
		t.Fatalf("persisted = %v: a first run must be able to save", got)
	}
}
