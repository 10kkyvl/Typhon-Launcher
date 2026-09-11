package accountsync

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// The real library retains installations when a cloud-only card is removed.
type retainedInstallationLibrary struct{ *fakeLibrary }

func (retainedInstallationLibrary) Remove(string) error { return nil }

func TestRemoteRemovalDoesNotPushRetainedInstallationBack(t *testing.T) {
	h := newHarness(t)
	h.catalog.link("installed", "1")
	h.library.setLocal("installed", 50)
	h.service.library = retainedInstallationLibrary{h.library}
	at := time.Now().Add(-time.Minute)
	h.server.get = func(w http.ResponseWriter) {
		writeJSON(w, http.StatusOK, snapshotBody{Games: []wireGame{{IGDBID: 1, Removed: true, RemovedAt: &at}}})
	}
	h.server.put = func(w http.ResponseWriter, req putRequest) {
		if _, ok := pushedFor(req, 1); ok {
			t.Error("retained installation was pushed over a cloud removal")
		}
		echoPut(http.StatusOK)(w, req)
	}
	for i := 0; i < 2; i++ {
		if err := h.service.Sync(context.Background()); err != nil {
			t.Fatal(err)
		}
		if got := h.library.gameOf("installed"); got.CanonicalGameID != "installed" {
			t.Fatal("local installation was removed")
		}
	}
}
