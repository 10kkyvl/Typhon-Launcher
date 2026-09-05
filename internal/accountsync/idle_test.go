package accountsync

import (
	"context"
	"net/http"
	"testing"
	"time"

	"typhon/internal/settings"
)

func idleHarness(t *testing.T) (*harness, *[]wireGame) {
	t.Helper()
	h := newHarness(t)
	h.catalog.link("game-42", "42")
	h.library.setLocalGame(Game{CanonicalGameID: "game-42", PlaytimeSeconds: 100, Owned: true})

	served := &[]wireGame{}
	remoteSettings := settings.PortableOf(h.settings.Get())
	h.server.get = func(w http.ResponseWriter) {
		writeJSON(w, http.StatusOK, snapshotBody{
			Settings:         remoteSettings,
			SettingsRevision: 1,
			Games:            *served,
		})
	}
	h.server.put = echoPut(http.StatusOK)

	if err := h.service.Sync(context.Background()); err != nil {
		t.Fatalf("first Sync: %v", err)
	}
	*served = []wireGame{{IGDBID: 42, Owned: true, PlaytimeSeconds: 100}}
	return h, served
}

func putsAfterSync(t *testing.T, h *harness) int {
	t.Helper()
	before := h.server.putCalls
	if err := h.service.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	return h.server.putCalls - before
}

func TestSyncSkipsThePushWhenNothingChanged(t *testing.T) {
	h, _ := idleHarness(t)

	if got := putsAfterSync(t, h); got != 0 {
		t.Fatalf("put calls on an idle sync = %d, want 0", got)
	}
	if got := putsAfterSync(t, h); got != 0 {
		t.Fatalf("put calls on a second idle sync = %d, want 0", got)
	}
}

func TestSyncPushesAfterAStatusChange(t *testing.T) {
	h, _ := idleHarness(t)
	at := time.Now()
	h.library.setLocalGame(Game{
		CanonicalGameID: "game-42",
		PlaytimeSeconds: 100,
		Owned:           true,
		Status:          "completed",
		StatusAt:        &at,
	})

	if got := putsAfterSync(t, h); got != 1 {
		t.Fatalf("put calls after a status change = %d, want 1", got)
	}
}

func TestSyncPushesAfterPlaytimeGrows(t *testing.T) {
	h, _ := idleHarness(t)
	h.library.setLocalGame(Game{CanonicalGameID: "game-42", PlaytimeSeconds: 160, Owned: true})

	if got := putsAfterSync(t, h); got != 1 {
		t.Fatalf("put calls after playtime grew = %d, want 1", got)
	}
}

func TestSyncPushesWhenALocalGameIsMissingRemotely(t *testing.T) {
	h, _ := idleHarness(t)
	h.catalog.link("game-7", "7")
	h.library.setLocalGame(Game{CanonicalGameID: "game-7", Owned: true})

	if got := putsAfterSync(t, h); got != 1 {
		t.Fatalf("put calls after a new local game = %d, want 1", got)
	}
}

func TestSyncPushesAfterSettingsChange(t *testing.T) {
	h, _ := idleHarness(t)
	current := h.settings.Get()
	current.MinimizeToTray = !current.MinimizeToTray
	if err := h.settings.Save(current); err != nil {
		t.Fatalf("Save settings: %v", err)
	}

	if got := putsAfterSync(t, h); got != 1 {
		t.Fatalf("put calls after a settings change = %d, want 1", got)
	}
}
