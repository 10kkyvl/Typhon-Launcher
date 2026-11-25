package diskusage

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	"typhon/internal/library"
	"typhon/internal/savebackup"
	"typhon/internal/screenshots"
)

func TestGroupOfAGameThatLeftTheLibraryHasItsArchivedTitle(t *testing.T) {
	r := newRig(t)
	r.addGame(t, "here", 1)
	r.lib.archived = []library.Game{{ID: "gone", Title: "Gone Game", Archived: true}}
	r.shots.usage = []screenshots.Usage{
		{GameID: "here", Count: 1, SizeBytes: 1},
		{GameID: "gone", Count: 1, SizeBytes: 2},
		{GameID: "stranger", Count: 1, SizeBytes: 3},
	}
	r.snaps.usage = []savebackup.Usage{
		{GameID: "gone", Count: 3, SizeBytes: 30},
		{GameID: "stranger", Count: 1, SizeBytes: 5},
	}
	r.shots.shots["here"] = []screenshots.Shot{{Name: "a.png", Size: 1}}
	r.shots.shots["gone"] = []screenshots.Shot{{Name: "b.png", Size: 2}}
	r.shots.shots["stranger"] = []screenshots.Shot{{Name: "c.png", Size: 3}}
	r.snaps.snaps["gone"] = []savebackup.Snapshot{{ID: "s1", SizeBytes: 10}, {ID: "s2", SizeBytes: 10}, {ID: "s3", SizeBytes: 10}}
	r.snaps.snaps["stranger"] = []savebackup.Snapshot{{ID: "s4", SizeBytes: 5}}

	rep, err := r.svc.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]GroupUsage{
		"here":     {GameID: "here", Title: "Game here", InLibrary: true},
		"gone":     {GameID: "gone", Title: "Gone Game"},
		"stranger": {GameID: "stranger", TitleUnknown: true},
	}
	for _, groups := range [][]GroupUsage{rep.Screenshots, rep.Snapshots} {
		for _, g := range groups {
			w := want[g.GameID]
			if g.Title != w.Title || g.TitleUnknown != w.TitleUnknown || g.InLibrary != w.InLibrary {
				t.Errorf("group %s = %+v, want title %q unknown %v inLibrary %v", g.GameID, g, w.Title, w.TitleUnknown, w.InLibrary)
			}
			if g.Title == g.GameID {
				t.Errorf("group %s carries its raw id as a title", g.GameID)
			}
		}
	}
}

func snapshotsRig(t *testing.T) *rig {
	t.Helper()
	r := newRig(t)
	r.snaps.snaps["g1"] = []savebackup.Snapshot{{ID: "s1", SizeBytes: 100}, {ID: "s2", SizeBytes: 200}, {ID: "s3", SizeBytes: 300}}
	r.snaps.usage = []savebackup.Usage{{GameID: "g1", Count: 3, SizeBytes: 600}}
	r.shots.shots["g1"] = []screenshots.Shot{{Name: "a.png", Size: 1}, {Name: "b.png", Size: 2}, {Name: "c.png", Size: 3}}
	r.shots.usage = []screenshots.Usage{{GameID: "g1", Count: 3, SizeBytes: 6}}
	if _, err := r.svc.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestDeleteAllDeletesOnlyWhatTheScanCounted(t *testing.T) {
	r := snapshotsRig(t)
	r.snaps.mu.Lock()
	r.snaps.snaps["g1"] = append(r.snaps.snaps["g1"], savebackup.Snapshot{ID: "s4", SizeBytes: 400})
	r.snaps.mu.Unlock()
	r.shots.mu.Lock()
	r.shots.shots["g1"] = append(r.shots.shots["g1"], screenshots.Shot{Name: "d.png", Size: 4})
	r.shots.mu.Unlock()

	got, err := r.svc.DeleteSnapshots(context.Background(), "g1")
	if err != nil {
		t.Fatal(err)
	}
	if left := r.snaps.snaps["g1"]; len(left) != 1 || left[0].ID != "s4" {
		t.Fatalf("snapshots left = %+v, want the fourth one the scan never counted", left)
	}
	if want := (GroupDelete{Deleted: 3}); got != want {
		t.Fatalf("DeleteSnapshots = %+v, want %+v", got, want)
	}
	if groups := r.svc.Cached().Snapshots; len(groups) != 1 || groups[0].Count != 1 || groups[0].SizeBytes != 400 {
		t.Fatalf("cached group = %+v, want what is on disk now", groups)
	}

	shots, err := r.svc.DeleteScreenshots(context.Background(), "g1")
	if err != nil {
		t.Fatal(err)
	}
	if want := (GroupDelete{Deleted: 3}); shots != want {
		t.Fatalf("DeleteScreenshots = %+v, want %+v", shots, want)
	}
	if left := r.shots.shots["g1"]; len(left) != 1 || left[0].Name != "d.png" {
		t.Fatalf("screenshots left = %+v, want the one taken after the scan", left)
	}
}

func TestDeleteAllCountsAnEntryGoneSinceTheScanAsNoError(t *testing.T) {
	r := snapshotsRig(t)
	r.snaps.mu.Lock()
	r.snaps.snaps["g1"] = r.snaps.snaps["g1"][1:]
	r.snaps.mu.Unlock()
	r.shots.mu.Lock()
	r.shots.shots["g1"] = r.shots.shots["g1"][1:]
	r.shots.failDel["a.png"] = fs.ErrNotExist
	r.shots.mu.Unlock()

	got, err := r.svc.DeleteSnapshots(context.Background(), "g1")
	if err != nil {
		t.Fatalf("DeleteSnapshots = %v, want no error for a copy gone since the scan", err)
	}
	if want := (GroupDelete{Deleted: 2, Gone: 1}); got != want {
		t.Fatalf("DeleteSnapshots = %+v, want %+v", got, want)
	}
	shots, err := r.svc.DeleteScreenshots(context.Background(), "g1")
	if err != nil {
		t.Fatalf("DeleteScreenshots = %v, want no error for a shot gone since the scan", err)
	}
	if want := (GroupDelete{Deleted: 2, Gone: 1}); shots != want {
		t.Fatalf("DeleteScreenshots = %+v, want %+v", shots, want)
	}
}

func TestDeleteAllSaysHowManyWentAndHowManyDidNot(t *testing.T) {
	r := snapshotsRig(t)
	r.snaps.mu.Lock()
	r.snaps.failDel["s2"] = errors.New("locked")
	r.snaps.mu.Unlock()
	r.shots.mu.Lock()
	r.shots.failDel["b.png"] = errors.New("in use")
	r.shots.failDel["c.png"] = errors.New("in use")
	r.shots.mu.Unlock()

	got, err := r.svc.DeleteSnapshots(context.Background(), "g1")
	if err != nil {
		t.Fatalf("DeleteSnapshots = %v, want the counts, not an error", err)
	}
	if want := (GroupDelete{Deleted: 2, Failed: 1, Code: DeletePartial}); got != want {
		t.Fatalf("DeleteSnapshots = %+v, want %+v", got, want)
	}
	if groups := r.svc.Cached().Snapshots; len(groups) != 1 || groups[0].Count != 1 || groups[0].SizeBytes != 200 {
		t.Fatalf("cached group = %+v, want the copy that stayed", groups)
	}
	shots, err := r.svc.DeleteScreenshots(context.Background(), "g1")
	if err != nil {
		t.Fatalf("DeleteScreenshots = %v, want the counts, not an error", err)
	}
	if want := (GroupDelete{Deleted: 1, Failed: 2, Code: DeletePartial}); shots != want {
		t.Fatalf("DeleteScreenshots = %+v, want %+v", shots, want)
	}
}

func TestDeleteAllOfAGroupTheScanDidNotListIsRefused(t *testing.T) {
	r := snapshotsRig(t)
	r.snaps.mu.Lock()
	r.snaps.snaps["late"] = []savebackup.Snapshot{{ID: "x", SizeBytes: 1}}
	r.snaps.mu.Unlock()
	if _, err := r.svc.DeleteSnapshots(context.Background(), "late"); !errors.Is(err, errNotScanned) {
		t.Fatalf("DeleteSnapshots = %v, want errNotScanned", err)
	}
	if len(r.snaps.snaps["late"]) != 1 {
		t.Fatal("a group the scan never showed was deleted")
	}
}

func TestGroupTheScanCouldNotListIsShownAndNotDeletable(t *testing.T) {
	r := newRig(t)
	r.shots.usage = []screenshots.Usage{{GameID: "g1", Count: 2, SizeBytes: 20}}
	r.shots.shots["g1"] = []screenshots.Shot{{Name: "a.png", Size: 10}, {Name: "b.png", Size: 10}}
	r.shots.listErr = errors.New("folder locked")
	rep, err := r.svc.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Screenshots) != 1 || rep.Screenshots[0].Count != 2 || len(rep.Warnings) != 1 || rep.Warnings[0].Area != "screenshots" {
		t.Fatalf("report = %+v", rep)
	}
	r.shots.mu.Lock()
	r.shots.listErr = nil
	r.shots.mu.Unlock()
	if _, err := r.svc.DeleteScreenshots(context.Background(), "g1"); !errors.Is(err, errNotDeletable) {
		t.Fatalf("DeleteScreenshots = %v, want errNotDeletable for a group with no listed set", err)
	}
	if len(r.shots.shots["g1"]) != 2 {
		t.Fatal("deleted screenshots the scan never listed")
	}
}
