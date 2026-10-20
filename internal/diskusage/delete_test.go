package diskusage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"typhon/internal/download"
	"typhon/internal/relocate"
	"typhon/internal/savebackup"
	"typhon/internal/screenshots"
)

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func TestDeleteLeftover(t *testing.T) {
	r := newRig(t)
	deletable := filepath.Join(r.games, "Done.partial")
	live := filepath.Join(r.games, "Live.partial")
	writeBytes(t, filepath.Join(deletable, "a.bin"), 40)
	writeBytes(t, filepath.Join(live, "b.bin"), 10)
	r.inst.partial[deletable] = true
	if _, err := r.svc.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}

	t.Run("path that was never scanned", func(t *testing.T) {
		stranger := filepath.Join(r.games, "Stranger.partial")
		writeBytes(t, filepath.Join(stranger, "x.bin"), 1)
		if err := r.svc.DeleteLeftover(context.Background(), stranger); !errors.Is(err, errNotScanned) {
			t.Fatalf("DeleteLeftover = %v, want errNotScanned", err)
		}
		if !exists(stranger) {
			t.Fatal("an unscanned folder was deleted")
		}
	})
	t.Run("not deletable at scan time", func(t *testing.T) {
		if err := r.svc.DeleteLeftover(context.Background(), live); !errors.Is(err, errNotDeletable) {
			t.Fatalf("DeleteLeftover = %v, want errNotDeletable", err)
		}
		if !exists(live) {
			t.Fatal("a refused leftover was deleted")
		}
	})
	t.Run("owner changed its mind after the scan", func(t *testing.T) {
		r.inst.mu.Lock()
		r.inst.partial[deletable] = false
		r.inst.mu.Unlock()
		if err := r.svc.DeleteLeftover(context.Background(), deletable); !errors.Is(err, errNotDeletable) {
			t.Fatalf("DeleteLeftover = %v, want errNotDeletable", err)
		}
		if !exists(deletable) {
			t.Fatal("a leftover a live job owns was deleted")
		}
		r.inst.mu.Lock()
		r.inst.partial[deletable] = true
		r.inst.mu.Unlock()
	})
	t.Run("cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := r.svc.DeleteLeftover(ctx, deletable); !errors.Is(err, context.Canceled) {
			t.Fatalf("DeleteLeftover = %v, want context.Canceled", err)
		}
		if !exists(deletable) {
			t.Fatal("deleted under a cancelled context")
		}
	})
	t.Run("removal failure is returned and the entry stays", func(t *testing.T) {
		old := removeAll
		removeAll = func(string) error { return errors.New("access denied") }
		t.Cleanup(func() { removeAll = old })
		if err := r.svc.DeleteLeftover(context.Background(), deletable); err == nil {
			t.Fatal("a failed removal must be an error")
		}
		if cached := r.svc.Cached(); len(cached.Leftovers) != 2 {
			t.Fatalf("report lost the leftover after a failed removal: %+v", cached.Leftovers)
		}
	})
	t.Run("deletes and updates the report", func(t *testing.T) {
		if err := r.svc.DeleteLeftover(context.Background(), deletable); err != nil {
			t.Fatal(err)
		}
		if exists(deletable) {
			t.Fatal("leftover still on disk")
		}
		cached := r.svc.Cached()
		if len(cached.Leftovers) != 1 || cached.Leftovers[0].Path != live || cached.Totals.LeftoversBytes != 10 {
			t.Fatalf("report = %+v", cached)
		}
		if err := r.svc.DeleteLeftover(context.Background(), deletable); !errors.Is(err, errNotScanned) {
			t.Fatalf("second delete = %v, want errNotScanned", err)
		}
	})
}

func TestDeleteLeftoverVanishedFolderIsDropped(t *testing.T) {
	r := newRig(t)
	path := filepath.Join(r.games, "Done.partial")
	writeBytes(t, filepath.Join(path, "a.bin"), 4)
	r.inst.partial[path] = true
	if _, err := r.svc.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.DeleteLeftover(context.Background(), path); err != nil {
		t.Fatalf("DeleteLeftover = %v", err)
	}
	if len(r.svc.Cached().Leftovers) != 0 {
		t.Fatal("the vanished leftover is still in the report")
	}
}

func TestDeleteLeftoverRefusesLinkThatReplacedTheFolder(t *testing.T) {
	r := newRig(t)
	path := filepath.Join(r.games, "Done.partial")
	writeBytes(t, filepath.Join(path, "a.bin"), 4)
	r.inst.partial[path] = true
	if _, err := r.svc.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	writeBytes(t, filepath.Join(target, "precious.bin"), 5)
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
	makeLink(t, path, target)
	if err := r.svc.DeleteLeftover(context.Background(), path); !errors.Is(err, errUnsafePath) {
		t.Fatalf("DeleteLeftover = %v, want errUnsafePath", err)
	}
	if !exists(filepath.Join(target, "precious.bin")) {
		t.Fatal("the link target was touched")
	}
}

func TestScanListsLinkedStagingAsNotDeletable(t *testing.T) {
	r := newRig(t)
	target := t.TempDir()
	writeBytes(t, filepath.Join(target, "precious.bin"), 5)
	link := filepath.Join(r.games, "Moved.staging")
	makeLink(t, link, target)
	r.rel.list = []relocate.Staging{{Path: link, Deletable: true}}
	rep, err := r.svc.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Leftovers) != 1 || rep.Leftovers[0].Deletable || rep.Leftovers[0].Reason == "" {
		t.Fatalf("leftovers = %+v", rep.Leftovers)
	}
	if err := r.svc.DeleteLeftover(context.Background(), link); !errors.Is(err, errNotDeletable) {
		t.Fatalf("DeleteLeftover = %v, want errNotDeletable", err)
	}
	if !exists(filepath.Join(target, "precious.bin")) {
		t.Fatal("the link target was touched")
	}
}

func TestDeleteDownloadData(t *testing.T) {
	setup := func(t *testing.T) *rig {
		r := newRig(t)
		root := filepath.Join(r.downloads, "pack")
		writeBytes(t, filepath.Join(root, "data.bin"), 60)
		r.dl.items = []download.Download{download.Download{ID: "pack", Name: "pack", Destination: r.downloads, Status: download.StatusCompleted}.WithRoot("pack")}
		if _, err := r.svc.Scan(context.Background()); err != nil {
			t.Fatal(err)
		}
		return r
	}

	t.Run("referenced download is refused before the manager is asked", func(t *testing.T) {
		r := setup(t)
		r.inst.blocked["pack"] = errDownloadBusy
		if err := r.svc.DeleteDownloadData(context.Background(), "pack"); !errors.Is(err, errDownloadBusy) {
			t.Fatalf("DeleteDownloadData = %v, want the install veto", err)
		}
		if len(r.dl.deleted) != 0 {
			t.Fatalf("manager asked to delete %v", r.dl.deleted)
		}
		if len(r.svc.Cached().Downloads) != 1 {
			t.Fatal("report lost a download that was kept")
		}
	})
	t.Run("unknown download", func(t *testing.T) {
		r := setup(t)
		if err := r.svc.DeleteDownloadData(context.Background(), "nope"); !errors.Is(err, download.ErrNotFound) {
			t.Fatalf("DeleteDownloadData = %v, want ErrNotFound", err)
		}
	})
	t.Run("deleted and dropped from the report", func(t *testing.T) {
		r := setup(t)
		if err := r.svc.DeleteDownloadData(context.Background(), "pack"); err != nil {
			t.Fatal(err)
		}
		if cached := r.svc.Cached(); len(cached.Downloads) != 0 || cached.Totals.DownloadsBytes != 0 {
			t.Fatalf("report = %+v", cached)
		}
	})
	t.Run("data left on disk still reaches the caller", func(t *testing.T) {
		r := setup(t)
		r.dl.delErr = errors.New("files locked")
		err := r.svc.DeleteDownloadData(context.Background(), "pack")
		if err == nil || err.Error() != "files locked" {
			t.Fatalf("DeleteDownloadData = %v, want the manager error", err)
		}
		if len(r.svc.Cached().Downloads) != 0 {
			t.Fatal("the record is gone, the report must not offer it again")
		}
	})
	t.Run("failed delete keeps the record in the report", func(t *testing.T) {
		r := setup(t)
		r.dl.delErr = errors.New("persist failed")
		keep := &keepingDownloads{fakeDownloads: r.dl}
		r.svc.deps.Downloads = keep
		if err := r.svc.DeleteDownloadData(context.Background(), "pack"); err == nil {
			t.Fatal("want the error")
		}
		if len(r.svc.Cached().Downloads) != 1 {
			t.Fatal("the record still exists, the report must keep it")
		}
	})
}

type keepingDownloads struct{ *fakeDownloads }

func (k *keepingDownloads) DeleteData(id string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.deleted = append(k.deleted, id)
	return k.delErr
}

func TestDeleteScreenshots(t *testing.T) {
	setup := func(t *testing.T) *rig {
		r := newRig(t)
		r.shots.shots["g1"] = []screenshots.Shot{{Name: "a.png", Size: 10}, {Name: "b.png", Size: 20}}
		r.shots.usage = []screenshots.Usage{{GameID: "g1", Count: 2, SizeBytes: 30}}
		if _, err := r.svc.Scan(context.Background()); err != nil {
			t.Fatal(err)
		}
		return r
	}
	t.Run("deletes all", func(t *testing.T) {
		r := setup(t)
		if err := r.svc.DeleteScreenshots(context.Background(), "g1"); err != nil {
			t.Fatal(err)
		}
		if len(r.shots.shots["g1"]) != 0 || len(r.svc.Cached().Screenshots) != 0 {
			t.Fatalf("left: %v / %v", r.shots.shots["g1"], r.svc.Cached().Screenshots)
		}
	})
	t.Run("one failure is reported and the rest is counted", func(t *testing.T) {
		r := setup(t)
		r.shots.failDel["b.png"] = errors.New("in use")
		if err := r.svc.DeleteScreenshots(context.Background(), "g1"); err == nil {
			t.Fatal("want the failure")
		}
		groups := r.svc.Cached().Screenshots
		if len(groups) != 1 || groups[0].Count != 1 || groups[0].SizeBytes != 20 {
			t.Fatalf("group = %+v", groups)
		}
	})
	t.Run("empty game id", func(t *testing.T) {
		r := setup(t)
		if err := r.svc.DeleteScreenshots(context.Background(), ""); !errors.Is(err, errEmptyGameID) {
			t.Fatalf("DeleteScreenshots = %v, want errEmptyGameID", err)
		}
	})
	t.Run("cancelled context", func(t *testing.T) {
		r := setup(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := r.svc.DeleteScreenshots(ctx, "g1"); !errors.Is(err, context.Canceled) {
			t.Fatalf("DeleteScreenshots = %v, want context.Canceled", err)
		}
		if len(r.shots.shots["g1"]) != 2 {
			t.Fatal("deleted under a cancelled context")
		}
	})
}

func TestDeleteSnapshots(t *testing.T) {
	setup := func(t *testing.T) *rig {
		r := newRig(t)
		r.snaps.snaps["g1"] = []savebackup.Snapshot{{ID: "s1", GameID: "g1", SizeBytes: 100}, {ID: "s2", GameID: "g1", SizeBytes: 200}}
		r.snaps.usage = []savebackup.Usage{{GameID: "g1", Count: 2, SizeBytes: 300}}
		if _, err := r.svc.Scan(context.Background()); err != nil {
			t.Fatal(err)
		}
		return r
	}
	t.Run("deletes all", func(t *testing.T) {
		r := setup(t)
		if err := r.svc.DeleteSnapshots(context.Background(), "g1"); err != nil {
			t.Fatal(err)
		}
		if len(r.snaps.snaps["g1"]) != 0 || len(r.svc.Cached().Snapshots) != 0 || r.svc.Cached().Totals.SnapshotsBytes != 0 {
			t.Fatalf("left: %v / %+v", r.snaps.snaps["g1"], r.svc.Cached())
		}
	})
	t.Run("empty game id", func(t *testing.T) {
		r := setup(t)
		if err := r.svc.DeleteSnapshots(context.Background(), ""); !errors.Is(err, errEmptyGameID) {
			t.Fatalf("DeleteSnapshots = %v, want errEmptyGameID", err)
		}
	})
	t.Run("cancelled context", func(t *testing.T) {
		r := setup(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := r.svc.DeleteSnapshots(ctx, "g1"); !errors.Is(err, context.Canceled) {
			t.Fatalf("DeleteSnapshots = %v, want context.Canceled", err)
		}
		if len(r.snaps.snaps["g1"]) != 2 {
			t.Fatal("deleted under a cancelled context")
		}
	})
}

func TestScanDeleteAndCachedRaceFree(t *testing.T) {
	r := newRig(t)
	paths := make([]string, 0, 6)
	for _, name := range []string{"a", "b", "c", "d", "e", "f"} {
		p := filepath.Join(r.games, name+".partial")
		writeBytes(t, filepath.Join(p, "x.bin"), 3)
		r.inst.partial[p] = true
		paths = append(paths, p)
	}
	if _, err := r.svc.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for _, p := range paths {
		wg.Add(3)
		go func() {
			defer wg.Done()
			if err := r.svc.DeleteLeftover(context.Background(), p); err != nil && !errors.Is(err, errNotScanned) {
				t.Errorf("DeleteLeftover: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := r.svc.Scan(context.Background()); err != nil {
				t.Errorf("Scan: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			r.svc.Cached()
		}()
	}
	wg.Wait()
	for _, p := range paths {
		if exists(p) {
			t.Fatalf("%s survived: a delete found it in the report and removed it", p)
		}
	}
}
