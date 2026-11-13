package metadata

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"typhon/internal/catalog"
)

// sizedImageServer serves a PNG of the size its path names: /1280x720-a.png is
// a 1280×720 picture. It counts the requests it answered.
func sizedImageServer(t *testing.T) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var hits atomic.Int64
	var mu sync.Mutex
	cache := map[[2]int][]byte{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		var width, height int
		var tag string
		if _, err := fmt.Sscanf(r.URL.Path, "/%dx%d-%s", &width, &height, &tag); err != nil || width <= 0 || height <= 0 {
			http.Error(w, "no such picture", http.StatusNotFound)
			return
		}
		mu.Lock()
		body, ok := cache[[2]int{width, height}]
		if !ok {
			body = pngBytes(t, width, height)
			cache[[2]int{width, height}] = body
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "image/png")
		if _, err := w.Write(body); err != nil {
			t.Errorf("write image: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func refs(base string, names ...string) []ImageRef {
	out := make([]ImageRef, 0, len(names))
	for _, name := range names {
		if name == "" || name[0] != '/' {
			out = append(out, ImageRef{URL: name})
			continue
		}
		out = append(out, ImageRef{URL: base + name})
	}
	return out
}

func storedOfKind(svc *Service, gameID string, kind AssetType) []MediaAsset {
	var out []MediaAsset
	for _, a := range svc.store.list(gameID) {
		if a.Type == kind {
			out = append(out, a)
		}
	}
	return out
}

func sources(assets []MediaAsset) []string {
	out := make([]string, 0, len(assets))
	for _, a := range assets {
		out = append(out, a.SourceURL)
	}
	return out
}

func TestApplyStoresArtworksUpToTheCap(t *testing.T) {
	srv, _ := sizedImageServer(t)
	meta := fullMetadata(srv.URL)
	meta.Cover = &ImageRef{URL: srv.URL + "/264x374-cover.png"}
	meta.Screenshots = refs(srv.URL, "/1280x720-s0.png")
	names := make([]string, 0, maxArtworks+2)
	for i := range maxArtworks + 2 {
		names = append(names, fmt.Sprintf("/1920x1080-a%d.png", i))
	}
	meta.Artworks = refs(srv.URL, names...)
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": meta}}
	svc, cat, dir := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{Title: "Prey"})

	view, err := svc.ApplyMatch(game.ID, "2657")
	if err != nil {
		t.Fatalf("apply match: %v", err)
	}
	arts := storedOfKind(svc, game.ID, AssetArtwork)
	want := sources(nil)
	for _, ref := range meta.Artworks[:maxArtworks] {
		want = append(want, ref.URL)
	}
	if got := sources(arts); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("stored artworks = %v, want the first %d in response order %v", got, maxArtworks, want)
	}
	for _, a := range arts {
		if _, err := os.Stat(filepath.Join(dir, mediaDirName, filepath.FromSlash(a.Path))); err != nil {
			t.Fatalf("artwork file: %v", err)
		}
	}
	if len(view.Screenshots) != 1 {
		t.Fatalf("view screenshots = %d, want only the screenshot without artworks", len(view.Screenshots))
	}

	reopened, err := newAssetStore(dir)
	if err != nil {
		t.Fatalf("reopen asset store: %v", err)
	}
	if got := len(reopened.list(game.ID)); got != 1+1+maxArtworks {
		t.Fatalf("persisted assets = %d, want cover, screenshot and %d artworks", got, maxArtworks)
	}
}

func TestApplyDropsAnArtworkLikeAScreenshot(t *testing.T) {
	srv, hits := sizedImageServer(t)
	meta := fullMetadata(srv.URL)
	meta.Cover = &ImageRef{URL: srv.URL + "/264x374-cover.png"}
	bad := []string{"http://images.example.com/plain-http.png", "ftp://images.example.com/x.png", "::not a url"}
	meta.Screenshots = refs(srv.URL, bad...)
	meta.Artworks = refs(srv.URL, bad[0], "/1280x720-a.png")
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": meta}}
	svc, cat, _ := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{Title: "Prey"})

	if _, err := svc.ApplyMatch(game.ID, "2657"); err != nil {
		t.Fatalf("apply match: %v", err)
	}
	if shots := storedOfKind(svc, game.ID, AssetScreenshot); len(shots) != 0 {
		t.Fatalf("screenshots = %v, want the refused ones dropped", sources(shots))
	}
	arts := storedOfKind(svc, game.ID, AssetArtwork)
	if len(arts) != 1 || arts[0].SourceURL != srv.URL+"/1280x720-a.png" {
		t.Fatalf("artworks = %v, want only the https/loopback one", sources(arts))
	}
	if got := hits.Load(); got != 2 {
		t.Fatalf("image requests = %d, want the cover and the one valid artwork", got)
	}
}

func TestArtRefreshKeepsStoredArtworks(t *testing.T) {
	srv, _ := sizedImageServer(t)
	meta := fullMetadata(srv.URL)
	meta.Cover = &ImageRef{URL: srv.URL + "/264x374-cover.png"}
	meta.Screenshots = refs(srv.URL, "/600x900-s0.png")
	meta.Artworks = refs(srv.URL, "/1280x720-a0.png")
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": meta}}
	svc, cat, _ := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{Title: "Prey"})

	if _, err := svc.ApplyMatch(game.ID, "2657"); err != nil {
		t.Fatalf("apply match: %v", err)
	}
	before := storedOfKind(svc, game.ID, AssetArtwork)
	if len(before) != 1 {
		t.Fatalf("artworks after the full refresh = %d, want 1", len(before))
	}

	svc.SetLanguage("ru")
	if _, err := svc.EnsureArt([]string{game.ID}); err != nil {
		t.Fatalf("ensure art: %v", err)
	}
	svc.wg.Wait()
	updated, err := cat.GetGame(game.ID)
	if err != nil {
		t.Fatalf("reload game: %v", err)
	}
	if updated.MetadataLanguage != "ru" {
		t.Fatalf("metadata language = %q, the art refresh did not run", updated.MetadataLanguage)
	}
	after := storedOfKind(svc, game.ID, AssetArtwork)
	if len(after) != 1 || after[0].ID != before[0].ID {
		t.Fatalf("artworks after the art refresh = %+v, want %+v kept", after, before)
	}
}
