package download

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func isolateConfigDir(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
}

func TestNewManagerKeepsItsStateInTheConfigDir(t *testing.T) {
	isolateConfigDir(t, t.TempDir())
	base, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}

	m, err := NewManager(nil)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(func() {
		if err := m.closePieceCompletion(); err != nil {
			t.Errorf("close piece completion: %v", err)
		}
	})

	if want := filepath.Join(base, "Typhon"); m.store.dir != want {
		t.Fatalf("state directory = %q, want %q", m.store.dir, want)
	}
	if m.proxyStore == nil {
		t.Fatal("the manager has no store for the proxy password")
	}
	if m.max != maxActive(m.config()) {
		t.Fatalf("max active = %d", m.max)
	}
	if err := m.closePieceCompletion(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

func TestNewManagerFailsWithoutAConfigDir(t *testing.T) {
	isolateConfigDir(t, "")

	m, err := NewManager(nil)

	if err == nil {
		if cerr := m.closePieceCompletion(); cerr != nil {
			t.Error(cerr)
		}
		t.Fatal("a manager was built with no place to keep its state: its paths would resolve against the working directory")
	}
	if m != nil {
		t.Fatal("NewManager returned a manager together with an error")
	}
}

func TestWaitForHonoursItsContext(t *testing.T) {
	if err := waitFor(context.Background(), 0); err != nil {
		t.Fatalf("a wait that is due returned %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if err := waitFor(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if time.Since(start) > time.Minute {
		t.Fatal("a cancelled wait sat out its delay")
	}
}
