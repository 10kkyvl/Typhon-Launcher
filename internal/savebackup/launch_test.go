package savebackup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"typhon/internal/hashdir"
	"typhon/internal/library"
	"typhon/internal/uierr"
)

func (h *harness) starting(ctx context.Context) error {
	return h.svc.SessionStarting(ctx, library.Game{ID: gameID})
}

func (h *harness) gateSaves() {
	h.games.mu.Lock()
	defer h.games.mu.Unlock()
	h.games.gate = make(chan struct{})
}

func TestSessionStartingTakesACopyBeforeTheLaunch(t *testing.T) {
	h := newHarness(t)

	if err := h.starting(context.Background()); err != nil {
		t.Fatal(err)
	}

	ev := h.next()
	if ev.Status != StatusCreated || ev.Kind != KindLaunch || ev.Snapshot == nil || ev.Error != "" {
		t.Fatalf("event = %+v, want a created launch copy", ev)
	}
	list := h.list()
	if len(list) != 1 || list[0].Kind != KindLaunch || list[0].ID != ev.Snapshot.ID {
		t.Fatalf("list = %+v, want the launch copy", list)
	}
	sameTree(t, tree(t, list[0].Path), map[string]string{"slot1.sav": "one", "profile/slot2.sav": "two"})
}

func TestSessionStartingDoesNotDuplicateUnchangedSaves(t *testing.T) {
	h := newHarness(t)
	if err := h.starting(context.Background()); err != nil {
		t.Fatal(err)
	}
	h.drain()

	if err := h.starting(context.Background()); err != nil {
		t.Fatalf("unchanged saves are not an error: %v", err)
	}

	if got := h.drain(); len(got) != 0 {
		t.Fatalf("events = %+v, want none for unchanged saves", got)
	}
	if n := len(h.list()); n != 1 {
		t.Fatalf("%d copies, want 1", n)
	}

	writeFile(t, filepath.Join(h.saves, "slot1.sav"), "changed")
	if err := h.starting(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := len(h.list()); n != 2 {
		t.Fatalf("%d copies after the saves changed, want 2", n)
	}
}

func TestSessionStartingSwitchedOffDoesNothing(t *testing.T) {
	h := newHarness(t)
	h.setLaunch(false)

	if err := h.starting(context.Background()); err != nil {
		t.Fatal(err)
	}

	if got := h.drain(); len(got) != 0 || len(h.list()) != 0 {
		t.Fatalf("events %+v, copies %d, want nothing when the switch is off", got, len(h.list()))
	}
}

func TestSessionStartingOutcomes(t *testing.T) {
	cases := []struct {
		name      string
		prepare   func(t *testing.T, h *harness)
		wantErr   bool
		status    Status
		wantCode  string
		wantEvent bool
	}{
		{"game without a saves folder", func(t *testing.T, h *harness) {
			h.games.setResult(gameID, library.SavesResult{})
		}, false, "", "", false},
		{"several folders", func(t *testing.T, h *harness) {
			h.games.setResult(gameID, library.SavesResult{Candidates: []string{"a", "b"}})
		}, true, StatusSkipped, "savebackup.saves_ambiguous", true},
		{"unreadable folders are not a clean miss", func(t *testing.T, h *harness) {
			h.games.setResult(gameID, library.SavesResult{Unreadable: 2})
		}, true, StatusSkipped, "savebackup.saves_not_found", true},
		{"lookup fails", func(t *testing.T, h *harness) {
			h.games.mu.Lock()
			h.games.err = uierr.New("library.saves_path_unavailable", "нет доступа")
			h.games.mu.Unlock()
		}, true, StatusFailed, "library.saves_path_unavailable", true},
		{"folder vanished", func(t *testing.T, h *harness) {
			h.games.setResult(gameID, library.SavesResult{Path: filepath.Join(t.TempDir(), "gone")})
		}, true, StatusFailed, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			c.prepare(t, h)

			err := h.starting(context.Background())

			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v, want error: %v", err, c.wantErr)
			}
			evs := h.drain()
			if !c.wantEvent {
				if len(evs) != 0 {
					t.Fatalf("events = %+v, want none", evs)
				}
				return
			}
			if len(evs) != 1 || evs[0].Status != c.status || evs[0].Code != c.wantCode || evs[0].Kind != KindLaunch || evs[0].Error == "" {
				t.Fatalf("events = %+v, want one %s with code %q", evs, c.status, c.wantCode)
			}
			if n := len(h.list()); n != 0 {
				t.Fatalf("%d copies after a failed attempt, want none", n)
			}
		})
	}
}

func TestSessionStartingTimeoutIsReportedAndLeavesNoCopy(t *testing.T) {
	h := newHarness(t)
	h.svc.launchTimeout = 20 * time.Millisecond
	h.gateSaves()

	err := h.starting(context.Background())

	if !errors.Is(err, errLaunchTimeout) {
		t.Fatalf("err = %v, want the launch timeout", err)
	}
	ev := h.next()
	if ev.Status != StatusFailed || ev.Code != "savebackup.launch_timeout" || ev.Kind != KindLaunch || ev.Error == "" {
		t.Fatalf("event = %+v, want a failed launch_timeout", ev)
	}
	if n := len(h.list()); n != 0 {
		t.Fatalf("%d copies, want none", n)
	}
}

func TestSessionStartingCancelledLaunchStopsTheCopySilently(t *testing.T) {
	h := newHarness(t)
	h.gateSaves()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- h.starting(ctx) }()
	cancel()

	err := <-done

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if got := h.drain(); len(got) != 0 {
		t.Fatalf("events = %+v, a cancelled launch is not a backup failure", got)
	}
	if n := len(h.list()); n != 0 {
		t.Fatalf("%d copies, want none", n)
	}
}

func TestSessionStartingAlreadyCancelledNeverTouchesTheSaves(t *testing.T) {
	h := newHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := h.starting(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if n := len(h.list()); n != 0 {
		t.Fatalf("%d copies, want none", n)
	}
}

func TestLaunchCopiesAreRotatedButManualOnesAreKept(t *testing.T) {
	h := newHarness(t)
	h.setLimit(2)
	manual := h.create()
	for i := range 4 {
		writeFile(t, filepath.Join(h.saves, "slot1.sav"), string(rune('a'+i)))
		if err := h.starting(context.Background()); err != nil {
			t.Fatal(err)
		}
	}

	list := h.list()
	if got := len(snapshotsOfKind(list, KindLaunch)); got != 2 {
		t.Fatalf("%d launch copies, want the limit of 2", got)
	}
	if got := snapshotsOfKind(list, KindManual); len(got) != 1 || got[0].ID != manual.ID {
		t.Fatalf("manual copies = %+v, want the original kept", got)
	}
}

func TestLargeSavesAreFlagged(t *testing.T) {
	h := newHarness(t)
	h.svc.largeAt = 4
	if err := h.starting(context.Background()); err != nil {
		t.Fatal(err)
	}
	ev := h.next()
	if ev.Snapshot == nil || !ev.Snapshot.Large {
		t.Fatalf("event snapshot = %+v, want Large", ev.Snapshot)
	}
	if list := h.list(); len(list) != 1 || !list[0].Large {
		t.Fatalf("list = %+v, want Large", list)
	}

	h.svc.largeAt = 1 << 20
	if list := h.list(); list[0].Large {
		t.Fatal("small saves flagged Large")
	}
}

func TestListReportsPlayTimeAfterEachSnapshot(t *testing.T) {
	h := newHarness(t)
	first := h.create()
	writeFile(t, filepath.Join(h.saves, "slot1.sav"), "later")
	second := h.create()
	h.svc.SetPlaytime(func(id string, since time.Time) (int64, error) {
		switch {
		case id != gameID:
			t.Errorf("asked for game %q", id)
		case since.Equal(first.CreatedAt):
			return 3600, nil
		case since.Equal(second.CreatedAt):
			return 0, nil
		}
		return 0, errors.New("unexpected snapshot")
	})

	byID := map[string]Snapshot{}
	for _, s := range h.list() {
		byID[s.ID] = s
	}

	if s := byID[first.ID]; !s.PlayedAfterKnown || s.PlayedAfterSeconds != 3600 {
		t.Fatalf("first = %+v, want 3600 s known", s)
	}
	if s := byID[second.ID]; !s.PlayedAfterKnown || s.PlayedAfterSeconds != 0 {
		t.Fatalf("second = %+v, want a known zero", s)
	}
}

func TestListMarksPlayTimeUnknownWhenThePlayLogFails(t *testing.T) {
	cases := []struct {
		name string
		fn   PlaytimeFunc
	}{
		{"play log errors", func(string, time.Time) (int64, error) { return 99, errors.New("degraded") }},
		{"no play log wired", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			h.create()
			h.svc.SetPlaytime(c.fn)

			list := h.list()

			if len(list) != 1 || list[0].PlayedAfterKnown || list[0].PlayedAfterSeconds != 0 {
				t.Fatalf("list = %+v, want unknown and no number", list)
			}
		})
	}
}

func TestSessionStartingDoesNotWaitBehindALongCopy(t *testing.T) {
	h := newHarness(t)
	h.svc.launchTimeout = 20 * time.Millisecond
	release := h.svc.lockGame(gameID)
	defer release()

	err := h.starting(context.Background())

	if !errors.Is(err, errLaunchTimeout) {
		t.Fatalf("err = %v, want the launch timeout while the game lock is held", err)
	}
	ev := h.next()
	if ev.Status != StatusFailed || ev.Code != "savebackup.launch_timeout" {
		t.Fatalf("event = %+v, want a failed launch_timeout", ev)
	}
}

func TestUnchangedSavesAreNotHashedAgain(t *testing.T) {
	h := newHarness(t)
	builds := 0
	h.svc.build = func(ctx context.Context, root string, p func(hashdir.Progress)) (hashdir.Manifest, error) {
		builds++
		return hashdir.Build(ctx, root, p)
	}
	file := filepath.Join(h.saves, "slot1.sav")
	later := time.Now().Add(time.Hour)
	cases := []struct {
		name       string
		change     func(t *testing.T)
		wantBuilds int
		wantCopies int
	}{
		{"first launch hashes and copies", func(*testing.T) {}, 1, 1},
		{"nothing changed reads no content", func(*testing.T) {}, 1, 1},
		{"new mtime only: hashed, same content, no new copy", func(t *testing.T) {
			if err := os.Chtimes(file, later, later); err != nil {
				t.Fatal(err)
			}
		}, 2, 1},
		{"record refreshed: unchanged again is free", func(*testing.T) {}, 2, 1},
		{"size changed: full path and new copy", func(t *testing.T) {
			writeFile(t, file, "much longer content")
		}, 3, 2},
		{"new file: full path and new copy", func(t *testing.T) {
			writeFile(t, filepath.Join(h.saves, "extra.sav"), "x")
		}, 4, 3},
	}
	for _, c := range cases {
		c.change(t)
		if err := h.starting(context.Background()); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if builds != c.wantBuilds {
			t.Fatalf("%s: hashed %d times, want %d", c.name, builds, c.wantBuilds)
		}
		if n := len(h.list()); n != c.wantCopies {
			t.Fatalf("%s: %d copies, want %d", c.name, n, c.wantCopies)
		}
	}
}

func TestSnapshotWithoutStatRecordFallsBackToHashing(t *testing.T) {
	h := newHarness(t)
	snap := h.create()
	if err := os.Remove(filepath.Join(filepath.Dir(snap.Path), statFile)); err != nil {
		t.Fatal(err)
	}
	builds := 0
	h.svc.build = func(ctx context.Context, root string, p func(hashdir.Progress)) (hashdir.Manifest, error) {
		builds++
		return hashdir.Build(ctx, root, p)
	}

	if err := h.starting(context.Background()); err != nil {
		t.Fatal(err)
	}

	if builds != 1 {
		t.Fatalf("hashed %d times, want 1 for a snapshot from before stat records", builds)
	}
	if n := len(h.list()); n != 1 {
		t.Fatalf("%d copies, same content must not be copied again", n)
	}
}
