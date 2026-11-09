package download

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/settings"
)

// Repoint drops every engine before moving the files. When the move itself
// fails, the downloads are still where they were and must keep seeding:
// the engines come back and the records keep their old destinations.
func TestRepointFailureRestoresTheEnginesAndKeepsDestinations(t *testing.T) {
	cfg := settings.Defaults()
	cfg.SeedAfterDownload = true
	m, _ := newManagerWithSettings(t, cfg)
	m.client = offlineClient(t)
	d, mi := realDownload(t, m, "a", StatusCompleted, true)
	// The seed data moves under an explicit root so that oldRoot and newRoot
	// are siblings, not the shared temp parent.
	oldRoot := filepath.Join(t.TempDir(), "old")
	gameDir := filepath.Join(oldRoot, "GameA")
	if err := os.MkdirAll(oldRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(d.Destination, gameDir); err != nil {
		t.Fatal(err)
	}
	setItem(m, "a", func(d *Download) { d.Destination = gameDir })
	d = mustGetPtr(m, "a")
	attachReal(t, m, m.client, "a", mi, gameDir)

	// A non-empty directory at the new location makes the move refuse (it
	// never replaces anything but an empty directory).
	newRoot := filepath.Join(t.TempDir(), "new")
	occupied := filepath.Join(newRoot, filepath.Base(d.Destination))
	if err := os.MkdirAll(occupied, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(occupied, "someone-elses.bin"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := m.Repoint(t.Context(), oldRoot, newRoot)
	if err == nil {
		t.Fatal("Repoint succeeded although the target directory is occupied")
	}
	if errors.Is(err, errRepointNested) || errors.Is(err, errRepointSamePath) {
		t.Fatalf("fixture error, the move itself never ran: %v", err)
	}
	m.wg.Wait()

	got := mustGet(t, m, "a")
	if got.Destination != d.Destination {
		t.Fatalf("destination = %q after a failed move, want the old %q", got.Destination, d.Destination)
	}
	if got.Status != StatusCompleted || got.Error != "" {
		t.Fatalf("download = %s error=%q after a failed move, want completed without an error", got.Status, got.Error)
	}
	if !hasEngine(m, "a") {
		t.Fatal("the engine was dropped for the move and never came back; seeding stopped until restart")
	}
	if _, statErr := os.Stat(filepath.Join(d.Destination, "payload.bin")); statErr != nil {
		t.Fatalf("payload moved or lost: %v", statErr)
	}
}

// Once the manager is shutting down, Repoint must refuse instead of adding a
// restore goroutine behind ServiceShutdown's wait.
func TestRepointRefusesWhileTheManagerIsClosing(t *testing.T) {
	m := managerWithClient(t, 2)
	m.mu.Lock()
	m.closing = true
	m.mu.Unlock()
	err := m.Repoint(t.Context(), filepath.Join(t.TempDir(), "old"), filepath.Join(t.TempDir(), "new"))
	if !errors.Is(err, errUnavailable) {
		t.Fatalf("Repoint while closing = %v, want %v", err, errUnavailable)
	}
}

func mustGetPtr(m *Manager, id string) *Download {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.findLocked(id)
}
