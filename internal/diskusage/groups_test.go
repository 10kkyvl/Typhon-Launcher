package diskusage

import (
	"context"
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
