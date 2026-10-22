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

func TestHidePresenceGameIsNotReported(t *testing.T) {
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

	h.svc.SessionStarted(library.Game{ID: "a", CanonicalGameID: "game-a"})
	h.awaitSend()
	h.nextRequest()
	h.svc.SessionStarted(library.Game{ID: "secret", CanonicalGameID: "game-secret", HidePresence: true})
	h.svc.Kick()
	h.awaitSend()
	if p := decodeBody(t, h.nextRequest()); p.GameID != "111" {
		t.Fatalf("gameId = %q, want 111: a hide-presence game must not be reported", p.GameID)
	}
}

func TestHidingRunningGameDropsItFromReport(t *testing.T) {
	resolve := func(id string) string {
		if id == "game-a" {
			return "111"
		}
		return "222"
	}
	h := newHarness(t, staticToken("tok"), http.StatusNoContent, resolve)
	h.start()
	h.awaitSend()
	h.nextRequest()

	h.svc.SessionStarted(library.Game{ID: "a", CanonicalGameID: "game-a"})
	h.awaitSend()
	if p := decodeBody(t, h.nextRequest()); p.GameID != "111" {
		t.Fatalf("gameId = %q, want 111", p.GameID)
	}

	h.svc.LibraryUpdated([]library.Game{{ID: "a", CanonicalGameID: "game-a", Hidden: true}})
	h.awaitSend()
	if p := decodeBody(t, h.nextRequest()); p.GameID != "" {
		t.Fatalf("gameId = %q, want empty: a game hidden mid-session must leave the report", p.GameID)
	}
}

func TestLibraryUpdateKeepsVisibleRunningGame(t *testing.T) {
	h := newHarness(t, staticToken("tok"), http.StatusNoContent, func(string) string { return "111" })
	h.start()
	h.awaitSend()
	h.nextRequest()
	h.svc.SessionStarted(library.Game{ID: "a", CanonicalGameID: "game-a"})
	h.awaitSend()
	h.nextRequest()

	h.svc.LibraryUpdated([]library.Game{{ID: "a", CanonicalGameID: "game-a", Favorite: true}})
	h.svc.mu.Lock()
	_, running := h.svc.running["a"]
	h.svc.mu.Unlock()
	if !running {
		t.Fatal("a visible running game must stay in the report")
	}
}
