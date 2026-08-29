package library

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSyncSnapshotSkipsGamesWithoutCanonicalID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)

	if _, err := s.AddGame(tempGameExe(t), "No Canonical ID"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddCatalogGame("canon-1", "Has Canonical ID", ""); err != nil {
		t.Fatal(err)
	}

	snapshot := s.SyncSnapshot()
	if len(snapshot) != 1 {
		t.Fatalf("snapshot = %+v, want exactly one entry", snapshot)
	}
	if snapshot[0].CanonicalGameID != "canon-1" {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

func TestSyncSnapshotCopiesLastPlayed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)

	exe := tempGameExe(t)
	game, err := s.AddGame(exe, "Played")
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	stored := s.findLocked(game.ID)
	stored.CanonicalGameID = "canon-1"
	when := time.Now().Add(-time.Hour)
	stored.LastPlayed = &when
	internalPtr := stored.LastPlayed
	s.mu.Unlock()

	snapshot := s.SyncSnapshot()
	if len(snapshot) != 1 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	if snapshot[0].LastPlayed == nil {
		t.Fatal("LastPlayed not copied")
	}
	if snapshot[0].LastPlayed == internalPtr {
		t.Fatal("SyncSnapshot leaked a pointer to internal state")
	}
	if !snapshot[0].LastPlayed.Equal(when) {
		t.Fatalf("LastPlayed = %v, want %v", snapshot[0].LastPlayed, when)
	}

	*snapshot[0].LastPlayed = time.Now()
	s.mu.Lock()
	after := *s.findLocked(game.ID).LastPlayed
	s.mu.Unlock()
	if !after.Equal(when) {
		t.Fatal("mutating snapshot mutated internal state")
	}
}

func TestSyncSnapshotEmptyLibrary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)

	snapshot := s.SyncSnapshot()
	if len(snapshot) != 0 {
		t.Fatalf("snapshot = %+v, want none", snapshot)
	}
}

func TestApplySyncRejectsEmptyCanonicalID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	if _, err := s.AddCatalogGame("canon-1", "Some Game", ""); err != nil {
		t.Fatal(err)
	}
	before := s.GetGames()

	err := s.ApplySync([]SyncGame{{CanonicalGameID: "canon-1", PlaytimeSeconds: 5}, {CanonicalGameID: ""}})
	if !errors.Is(err, errEmptyCanonicalGameID) {
		t.Fatalf("err = %v, want %v", err, errEmptyCanonicalGameID)
	}
	after := s.GetGames()
	if len(after) != len(before) || after[0].PlaytimeSeconds != before[0].PlaytimeSeconds {
		t.Fatalf("state changed on rejected sync: before=%+v after=%+v", before, after)
	}
}

func TestApplySyncUnknownCanonicalIDSkipped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	if _, err := s.AddCatalogGame("canon-1", "Some Game", ""); err != nil {
		t.Fatal(err)
	}
	before := s.GetGames()

	if err := s.ApplySync([]SyncGame{{CanonicalGameID: "canon-unknown", PlaytimeSeconds: 999}}); err != nil {
		t.Fatalf("ApplySync: %v", err)
	}
	after := s.GetGames()
	if len(after) != 1 || after[0].PlaytimeSeconds != before[0].PlaytimeSeconds {
		t.Fatalf("state changed for unknown canonical id: before=%+v after=%+v", before, after)
	}
}

func TestApplySyncPlaytimeNeverDecreases(t *testing.T) {
	cases := []struct {
		name       string
		local      int64
		incoming   int64
		wantResult int64
	}{
		{"incoming lower kept local", 100, 40, 100},
		{"incoming higher wins", 40, 100, 100},
		{"equal stays equal", 50, 50, 50},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "library.json")
			s := mustServiceAt(t, path)
			game, err := s.AddCatalogGame("canon-1", "Some Game", "")
			if err != nil {
				t.Fatal(err)
			}
			s.mu.Lock()
			s.findLocked(game.ID).PlaytimeSeconds = tc.local
			s.mu.Unlock()

			if err := s.ApplySync([]SyncGame{{CanonicalGameID: "canon-1", PlaytimeSeconds: tc.incoming}}); err != nil {
				t.Fatalf("ApplySync: %v", err)
			}
			got := s.GetGames()[0].PlaytimeSeconds
			if got != tc.wantResult {
				t.Fatalf("PlaytimeSeconds = %d, want %d", got, tc.wantResult)
			}
		})
	}
}

func TestApplySyncLastPlayedMerge(t *testing.T) {
	older := time.Now().Add(-time.Hour)
	newer := time.Now()

	cases := []struct {
		name     string
		local    *time.Time
		incoming *time.Time
		want     *time.Time
	}{
		{"both nil stays nil", nil, nil, nil},
		{"incoming nil keeps local", &newer, nil, &newer},
		{"local nil takes incoming", nil, &newer, &newer},
		{"both set takes later", &older, &newer, &newer},
		{"both set local already later", &newer, &older, &newer},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "library.json")
			s := mustServiceAt(t, path)
			game, err := s.AddCatalogGame("canon-1", "Some Game", "")
			if err != nil {
				t.Fatal(err)
			}
			s.mu.Lock()
			s.findLocked(game.ID).LastPlayed = tc.local
			s.mu.Unlock()

			if err := s.ApplySync([]SyncGame{{CanonicalGameID: "canon-1", LastPlayed: tc.incoming}}); err != nil {
				t.Fatalf("ApplySync: %v", err)
			}
			got := s.GetGames()[0].LastPlayed
			switch {
			case tc.want == nil && got != nil:
				t.Fatalf("LastPlayed = %v, want nil", got)
			case tc.want != nil && got == nil:
				t.Fatalf("LastPlayed = nil, want %v", tc.want)
			case tc.want != nil && !got.Equal(*tc.want):
				t.Fatalf("LastPlayed = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestApplySyncOwnedMerge(t *testing.T) {
	cases := []struct {
		name     string
		local    bool
		incoming bool
		want     bool
	}{
		{"local false incoming true becomes true", false, true, true},
		{"local true incoming false stays true", true, false, true},
		{"both false stays false", false, false, false},
		{"both true stays true", true, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "library.json")
			s := mustServiceAt(t, path)
			game, err := s.AddCatalogGame("canon-1", "Some Game", "")
			if err != nil {
				t.Fatal(err)
			}
			s.mu.Lock()
			s.findLocked(game.ID).Owned = tc.local
			s.mu.Unlock()

			if err := s.ApplySync([]SyncGame{{CanonicalGameID: "canon-1", Owned: tc.incoming}}); err != nil {
				t.Fatalf("ApplySync: %v", err)
			}
			if got := s.GetGames()[0].Owned; got != tc.want {
				t.Fatalf("Owned = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestApplySyncNoChangeSkipsPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	when := time.Now().Add(-time.Hour)
	game, err := s.AddCatalogGame("canon-1", "Some Game", "")
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.findLocked(game.ID).PlaytimeSeconds = 500
	s.findLocked(game.ID).LastPlayed = &when
	s.findLocked(game.ID).Owned = true
	s.mu.Unlock()
	if err := s.persist(); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	mtimeBefore := info.ModTime()

	err = s.ApplySync([]SyncGame{{
		CanonicalGameID: "canon-1",
		PlaytimeSeconds: 100,
		LastPlayed:      &when,
		Owned:           true,
	}})
	if err != nil {
		t.Fatalf("ApplySync returned error though nothing changed: %v", err)
	}

	infoAfter, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !infoAfter.ModTime().Equal(mtimeBefore) {
		t.Fatal("library.json was rewritten though no field changed")
	}
}

func TestApplySyncPersistFailureRollsBackMemory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	first, err := s.AddCatalogGame("canon-1", "First", "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.AddCatalogGame("canon-2", "Second", "")
	if err != nil {
		t.Fatal(err)
	}
	before := s.GetGames()

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}

	err = s.ApplySync([]SyncGame{
		{CanonicalGameID: first.CanonicalGameID, PlaytimeSeconds: 300},
		{CanonicalGameID: second.CanonicalGameID, Owned: true},
	})
	if err == nil {
		t.Fatal("expected persist failure")
	}
	after := s.GetGames()
	if len(after) != len(before) {
		t.Fatalf("games = %+v, want %+v", after, before)
	}
	for i := range before {
		if after[i].PlaytimeSeconds != before[i].PlaytimeSeconds || after[i].Owned != before[i].Owned {
			t.Fatalf("memory not rolled back: before=%+v after=%+v", before, after)
		}
	}
}
