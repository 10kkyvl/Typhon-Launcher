package savebackup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"typhon/internal/library"
	"typhon/internal/settings"
	"typhon/internal/uierr"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestCreateAndList(t *testing.T) {
	h := newHarness(t)

	snap := h.create()

	if snap.Kind != KindManual || snap.GameID != gameID || snap.SourcePath != h.saves {
		t.Fatalf("snapshot = %+v, want a manual copy of %s", snap, h.saves)
	}
	if snap.Files != 2 || snap.SizeBytes != 6 || snap.Digest == "" || snap.ID == "" {
		t.Fatalf("snapshot = %+v, want 2 files, 6 bytes, a digest and an id", snap)
	}
	sameTree(t, tree(t, snap.Path), map[string]string{"slot1.sav": "one", "profile/slot2.sav": "two"})
	if !exists(t, filepath.Join(filepath.Dir(snap.Path), snapshotFile)) {
		t.Fatal("snapshot.json missing next to files")
	}
	entries, err := os.ReadDir(h.gameDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), partialSuffix) {
			t.Fatalf("partial directory %s left after a finished snapshot", e.Name())
		}
	}
	list := h.list()
	if len(list) != 1 || list[0].ID != snap.ID || list[0].Path != snap.Path || list[0].Digest != snap.Digest {
		t.Fatalf("list = %+v, want the created snapshot", list)
	}
	ev := h.next()
	if ev.Status != StatusCreated || ev.Snapshot == nil || ev.Snapshot.ID != snap.ID || ev.GameID != gameID {
		t.Fatalf("event = %+v, want created for %s", ev, snap.ID)
	}
}

func TestListIsNewestFirstAndEmptyForUnknownGame(t *testing.T) {
	h := newHarness(t)
	first := h.create()
	writeFile(t, filepath.Join(h.saves, "slot1.sav"), "changed")
	second := h.create()

	list := h.list()
	if len(list) != 2 || list[0].ID != second.ID || list[1].ID != first.ID {
		t.Fatalf("list = %+v, want newest first: %s then %s", list, second.ID, first.ID)
	}
	other, err := h.svc.List(context.Background(), "g2")
	if err != nil || other == nil || len(other) != 0 {
		t.Fatalf("list of a game without copies = %v, %v, want an empty non-nil list", other, err)
	}
}

func TestIDsDoNotCollideWithinOneSecond(t *testing.T) {
	h := newHarness(t)
	fixed := time.Date(2026, 9, 29, 15, 30, 0, 0, time.UTC)
	h.svc.now = func() time.Time { return fixed }
	seen := map[string]bool{}
	for i := 0; i < 4; i++ {
		snap := h.create()
		if seen[snap.ID] {
			t.Fatalf("id %s issued twice", snap.ID)
		}
		seen[snap.ID] = true
	}
	if got := len(h.list()); got != 4 {
		t.Fatalf("list has %d snapshots, want 4", got)
	}
}

func TestRotationKeepsTheNewestAutomaticCopies(t *testing.T) {
	cases := []struct {
		name  string
		limit int
		count int
		want  int
	}{
		{"limit one", 1, 3, 1},
		{"under limit", 5, 3, 3},
		{"at limit", 3, 3, 3},
		{"over limit", 2, 5, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			h.setLimit(c.limit)
			var ids []string
			for i := 0; i < c.count; i++ {
				writeFile(t, filepath.Join(h.saves, "slot1.sav"), strings.Repeat("x", i+1))
				ids = append(ids, snapOf(t, h, KindUpdate).ID)
			}
			list := h.list()
			if len(list) != c.want {
				t.Fatalf("kept %d snapshots, want %d", len(list), c.want)
			}
			for i, snap := range list {
				if want := ids[len(ids)-1-i]; snap.ID != want {
					t.Fatalf("kept %s at position %d, want the newest ones (%s)", snap.ID, i, want)
				}
			}
		})
	}
}

func TestRotationSparesProtectedSnapshotAndBrokenOnes(t *testing.T) {
	h := newHarness(t)
	h.setLimit(1)
	oldest := snapOf(t, h, KindUpdate)
	writeFile(t, filepath.Join(h.saves, "slot1.sav"), "v2")
	h.setLimit(5)
	middle := snapOf(t, h, KindUpdate)
	broken := filepath.Join(h.gameDir(), "20260101-000000-manual")
	writeFile(t, filepath.Join(broken, snapshotFile), "{")
	h.setLimit(1)

	err := h.svc.rotate(context.Background(), gameID, "unrelated", oldest.ID)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, s := range h.list() {
		ids[s.ID] = true
	}
	if !ids[oldest.ID] {
		t.Fatal("protected snapshot was rotated out")
	}
	if !ids[middle.ID] {
		t.Fatal("the newest snapshot was rotated out")
	}
	if !ids["20260101-000000-manual"] {
		t.Fatal("a broken snapshot was deleted by rotation; only the player may do that")
	}
}

func TestSessionAndPreRestoreCopiesAreDeduplicated(t *testing.T) {
	cases := []struct {
		name string
		kind Kind
		want int
	}{
		{"session", KindSession, 1},
		{"pre-restore", KindPreRestore, 1},
		{"manual", KindManual, 3},
		{"update", KindUpdate, 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			first, err := h.svc.SnapshotPath(context.Background(), gameID, h.saves, c.kind)
			if err != nil {
				t.Fatal(err)
			}
			second, err := h.svc.SnapshotPath(context.Background(), gameID, h.saves, c.kind)
			if err != nil {
				t.Fatal(err)
			}
			if c.want == 1 && second.ID != first.ID {
				t.Fatalf("second call returned %s, want the existing %s", second.ID, first.ID)
			}
			if _, err := h.svc.SnapshotPath(context.Background(), gameID, h.saves, c.kind); err != nil {
				t.Fatal(err)
			}
			if got := len(h.list()); got != c.want {
				t.Fatalf("%d snapshots after three identical %s copies, want %d", got, c.name, c.want)
			}
			writeFile(t, filepath.Join(h.saves, "slot1.sav"), "changed")
			if _, err := h.svc.SnapshotPath(context.Background(), gameID, h.saves, c.kind); err != nil {
				t.Fatal(err)
			}
			if got := len(h.list()); got != c.want+1 {
				t.Fatalf("%d snapshots after a change, want %d", got, c.want+1)
			}
		})
	}
}

func TestDedupComparesWithTheNewestSnapshotOnly(t *testing.T) {
	h := newHarness(t)
	if _, err := h.svc.SnapshotPath(context.Background(), gameID, h.saves, KindSession); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(h.saves, "slot1.sav"), "changed")
	if _, err := h.svc.SnapshotPath(context.Background(), gameID, h.saves, KindSession); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(h.saves, "slot1.sav"), "one")
	if _, err := h.svc.SnapshotPath(context.Background(), gameID, h.saves, KindSession); err != nil {
		t.Fatal(err)
	}
	if got := len(h.list()); got != 3 {
		t.Fatalf("%d snapshots, want 3: going back to old content is a change against the newest copy", got)
	}
}

func TestCreateDistinguishesMissingAndAmbiguousFolders(t *testing.T) {
	cases := []struct {
		name     string
		result   library.SavesResult
		locateEr error
		wantIs   error
		wantCode string
	}{
		{"not found", library.SavesResult{}, nil, errSavesNotFound, "savebackup.saves_not_found"},
		{"not found with unreadable", library.SavesResult{Unreadable: 2}, nil, errSavesNotFound, "savebackup.saves_not_found"},
		{"several", library.SavesResult{Candidates: []string{"a", "b"}}, nil, errSavesAmbiguous, "savebackup.saves_ambiguous"},
		{"locate fails", library.SavesResult{}, uierr.New("library.saves_path_unavailable", "нет доступа"), nil, "library.saves_path_unavailable"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			h.games.setResult(gameID, c.result)
			h.games.mu.Lock()
			h.games.err = c.locateEr
			h.games.mu.Unlock()

			_, err := h.svc.Create(context.Background(), gameID)

			if err == nil {
				t.Fatal("Create succeeded, want an error")
			}
			if c.wantIs != nil && !errors.Is(err, c.wantIs) {
				t.Fatalf("error = %v, want %v", err, c.wantIs)
			}
			if code := uierr.Code(err); code != c.wantCode {
				t.Fatalf("code = %q, want %q", code, c.wantCode)
			}
			if len(h.list()) != 0 {
				t.Fatal("a snapshot was created although the folder is unknown")
			}
			if len(h.drain()) != 0 {
				t.Fatal("an event was emitted for a failed manual copy")
			}
		})
	}
	if errors.Is(errSavesNotFound, errSavesAmbiguous) || errors.Is(errSavesAmbiguous, errSavesNotFound) {
		t.Fatal("not-found and ambiguous must be different errors")
	}
}

func TestSnapshotPathRejectsBadInput(t *testing.T) {
	h := newHarness(t)
	file := filepath.Join(t.TempDir(), "file.txt")
	writeFile(t, file, "x")
	cases := []struct {
		name   string
		game   string
		source string
		kind   Kind
		wantIs error
	}{
		{"empty source", gameID, "", KindUpdate, errNoSource},
		{"unknown kind", gameID, h.saves, Kind("other"), errInvalidKind},
		{"empty kind", gameID, h.saves, Kind(""), errInvalidKind},
		{"bad game id", "../x", h.saves, KindUpdate, errInvalidID},
		{"source is a file", gameID, file, KindUpdate, errSavesNotDir},
		{"source is missing", gameID, filepath.Join(t.TempDir(), "gone"), KindUpdate, os.ErrNotExist},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := h.svc.SnapshotPath(context.Background(), c.game, c.source, c.kind)
			if !errors.Is(err, c.wantIs) {
				t.Fatalf("error = %v, want %v", err, c.wantIs)
			}
		})
	}
	if len(h.list()) != 0 {
		t.Fatal("a rejected request left a snapshot behind")
	}
	entries, err := os.ReadDir(h.gameDir())
	if err == nil {
		for _, e := range entries {
			t.Fatalf("rejected requests left %s in the game directory", e.Name())
		}
	}
}

func TestBrokenSnapshotsAreListedNotDropped(t *testing.T) {
	cases := []struct {
		name   string
		tamper func(t *testing.T, dir string)
	}{
		{"invalid json", func(t *testing.T, dir string) { writeFile(t, filepath.Join(dir, snapshotFile), "{not json") }},
		{"empty file", func(t *testing.T, dir string) { writeFile(t, filepath.Join(dir, snapshotFile), "") }},
		{"missing json", func(t *testing.T, dir string) {
			if err := os.Remove(filepath.Join(dir, snapshotFile)); err != nil {
				t.Fatal(err)
			}
		}},
		{"id mismatch", func(t *testing.T, dir string) {
			writeFile(t, filepath.Join(dir, snapshotFile), `{"id":"other","gameId":"g1","kind":"manual"}`)
		}},
		{"foreign game", func(t *testing.T, dir string) {
			writeFile(t, filepath.Join(dir, snapshotFile), `{"id":"`+filepath.Base(dir)+`","gameId":"g2","kind":"manual"}`)
		}},
		{"unknown kind", func(t *testing.T, dir string) {
			writeFile(t, filepath.Join(dir, snapshotFile), `{"id":"`+filepath.Base(dir)+`","gameId":"g1","kind":"weird"}`)
		}},
		{"oversized json", func(t *testing.T, dir string) {
			writeFile(t, filepath.Join(dir, snapshotFile), strings.Repeat(" ", maxMetadataSize+1))
		}},
		{"missing files dir", func(t *testing.T, dir string) {
			if err := os.RemoveAll(filepath.Join(dir, filesDirName)); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			good := h.create()
			writeFile(t, filepath.Join(h.saves, "slot1.sav"), "changed")
			victim := h.create()
			c.tamper(t, filepath.Join(h.gameDir(), victim.ID))

			list, err := h.svc.List(context.Background(), gameID)
			if err != nil {
				t.Fatalf("list failed because of one damaged snapshot: %v", err)
			}
			if len(list) != 2 {
				t.Fatalf("list = %+v, want both snapshots", list)
			}
			byID := map[string]Snapshot{}
			for _, s := range list {
				byID[s.ID] = s
			}
			if bad := byID[victim.ID]; !bad.Broken || bad.Problem == "" || bad.GameID != gameID {
				t.Fatalf("damaged snapshot = %+v, want it flagged broken with a reason", bad)
			}
			if ok := byID[good.ID]; ok.Broken || ok.Digest != good.Digest {
				t.Fatalf("intact snapshot = %+v, want it untouched", ok)
			}

			err = h.svc.Restore(context.Background(), gameID, victim.ID)
			if !errors.Is(err, errSnapshotBroken) {
				t.Fatalf("restore of a broken snapshot = %v, want %v", err, errSnapshotBroken)
			}
			if got := readFile(t, filepath.Join(h.saves, "slot1.sav")); got != "changed" {
				t.Fatalf("saves = %q, a refused restore must not touch them", got)
			}

			h.setLimit(1)
			if _, err := h.svc.SnapshotPath(context.Background(), gameID, h.saves, KindUpdate); err != nil {
				t.Fatal(err)
			}
			if !exists(t, filepath.Join(h.gameDir(), victim.ID)) {
				t.Fatal("rotation deleted a broken snapshot")
			}

			if err := h.svc.Delete(gameID, victim.ID); err != nil {
				t.Fatalf("deleting a broken snapshot: %v", err)
			}
			if exists(t, filepath.Join(h.gameDir(), victim.ID)) {
				t.Fatal("broken snapshot still on disk after Delete")
			}
		})
	}
}

func TestDeleteRemovesOnlyTheNamedSnapshot(t *testing.T) {
	h := newHarness(t)
	first := h.create()
	writeFile(t, filepath.Join(h.saves, "slot1.sav"), "changed")
	second := h.create()
	h.drain()

	if err := h.svc.Delete(gameID, first.ID); err != nil {
		t.Fatal(err)
	}

	list := h.list()
	if len(list) != 1 || list[0].ID != second.ID {
		t.Fatalf("list = %+v, want only %s", list, second.ID)
	}
	ev := h.next()
	if ev.Status != StatusDeleted || ev.Snapshot == nil || ev.Snapshot.ID != first.ID {
		t.Fatalf("event = %+v, want deleted for %s", ev, first.ID)
	}
	if err := h.svc.Delete(gameID, first.ID); !errors.Is(err, errSnapshotNotFound) {
		t.Fatalf("second delete = %v, want %v", err, errSnapshotNotFound)
	}
}

func TestDeleteRejectsUnsafeIdentifiers(t *testing.T) {
	h := newHarness(t)
	snap := h.create()
	outside := filepath.Join(h.dir, rootDirName, "keep")
	writeFile(t, filepath.Join(outside, "file.txt"), "x")
	long := strings.Repeat("a", maxNameLen+1)

	cases := []struct{ name, game, id string }{
		{"parent", gameID, "../keep"},
		{"parent of game", "../keep", snap.ID},
		{"slash", gameID, "a/b"},
		{"backslash", gameID, `a\b`},
		{"dotdot", gameID, ".."},
		{"dot", gameID, "."},
		{"empty id", gameID, ""},
		{"empty game", "", snap.ID},
		{"partial suffix", gameID, snap.ID + partialSuffix},
		{"hidden", gameID, ".hidden"},
		{"drive", gameID, "C:"},
		{"colon stream", gameID, snap.ID + ":stream"},
		{"too long", gameID, long},
		{"space", gameID, "a b"},
		{"double dots inside", gameID, "a..b"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := h.svc.Delete(c.game, c.id); !errors.Is(err, errInvalidID) {
				t.Fatalf("Delete(%q, %q) = %v, want %v", c.game, c.id, err, errInvalidID)
			}
			if err := h.svc.Restore(context.Background(), c.game, c.id); !errors.Is(err, errInvalidID) {
				t.Fatalf("Restore(%q, %q) = %v, want %v", c.game, c.id, err, errInvalidID)
			}
		})
	}
	if !exists(t, filepath.Join(outside, "file.txt")) || len(h.list()) != 1 {
		t.Fatal("a rejected identifier deleted something")
	}
	for _, id := range []string{"../x", "a/b", "", "."} {
		if _, err := h.svc.List(context.Background(), id); !errors.Is(err, errInvalidID) {
			t.Fatalf("List(%q) = %v, want %v", id, err, errInvalidID)
		}
		if _, err := h.svc.Create(context.Background(), id); !errors.Is(err, errInvalidID) {
			t.Fatalf("Create(%q) = %v, want %v", id, err, errInvalidID)
		}
	}
}

func TestCanceledContextStopsEveryOperation(t *testing.T) {
	h := newHarness(t)
	snap := h.create()
	h.drain()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := h.svc.Create(ctx, gameID); !errors.Is(err, context.Canceled) {
		t.Fatalf("Create = %v, want context.Canceled", err)
	}
	if _, err := h.svc.List(ctx, gameID); !errors.Is(err, context.Canceled) {
		t.Fatalf("List = %v, want context.Canceled", err)
	}
	if err := h.svc.Restore(ctx, gameID, snap.ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("Restore = %v, want context.Canceled", err)
	}
	if _, err := h.svc.SnapshotPath(ctx, gameID, h.saves, KindUpdate); !errors.Is(err, context.Canceled) {
		t.Fatalf("SnapshotPath = %v, want context.Canceled", err)
	}
	if got := len(h.list()); got != 1 {
		t.Fatalf("%d snapshots after canceled calls, want 1", got)
	}
	if len(h.drain()) != 0 {
		t.Fatal("canceled calls emitted events")
	}
}

func TestCaptureCanceledMidwayLeavesNoPartial(t *testing.T) {
	h := newHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, created, err := h.svc.capture(ctx, gameID, h.saves, KindManual, captureOpts{})
	if !errors.Is(err, context.Canceled) || created {
		t.Fatalf("capture = created %v, err %v, want context.Canceled", created, err)
	}
	entries, err := os.ReadDir(h.gameDir())
	if err == nil && len(entries) != 0 {
		t.Fatalf("canceled capture left %d entries behind", len(entries))
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}

func TestCreateFailsCleanlyWhenTheBackupDirectoryIsBlocked(t *testing.T) {
	h := newHarness(t)
	if err := os.MkdirAll(filepath.Dir(h.gameDir()), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, h.gameDir(), "a file where the game directory should be")

	_, err := h.svc.Create(context.Background(), gameID)

	if err == nil {
		t.Fatal("Create succeeded although the game directory cannot be created")
	}
	sameTree(t, tree(t, h.saves), map[string]string{"slot1.sav": "one", "profile/slot2.sav": "two"})
}

func TestDigestFollowsContentNotTimestamps(t *testing.T) {
	h := newHarness(t)
	first := h.create()
	writeFile(t, filepath.Join(h.saves, "slot1.sav"), "one")
	second := h.create()
	if first.Digest != second.Digest {
		t.Fatalf("digests differ for identical content: %s vs %s", first.Digest, second.Digest)
	}
	writeFile(t, filepath.Join(h.saves, "slot1.sav"), "ONE")
	if third := h.create(); third.Digest == first.Digest {
		t.Fatal("digest did not change with the content")
	}
	if err := os.Rename(filepath.Join(h.saves, "slot1.sav"), filepath.Join(h.saves, "renamed.sav")); err != nil {
		t.Fatal(err)
	}
	if fourth := h.create(); fourth.Digest == first.Digest {
		t.Fatal("digest did not change with a renamed file")
	}
}

func TestEmptySavesFolderProducesAnEmptySnapshot(t *testing.T) {
	h := newHarness(t)
	empty := filepath.Join(t.TempDir(), "empty")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	h.games.setResult(gameID, library.SavesResult{Path: empty})

	snap := h.create()

	if snap.Files != 0 || snap.SizeBytes != 0 || snap.Digest == "" {
		t.Fatalf("snapshot = %+v, want an empty but valid copy", snap)
	}
	if !exists(t, snap.Path) {
		t.Fatal("files directory missing in an empty snapshot")
	}
}

func TestServiceRefusesWorkOutsideItsLifetime(t *testing.T) {
	dir := t.TempDir()
	games := &fakeGames{games: map[string]library.Game{}, results: map[string]library.SavesResult{}, running: map[string]bool{}}
	svc, err := newServiceAt(dir, settings.Defaults, games)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(context.Background(), gameID); !errors.Is(err, errNotStarted) {
		t.Fatalf("Create before startup = %v, want %v", err, errNotStarted)
	}
	if err := svc.Delete(gameID, "x"); !errors.Is(err, errNotStarted) {
		t.Fatalf("Delete before startup = %v, want %v", err, errNotStarted)
	}
	if err := svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := svc.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.List(context.Background(), gameID); !errors.Is(err, errNotStarted) {
		t.Fatalf("List after shutdown = %v, want %v", err, errNotStarted)
	}
}

func TestStartupFailsWhenTheBackupRootIsUnreadable(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, rootDirName), "a file where the root directory should be")
	games := &fakeGames{games: map[string]library.Game{}, results: map[string]library.SavesResult{}, running: map[string]bool{}}
	svc, err := newServiceAt(dir, settings.Defaults, games)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err == nil {
		t.Fatal("startup succeeded although the backup root cannot be read")
	}
}

func TestConstructorRejectsMissingDependencies(t *testing.T) {
	fake := &fakeGames{}
	cases := []struct {
		name   string
		dir    string
		config func() settings.Settings
		games  games
	}{
		{"empty dir", "", settings.Defaults, fake},
		{"relative dir", "config", settings.Defaults, fake},
		{"nil settings", t.TempDir(), nil, fake},
		{"nil library", t.TempDir(), settings.Defaults, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if svc, err := newServiceAt(c.dir, c.config, c.games); err == nil || svc != nil {
				t.Fatalf("newServiceAt = %v, %v, want an error", svc, err)
			}
		})
	}
	if svc, err := NewService(nil, nil); err == nil || svc != nil {
		t.Fatalf("NewService(nil, nil) = %v, %v, want an error", svc, err)
	}
}

func TestConcurrentCreateDeleteListOnOneGame(t *testing.T) {
	h := newHarness(t)
	h.setLimit(3)
	var wg sync.WaitGroup
	errs := make(chan error, 256)
	report := func(err error, allowed ...error) {
		if err == nil {
			return
		}
		for _, a := range allowed {
			if errors.Is(err, a) {
				return
			}
		}
		errs <- err
	}

	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 6; i++ {
				_, err := h.svc.SnapshotPath(context.Background(), gameID, h.saves, KindUpdate)
				report(err)
			}
		}()
	}
	for w := 0; w < 2; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 12; i++ {
				list, err := h.svc.List(context.Background(), gameID)
				report(err)
				for _, s := range list {
					if s.Broken {
						errs <- errors.New("List reported a snapshot broken while others were creating and deleting: " + s.Problem)
					}
				}
				if len(list) > 0 {
					report(h.svc.Delete(gameID, list[len(list)-1].ID), errSnapshotNotFound)
				}
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 6; i++ {
			h.svc.SessionStopped(gameID)
		}
	}()
	wg.Wait()
	h.svc.wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}

	final, err := h.svc.list(context.Background(), gameID)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range final {
		if s.Broken {
			t.Fatalf("snapshot %s is broken after the run: %s", s.ID, s.Problem)
		}
		sameTree(t, tree(t, s.Path), map[string]string{"slot1.sav": "one", "profile/slot2.sav": "two"})
	}
	if got := len(final); got > 3 {
		t.Fatalf("%d snapshots, want at most the limit of 3", got)
	}
	entries, err := os.ReadDir(h.gameDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), partialSuffix) {
			t.Fatalf("partial directory %s left after concurrent work", e.Name())
		}
	}
}

func TestSnapshotKeepsSymlinksInsteadOfDroppingThem(t *testing.T) {
	h := newHarness(t)
	link := filepath.Join(h.saves, "latest.sav")
	if err := os.Symlink("slot1.sav", link); err != nil {
		t.Skipf("this machine cannot create symlinks: %v", err)
	}

	snap := h.create()

	copied := filepath.Join(snap.Path, "latest.sav")
	info, err := os.Lstat(copied)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s is not a symlink in the snapshot: a link that is silently skipped is lost data", copied)
	}
	if target, err := os.Readlink(copied); err != nil || target != "slot1.sav" {
		t.Fatalf("link target = %q, %v, want slot1.sav", target, err)
	}
}
