package diskusage

import (
	"context"
	"path/filepath"
	"testing"

	"typhon/internal/diskreason"
	"typhon/internal/download"
	"typhon/internal/library"
)

func leftoversAt(rep Report, path string) []Leftover {
	var out []Leftover
	for _, l := range rep.Leftovers {
		if l.Path == path {
			out = append(out, l)
		}
	}
	return out
}

func TestPartFileOfADeletedDownloadIsOfferedAsItsLeftover(t *testing.T) {
	r := newRig(t)
	part := filepath.Join(r.downloads, "Game.part")
	writeBytes(t, part, 30)
	r.dl.pending = []download.Cleanup{{Path: part}}
	r.dl.orphanOK[part] = true

	rep, err := r.svc.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := leftoversAt(rep, part)
	if len(got) != 1 || got[0].Kind != KindDownloadOrphan || !got[0].Deletable {
		t.Fatalf("leftovers at %s = %+v, want the deleted download's leftover, deletable through the manager", part, got)
	}
	if rep.Totals.LeftoversBytes != 30 {
		t.Fatalf("totals = %+v", rep.Totals)
	}
}

func TestPartFileOfAnotherLiveDownloadIsNotTheDeletedOnesLeftover(t *testing.T) {
	r := newRig(t)
	part := filepath.Join(r.downloads, "Game.part")
	writeBytes(t, part, 30)
	r.dl.pending = []download.Cleanup{{Path: part}}
	r.dl.owned[part] = true
	r.dl.items = []download.Download{download.Download{ID: "again", Name: "Game", Destination: r.downloads, Status: download.StatusDownloading}.WithRoot("Game")}

	rep, err := r.svc.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := leftoversAt(rep, part)
	if len(got) != 1 || got[0].Kind != KindDownloadPart || got[0].Deletable || got[0].ReasonCode != diskreason.OwnedByDownload {
		t.Fatalf("leftovers at %s = %+v, want it once, as the live download's part file", part, got)
	}
	if rep.Totals.LeftoversBytes != 30 {
		t.Fatalf("totals = %+v, want the part file counted once", rep.Totals)
	}
}

func TestOrphanFolderAtALiveDownloadsRootIsCountedOnce(t *testing.T) {
	r := newRig(t)
	root := filepath.Join(r.downloads, "pack")
	writeBytes(t, filepath.Join(root, "data.bin"), 50)
	r.dl.pending = []download.Cleanup{{Path: root}}
	r.dl.items = []download.Download{download.Download{ID: "again", Name: "pack", Destination: r.downloads, Status: download.StatusCompleted}.WithRoot("pack")}

	rep, err := r.svc.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := leftoversAt(rep, root); len(got) != 0 {
		t.Fatalf("the live download's folder is listed as a leftover of the deleted one: %+v", got)
	}
	if rep.Totals.DownloadsBytes != 50 || rep.Totals.LeftoversBytes != 0 || rep.Totals.TotalBytes != 50 {
		t.Fatalf("totals = %+v, want the folder counted once", rep.Totals)
	}
}

func TestOwnBytesTakesOffOnlyTheRowsDirectlyInside(t *testing.T) {
	base := t.TempDir()
	rows := []countedRow{
		{path: filepath.Join(base, "d"), rank: rankDownload, size: 100},
		{path: filepath.Join(base, "d", "x"), rank: rankLeftover, size: 40},
		{path: filepath.Join(base, "d", "x", "g"), rank: rankGame, size: 10},
		{path: filepath.Join(base, "e"), rank: rankGame, size: 7},
		{path: "", rank: rankGame, size: 5},
	}
	got := ownBytes(rows)
	want := []int64{60, 30, 10, 7, 5}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ownBytes = %v, want %v", got, want)
		}
	}
}

// The rule: a row's size is what its own folder holds; the totals count each
// byte once, in the innermost row that holds it. A folder listed by two rows
// at the same path counts as the game's, then the leftover's, then the
// download's. A row reached through a link is not in the walk of a folder
// around the link, so nothing is taken off that folder for it.
func TestGameInsideItsDownloadFolderIsCountedOnce(t *testing.T) {
	cases := []struct {
		name                           string
		gameAt                         func(r *rig, root string) string
		games, downloads, total, rowDL int64
	}{
		{"game in a folder inside the download", func(r *rig, root string) string {
			dir := filepath.Join(root, "Game")
			writeBytes(t, filepath.Join(dir, "game.exe"), 50)
			return dir
		}, 50, 10, 60, 60},
		{"game installed in the download's own folder", func(r *rig, root string) string {
			writeBytes(t, filepath.Join(root, "game.exe"), 50)
			return root
		}, 60, 0, 60, 60},
		{"game linked from inside the download", func(r *rig, root string) string {
			outside := t.TempDir()
			writeBytes(t, filepath.Join(outside, "game.exe"), 50)
			link := filepath.Join(root, "Game")
			makeLink(t, link, outside)
			return link
		}, 50, 10, 60, 10},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t)
			root := filepath.Join(r.downloads, "pack")
			writeBytes(t, filepath.Join(root, "data.bin"), 10)
			dir := c.gameAt(r, root)
			r.lib.games = []library.Game{{ID: "g", Title: "G", InstallDir: dir, SourceDownloadID: "pack"}}
			r.dl.items = []download.Download{download.Download{ID: "pack", Name: "pack", Destination: r.downloads, Status: download.StatusCompleted}.WithRoot("pack")}

			rep, err := r.svc.Scan(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(rep.Downloads) != 1 || rep.Downloads[0].SizeBytes != c.rowDL {
				t.Fatalf("download row = %+v, want its folder's own size %d", rep.Downloads, c.rowDL)
			}
			tot := rep.Totals
			if tot.GamesBytes != c.games || tot.DownloadsBytes != c.downloads || tot.TotalBytes != c.total {
				t.Fatalf("totals = %+v, want games %d downloads %d total %d", tot, c.games, c.downloads, c.total)
			}
		})
	}
}
