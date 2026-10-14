package updates

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"typhon/internal/download"
	"typhon/internal/library"
)

func TestRepairKeepsExistingRollback(t *testing.T) {
	h := newHarness(t)
	h.withTorrent(damagedTorrentReport())
	old := h.installDir + previousSuffix
	writeFile(t, old, "game.exe", "PREVIOUS VERSION")
	if err := h.service.registerRollback(h.library.games[0], old); err != nil {
		t.Fatal(err)
	}
	if err := h.service.VerifyGame("local-1"); err != nil {
		t.Fatal(err)
	}
	h.awaitJob(t, "local-1")
	if err := h.service.RepairGame("local-1"); err != nil {
		t.Fatal(err)
	}
	h.awaitJob(t, "local-1")
	if b, err := os.ReadFile(filepath.Join(old, "game.exe")); err != nil || string(b) != "PREVIOUS VERSION" {
		t.Fatal("rollback lost")
	}
}

type cancellingDownloads struct {
	*fakeDownloads
	cancelled     int
	cancelContext context.CancelFunc
}

func (d *cancellingDownloads) AddTask(ctx context.Context, r download.AddRequest) (download.Download, error) {
	v, e := d.fakeDownloads.AddTask(ctx, r)
	if e == nil {
		d.mu.Lock()
		d.tasks[v.ID].Status = download.StatusDownloading
		d.mu.Unlock()
		d.cancelContext()
	}
	return v, e
}
func (d *cancellingDownloads) Cancel(id string) error { d.cancelled++; return nil }
func TestRepairCancelStopsDownload(t *testing.T) {
	h := newHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	d := &cancellingDownloads{fakeDownloads: h.downloads, cancelContext: cancel}
	h.service.downloads = d
	h.service.repair(ctx, h.library.games[0], h.releases.list[0], true)
	if d.cancelled != 1 || len(d.tasks) != 1 {
		t.Fatalf("download not cancelled: cancelled=%d tasks=%d", d.cancelled, len(d.tasks))
	}
	for _, task := range d.tasks {
		if task.Status != download.StatusDownloading {
			t.Fatal("not running")
		}
	}
}
func TestRecoveryRestoresVersionWithRollback(t *testing.T) {
	h := newHarness(t)
	old := h.installDir + previousSuffix
	writeFile(t, old, "game.exe", "OLD VERSION")
	h.library.games[0].Version = "2.0"
	j := SwapJournal{GameID: "local-1", Kind: JournalInplace, Original: &library.InstalledUpdate{ID: "local-1", InstallDir: h.installDir, Executable: filepath.Join(h.installDir, "game.exe"), Version: "1.0"}, InstallDir: h.installDir, Previous: old, Version: "2.0"}
	h.service.recoverInplaceJournal(j)
	b, err := os.ReadFile(filepath.Join(h.installDir, "game.exe"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "OLD VERSION" || h.library.games[0].Version != "1.0" {
		t.Fatalf("recovery left files %q and version %q", b, h.library.games[0].Version)
	}
}
