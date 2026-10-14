package online

import (
	"net/http"
	"testing"

	"typhon/internal/library"
)

func TestHiddenGameIsNotReported(t *testing.T) {
	resolve := func(id string) string {
		if id == "game-a" {
			return "111"
		}
		return "999"
	}
	h := newHarness(t, staticToken("tok"), http.StatusNoContent, resolve)
	h.start()
	h.awaitSend()
	h.nextRequest()

	h.svc.SessionStarted(library.Game{ID: "secret", CanonicalGameID: "game-secret", Hidden: true})
	h.svc.SessionStarted(library.Game{ID: "a", CanonicalGameID: "game-a"})
	h.awaitSend()
	if p := decodeBody(t, h.nextRequest()); p.GameID != "111" {
		t.Fatalf("gameId = %q, want 111: a hidden game must not be reported", p.GameID)
	}
}
