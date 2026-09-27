package accountsync

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRegressionReaddedGameDisappears(t *testing.T) {
	h := newHarness(t)
	h.catalog.link("game-1", "1")
	at := time.Now().Add(-time.Hour)
	h.server.get = func(w http.ResponseWriter) {
		writeJSON(w, 200, snapshotBody{Games: []wireGame{{IGDBID: 1, Removed: true, RemovedAt: &at}}})
	}
	h.server.put = echoPut(200)
	for i := 0; i < 2; i++ {
		h.library.setLocal("game-1", 0)
		if err := h.service.Sync(context.Background()); err != nil {
			t.Fatal(err)
		}
		if (h.library.gameOf("game-1").CanonicalGameID != "") != (i == 1) {
			t.Fatal("re-added game not restored")
		}
	}

}
func TestRegressionTokenChangesBetweenGetAndPut(t *testing.T) {
	h := newHarness(t)
	token := "account-A"
	h.service.client.token = func() (string, error) { return token, nil }
	var getAuth, putAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			getAuth = r.Header.Get("Authorization")
			token = "account-B"
			writeJSON(w, 200, snapshotBody{Games: []wireGame{{IGDBID: 1, PlaytimeSeconds: 123}}})
		} else {
			putAuth = r.Header.Get("Authorization")
			writeJSON(w, 200, applyBody{})
		}
	}))
	defer srv.Close()
	h.service.client.baseURL = srv.URL
	if err := h.service.Sync(context.Background()); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected changed account error: %v", err)
	}
	if getAuth != "Bearer account-A" || putAuth != "" {
		t.Fatalf("not reproduced %s %s", getAuth, putAuth)
	}
}
func TestRegressionLargeLibraryCannotSync(t *testing.T) {
	h := newHarness(t)
	h.server.put = echoPut(200)
	gs := make([]wireGame, 10000)
	for i := range gs {
		gs[i] = wireGame{IGDBID: int64(i + 1)}
	}
	h.server.get = func(w http.ResponseWriter) { writeJSON(w, 200, snapshotBody{Games: gs}) }
	err := h.service.Sync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
}
func TestRegressionPendingRemovalCrossesAccounts(t *testing.T) {
	h := newHarness(t)
	h.service.state.Games["1"] = gameState{DeviceSeconds: 10, Baseline: 10}
	h.service.state.DeviceID = "00000000-0000-4000-8000-000000000001"
	h.service.client.token = func() (string, error) { return "new-account-B", nil }
	h.server.get = func(w http.ResponseWriter) {
		writeJSON(w, 200, snapshotBody{Games: []wireGame{{IGDBID: 1, PlaytimeSeconds: 200}}})
	}
	h.server.put = echoPut(200)
	if err := h.service.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	g, ok := pushedFor(h.server.putBodies[0], 1)
	if ok && g.Removed {
		t.Fatal("removal crossed accounts")
	}
}
