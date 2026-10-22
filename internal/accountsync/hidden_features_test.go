package accountsync

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"

	"typhon/internal/settings"
)

type featureServer struct {
	features string
	remote   []wireGame
	settings settings.Portable
	revision int64
}

func (f *featureServer) install(h *harness) {
	h.server.get = func(w http.ResponseWriter) {
		if f.features != "" {
			w.Header().Set(featuresHeader, f.features)
		}
		writeJSON(w, http.StatusOK, snapshotBody{Games: f.remote, Settings: f.settings, SettingsRevision: f.revision})
	}
	h.server.put = func(w http.ResponseWriter, req putRequest) {
		if f.features != "" {
			w.Header().Set(featuresHeader, f.features)
		}
		f.remote = req.Games
		if req.Settings != nil {
			f.settings = *req.Settings
		}
		f.revision = req.SettingsRevision
		echoPut(http.StatusOK)(w, req)
	}
}

func lastPutRaw(h *harness) []byte {
	h.server.mu.Lock()
	defer h.server.mu.Unlock()
	return h.server.putRaw[len(h.server.putRaw)-1]
}

func TestHiddenFieldsFollowServerFeatureHeader(t *testing.T) {
	at := time.Now().Add(-time.Hour)

	t.Run("no header: no hidden keys and no repeated push", func(t *testing.T) {
		h := newHarness(t)
		h.catalog.link("game-1", "1")
		h.library.setLocalGame(Game{CanonicalGameID: "game-1", Hidden: false, HiddenAt: &at})
		srv := &featureServer{}
		srv.install(h)

		if err := h.service.Sync(context.Background()); err != nil {
			t.Fatalf("Sync: %v", err)
		}
		raw := lastPutRaw(h)
		for _, key := range []string{"hidden", "hiddenAt"} {
			if bytes.Contains(raw, []byte(`"`+key+`"`)) {
				t.Fatalf("PUT carries %q for a server without the feature: %s", key, raw)
			}
		}
		puts := h.server.putCalls
		if err := h.service.Sync(context.Background()); err != nil {
			t.Fatalf("second Sync: %v", err)
		}
		if h.server.putCalls != puts {
			t.Fatalf("second Sync pushed again (%d -> %d puts): hidden fields must not drive upToDate", puts, h.server.putCalls)
		}
	})

	t.Run("no header: server hidden state is not applied", func(t *testing.T) {
		h := newHarness(t)
		h.catalog.link("game-1", "1")
		h.library.setLocalGame(Game{CanonicalGameID: "game-1"})
		srv := &featureServer{remote: []wireGame{{IGDBID: 1, Hidden: true, HiddenAt: &at}}}
		srv.install(h)
		if err := h.service.Sync(context.Background()); err != nil {
			t.Fatalf("Sync: %v", err)
		}
		if h.library.gameOf("game-1").Hidden {
			t.Fatal("hidden applied from a server that did not announce the feature")
		}
	})

	t.Run("header: fields are sent", func(t *testing.T) {
		h := newHarness(t)
		h.catalog.link("game-1", "1")
		h.library.setLocalGame(Game{CanonicalGameID: "game-1", Hidden: true, HiddenAt: &at})
		srv := &featureServer{features: featureHidden}
		srv.install(h)
		if err := h.service.Sync(context.Background()); err != nil {
			t.Fatalf("Sync: %v", err)
		}
		raw := lastPutRaw(h)
		if !bytes.Contains(raw, []byte(`"hidden":true`)) || !bytes.Contains(raw, []byte(`"hiddenAt"`)) {
			t.Fatalf("PUT lacks hidden fields: %s", raw)
		}
	})

	t.Run("header disappears: fields stop again", func(t *testing.T) {
		h := newHarness(t)
		h.catalog.link("game-1", "1")
		h.library.setLocalGame(Game{CanonicalGameID: "game-1", Hidden: true, HiddenAt: &at})
		srv := &featureServer{features: featureHidden}
		srv.install(h)
		if err := h.service.Sync(context.Background()); err != nil {
			t.Fatalf("Sync: %v", err)
		}
		srv.features = ""
		h.library.setLocalGame(Game{CanonicalGameID: "game-1", Hidden: true, HiddenAt: &at, PlaytimeSeconds: 5})
		if err := h.service.Sync(context.Background()); err != nil {
			t.Fatalf("second Sync: %v", err)
		}
		raw := lastPutRaw(h)
		if bytes.Contains(raw, []byte(`"hidden`)) {
			t.Fatalf("PUT still carries hidden fields after the header vanished: %s", raw)
		}
	})
}
