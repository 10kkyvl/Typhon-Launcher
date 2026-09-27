package accountsync

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestAuditFix001CloudOwnershipIsNotAppliedOrEchoed(t *testing.T) {
	h := newHarness(t)
	h.catalog.link("game-1", "1")
	h.library.setLocalGame(Game{CanonicalGameID: "game-1", Owned: false})
	h.server.get = func(w http.ResponseWriter) {
		writeJSON(w, http.StatusOK, snapshotBody{Games: []wireGame{{IGDBID: 1, Owned: true}}})
	}
	var pushed putRequest
	h.server.put = func(w http.ResponseWriter, req putRequest) { pushed = req; echoPut(http.StatusOK)(w, req) }
	if err := h.service.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if h.library.gameOf("game-1").Owned {
		t.Fatal("remote ownership applied locally")
	}
	g, ok := pushedFor(pushed, 1)
	if !ok || g.Owned {
		t.Fatalf("echoed remote ownership: %+v", g)
	}
	results := map[string]gameCompute{"1": {combined: Game{Owned: false}}}
	if !upToDate(syncState{Games: map[string]gameState{"1": {}}}, results, map[string]wireGame{"1": {Owned: true}}) {
		t.Fatal("remote ownership alone causes endless uploads")
	}
}

func TestAuditFix002FailedRemovalRetriesWithoutRestoringCloudGame(t *testing.T) {
	h := newHarness(t)
	h.catalog.link("game-1", "1")
	h.library.setLocal("game-1", 0)
	h.library.removeErr["game-1"] = errors.New("game is starting")
	at := time.Now().UTC().Add(-time.Hour)
	h.server.get = func(w http.ResponseWriter) {
		writeJSON(w, http.StatusOK, snapshotBody{Games: []wireGame{{IGDBID: 1, Removed: true, RemovedAt: &at}}})
	}
	h.server.put = func(w http.ResponseWriter, req putRequest) {
		if g, ok := pushedFor(req, 1); ok && !g.Removed && g.RemovedAt != nil {
			t.Error("failed local removal became an explicit cloud restore")
		}
		echoPut(http.StatusOK)(w, req)
	}
	for i := 0; i < 2; i++ {
		if err := h.service.Sync(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	h.library.mu.Lock()
	delete(h.library.removeErr, "game-1")
	h.library.mu.Unlock()
	if err := h.service.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	h.library.mu.Lock()
	defer h.library.mu.Unlock()
	if _, present := h.library.games["game-1"]; present || len(h.library.removeCalls) != 3 {
		t.Fatalf("removal was not retried successfully: present=%v calls=%v", present, h.library.removeCalls)
	}
}
