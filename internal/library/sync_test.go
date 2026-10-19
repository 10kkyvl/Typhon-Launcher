package library

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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
		{"remote ownership cannot grant local deletion", false, true, false},
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

func TestApplySyncMergesStatusByStamp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	game, err := s.AddCatalogGame("canon-1", "Some Game", "")
	if err != nil {
		t.Fatal(err)
	}

	t0 := time.Now().Add(-6 * time.Hour)
	t1 := time.Now().Add(-5 * time.Hour)
	t2 := time.Now().Add(-4 * time.Hour)
	t3 := time.Now().Add(-3 * time.Hour)
	t4 := time.Now().Add(-2 * time.Hour)
	t5 := time.Now().Add(-time.Hour)

	s.mu.Lock()
	s.findLocked(game.ID).Status = StatusPlaying
	s.findLocked(game.ID).StatusAt = &t1
	s.mu.Unlock()

	if err := s.ApplySync([]SyncGame{{
		CanonicalGameID: "canon-1",
		Status:          StatusCompleted,
		StatusAt:        &t2,
	}}); err != nil {
		t.Fatalf("ApplySync: %v", err)
	}
	got := s.GetGames()[0]
	if got.Status != StatusCompleted || got.StatusAt == nil || !got.StatusAt.Equal(t2) {
		t.Fatalf("after completed@t2: got status=%q statusAt=%v", got.Status, got.StatusAt)
	}

	if err := s.ApplySync([]SyncGame{{
		CanonicalGameID: "canon-1",
		Status:          StatusDropped,
		StatusAt:        &t0,
	}}); err != nil {
		t.Fatalf("ApplySync: %v", err)
	}
	got = s.GetGames()[0]
	if got.Status != StatusCompleted || got.StatusAt == nil || !got.StatusAt.Equal(t2) {
		t.Fatalf("older stamp should not override: got status=%q statusAt=%v", got.Status, got.StatusAt)
	}

	if err := s.ApplySync([]SyncGame{{
		CanonicalGameID: "canon-1",
		Status:          StatusDropped,
		StatusAt:        &t2,
	}}); err != nil {
		t.Fatalf("ApplySync: %v", err)
	}
	got = s.GetGames()[0]
	if got.Status != StatusCompleted {
		t.Fatalf("equal stamp should keep local: got status=%q", got.Status)
	}

	if err := s.ApplySync([]SyncGame{{
		CanonicalGameID: "canon-1",
		Favorite:        true,
		FavoriteAt:      &t3,
		Status:          StatusDropped,
		StatusAt:        &t0,
	}}); err != nil {
		t.Fatalf("ApplySync: %v", err)
	}
	got = s.GetGames()[0]
	if !got.Favorite || got.FavoriteAt == nil || !got.FavoriteAt.Equal(t3) {
		t.Fatalf("newer FavoriteAt must set the favorite: got favorite=%v favoriteAt=%v", got.Favorite, got.FavoriteAt)
	}
	if got.Status != StatusCompleted || got.StatusAt == nil || !got.StatusAt.Equal(t2) {
		t.Fatalf("older StatusAt must not move the status: got status=%q statusAt=%v", got.Status, got.StatusAt)
	}

	if err := s.ApplySync([]SyncGame{{
		CanonicalGameID: "canon-1",
		Favorite:        false,
		FavoriteAt:      &t0,
		Status:          StatusBacklog,
		StatusAt:        &t4,
	}}); err != nil {
		t.Fatalf("ApplySync: %v", err)
	}
	got = s.GetGames()[0]
	if got.Status != StatusBacklog || got.StatusAt == nil || !got.StatusAt.Equal(t4) {
		t.Fatalf("newer StatusAt must move the status: got status=%q statusAt=%v", got.Status, got.StatusAt)
	}
	if !got.Favorite || got.FavoriteAt == nil || !got.FavoriteAt.Equal(t3) {
		t.Fatalf("older FavoriteAt must not clear the favorite: got favorite=%v favoriteAt=%v", got.Favorite, got.FavoriteAt)
	}

	if err := s.persist(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	mtimeBefore := info.ModTime()

	if err := s.ApplySync([]SyncGame{{
		CanonicalGameID: "canon-1",
		Favorite:        false,
		Status:          StatusDropped,
	}}); err != nil {
		t.Fatalf("ApplySync: %v", err)
	}
	got = s.GetGames()[0]
	if !got.Favorite || got.Status != StatusBacklog || got.StatusAt == nil || !got.StatusAt.Equal(t4) {
		t.Fatalf("nil incoming stamps must never change anything: got favorite=%v status=%q statusAt=%v", got.Favorite, got.Status, got.StatusAt)
	}
	infoAfter, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !infoAfter.ModTime().Equal(mtimeBefore) {
		t.Fatal("library.json was rewritten though no field changed")
	}

	if err := s.ApplySync([]SyncGame{{
		CanonicalGameID: "canon-1",
		Status:          "",
		StatusAt:        &t5,
	}}); err != nil {
		t.Fatalf("ApplySync: %v", err)
	}
	got = s.GetGames()[0]
	if got.Status != "" || got.StatusAt == nil || !got.StatusAt.Equal(t5) {
		t.Fatalf("remote clear must clear the status: got status=%q statusAt=%v", got.Status, got.StatusAt)
	}
}

func TestApplySyncIgnoresInvalidRemoteStatus(t *testing.T) {
	cases := []struct {
		name   string
		status string
	}{
		{"unknown word", "won"},
		{"overlong string", strings.Repeat("a", 200)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "library.json")
			s := mustServiceAt(t, path)
			game, err := s.AddCatalogGame("canon-1", "Some Game", "")
			if err != nil {
				t.Fatal(err)
			}
			local := time.Now().Add(-2 * time.Hour)
			newer := time.Now().Add(-time.Hour)
			s.mu.Lock()
			s.findLocked(game.ID).Status = StatusPlaying
			s.findLocked(game.ID).StatusAt = &local
			s.mu.Unlock()

			if err := s.ApplySync([]SyncGame{{
				CanonicalGameID: "canon-1",
				Status:          tc.status,
				StatusAt:        &newer,
				Favorite:        true,
				FavoriteAt:      &newer,
			}}); err != nil {
				t.Fatalf("ApplySync: %v", err)
			}
			got := s.GetGames()[0]
			if got.Status != StatusPlaying || got.StatusAt == nil || !got.StatusAt.Equal(local) {
				t.Fatalf("invalid remote status must be ignored: got status=%q statusAt=%v", got.Status, got.StatusAt)
			}
			if !got.Favorite || got.FavoriteAt == nil || !got.FavoriteAt.Equal(newer) {
				t.Fatalf("favorite must still apply: got favorite=%v favoriteAt=%v", got.Favorite, got.FavoriteAt)
			}
		})
	}
}

func TestSyncSnapshotCopiesStamps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	game, err := s.AddCatalogGame("canon-1", "Some Game", "")
	if err != nil {
		t.Fatal(err)
	}
	statusAt := time.Now().Add(-2 * time.Hour)
	favoriteAt := time.Now().Add(-time.Hour)
	s.mu.Lock()
	stored := s.findLocked(game.ID)
	stored.Status = StatusCompleted
	stored.StatusAt = &statusAt
	stored.Favorite = true
	stored.FavoriteAt = &favoriteAt
	statusPtr, favoritePtr := stored.StatusAt, stored.FavoriteAt
	s.mu.Unlock()

	snapshot := s.SyncSnapshot()
	if len(snapshot) != 1 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	if snapshot[0].FavoriteAt == nil || !snapshot[0].FavoriteAt.Equal(favoriteAt) {
		t.Fatalf("FavoriteAt = %v, want %v", snapshot[0].FavoriteAt, favoriteAt)
	}
	if snapshot[0].StatusAt == favoritePtr || snapshot[0].StatusAt == statusPtr || snapshot[0].FavoriteAt == favoritePtr {
		t.Fatal("SyncSnapshot leaked a pointer to internal state")
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

func TestSetHiddenStampsTimeAndSyncSnapshotCarriesIt(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	game, err := s.AddCatalogGame("canon-1", "Game", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := s.SyncSnapshot()[0]; got.Hidden || got.HiddenAt != nil {
		t.Fatalf("fresh game snapshot = %+v, want not hidden and no stamp", got)
	}
	hidden, err := s.SetHidden(game.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if hidden.HiddenAt == nil {
		t.Fatal("SetHidden did not stamp HiddenAt")
	}
	got := s.SyncSnapshot()[0]
	if !got.Hidden || got.HiddenAt == nil || !got.HiddenAt.Equal(*hidden.HiddenAt) {
		t.Fatalf("snapshot = %+v, want hidden at %v", got, hidden.HiddenAt)
	}
	if got.HiddenAt == s.games[0].HiddenAt {
		t.Fatal("snapshot aliases the stored HiddenAt")
	}
	shown, err := s.SetHidden(game.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if shown.Hidden || shown.HiddenAt == nil || shown.HiddenAt.Before(*hidden.HiddenAt) {
		t.Fatalf("unhide = %+v, want visible with a stamp not older than the hide", shown)
	}
}

func TestSetHiddenRollsBackFlagAndStampWhenPersistFails(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	game, err := s.AddCatalogGame("canon-1", "Game", "")
	if err != nil {
		t.Fatal(err)
	}
	blockPersist(t, s)
	if _, err := s.SetHidden(game.ID, true); err == nil {
		t.Fatal("expected persist failure")
	}
	after, err := s.Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Hidden || after.HiddenAt != nil {
		t.Fatalf("after failed persist = hidden %v at %v, want rolled back", after.Hidden, after.HiddenAt)
	}
}

func TestApplySyncMergesHiddenByStamp(t *testing.T) {
	old := time.Now().Add(-3 * time.Hour)
	mid := time.Now().Add(-2 * time.Hour)
	fresh := time.Now().Add(-time.Hour)
	cases := []struct {
		name       string
		localHid   bool
		localAt    *time.Time
		remoteHid  bool
		remoteAt   *time.Time
		wantHidden bool
		wantAt     *time.Time
	}{
		{"remote newer hides", false, &old, true, &fresh, true, &fresh},
		{"remote newer shows", true, &old, false, &fresh, false, &fresh},
		{"local newer wins", true, &fresh, false, &mid, true, &fresh},
		{"remote without stamp ignored", true, &mid, false, nil, true, &mid},
		{"local without stamp takes remote", false, nil, true, &mid, true, &mid},
		{"both without stamp unchanged", true, nil, false, nil, true, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
			game, err := s.AddCatalogGame("canon-1", "Game", "")
			if err != nil {
				t.Fatal(err)
			}
			s.mu.Lock()
			stored := s.findLocked(game.ID)
			stored.Hidden = tc.localHid
			stored.HiddenAt = tc.localAt
			s.mu.Unlock()

			err = s.ApplySync([]SyncGame{{CanonicalGameID: "canon-1", Hidden: tc.remoteHid, HiddenAt: tc.remoteAt}})
			if err != nil {
				t.Fatal(err)
			}
			got, err := s.Find(game.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.Hidden != tc.wantHidden || !sameTime(got.HiddenAt, tc.wantAt) {
				t.Fatalf("hidden=%v at=%v, want hidden=%v at=%v", got.Hidden, got.HiddenAt, tc.wantHidden, tc.wantAt)
			}
		})
	}
}

func TestLoadStampsLegacyHiddenAndServerWithoutStampKeepsIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	raw := `[{"id":"a","title":"A","canonicalGameId":"canon-1","hidden":true},{"id":"b","title":"B","canonicalGameId":"canon-2"}]`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	s := mustServiceAt(t, path)
	epoch := time.Unix(0, 0).UTC()
	byID := map[string]Game{}
	for _, g := range s.GetGames() {
		byID[g.ID] = g
	}
	if byID["a"].HiddenAt == nil || !byID["a"].HiddenAt.Equal(epoch) {
		t.Fatalf("a = %+v, want hidden stamped at the epoch", byID["a"])
	}
	if byID["b"].HiddenAt != nil {
		t.Fatalf("b = %+v, want no hidden stamp for a visible game", byID["b"])
	}

	if err := s.ApplySync([]SyncGame{{CanonicalGameID: "canon-1", Hidden: false}}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Find("a")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Hidden {
		t.Fatal("a server record without a stamp must not unhide a legacy hidden game")
	}
}

func TestUpdatedPayloadDoesNotAliasGames(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	game, err := s.AddCatalogGame("canon-1", "Game", "")
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	payload := s.updatedPayload()
	s.mu.Unlock()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 50 {
			if _, err := s.SetHidden(game.ID, i%2 == 0); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	for reading := true; reading; {
		select {
		case <-done:
			reading = false
		default:
			if payload[0].Hidden {
				t.Error("payload changed after it was taken")
				reading = false
			}
		}
	}
	<-done
}

func TestApplySyncEmitsUpdateOnlyWhenSomethingChanged(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	game, err := s.AddCatalogGame("canon-1", "Game", "")
	if err != nil {
		t.Fatal(err)
	}
	var payloads [][]Game
	s.onUpdated = func(games []Game) { payloads = append(payloads, games) }

	at := time.Now().Add(-time.Hour)
	if err := s.ApplySync([]SyncGame{{CanonicalGameID: "canon-1", Hidden: true, HiddenAt: &at}}); err != nil {
		t.Fatal(err)
	}
	if len(payloads) != 1 || len(payloads[0]) != 1 || payloads[0][0].ID != game.ID || !payloads[0][0].Hidden {
		t.Fatalf("payloads after a hide from another device = %+v, want one update with the game hidden", payloads)
	}
	if err := s.ApplySync([]SyncGame{{CanonicalGameID: "canon-1", Hidden: true, HiddenAt: &at}}); err != nil {
		t.Fatal(err)
	}
	if len(payloads) != 1 {
		t.Fatalf("an ApplySync that changed nothing emitted again: %d updates", len(payloads))
	}
}

func TestApplySyncDoesNotEmitWhenPersistFails(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	if _, err := s.AddCatalogGame("canon-1", "Game", ""); err != nil {
		t.Fatal(err)
	}
	blockPersist(t, s)
	emitted := 0
	s.onUpdated = func([]Game) { emitted++ }
	at := time.Now()
	if err := s.ApplySync([]SyncGame{{CanonicalGameID: "canon-1", Hidden: true, HiddenAt: &at}}); err == nil {
		t.Fatal("expected persist failure")
	}
	if emitted != 0 {
		t.Fatal("update emitted for a state that was rolled back")
	}
}

func TestLegacyHiddenEpochLosesToAnyRealStampAndBeatsNone(t *testing.T) {
	real := time.Now().Add(-time.Hour)
	cases := []struct {
		name       string
		remote     *time.Time
		remoteHid  bool
		wantHidden bool
	}{
		{"real stamp showing the game wins over the legacy epoch", &real, false, false},
		{"no stamp on the other device keeps the legacy game hidden", nil, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "library.json")
			raw := `[{"id":"a","title":"A","canonicalGameId":"canon-1","hidden":true}]`
			if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
				t.Fatal(err)
			}
			s := mustServiceAt(t, path)
			if err := s.ApplySync([]SyncGame{{CanonicalGameID: "canon-1", Hidden: tc.remoteHid, HiddenAt: tc.remote}}); err != nil {
				t.Fatal(err)
			}
			got, err := s.Find("a")
			if err != nil {
				t.Fatal(err)
			}
			if got.Hidden != tc.wantHidden {
				t.Fatalf("hidden = %v, want %v", got.Hidden, tc.wantHidden)
			}
		})
	}
}
