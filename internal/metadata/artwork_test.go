package metadata

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
		ref := ImageRef{URL: base + name}
		var tag string
		if _, err := fmt.Sscanf(name, "/%dx%d-%s", &ref.Width, &ref.Height, &tag); err != nil {
			ref.Width, ref.Height = 0, 0
		}
		out = append(out, ref)
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

func (p *fakeProvider) setMeta(providerID string, meta GameMetadata) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.meta[providerID] = meta
}

// carryCase stores a screenshot and an artwork for match 2657, then applies the
// answer next builds under providerID.
func carryCase(t *testing.T, providerID string, next func(base string) GameMetadata) (*Service, catalog.Game, MediaAsset, MediaAsset, View) {
	t.Helper()
	srv, _ := sizedImageServer(t)
	meta := fullMetadata(srv.URL)
	meta.Cover = &ImageRef{URL: srv.URL + "/264x374-cover.png"}
	meta.Screenshots = refs(srv.URL, "/1000x600-s0.png")
	meta.Artworks = refs(srv.URL, "/1280x720-a0.png")
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": meta}}
	svc, cat, _ := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{Title: "Prey"})
	if _, err := svc.ApplyMatch(game.ID, "2657"); err != nil {
		t.Fatalf("first match: %v", err)
	}
	shots, arts := storedOfKind(svc, game.ID, AssetScreenshot), storedOfKind(svc, game.ID, AssetArtwork)
	if len(shots) != 1 || len(arts) != 1 {
		t.Fatalf("first match stored %d screenshots and %d artworks, want 1 and 1", len(shots), len(arts))
	}
	following := next(srv.URL)
	following.ProviderID = providerID
	provider.setMeta(providerID, following)
	view, err := svc.ApplyMatch(game.ID, providerID)
	if err != nil {
		t.Fatalf("second refresh: %v", err)
	}
	return svc, game, shots[0], arts[0], view
}

func TestFullRefreshCarriesPicturesOnlyWhenTheirDownloadsFailed(t *testing.T) {
	failing := func(base string) GameMetadata {
		meta := fullMetadata(base)
		meta.Cover = &ImageRef{URL: base + "/264x374-cover.png"}
		meta.Screenshots = []ImageRef{{URL: base + "/gone-s.png", Width: 1920, Height: 1080}}
		meta.Artworks = []ImageRef{{URL: base + "/gone-a.png", Width: 1920, Height: 1080}}
		return meta
	}
	none := func(base string) GameMetadata {
		meta := fullMetadata(base)
		meta.Cover = &ImageRef{URL: base + "/264x374-cover.png"}
		meta.Screenshots, meta.Artworks = nil, nil
		return meta
	}

	t.Run("same game, every download failed: the stored ones stay", func(t *testing.T) {
		svc, game, shot, art, view := carryCase(t, "2657", failing)
		if got := storedOfKind(svc, game.ID, AssetArtwork); len(got) != 1 || got[0].ID != art.ID {
			t.Fatalf("artworks = %v, want the stored one carried", sources(got))
		}
		if got := storedOfKind(svc, game.ID, AssetScreenshot); len(got) != 1 || got[0].ID != shot.ID {
			t.Fatalf("screenshots = %v, want the stored one carried", sources(got))
		}
		if view.HeroIsPoster || view.Hero != art.URL {
			t.Fatalf("hero = %q poster=%v, want the carried artwork", view.Hero, view.HeroIsPoster)
		}
	})

	t.Run("same game, the response lists none: nothing stays", func(t *testing.T) {
		svc, game, _, _, view := carryCase(t, "2657", none)
		if got := storedOfKind(svc, game.ID, AssetArtwork); len(got) != 0 {
			t.Fatalf("artworks = %v, want none", sources(got))
		}
		if got := storedOfKind(svc, game.ID, AssetScreenshot); len(got) != 0 {
			t.Fatalf("screenshots = %v, want none", sources(got))
		}
		if !view.HeroIsPoster {
			t.Fatalf("hero = %q, want the poster", view.Hero)
		}
	})

	for name, next := range map[string]func(string) GameMetadata{"downloads failed": failing, "response lists none": none} {
		t.Run("another game, "+name+": nothing of the old game stays", func(t *testing.T) {
			svc, game, _, _, view := carryCase(t, "7", next)
			if view.Game.ExternalIDs.IGDB != "7" {
				t.Fatalf("igdb id = %q, want the new match", view.Game.ExternalIDs.IGDB)
			}
			if got := storedOfKind(svc, game.ID, AssetArtwork); len(got) != 0 {
				t.Fatalf("artworks = %v, want none of the old game", sources(got))
			}
			if got := storedOfKind(svc, game.ID, AssetScreenshot); len(got) != 0 || len(view.Screenshots) != 0 {
				t.Fatalf("screenshots = %v (view %d), want none of the old game", sources(got), len(view.Screenshots))
			}
			if !view.HeroIsPoster {
				t.Fatalf("hero = %q, want the new game's poster", view.Hero)
			}
		})
	}
}

func TestCoverIsCarriedOnlyForTheSameGame(t *testing.T) {
	brokenCover := func(base string) GameMetadata {
		meta := fullMetadata(base)
		meta.Cover = &ImageRef{URL: base + "/gone-cover.png"}
		meta.Screenshots, meta.Artworks = nil, nil
		return meta
	}
	noCover := func(base string) GameMetadata {
		meta := fullMetadata(base)
		meta.Cover = nil
		meta.Screenshots, meta.Artworks = nil, nil
		return meta
	}

	for name, next := range map[string]func(string) GameMetadata{"its cover fails": brokenCover, "its answer has no cover": noCover} {
		t.Run("another game, "+name+": the old cover goes", func(t *testing.T) {
			svc, game, _, _, view := carryCase(t, "7", next)
			if covers := storedOfKind(svc, game.ID, AssetCover); len(covers) != 0 {
				t.Fatalf("covers = %v, want none of the old game", sources(covers))
			}
			if view.Cover != "" || view.Hero != "" || view.HeroIsPoster {
				t.Fatalf("cover %q hero %q poster=%v, want no cover and no poster hero", view.Cover, view.Hero, view.HeroIsPoster)
			}
		})
	}

	t.Run("same game, its cover fails: the stored cover stays", func(t *testing.T) {
		svc, game, _, _, view := carryCase(t, "2657", brokenCover)
		covers := storedOfKind(svc, game.ID, AssetCover)
		if len(covers) != 1 || !strings.HasSuffix(covers[0].SourceURL, "/264x374-cover.png") || view.Cover != covers[0].URL {
			t.Fatalf("covers = %v, view cover %q; want the stored cover carried", sources(covers), view.Cover)
		}
		if !view.HeroIsPoster || view.Hero != view.Cover {
			t.Fatalf("hero %q poster=%v, want the carried cover as the poster", view.Hero, view.HeroIsPoster)
		}
	})
}
