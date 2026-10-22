package diskusage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/diskreason"
	"typhon/internal/download"
	"typhon/internal/library"
	"typhon/internal/relocate"
	"typhon/internal/uierr"
)

func TestScanMeasuresALinkedGameThroughItsTargetAndDoesNotRefreshTheLibrary(t *testing.T) {
	r := newRig(t)
	target := t.TempDir()
	writeBytes(t, filepath.Join(target, "game.exe"), 500)
	link := filepath.Join(r.games, "linked")
	makeLink(t, link, target)
	r.lib.games = append(r.lib.games, library.Game{ID: "lk", Title: "Linked", InstallDir: link})

	rep, err := r.svc.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Games) != 1 || !rep.Games[0].Linked || rep.Games[0].SizeBytes != 500 || rep.Games[0].SizeUnknown {
		t.Fatalf("games = %+v, want the target measured and flagged as linked", rep.Games)
	}
	if _, refreshed := r.lib.updates["lk"]; refreshed {
		t.Fatal("a size taken through a link must not be written to the library")
	}
}

func TestScanLinkToNowhereIsAnUnknownSize(t *testing.T) {
	r := newRig(t)
	target := filepath.Join(t.TempDir(), "gone")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(r.games, "dangling")
	makeLink(t, link, target)
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	r.lib.games = append(r.lib.games, library.Game{ID: "dg", Title: "Dangling", InstallDir: link})

	rep, err := r.svc.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Games) != 1 || !rep.Games[0].SizeUnknown || rep.Games[0].SizeBytes != 0 {
		t.Fatalf("games = %+v, want an unknown size", rep.Games)
	}
	if _, refreshed := r.lib.updates["dg"]; refreshed {
		t.Fatal("an unknown size must not be written to the library")
	}
}

func TestDownloadOrphansAreListedAndRemovedByTheManager(t *testing.T) {
	r := newRig(t)
	left := filepath.Join(r.downloads, "Deleted")
	writeBytes(t, filepath.Join(left, "x.bin"), 25)
	busy := filepath.Join(r.downloads, "Reclaimed")
	writeBytes(t, filepath.Join(busy, "y.bin"), 5)
	r.dl.pending = []download.Cleanup{{Path: left}, {Path: busy}, {Path: filepath.Join(r.downloads, "AlreadyGone")}}
	r.dl.orphanOK[left] = true

	rep, err := r.svc.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]Leftover{}
	for _, l := range rep.Leftovers {
		by[l.Path] = l
	}
	if len(by) != 2 {
		t.Fatalf("leftovers = %+v, want only the two that exist", rep.Leftovers)
	}
	if l := by[left]; l.Kind != KindDownloadOrphan || !l.Deletable || l.SizeBytes != 25 {
		t.Fatalf("orphan = %+v", l)
	}
	if l := by[busy]; l.Deletable || l.ReasonCode != diskreason.OwnedByDownload {
		t.Fatalf("claimed orphan = %+v", l)
	}
	if err := r.svc.DeleteLeftover(context.Background(), busy); !errors.Is(err, errNotDeletable) {
		t.Fatalf("DeleteLeftover(claimed) = %v, want errNotDeletable", err)
	}
	if err := r.svc.DeleteLeftover(context.Background(), left); err != nil {
		t.Fatal(err)
	}
	if len(r.dl.cleaned) != 1 || r.dl.cleaned[0] != left {
		t.Fatalf("manager asked to clean %v, want only %s", r.dl.cleaned, left)
	}
	if !exists(left) {
		t.Fatal("the removal belongs to the manager, the service must not delete behind its back")
	}
}

func TestDeleteLeftoverReservesThePathWithItsOwner(t *testing.T) {
	setup := func(t *testing.T) (*rig, string) {
		r := newRig(t)
		path := filepath.Join(r.games, "Done.partial")
		writeBytes(t, filepath.Join(path, "a.bin"), 4)
		r.inst.partial[path] = true
		if _, err := r.svc.Scan(context.Background()); err != nil {
			t.Fatal(err)
		}
		return r, path
	}
	t.Run("held during the removal and released after", func(t *testing.T) {
		r, path := setup(t)
		old := removeAll
		held := false
		removeAll = func(p string) error {
			r.inst.mu.Lock()
			held = len(r.inst.reserved) == 1 && len(r.inst.released) == 0
			r.inst.mu.Unlock()
			return old(p)
		}
		t.Cleanup(func() { removeAll = old })
		if err := r.svc.DeleteLeftover(context.Background(), path); err != nil {
			t.Fatal(err)
		}
		if !held || len(r.inst.released) != 1 {
			t.Fatalf("held during removal = %v, released = %v", held, r.inst.released)
		}
	})
	t.Run("a refused reservation stops the removal", func(t *testing.T) {
		r, path := setup(t)
		r.inst.reserveErr = errors.New("a job just started on it")
		if err := r.svc.DeleteLeftover(context.Background(), path); !errors.Is(err, errNotDeletable) {
			t.Fatalf("DeleteLeftover = %v, want errNotDeletable", err)
		}
		if !exists(path) {
			t.Fatal("deleted although the owner refused the reservation")
		}
	})
	t.Run("released also when the removal fails", func(t *testing.T) {
		r, path := setup(t)
		old := removeAll
		removeAll = func(string) error { return errors.New("locked") }
		t.Cleanup(func() { removeAll = old })
		if err := r.svc.DeleteLeftover(context.Background(), path); err == nil {
			t.Fatal("want the error")
		}
		if len(r.inst.released) != 1 {
			t.Fatalf("released = %v", r.inst.released)
		}
	})
}

func TestDeleteLeftoverAsksAgainRightBeforeRemovingWhenTheOwnerCannotReserve(t *testing.T) {
	r := newRig(t)
	part := filepath.Join(r.downloads, "Old.part")
	writeBytes(t, part, 9)
	r.dl.partOK[part] = true
	if _, err := r.svc.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	r.dl.mu.Lock()
	r.dl.partOK[part] = false
	r.dl.owned[part] = true
	r.dl.mu.Unlock()
	if err := r.svc.DeleteLeftover(context.Background(), part); !errors.Is(err, errNotDeletable) {
		t.Fatalf("DeleteLeftover = %v, want errNotDeletable", err)
	}
	if !exists(part) {
		t.Fatal("a part file a download claimed after the scan was deleted")
	}
}

func TestScanStagingReservedThroughTheRelocation(t *testing.T) {
	r := newRig(t)
	staging := filepath.Join(r.games, "Moved.staging")
	writeBytes(t, filepath.Join(staging, "c.bin"), 9)
	r.rel.list = []relocate.Staging{{Path: staging, Deletable: true}}
	if _, err := r.svc.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.DeleteLeftover(context.Background(), staging); err != nil {
		t.Fatal(err)
	}
	if len(r.rel.reserved) != 1 || r.rel.reserved[0] != staging {
		t.Fatalf("reserved = %v", r.rel.reserved)
	}
}

func TestScanStagingWithAnUnexpectedNameIsNotDeletable(t *testing.T) {
	r := newRig(t)
	odd := filepath.Join(r.games, "MyGame")
	writeBytes(t, filepath.Join(odd, "game.exe"), 9)
	r.rel.list = []relocate.Staging{{Path: odd, Deletable: true}}
	rep, err := r.svc.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Leftovers) != 1 || rep.Leftovers[0].Deletable || rep.Leftovers[0].ReasonCode != diskreason.RelocateJournal {
		t.Fatalf("leftovers = %+v, want a journal mismatch refusal", rep.Leftovers)
	}
	if err := r.svc.DeleteLeftover(context.Background(), odd); !errors.Is(err, errNotDeletable) {
		t.Fatalf("DeleteLeftover = %v, want errNotDeletable", err)
	}
	if !exists(odd) {
		t.Fatal("a folder with a game-like name was deleted")
	}
}

func TestScanDownloadReasonCodes(t *testing.T) {
	r := newRig(t)
	mk := func(id string, status download.Status) download.Download {
		writeBytes(t, filepath.Join(r.downloads, id, "d.bin"), 5)
		return download.Download{ID: id, Name: id, Destination: r.downloads, Status: status}.WithRoot(id)
	}
	seed := mk("seed", download.StatusCompleted)
	seed.Seeding = true
	r.dl.items = []download.Download{
		mk("run", download.StatusDownloading), seed, mk("job", download.StatusCompleted),
		mk("game", download.StatusCompleted), mk("odd", download.StatusCompleted), mk("ok", download.StatusCompleted),
	}
	r.inst.blocked["job"] = uierr.New("install.download_in_use", "x")
	r.inst.blocked["game"] = uierr.New("install.download_is_install", "x")
	r.inst.blocked["odd"] = errors.New("something else")

	rep, err := r.svc.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"run": diskreason.Incomplete, "seed": diskreason.Seeding, "job": diskreason.OwnedByInstall,
		"game": diskreason.HoldsGame, "odd": diskreason.Unknown, "ok": "",
	}
	if len(rep.Downloads) != len(want) {
		t.Fatalf("downloads = %+v", rep.Downloads)
	}
	for _, d := range rep.Downloads {
		if d.ReasonCode != want[d.ID] {
			t.Fatalf("%s reasonCode = %q, want %q", d.ID, d.ReasonCode, want[d.ID])
		}
		if d.Referenced && d.Reason == "" {
			t.Fatalf("%s is referenced without a reason", d.ID)
		}
		if d.ID == "seed" && d.Referenced {
			t.Fatalf("seed = %+v, want deletable: removal stops the seeding itself", d)
		}
	}
}

func TestScanFlagsALinkInsideTheTreeAndDoesNotFollowIt(t *testing.T) {
	r := newRig(t)
	g := r.addGame(t, "inner", 100)
	outside := t.TempDir()
	writeBytes(t, filepath.Join(outside, "huge.bin"), 9000)
	makeLink(t, filepath.Join(g.InstallDir, "to-outside"), outside)
	makeLink(t, filepath.Join(g.InstallDir, "loop"), g.InstallDir)

	rep, err := r.svc.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Games) != 1 || !rep.Games[0].Linked || rep.Games[0].SizeBytes != 100 {
		t.Fatalf("games = %+v, want only the regular files counted and the game flagged as linked", rep.Games)
	}
	if _, refreshed := r.lib.updates["inner"]; refreshed {
		t.Fatal("a size that skipped a link must not be written to the library")
	}
}

func TestScanSizesWithALinkAreAlwaysLowerBounds(t *testing.T) {
	r := newRig(t)
	outside := t.TempDir()
	partial := filepath.Join(r.games, "Done.partial")
	writeBytes(t, filepath.Join(partial, "a.bin"), 40)
	makeLink(t, filepath.Join(partial, "link"), outside)
	r.inst.partial[partial] = true
	root := filepath.Join(r.downloads, "pack")
	writeBytes(t, filepath.Join(root, "d.bin"), 7)
	makeLink(t, filepath.Join(root, "link"), outside)
	r.dl.items = []download.Download{download.Download{ID: "pack", Name: "pack", Destination: r.downloads, Status: download.StatusCompleted}.WithRoot("pack")}

	rep, err := r.svc.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Leftovers) != 1 || rep.Leftovers[0].SizeBytes != 40 || !rep.Leftovers[0].SizeUnknown {
		t.Fatalf("leftovers = %+v", rep.Leftovers)
	}
	if len(rep.Downloads) != 1 || rep.Downloads[0].SizeBytes != 7 || !rep.Downloads[0].SizeUnknown {
		t.Fatalf("downloads = %+v", rep.Downloads)
	}
}

func TestOrphanIsNeverDeletedOnceSomethingElseUsesTheFolder(t *testing.T) {
	cases := []struct {
		name string
		use  func(r *rig, path string)
		code string
	}{
		{"a game was added to the library there", func(r *rig, path string) {
			r.lib.mu.Lock()
			r.lib.games = append(r.lib.games, library.Game{ID: "late", Title: "Late", InstallDir: filepath.Join(path, "Game")})
			r.lib.mu.Unlock()
		}, diskreason.HoldsGame},
		{"a game was added around the folder", func(r *rig, path string) {
			r.lib.mu.Lock()
			r.lib.games = append(r.lib.games, library.Game{ID: "late", Title: "Late", InstallDir: filepath.Dir(path)})
			r.lib.mu.Unlock()
		}, diskreason.HoldsGame},
		{"an install started there", func(r *rig, path string) {
			r.inst.mu.Lock()
			r.inst.inUse[path] = true
			r.inst.mu.Unlock()
		}, diskreason.OwnedByInstall},
		{"an update started there", func(r *rig, path string) {
			r.upd.mu.Lock()
			r.upd.paths[path] = true
			r.upd.mu.Unlock()
		}, diskreason.UpdateOwned},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t)
			left := filepath.Join(r.downloads, "Deleted")
			writeBytes(t, filepath.Join(left, "x.bin"), 25)
			r.dl.pending = []download.Cleanup{{Path: left}}
			r.dl.orphanOK[left] = true

			rep, err := r.svc.Scan(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(rep.Leftovers) != 1 || !rep.Leftovers[0].Deletable {
				t.Fatalf("before: leftovers = %+v, want the orphan deletable", rep.Leftovers)
			}
			c.use(r, left)
			if err := r.svc.DeleteLeftover(context.Background(), left); !errors.Is(err, errNotDeletable) {
				t.Fatalf("DeleteLeftover = %v, want errNotDeletable", err)
			}
			if len(r.dl.cleaned) != 0 || !exists(left) {
				t.Fatalf("the folder was removed (%v) although something else uses it", r.dl.cleaned)
			}
			rep, err = r.svc.Scan(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(rep.Leftovers) != 1 || rep.Leftovers[0].Deletable || rep.Leftovers[0].ReasonCode != c.code {
				t.Fatalf("after: leftovers = %+v, want a refusal with %s", rep.Leftovers, c.code)
			}
		})
	}
}

func TestOrphanClaimIsHeldAcrossTheRemoval(t *testing.T) {
	r := newRig(t)
	left := filepath.Join(r.downloads, "Deleted")
	writeBytes(t, filepath.Join(left, "x.bin"), 25)
	r.dl.pending = []download.Cleanup{{Path: left}}
	r.dl.orphanOK[left] = true
	if _, err := r.svc.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.DeleteLeftover(context.Background(), left); err != nil {
		t.Fatal(err)
	}
	if len(r.dl.claimed) != 1 || r.dl.claimed[0] != left || len(r.dl.cleaned) != 1 {
		t.Fatalf("claimed = %v, cleaned = %v", r.dl.claimed, r.dl.cleaned)
	}
}
