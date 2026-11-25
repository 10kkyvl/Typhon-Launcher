package updates

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/handoffgate"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func writeHeldBackUpdates(t *testing.T) (dir, path string, orig []byte) {
	t.Helper()
	dir = t.TempDir()
	if err := newStore(dir).saveUpdates([]Update{{GameID: "g1"}}); err != nil {
		t.Fatal(err)
	}
	path = filepath.Join(dir, "updates.json")
	orig, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return dir, path, orig
}

func assertUpdatesUntouched(t *testing.T, path string, orig []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(orig) {
		t.Fatalf("updates.json = %s\nwant it untouched %s", got, orig)
	}
}

func TestHeldBackUpdatesShutdownKeepsTheList(t *testing.T) {
	dir, path, orig := writeHeldBackUpdates(t)
	s, err := newServiceAt(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	handoffgate.Arm()
	t.Cleanup(handoffgate.Open)
	ctx, cancel := context.WithCancel(t.Context())
	if err := s.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := s.ServiceShutdown(); err != nil {
		t.Fatalf("ServiceShutdown: %v", err)
	}
	assertUpdatesUntouched(t, path, orig)
}

// A check asked for from the successor's window before the start ran would
// save a list holding only that game over the user's.
func TestHeldBackUpdatesRefuseChanges(t *testing.T) {
	h := newHarness(t)
	if err := newStore(h.configDir).saveUpdates([]Update{{GameID: "other"}}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(h.configDir, "updates.json")
	orig, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	handoffgate.Arm()
	t.Cleanup(handoffgate.Open)
	if err := h.service.ServiceStartup(t.Context(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.CheckGame("local-1"); err == nil {
		t.Error("CheckGame on a held-back service = nil, want a refusal")
	}
	if err := h.service.PreparePlan("local-1"); err == nil {
		t.Error("PreparePlan on a held-back service = nil, want a refusal")
	}
	h.service.HandleSessionEnded("local-1", 3600)
	if err := h.service.ServiceShutdown(); err != nil {
		t.Fatalf("ServiceShutdown: %v", err)
	}
	assertUpdatesUntouched(t, path, orig)
}

func TestUnstartedUpdatesShutdownKeepsTheList(t *testing.T) {
	dir, path, orig := writeHeldBackUpdates(t)
	s, err := newServiceAt(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ServiceShutdown(); err != nil {
		t.Fatalf("ServiceShutdown: %v", err)
	}
	assertUpdatesUntouched(t, path, orig)
}
