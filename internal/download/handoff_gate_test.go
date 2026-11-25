package download

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/handoffgate"
	"typhon/internal/storage"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const heldBackDownloads = `[{"id":"d1","name":"Game","type":"torrent","source":"magnet:?xt=urn:btih:0000000000000000000000000000000000000000","infoHash":"0000000000000000000000000000000000000000","destination":"/tmp/x","status":"paused","selected":null,"downloaded":10,"total":100,"seeding":false,"addedAt":"2026-01-01T00:00:00Z","completedAt":null,"error":""}]`

func writeHeldBackDownloads(t *testing.T) (dir, list string) {
	t.Helper()
	dir = t.TempDir()
	list = filepath.Join(dir, "downloads.json")
	if err := storage.WriteAtomic(list, []byte(heldBackDownloads)); err != nil {
		t.Fatal(err)
	}
	return dir, list
}

func assertDownloadsUntouched(t *testing.T, list string) {
	t.Helper()
	got, err := os.ReadFile(list)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != heldBackDownloads {
		t.Fatalf("downloads.json = %s\nwant it untouched", got)
	}
}

// A successor launcher that quits before the gate opens (the old launcher
// hangs, the window is closed) still gets ServiceShutdown on the held-back
// manager, whose list was never read.
func TestHeldBackDownloadsShutdownKeepsTheList(t *testing.T) {
	dir, list := writeHeldBackDownloads(t)
	m, err := newManagerAt(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	closePieceCompletionOnCleanup(t, m)
	handoffgate.Arm()
	t.Cleanup(handoffgate.Open)
	ctx, cancel := context.WithCancel(t.Context())
	if err := m.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	cancel() // the application's quit ends the context before ServiceShutdown
	if err := m.ServiceShutdown(); err != nil {
		t.Fatalf("ServiceShutdown: %v", err)
	}
	assertDownloadsUntouched(t, list)
}

// A call from the successor's window reaches the manager before its start
// ran: every kind of change refuses and the list on disk stays as it was.
func TestHeldBackDownloadsRefuseChanges(t *testing.T) {
	dir, list := writeHeldBackDownloads(t)
	m, err := newManagerAt(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	closePieceCompletionOnCleanup(t, m)
	handoffgate.Arm()
	t.Cleanup(handoffgate.Open)
	if err := m.ServiceStartup(t.Context(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	moved := t.TempDir()
	calls := map[string]func() error{
		"start": func() error {
			_, err := m.StartDownloadFrom("0000000000000000000000000000000000000000", t.TempDir(), nil, Origin{})
			return err
		},
		"add": func() error {
			_, err := m.AddTask(t.Context(), AddRequest{Source: "magnet:?xt=urn:btih:1111111111111111111111111111111111111111", Destination: t.TempDir()})
			return err
		},
		"pause":  func() error { return m.Pause("d1") },
		"remove": func() error { return m.Remove("d1") },
		"move":   func() error { return m.MoveTo("d1", 0) },
		"delete": func() error { return m.DeleteData("d1") },
		"image":  func() error { return m.MarkImageAttempted("d1") },
		"repoint": func() error {
			return m.Repoint(t.Context(), filepath.Join(moved, "a"), filepath.Join(moved, "b"))
		},
		"cleanup": func() error { return m.RemoveCleanup(filepath.Join(dir, "leftover")) },
	}
	for name, call := range calls {
		if err := call(); err == nil {
			t.Errorf("%s on a held-back manager = nil, want a refusal", name)
		}
	}
	assertDownloadsUntouched(t, list)
}

// I-2 without the gate: a manager that never started never read its list,
// so its shutdown must not save the empty one over the file.
func TestUnstartedDownloadsShutdownKeepsTheList(t *testing.T) {
	dir, list := writeHeldBackDownloads(t)
	m, err := newManagerAt(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	closePieceCompletionOnCleanup(t, m)
	if err := m.ServiceShutdown(); err != nil {
		t.Fatalf("ServiceShutdown: %v", err)
	}
	assertDownloadsUntouched(t, list)
}
