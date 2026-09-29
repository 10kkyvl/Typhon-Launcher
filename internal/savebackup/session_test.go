package savebackup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"typhon/internal/library"
	"typhon/internal/uierr"
)

func TestSessionStoppedTakesACopyAndReportsIt(t *testing.T) {
	h := newHarness(t)

	h.svc.SessionStopped(gameID)
	ev := h.next()

	if ev.Status != StatusCreated || ev.Kind != KindSession || ev.GameID != gameID || ev.Snapshot == nil || ev.Error != "" {
		t.Fatalf("event = %+v, want a created session copy", ev)
	}
	list := h.list()
	if len(list) != 1 || list[0].ID != ev.Snapshot.ID || list[0].Kind != KindSession {
		t.Fatalf("list = %+v, want the session copy", list)
	}
}

func TestSessionStoppedReportsEveryOutcome(t *testing.T) {
	cases := []struct {
		name     string
		prepare  func(t *testing.T, h *harness)
		status   Status
		wantCode string
		snapshot bool
	}{
		{"unchanged", func(t *testing.T, h *harness) {
			h.svc.SessionStopped(gameID)
			h.next()
		}, StatusSkipped, "savebackup.unchanged", true},
		{"folder unknown", func(t *testing.T, h *harness) {
			h.games.setResult(gameID, library.SavesResult{})
		}, StatusSkipped, "savebackup.saves_not_found", false},
		{"several folders", func(t *testing.T, h *harness) {
			h.games.setResult(gameID, library.SavesResult{Candidates: []string{"a", "b"}})
		}, StatusSkipped, "savebackup.saves_ambiguous", false},
		{"lookup fails", func(t *testing.T, h *harness) {
			h.games.mu.Lock()
			h.games.err = uierr.New("library.saves_path_unavailable", "нет доступа")
			h.games.mu.Unlock()
		}, StatusFailed, "library.saves_path_unavailable", false},
		{"folder is a file", func(t *testing.T, h *harness) {
			file := filepath.Join(t.TempDir(), "file")
			writeFile(t, file, "x")
			h.games.setResult(gameID, library.SavesResult{Path: file})
		}, StatusFailed, "savebackup.saves_not_a_directory", false},
		{"folder vanished", func(t *testing.T, h *harness) {
			h.games.setResult(gameID, library.SavesResult{Path: filepath.Join(t.TempDir(), "gone")})
		}, StatusFailed, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			c.prepare(t, h)

			h.svc.SessionStopped(gameID)
			ev := h.next()

			if ev.Status != c.status || ev.Code != c.wantCode || ev.Error == "" || ev.GameID != gameID || ev.Kind != KindSession {
				t.Fatalf("event = %+v, want %s with code %q and a message", ev, c.status, c.wantCode)
			}
			if (ev.Snapshot != nil) != c.snapshot {
				t.Fatalf("event snapshot = %+v, want present: %v", ev.Snapshot, c.snapshot)
			}
		})
	}
}

func TestSessionStoppedHonoursTheSwitch(t *testing.T) {
	h := newHarness(t)
	h.setAfter(false)

	h.svc.SessionStopped(gameID)
	h.svc.wg.Wait()

	if len(h.drain()) != 0 || len(h.list()) != 0 {
		t.Fatal("a copy was taken although the switch is off")
	}
}

func TestSessionStoppedDoesNotBlockTheCaller(t *testing.T) {
	h := newHarness(t)
	gate := make(chan struct{})
	h.games.mu.Lock()
	h.games.gate = gate
	h.games.mu.Unlock()

	returned := make(chan struct{})
	go func() {
		h.svc.SessionStopped(gameID)
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("SessionStopped blocked on the library lookup: the library calls it under its own lock")
	}

	close(gate)
	if ev := h.next(); ev.Status != StatusCreated {
		t.Fatalf("event = %+v, want the copy once the lookup is released", ev)
	}
}

func TestShutdownCancelsAndWaitsForSessionCopies(t *testing.T) {
	h := newHarness(t)
	gate := make(chan struct{})
	h.games.mu.Lock()
	h.games.gate = gate
	h.games.mu.Unlock()
	h.svc.SessionStopped(gameID)

	done := make(chan error, 1)
	go func() { done <- h.svc.ServiceShutdown() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown did not cancel a blocked session copy")
	}

	if len(h.drain()) != 0 {
		t.Fatal("an interrupted session copy still reported a result")
	}
	h.svc.SessionStopped(gameID)
	h.svc.wg.Wait()
	if len(h.drain()) != 0 {
		t.Fatal("a session copy started after shutdown")
	}
	if _, err := h.svc.List(context.Background(), gameID); !errors.Is(err, errNotStarted) {
		t.Fatalf("List after shutdown = %v, want %v", err, errNotStarted)
	}
}

func TestSessionStartedIsANoOp(t *testing.T) {
	h := newHarness(t)
	h.svc.SessionStarted(library.Game{ID: gameID})
	h.svc.wg.Wait()
	if len(h.drain()) != 0 || len(h.list()) != 0 {
		t.Fatal("SessionStarted did work")
	}
}

func TestCreatedEventCarriesTheRotationError(t *testing.T) {
	ev := createdEvent(Snapshot{ID: "x", GameID: gameID, Kind: KindSession}, uierr.New(codeRotationFailed, "locked"))
	if ev.Status != StatusCreated || ev.Code != codeRotationFailed || ev.Error == "" || ev.Snapshot == nil {
		t.Fatalf("event = %+v, want created with the rotation error attached", ev)
	}
	if ok := createdEvent(Snapshot{ID: "x"}, nil); ok.Code != "" || ok.Error != "" {
		t.Fatalf("event = %+v, want no error for a clean rotation", ok)
	}
}

func TestLegacySnapshotIsMigratedOnStartup(t *testing.T) {
	h := newHarness(t)
	legacyRoot := filepath.Join(h.dir, legacyDirName)
	mtime := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	legacy := filepath.Join(legacyRoot, gameID)
	writeFile(t, filepath.Join(legacy, "slot1.sav"), "old one")
	writeFile(t, filepath.Join(legacy, "profile", "slot2.sav"), "old two")
	if err := os.Chtimes(legacy, mtime, mtime); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(legacy+partialSuffix, "half"), "half")
	replacedOnly := filepath.Join(legacyRoot, "g2"+legacyReplacedSuffix)
	writeFile(t, filepath.Join(replacedOnly, "slot.sav"), "from replaced")
	if err := os.Chtimes(replacedOnly, mtime, mtime); err != nil {
		t.Fatal(err)
	}

	h.restart()

	list := h.list()
	if len(list) != 1 || list[0].Kind != KindUpdate || !list[0].CreatedAt.Equal(mtime) || list[0].Broken {
		t.Fatalf("list = %+v, want one update snapshot dated %s", list, mtime)
	}
	sameTree(t, tree(t, list[0].Path), map[string]string{"slot1.sav": "old one", "profile/slot2.sav": "old two"})
	other, err := h.svc.List(context.Background(), "g2")
	if err != nil || len(other) != 1 {
		t.Fatalf("g2 list = %+v, %v, want the snapshot from the .replaced leftover", other, err)
	}
	sameTree(t, tree(t, other[0].Path), map[string]string{"slot.sav": "from replaced"})
	for _, gone := range []string{legacy, legacy + partialSuffix, replacedOnly} {
		if exists(t, gone) {
			t.Fatalf("%s left after the migration", gone)
		}
	}
}

func TestLegacyMigrationFinishesAfterACrashWithoutDuplicating(t *testing.T) {
	h := newHarness(t)
	legacy := filepath.Join(h.dir, legacyDirName, gameID)
	writeFile(t, filepath.Join(legacy, "a.sav"), "same")
	h.restart()
	if got := len(h.list()); got != 1 {
		t.Fatalf("%d snapshots after the first migration, want 1", got)
	}

	writeFile(t, filepath.Join(legacy, "a.sav"), "same")
	h.restart()

	if got := len(h.list()); got != 1 {
		t.Fatalf("%d snapshots, want 1: the same content must not be migrated twice", got)
	}
	if exists(t, legacy) {
		t.Fatal("legacy directory left after the second startup")
	}
}

func TestLegacyMigrationKeepsTheOldCopyWhenItCannotBeMoved(t *testing.T) {
	h := newHarness(t)
	legacy := filepath.Join(h.dir, legacyDirName, gameID)
	writeFile(t, filepath.Join(legacy, "a.sav"), "precious")
	writeFile(t, h.gameDir(), "a file where the game directory should be")

	h.restart()

	if got := readFile(t, filepath.Join(legacy, "a.sav")); got != "precious" {
		t.Fatalf("legacy copy = %q, it must survive a failed migration", got)
	}
	if _, err := h.svc.Create(context.Background(), gameID); err == nil {
		t.Fatal("Create succeeded on a game whose migration is stuck")
	}
}
