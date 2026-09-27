package accountsync

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestSyncDoesNotRehydrateALocallyRemovedGame(t *testing.T) {
	h := newHarness(t)
	h.catalog.link("game-1", "1")
	h.library.setLocal("game-1", 50)
	h.server.get = func(w http.ResponseWriter) {
		writeJSON(w, http.StatusOK, snapshotBody{Games: []wireGame{{IGDBID: 1, Owned: true, PlaytimeSeconds: 50}}})
	}
	h.server.put = echoPut(http.StatusOK)

	if err := h.service.Sync(context.Background()); err != nil {
		t.Fatalf("seed sync: %v", err)
	}

	if err := h.library.Remove("game-1"); err != nil {
		t.Fatalf("remove local game: %v", err)
	}

	if err := h.service.Sync(context.Background()); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	for _, canonical := range h.library.addCalls {
		if canonical == "game-1" {
			t.Fatal("hydrate must not re-add a game this device just removed locally")
		}
	}
	if got := h.library.gameOf("game-1"); got.CanonicalGameID != "" {
		t.Fatalf("expected game-1 to remain absent from the local library, got %+v", got)
	}
}

func TestSyncAppliesRemoteRemovalLocallyBeforePushing(t *testing.T) {
	h := newHarness(t)
	h.catalog.link("game-1", "1")
	h.library.setLocal("game-1", 50)

	removedAt := time.Now().Add(-time.Minute)
	h.server.get = func(w http.ResponseWriter) {
		writeJSON(w, http.StatusOK, snapshotBody{Games: []wireGame{
			{IGDBID: 1, Removed: true, RemovedAt: &removedAt},
		}})
	}
	var lastPut putRequest
	h.server.put = func(w http.ResponseWriter, req putRequest) {
		lastPut = req
		echoPut(http.StatusOK)(w, req)
	}

	if err := h.service.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if got := h.library.gameOf("game-1"); got.CanonicalGameID != "" {
		t.Fatalf("expected the remotely removed game to be gone locally, got %+v", got)
	}
	found := false
	for _, canonical := range h.library.removeCalls {
		if canonical == "game-1" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected library.Remove to be called for the remotely removed game")
	}
	if _, ok := pushedFor(lastPut, 1); ok {
		t.Fatal("a routine push must not resurrect a game the server just told us is removed")
	}
}

func TestSyncPushesOwnRemovalWithTimestamp(t *testing.T) {
	h := newHarness(t)
	h.catalog.link("game-1", "1")
	h.library.setLocal("game-1", 50)
	h.server.get = func(w http.ResponseWriter) {
		writeJSON(w, http.StatusOK, snapshotBody{Games: []wireGame{{IGDBID: 1, Owned: true, PlaytimeSeconds: 50}}})
	}
	h.server.put = echoPut(http.StatusOK)
	if err := h.service.Sync(context.Background()); err != nil {
		t.Fatalf("seed sync: %v", err)
	}

	if err := h.library.Remove("game-1"); err != nil {
		t.Fatalf("remove local game: %v", err)
	}

	before := time.Now().Add(-time.Second)
	var lastPut putRequest
	h.server.put = func(w http.ResponseWriter, req putRequest) {
		lastPut = req
		echoPut(http.StatusOK)(w, req)
	}
	if err := h.service.Sync(context.Background()); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	pushed, ok := pushedFor(lastPut, 1)
	if !ok || !pushed.Removed || pushed.RemovedAt == nil {
		t.Fatalf("expected removed:true with a timestamp for game 1, got %+v (ok=%v)", pushed, ok)
	}
	if pushed.RemovedAt.Before(before) {
		t.Fatalf("removedAt looks stale: %v", pushed.RemovedAt)
	}

	after := h.readState()
	if _, pending := after.Removed["1"]; pending {
		t.Fatal("expected the confirmed removal to be cleared from local pending state")
	}
}

func TestSyncOrdinaryUpdateOmitsRemovedAtField(t *testing.T) {
	h := newHarness(t)
	h.catalog.link("game-1", "1")
	at := time.Now()
	h.library.setLocalGame(Game{CanonicalGameID: "game-1", Status: "playing", StatusAt: &at})
	h.server.get = func(w http.ResponseWriter) {
		writeJSON(w, http.StatusOK, snapshotBody{Games: []wireGame{}})
	}
	h.server.put = echoPut(http.StatusOK)

	if err := h.service.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if len(h.server.putRaw) == 0 {
		t.Fatal("expected at least one PUT request")
	}
	raw := h.server.putRaw[len(h.server.putRaw)-1]
	if bytes.Contains(raw, []byte("removedAt")) {
		t.Fatalf("ordinary sync must not send a removedAt field at all, got body: %s", raw)
	}
}

func TestSyncPersistsOfflineRemovalUntilConfirmed(t *testing.T) {
	h := newHarness(t)
	h.catalog.link("game-1", "1")
	h.library.setLocal("game-1", 10)
	h.server.get = func(w http.ResponseWriter) {
		writeJSON(w, http.StatusOK, snapshotBody{Games: []wireGame{{IGDBID: 1, PlaytimeSeconds: 10}}})
	}
	h.server.put = echoPut(http.StatusOK)
	if err := h.service.Sync(context.Background()); err != nil {
		t.Fatalf("seed sync: %v", err)
	}

	h.server.Close()
	if err := h.library.Remove("game-1"); err != nil {
		t.Fatalf("remove local game: %v", err)
	}

	err := h.service.Sync(context.Background())
	var netErr *NetworkError
	if !errors.As(err, &netErr) {
		t.Fatalf("expected *NetworkError while offline, got %v", err)
	}

	st := h.readState()
	removedAt, ok := st.Removed["1"]
	if !ok || removedAt.IsZero() {
		t.Fatalf("expected the removal to be persisted to disk while offline, got %+v", st.Removed)
	}

	server2 := newMockSyncServer(t)
	var lastPut putRequest
	server2.put = func(w http.ResponseWriter, req putRequest) {
		lastPut = req
		echoPut(http.StatusOK)(w, req)
	}
	server2.get = func(w http.ResponseWriter) {
		writeJSON(w, http.StatusOK, snapshotBody{Games: []wireGame{{IGDBID: 1, PlaytimeSeconds: 10}}})
	}

	svc, err := NewService(h.dir, server2.URL, testToken, h.settings, h.library, h.catalog, h.metadata)
	if err != nil {
		t.Fatalf("NewService after restart: %v", err)
	}
	h.service = svc

	if err := h.service.Sync(context.Background()); err != nil {
		t.Fatalf("Sync after network returns: %v", err)
	}

	pushed, ok := pushedFor(lastPut, 1)
	if !ok || !pushed.Removed || pushed.RemovedAt == nil {
		t.Fatalf("expected the deferred removal to be pushed once the network returned, got %+v (ok=%v)", pushed, ok)
	}

	after := h.readState()
	if _, stillPending := after.Removed["1"]; stillPending {
		t.Fatal("expected the removal to be forgotten locally once the server confirmed it")
	}
}
