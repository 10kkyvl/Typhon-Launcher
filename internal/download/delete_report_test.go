package download

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/uierr"
)

func completedWithFolder(t *testing.T) (*Manager, string) {
	t.Helper()
	dest := t.TempDir()
	root := filepath.Join(dest, "a")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "game.bin"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newTestManager(t, 2)
	m.addTestItem("a", StatusCompleted)
	m.mu.Lock()
	m.findLocked("a").Destination = dest
	m.mu.Unlock()
	return m, root
}

func TestDeleteDataReportsFilesLeftOnDisk(t *testing.T) {
	failure := errors.New("access denied")
	t.Run("folder removal fails", func(t *testing.T) {
		m, root := completedWithFolder(t)
		old := removeAll
		removeAll = func(string) error { return failure }
		t.Cleanup(func() { removeAll = old })

		err := m.DeleteData("a")
		if !errors.Is(err, errDataLeft) || !errors.Is(err, failure) {
			t.Fatalf("DeleteData = %v, want errDataLeft wrapping the cause", err)
		}
		if got := uierr.Code(err); got != "download.data_left" {
			t.Fatalf("code = %q", got)
		}
		if _, statErr := os.Stat(root); statErr != nil {
			t.Fatalf("folder must still be there: %v", statErr)
		}
	})
	t.Run("flat file removal fails", func(t *testing.T) {
		dest := t.TempDir()
		stuck := filepath.Join(dest, "stuck")
		if err := os.MkdirAll(stuck, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(stuck, "inner.txt"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		m := newTestManager(t, 2)
		m.addTestItem("a", StatusCompleted)
		m.mu.Lock()
		d := m.findLocked("a")
		d.Destination = dest
		d.Flat = true
		d.Files = []FileState{{Path: "stuck", Size: 1, Selected: true}}
		m.mu.Unlock()

		if err := m.DeleteData("a"); !errors.Is(err, errDataLeft) {
			t.Fatalf("DeleteData = %v, want errDataLeft", err)
		}
	})
	t.Run("clean removal returns nil", func(t *testing.T) {
		m, root := completedWithFolder(t)
		if err := m.DeleteData("a"); err != nil {
			t.Fatalf("DeleteData: %v", err)
		}
		if _, statErr := os.Stat(root); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("folder survived: %v", statErr)
		}
	})
}

func TestDeleteDataHonoursGuard(t *testing.T) {
	refused := uierr.New("test.refused", "in use")
	m, root := completedWithFolder(t)
	var seen Download
	m.SetDeleteGuard(func(d Download) error {
		seen = d
		return refused
	})

	if err := m.DeleteData("a"); !errors.Is(err, refused) {
		t.Fatalf("DeleteData = %v, want the guard error", err)
	}
	if _, err := m.Get("a"); err != nil {
		t.Fatalf("record dropped despite the guard: %v", err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("files removed despite the guard: %v", err)
	}
	if got, ok := seen.DataRoot(); !ok || got != root {
		t.Fatalf("guard saw root %q (%v), want %q", got, ok, root)
	}
}

func TestDataRoot(t *testing.T) {
	dest := t.TempDir()
	cases := []struct {
		name string
		d    Download
		want string
		ok   bool
	}{
		{"own folder", Download{Destination: dest, root: "Game"}, filepath.Join(dest, "Game"), true},
		{"name differs from folder", Download{Destination: dest, Name: "Shown", root: "Real"}, filepath.Join(dest, "Real"), true},
		{"flat has none", Download{Destination: dest, root: "Game", Flat: true}, "", false},
		{"in place has none", Download{Destination: dest, root: "Game", InPlace: true}, "", false},
		{"unsafe root", Download{Destination: dest, root: "../x"}, "", false},
		{"empty root", Download{Destination: dest}, "", false},
		{"empty destination", Download{root: "Game"}, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := c.d.DataRoot()
			if got != c.want || ok != c.ok {
				t.Fatalf("DataRoot = %q, %v; want %q, %v", got, ok, c.want, c.ok)
			}
		})
	}
}
