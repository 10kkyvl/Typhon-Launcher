package updates

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"typhon/internal/download"
	"typhon/internal/library"
)

func TestRegressionRepairDestroysRollback(t *testing.T) {
	h := newHarness(t)
	h.withTorrent(damagedTorrentReport())
	old := h.installDir + previousSuffix
	writeFile(t, old, "game.exe", "PREVIOUS VERSION")
	h.service.registerRollback(h.library.games[0], old)
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
	t.Log("Regression: repair removed pre-existing rollback files")
}

type auditDownloads struct {
	*fakeDownloads
	cancelled     int
	cancelContext context.CancelFunc
}

func (d *auditDownloads) AddTask(ctx context.Context, r download.AddRequest) (download.Download, error) {
	v, e := d.fakeDownloads.AddTask(ctx, r)
	if e == nil {
		d.mu.Lock()
		d.tasks[v.ID].Status = download.StatusDownloading
		d.mu.Unlock()
		d.cancelContext()
	}
	return v, e
}
func (d *auditDownloads) Cancel(id string) error { d.cancelled++; return nil }
func TestRegressionRepairCancellationLeavesDownload(t *testing.T) {
	h := newHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	d := &auditDownloads{fakeDownloads: h.downloads, cancelContext: cancel}
	h.service.downloads = d
	h.service.repair(ctx, h.library.games[0], h.releases.list[0], true)
	if d.cancelled != 1 || len(d.tasks) != 1 {
		t.Fatal("not reproduced")
	}
	for _, task := range d.tasks {
		if task.Status != download.StatusDownloading {
			t.Fatal("not running")
		}
	}
	t.Log("Regression: repair rolled back without canceling live download")
}
func TestRegressionRecoveryKeepsNewVersionLabelAfterRollback(t *testing.T) {
	h := newHarness(t)
	old := h.installDir + previousSuffix
	writeFile(t, old, "game.exe", "OLD VERSION")
	h.library.games[0].Version = "2.0"
	j := SwapJournal{GameID: "local-1", Kind: JournalInplace, Original: &library.InstalledUpdate{ID: "local-1", InstallDir: h.installDir, Executable: filepath.Join(h.installDir, "game.exe"), Version: "1.0"}, InstallDir: h.installDir, Previous: old, Version: "2.0"}
	h.service.recoverInplaceJournal(j)
	b, _ := os.ReadFile(filepath.Join(h.installDir, "game.exe"))
	if string(b) != "OLD VERSION" || h.library.games[0].Version != "1.0" {
		t.Fatal("not reproduced")
	}
	t.Log("Regression: recovery restores old files but retains version 2.0")
}
