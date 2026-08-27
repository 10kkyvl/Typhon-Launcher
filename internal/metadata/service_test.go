package metadata

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"typhon/internal/catalog"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type fakeProvider struct {
	mu         sync.Mutex
	searches   int
	gets       int
	candidates []Candidate
	meta       map[string]GameMetadata
	searchErr  error
	getErr     error
}

func (p *fakeProvider) Name() string { return "fake" }

func (p *fakeProvider) Search(_ context.Context, _ string, _ int) ([]Candidate, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.searches++
	if p.searchErr != nil {
		return nil, p.searchErr
	}
	return append([]Candidate(nil), p.candidates...), nil
}

func (p *fakeProvider) Get(_ context.Context, providerID string) (GameMetadata, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.gets++
	if p.getErr != nil {
		return GameMetadata{}, p.getErr
	}
	meta, ok := p.meta[providerID]
	if !ok {
		return GameMetadata{}, ErrNoMatch
	}
	return meta, nil
}

func (p *fakeProvider) counts() (int, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.searches, p.gets
}

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func imageServer(t *testing.T) *httptest.Server {
	t.Helper()
	landscape := pngBytes(t, 1920, 1080)
	portrait := pngBytes(t, 600, 900)
	cover := pngBytes(t, 264, 374)

	mux := http.NewServeMux()
	write := func(w http.ResponseWriter, body []byte) {
		w.Header().Set("Content-Type", "image/png")
		if _, err := w.Write(body); err != nil {
			t.Errorf("write image: %v", err)
		}
	}
	mux.HandleFunc("/cover.png", func(w http.ResponseWriter, _ *http.Request) { write(w, cover) })
	mux.HandleFunc("/portrait.png", func(w http.ResponseWriter, _ *http.Request) { write(w, portrait) })
	mux.HandleFunc("/broken.png", func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write([]byte("<html>not an image</html>")); err != nil {
			t.Errorf("write body: %v", err)
		}
	})
	mux.HandleFunc("/missing.png", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "gone", http.StatusNotFound)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) { write(w, landscape) })

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func newTestService(t *testing.T, provider Provider) (*Service, *catalog.Service, string) {
	t.Helper()
	dir := t.TempDir()
	cat, err := catalog.NewServiceAt(dir)
	if err != nil {
		t.Fatalf("catalog service: %v", err)
	}
	svc, err := NewServiceAt(dir, cat, provider)
	if err != nil {
		t.Fatalf("metadata service: %v", err)
	}
	unthrottle(svc)
	if err := svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("startup: %v", err)
	}
	t.Cleanup(func() {
		if err := svc.ServiceShutdown(); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})
	return svc, cat, dir
}

// Темп запросов проверяется отдельно на подставных часах; остальным тестам
// пауза между вызовами провайдера только добавляет секунды к прогону.
func unthrottle(svc *Service) {
	svc.budget.mu.Lock()
	defer svc.budget.mu.Unlock()
	svc.budget.rate = 1e6
	svc.budget.tokens = burst
	svc.budget.pausedUntil = time.Time{}
}

func resetBudget(svc *Service) {
	svc.budget.mu.Lock()
	defer svc.budget.mu.Unlock()
	svc.budget.rate = startRate
	svc.budget.tokens = burst
	svc.budget.pausedUntil = time.Time{}
}

func addGame(t *testing.T, cat *catalog.Service, game catalog.Game) catalog.Game {
	t.Helper()
	stored, err := cat.AddGame(game)
	if err != nil {
		t.Fatalf("add game: %v", err)
	}
	return stored
}

func fullMetadata(base string) GameMetadata {
	released := time.Date(2017, time.May, 5, 0, 0, 0, 0, time.UTC)
	shots := make([]ImageRef, 0, 12)
	for i := range 12 {
		shots = append(shots, ImageRef{URL: fmt.Sprintf("%s/shot-%d.png", base, i), Width: 1920, Height: 1080})
	}
	return GameMetadata{
		ProviderID:  "2657",
		Title:       "Prey",
		Summary:     "Морган Ю просыпается на станции «Талос-1».",
		ReleaseDate: &released,
		Developer:   "Arkane Studios",
		Publisher:   "Bethesda Softworks",
		Genres:      []string{"Shooter", "Adventure"},
		Themes:      []string{"Science fiction", "Horror"},
		Platforms:   []string{"PC (Microsoft Windows)", "PlayStation 4"},
		Cover:       &ImageRef{URL: base + "/cover.png", Width: 264, Height: 374},
		Screenshots: shots,
	}
}

func TestApplyMatchPersistsMetadataAndAssets(t *testing.T) {
	srv := imageServer(t)
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": fullMetadata(srv.URL)}}
	svc, cat, dir := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{Title: "Prey", Provisional: true})

	view, err := svc.ApplyMatch(game.ID, "2657")
	if err != nil {
		t.Fatalf("apply match: %v", err)
	}

	if view.Game.ExternalIDs.IGDB != "2657" {
		t.Fatalf("igdb id = %q, want 2657", view.Game.ExternalIDs.IGDB)
	}
	if view.Game.Summary == "" || view.Game.Developer != "Arkane Studios" || view.Game.Publisher != "Bethesda Softworks" {
		t.Fatalf("text metadata not stored: %+v", view.Game)
	}
	if len(view.Game.Genres) != 2 || len(view.Game.Themes) != 2 || len(view.Game.Platforms) != 2 {
		t.Fatalf("lists not stored: %+v", view.Game)
	}
	if view.Game.ReleaseYear == nil || *view.Game.ReleaseYear != 2017 {
		t.Fatalf("release year not derived: %+v", view.Game.ReleaseYear)
	}
	if view.Game.MetadataUpdatedAt == nil {
		t.Fatal("metadata timestamp not set")
	}
	if view.Cover == "" {
		t.Fatal("cover url missing")
	}
	if len(view.Screenshots) != maxScreenshots {
		t.Fatalf("screenshots = %d, want %d", len(view.Screenshots), maxScreenshots)
	}
	if view.Hero == "" {
		t.Fatal("hero not selected")
	}

	stored, err := cat.GetGame(game.ID)
	if err != nil {
		t.Fatalf("reload game: %v", err)
	}
	if stored.CoverAssetID == "" || stored.HeroAssetID == "" {
		t.Fatalf("asset ids not persisted: %+v", stored)
	}

	for _, asset := range append(view.Screenshots, MediaAsset{Path: strings.TrimPrefix(view.Cover, "/"+mediaDirName+"/")}) {
		if asset.Path == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, mediaDirName, filepath.FromSlash(asset.Path))); err != nil {
			t.Fatalf("asset file missing: %v", err)
		}
	}

	reopened, err := newAssetStore(dir)
	if err != nil {
		t.Fatalf("reopen asset store: %v", err)
	}
	if got := len(reopened.list(game.ID)); got != maxScreenshots+1 {
		t.Fatalf("persisted assets = %d, want %d", got, maxScreenshots+1)
	}
}

func TestApplyMatchCachePathUsesGameIDNotTitle(t *testing.T) {
	srv := imageServer(t)
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": fullMetadata(srv.URL)}}
	svc, cat, _ := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{Title: "Prey: Mooncrash / Издание", Provisional: true})

	view, err := svc.ApplyMatch(game.ID, "2657")
	if err != nil {
		t.Fatalf("apply match: %v", err)
	}
	for _, asset := range view.Screenshots {
		if !strings.HasPrefix(asset.Path, "games/"+game.ID+"/") {
			t.Fatalf("asset path %q does not use game id", asset.Path)
		}
		if strings.Contains(strings.ToLower(asset.Path), "prey") || strings.Contains(asset.Path, " ") {
			t.Fatalf("asset path %q leaks the title", asset.Path)
		}
	}
}

func TestHeroSkipsPortraitScreenshot(t *testing.T) {
	srv := imageServer(t)
	meta := fullMetadata(srv.URL)
	meta.Screenshots = []ImageRef{
		{URL: srv.URL + "/portrait.png"},
		{URL: srv.URL + "/landscape.png"},
	}
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": meta}}
	svc, cat, _ := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{Title: "Prey"})

	view, err := svc.ApplyMatch(game.ID, "2657")
	if err != nil {
		t.Fatalf("apply match: %v", err)
	}
	stored, err := cat.GetGame(game.ID)
	if err != nil {
		t.Fatalf("reload game: %v", err)
	}
	hero, ok := svc.store.find(stored.HeroAssetID)
	if !ok {
		t.Fatal("hero asset not found")
	}
	if hero.Width <= hero.Height {
		t.Fatalf("hero is not landscape: %dx%d", hero.Width, hero.Height)
	}
	if view.Hero != hero.URL {
		t.Fatalf("view hero = %q, want %q", view.Hero, hero.URL)
	}
}

func TestHeroEmptyWhenOnlyPortraitScreenshots(t *testing.T) {
	srv := imageServer(t)
	meta := fullMetadata(srv.URL)
	meta.Screenshots = []ImageRef{{URL: srv.URL + "/portrait.png"}}
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": meta}}
	svc, cat, _ := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{Title: "Prey"})

	if _, err := svc.ApplyMatch(game.ID, "2657"); err != nil {
		t.Fatalf("apply match: %v", err)
	}
	stored, err := cat.GetGame(game.ID)
	if err != nil {
		t.Fatalf("reload game: %v", err)
	}
	if stored.HeroAssetID != "" {
		t.Fatalf("hero = %q, want empty", stored.HeroAssetID)
	}
}

func TestMissingCoverKeepsTextMetadata(t *testing.T) {
	srv := imageServer(t)
	meta := fullMetadata(srv.URL)
	meta.Cover = nil
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": meta}}
	svc, cat, _ := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{Title: "Prey"})

	view, err := svc.ApplyMatch(game.ID, "2657")
	if err != nil {
		t.Fatalf("apply match: %v", err)
	}
	if view.Cover != "" {
		t.Fatalf("cover = %q, want empty", view.Cover)
	}
	if view.Game.Summary == "" {
		t.Fatal("summary dropped together with the cover")
	}
}

func TestInvalidImageContentKeepsTextMetadata(t *testing.T) {
	srv := imageServer(t)
	meta := fullMetadata(srv.URL)
	meta.Cover = &ImageRef{URL: srv.URL + "/broken.png"}
	meta.Screenshots = []ImageRef{{URL: srv.URL + "/missing.png"}, {URL: srv.URL + "/ok.png"}}
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": meta}}
	svc, cat, _ := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{Title: "Prey"})

	view, err := svc.ApplyMatch(game.ID, "2657")
	if err != nil {
		t.Fatalf("apply match: %v", err)
	}
	if view.Cover != "" {
		t.Fatalf("cover = %q, want empty", view.Cover)
	}
	if view.Game.Developer != "Arkane Studios" {
		t.Fatalf("developer = %q, text metadata lost", view.Game.Developer)
	}
	if len(view.Screenshots) != 1 {
		t.Fatalf("screenshots = %d, want the single healthy one", len(view.Screenshots))
	}
}

func TestFailedCoverKeepsPreviousCover(t *testing.T) {
	srv := imageServer(t)
	meta := fullMetadata(srv.URL)
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": meta}}
	svc, cat, dir := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{Title: "Prey"})

	first, err := svc.ApplyMatch(game.ID, "2657")
	if err != nil {
		t.Fatalf("first match: %v", err)
	}

	broken := fullMetadata(srv.URL)
	broken.Cover = &ImageRef{URL: srv.URL + "/broken.png"}
	provider.mu.Lock()
	provider.meta["2657"] = broken
	provider.mu.Unlock()

	second, err := svc.Refresh(game.ID)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if second.Cover != first.Cover {
		t.Fatalf("cover = %q, want preserved %q", second.Cover, first.Cover)
	}
	rel := strings.TrimPrefix(second.Cover, "/"+mediaDirName+"/")
	if _, err := os.Stat(filepath.Join(dir, mediaDirName, filepath.FromSlash(rel))); err != nil {
		t.Fatalf("previous cover file removed: %v", err)
	}
}

func TestRefreshUsesStoredIDWithoutSearch(t *testing.T) {
	srv := imageServer(t)
	provider := &fakeProvider{
		meta:       map[string]GameMetadata{"2657": fullMetadata(srv.URL)},
		candidates: []Candidate{{ProviderID: "999", Title: "Prey"}},
	}
	svc, cat, _ := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{
		Title:       "Prey",
		ExternalIDs: catalog.ExternalIDs{IGDB: "2657"},
	})

	if _, err := svc.Refresh(game.ID); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	searches, gets := provider.counts()
	if searches != 0 {
		t.Fatalf("searches = %d, want 0 for a game with a stored igdb id", searches)
	}
	if gets != 1 {
		t.Fatalf("gets = %d, want 1", gets)
	}
}

func TestRefreshWithoutIDRefusesAmbiguousMatch(t *testing.T) {
	srv := imageServer(t)
	provider := &fakeProvider{
		meta: map[string]GameMetadata{"2657": fullMetadata(srv.URL)},
		candidates: []Candidate{
			{ProviderID: "2657", Title: "Prey", ReleaseYear: 2017},
			{ProviderID: "7", Title: "Prey", ReleaseYear: 2006},
		},
	}
	svc, cat, _ := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{Title: "Prey"})

	if _, err := svc.Refresh(game.ID); !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("err = %v, want ErrAmbiguous", err)
	}
	stored, err := cat.GetGame(game.ID)
	if err != nil {
		t.Fatalf("reload game: %v", err)
	}
	if stored.ExternalIDs.IGDB != "" {
		t.Fatalf("igdb id = %q, want empty after an ambiguous search", stored.ExternalIDs.IGDB)
	}
}

func TestRefreshWithoutIDAcceptsConfidentMatch(t *testing.T) {
	srv := imageServer(t)
	provider := &fakeProvider{
		meta: map[string]GameMetadata{"2657": fullMetadata(srv.URL)},
		candidates: []Candidate{
			{ProviderID: "2657", Title: "Prey", ReleaseYear: 2017},
			{ProviderID: "7", Title: "Prey", ReleaseYear: 2006},
		},
	}
	svc, cat, _ := newTestService(t, provider)
	year := 2017
	game := addGame(t, cat, catalog.Game{Title: "Prey", ReleaseYear: &year})

	view, err := svc.Refresh(game.ID)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if view.Game.ExternalIDs.IGDB != "2657" {
		t.Fatalf("igdb id = %q, want 2657", view.Game.ExternalIDs.IGDB)
	}
}

func TestProviderFailureKeepsStoredMetadata(t *testing.T) {
	srv := imageServer(t)
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": fullMetadata(srv.URL)}}
	svc, cat, _ := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{Title: "Prey"})

	before, err := svc.ApplyMatch(game.ID, "2657")
	if err != nil {
		t.Fatalf("apply match: %v", err)
	}

	offline := errors.New("igdb недоступен")
	provider.mu.Lock()
	provider.getErr = offline
	provider.mu.Unlock()

	if _, err := svc.Refresh(game.ID); !errors.Is(err, offline) {
		t.Fatalf("err = %v, want the provider error", err)
	}

	after, err := svc.GetView(game.ID)
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if after.Cover != before.Cover || len(after.Screenshots) != len(before.Screenshots) {
		t.Fatalf("artwork changed after a provider failure: %+v", after)
	}
	if after.Game.Summary != before.Game.Summary {
		t.Fatal("summary changed after a provider failure")
	}
	if !after.Game.MetadataUpdatedAt.Equal(*before.Game.MetadataUpdatedAt) {
		t.Fatal("refresh timestamp moved despite the failure")
	}
}

func TestCatalogWriteFailureKeepsPreviousMetadata(t *testing.T) {
	srv := imageServer(t)
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": fullMetadata(srv.URL)}}
	svc, cat, dir := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{Title: "Prey"})

	before, err := svc.ApplyMatch(game.ID, "2657")
	if err != nil {
		t.Fatalf("apply match: %v", err)
	}

	catalogPath := filepath.Join(dir, "catalog.json")
	if err := os.Remove(catalogPath); err != nil {
		t.Fatalf("remove catalog: %v", err)
	}
	if err := os.Mkdir(catalogPath, 0o755); err != nil {
		t.Fatalf("block catalog path: %v", err)
	}

	if _, err := svc.Refresh(game.ID); err == nil {
		t.Fatal("refresh succeeded despite an unwritable catalog")
	}

	after, err := svc.GetView(game.ID)
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if after.Cover != before.Cover {
		t.Fatalf("cover = %q, want preserved %q", after.Cover, before.Cover)
	}
	if len(after.Screenshots) != len(before.Screenshots) {
		t.Fatalf("screenshots = %d, want %d", len(after.Screenshots), len(before.Screenshots))
	}
	for _, asset := range after.Screenshots {
		if _, err := os.Stat(filepath.Join(dir, mediaDirName, filepath.FromSlash(asset.Path))); err != nil {
			t.Fatalf("previous asset file removed: %v", err)
		}
	}

	entries, err := os.ReadDir(filepath.Join(dir, mediaDirName, gamesDirName, game.ID))
	if err != nil {
		t.Fatalf("read media dir: %v", err)
	}
	if len(entries) != len(before.Screenshots)+1 {
		t.Fatalf("media files = %d, want %d: temporary assets were not cleaned up", len(entries), len(before.Screenshots)+1)
	}
}

func TestChangeMatchReplacesAssets(t *testing.T) {
	srv := imageServer(t)
	other := fullMetadata(srv.URL)
	other.ProviderID = "7"
	other.Title = "Prey"
	other.Developer = "Human Head Studios"
	other.Screenshots = other.Screenshots[:2]
	provider := &fakeProvider{meta: map[string]GameMetadata{
		"2657": fullMetadata(srv.URL),
		"7":    other,
	}}
	svc, cat, dir := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{Title: "Prey"})

	first, err := svc.ApplyMatch(game.ID, "2657")
	if err != nil {
		t.Fatalf("first match: %v", err)
	}
	second, err := svc.ApplyMatch(game.ID, "7")
	if err != nil {
		t.Fatalf("second match: %v", err)
	}

	if second.Game.ExternalIDs.IGDB != "7" {
		t.Fatalf("igdb id = %q, want 7", second.Game.ExternalIDs.IGDB)
	}
	if second.Game.Developer != "Human Head Studios" {
		t.Fatalf("developer = %q", second.Game.Developer)
	}
	if len(second.Screenshots) != 2 {
		t.Fatalf("screenshots = %d, want 2", len(second.Screenshots))
	}
	rel := strings.TrimPrefix(first.Cover, "/"+mediaDirName+"/")
	if _, err := os.Stat(filepath.Join(dir, mediaDirName, filepath.FromSlash(rel))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale cover file kept: %v", err)
	}
}

func TestGameWithoutDeveloperStaysEmpty(t *testing.T) {
	srv := imageServer(t)
	meta := fullMetadata(srv.URL)
	meta.Developer = ""
	meta.Publisher = ""
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": meta}}
	svc, cat, _ := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{Title: "Prey"})

	view, err := svc.ApplyMatch(game.ID, "2657")
	if err != nil {
		t.Fatalf("apply match: %v", err)
	}
	if view.Game.Developer != "" || view.Game.Publisher != "" {
		t.Fatalf("invented studio: developer=%q publisher=%q", view.Game.Developer, view.Game.Publisher)
	}
}

func TestStaleFollowsTTL(t *testing.T) {
	svc, cat, _ := newTestService(t, &fakeProvider{})
	game := addGame(t, cat, catalog.Game{Title: "Prey"})

	if !svc.stale(game) {
		t.Fatal("game without metadata must be stale")
	}

	fresh := time.Now().Add(-time.Hour)
	game.MetadataUpdatedAt = &fresh
	if svc.stale(game) {
		t.Fatal("metadata refreshed an hour ago must not be stale")
	}

	old := time.Now().Add(-defaultTTL - time.Hour)
	game.MetadataUpdatedAt = &old
	if !svc.stale(game) {
		t.Fatal("metadata older than the ttl must be stale")
	}
}

func TestEnsureFreshSkipsFreshMetadata(t *testing.T) {
	svc, cat, _ := newTestService(t, &fakeProvider{})

	fresh := time.Now()
	resolved := addGame(t, cat, catalog.Game{
		Title:             "Dishonored",
		ExternalIDs:       catalog.ExternalIDs{IGDB: "11"},
		MetadataUpdatedAt: &fresh,
	})
	started, err := svc.EnsureFresh(resolved.ID)
	if err != nil {
		t.Fatalf("ensure fresh: %v", err)
	}
	if started {
		t.Fatal("refresh started for fresh metadata")
	}
}

func TestEnsureFreshRefreshesStaleMetadata(t *testing.T) {
	srv := imageServer(t)
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": fullMetadata(srv.URL)}}
	svc, cat, _ := newTestService(t, provider)
	old := time.Now().Add(-defaultTTL - time.Hour)
	game := addGame(t, cat, catalog.Game{
		Title:             "Prey",
		ExternalIDs:       catalog.ExternalIDs{IGDB: "2657"},
		MetadataUpdatedAt: &old,
	})

	started, err := svc.EnsureFresh(game.ID)
	if err != nil {
		t.Fatalf("ensure fresh: %v", err)
	}
	if !started {
		t.Fatal("stale metadata was not refreshed")
	}
	svc.wg.Wait()

	updated, err := cat.GetGame(game.ID)
	if err != nil {
		t.Fatalf("reload game: %v", err)
	}
	if updated.MetadataUpdatedAt == nil || !updated.MetadataUpdatedAt.After(old) {
		t.Fatalf("timestamp not advanced: %v", updated.MetadataUpdatedAt)
	}
}

func TestGetViewFallsBackToPlaceholderWhenFileGone(t *testing.T) {
	srv := imageServer(t)
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": fullMetadata(srv.URL)}}
	svc, cat, dir := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{Title: "Prey"})

	view, err := svc.ApplyMatch(game.ID, "2657")
	if err != nil {
		t.Fatalf("apply match: %v", err)
	}
	rel := strings.TrimPrefix(view.Cover, "/"+mediaDirName+"/")
	if err := os.Remove(filepath.Join(dir, mediaDirName, filepath.FromSlash(rel))); err != nil {
		t.Fatalf("remove cover: %v", err)
	}

	again, err := svc.GetView(game.ID)
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if again.Game.Summary == "" {
		t.Fatal("text metadata lost together with the file")
	}
}

func TestGetArtReturnsLocalURLs(t *testing.T) {
	srv := imageServer(t)
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": fullMetadata(srv.URL)}}
	svc, cat, _ := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{Title: "Prey"})
	other := addGame(t, cat, catalog.Game{Title: "Dishonored"})

	view, err := svc.ApplyMatch(game.ID, "2657")
	if err != nil {
		t.Fatalf("apply match: %v", err)
	}

	art, err := svc.GetArt([]string{game.ID, other.ID, "missing", ""})
	if err != nil {
		t.Fatalf("art: %v", err)
	}
	if len(art) != 1 {
		t.Fatalf("art = %v, want a single entry", art)
	}
	if !strings.HasPrefix(art[game.ID].Cover, "/"+mediaDirName+"/") {
		t.Fatalf("cover url %q is not local", art[game.ID].Cover)
	}
	if art[game.ID].Hero != view.Hero || view.Hero == "" {
		t.Fatalf("hero = %q, want view hero %q", art[game.ID].Hero, view.Hero)
	}
}

func TestGetArtKeepsHeroWithoutCover(t *testing.T) {
	srv := imageServer(t)
	meta := fullMetadata(srv.URL)
	meta.Cover = nil
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": meta}}
	svc, cat, _ := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{Title: "Prey"})

	if _, err := svc.ApplyMatch(game.ID, "2657"); err != nil {
		t.Fatalf("apply match: %v", err)
	}

	art, err := svc.GetArt([]string{game.ID})
	if err != nil {
		t.Fatalf("art: %v", err)
	}
	if art[game.ID].Cover != "" {
		t.Fatalf("cover = %q, want empty", art[game.ID].Cover)
	}
	if !strings.HasPrefix(art[game.ID].Hero, "/"+mediaDirName+"/") {
		t.Fatalf("hero url %q is not local", art[game.ID].Hero)
	}
}

func TestGetArtSkipsGameWithoutAssets(t *testing.T) {
	srv := imageServer(t)
	meta := fullMetadata(srv.URL)
	meta.Cover = nil
	meta.Screenshots = nil
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": meta}}
	svc, cat, _ := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{Title: "Prey"})

	if _, err := svc.ApplyMatch(game.ID, "2657"); err != nil {
		t.Fatalf("apply match: %v", err)
	}

	art, err := svc.GetArt([]string{game.ID})
	if err != nil {
		t.Fatalf("art: %v", err)
	}
	if len(art) != 0 {
		t.Fatalf("art = %v, want no entries", art)
	}
}

func TestProviderNotConfigured(t *testing.T) {
	svc, cat, _ := newTestService(t, nil)
	game := addGame(t, cat, catalog.Game{Title: "Prey"})

	if svc.Available() {
		t.Fatal("service reports an available provider")
	}
	if _, err := svc.Refresh(game.ID); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
	if _, err := svc.FindCandidates(game.ID); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
	view, err := svc.GetView(game.ID)
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if view.Game.Title != "Prey" {
		t.Fatal("game is not displayable without a provider")
	}
}

func TestRefreshRejectsUnknownGame(t *testing.T) {
	svc, _, _ := newTestService(t, &fakeProvider{})
	if _, err := svc.Refresh("нет-такой-игры"); err == nil {
		t.Fatal("refresh accepted an unknown game")
	}
	if _, err := svc.Refresh(""); !errors.Is(err, errNoGameID) {
		t.Fatalf("err = %v, want errNoGameID", err)
	}
}

func TestServeMediaRejectsTraversal(t *testing.T) {
	srv := imageServer(t)
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": fullMetadata(srv.URL)}}
	svc, cat, _ := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{Title: "Prey"})
	view, err := svc.ApplyMatch(game.ID, "2657")
	if err != nil {
		t.Fatalf("apply match: %v", err)
	}

	handler := svc.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))

	cases := []struct {
		name string
		path string
		want int
	}{
		{"asset", view.Cover, http.StatusOK},
		{"traversal", "/media/../catalog.json", http.StatusNotFound},
		{"absolute", "/media//etc/passwd", http.StatusNotFound},
		{"unknown", "/media/games/nope/nope.jpg", http.StatusNotFound},
		{"passthrough", "/index.html", http.StatusTeapot},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}

func TestConcurrentRefreshIsSerialised(t *testing.T) {
	srv := imageServer(t)
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": fullMetadata(srv.URL)}}
	svc, cat, _ := newTestService(t, provider)
	old := time.Now().Add(-defaultTTL - time.Hour)
	game := addGame(t, cat, catalog.Game{
		Title:             "Prey",
		ExternalIDs:       catalog.ExternalIDs{IGDB: "2657"},
		MetadataUpdatedAt: &old,
	})

	const workers = 8
	start := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	var succeeded, busy int

	for i := range workers {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			<-start
			switch n % 3 {
			case 0:
				_, err := svc.Refresh(game.ID)
				mu.Lock()
				switch {
				case err == nil:
					succeeded++
				case errors.Is(err, errBusy):
					busy++
				default:
					t.Errorf("refresh: %v", err)
				}
				mu.Unlock()
			case 1:
				if _, err := svc.EnsureFresh(game.ID); err != nil {
					t.Errorf("ensure fresh: %v", err)
				}
			default:
				if _, err := svc.GetView(game.ID); err != nil {
					t.Errorf("view: %v", err)
				}
			}
		}(i)
	}
	close(start)
	wg.Wait()
	svc.wg.Wait()

	if succeeded+busy == 0 {
		t.Fatal("no refresh attempt was accounted for")
	}
	stored, err := cat.GetGame(game.ID)
	if err != nil {
		t.Fatalf("reload game: %v", err)
	}
	if stored.MetadataUpdatedAt == nil || !stored.MetadataUpdatedAt.After(old) {
		t.Fatalf("metadata not refreshed: %v", stored.MetadataUpdatedAt)
	}
	if len(svc.store.list(game.ID)) != maxScreenshots+1 {
		t.Fatalf("asset store diverged: %d entries", len(svc.store.list(game.ID)))
	}
}

func TestEnsureFreshAutoMatchesUnresolvedGame(t *testing.T) {
	srv := imageServer(t)
	provider := &fakeProvider{
		meta: map[string]GameMetadata{"2657": fullMetadata(srv.URL)},
		candidates: []Candidate{
			{ProviderID: "2657", Title: "Workers & Resources: Soviet Republic", ReleaseYear: 2024},
			{ProviderID: "9", Title: "Workers & Resources: Soviet Republic - Biomes", ReleaseYear: 2024},
		},
	}
	svc, cat, _ := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{Title: "Workers & Resources Soviet Republic", Provisional: true})

	started, err := svc.EnsureFresh(game.ID)
	if err != nil {
		t.Fatalf("ensure fresh: %v", err)
	}
	if !started {
		t.Fatal("unresolved game was not matched in the background")
	}
	svc.wg.Wait()

	stored, err := cat.GetGame(game.ID)
	if err != nil {
		t.Fatalf("reload game: %v", err)
	}
	if stored.ExternalIDs.IGDB != "2657" {
		t.Fatalf("igdb id = %q, want the confident candidate", stored.ExternalIDs.IGDB)
	}
	if stored.CoverAssetID == "" {
		t.Fatal("cover not fetched by the background match")
	}
}

func TestEnsureFreshLeavesAmbiguousGameAloneAndThrottles(t *testing.T) {
	srv := imageServer(t)
	provider := &fakeProvider{
		meta: map[string]GameMetadata{"2657": fullMetadata(srv.URL)},
		candidates: []Candidate{
			{ProviderID: "2657", Title: "Prey", ReleaseYear: 2017},
			{ProviderID: "7", Title: "Prey", ReleaseYear: 2006},
		},
	}
	svc, cat, _ := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{Title: "Prey", Provisional: true})

	started, err := svc.EnsureFresh(game.ID)
	if err != nil {
		t.Fatalf("ensure fresh: %v", err)
	}
	if !started {
		t.Fatal("no match attempt was made")
	}
	svc.wg.Wait()

	stored, err := cat.GetGame(game.ID)
	if err != nil {
		t.Fatalf("reload game: %v", err)
	}
	if stored.ExternalIDs.IGDB != "" {
		t.Fatalf("ambiguous game got id %q", stored.ExternalIDs.IGDB)
	}

	searches, _ := provider.counts()
	started, err = svc.EnsureFresh(game.ID)
	if err != nil {
		t.Fatalf("second ensure fresh: %v", err)
	}
	if started {
		t.Fatal("second open retried the search instead of waiting out the retry window")
	}
	svc.wg.Wait()
	if again, _ := provider.counts(); again != searches {
		t.Fatalf("searches = %d, want no extra provider call", again)
	}
}

func TestEnsureFreshThrottlesFailedRefreshOfMatchedGame(t *testing.T) {
	provider := &fakeProvider{getErr: errors.New("сервер прилёг")}
	svc, cat, _ := newTestService(t, provider)
	old := time.Now().Add(-defaultTTL - time.Hour)
	game := addGame(t, cat, catalog.Game{
		Title:             "Prey",
		ExternalIDs:       catalog.ExternalIDs{IGDB: "2657"},
		MetadataUpdatedAt: &old,
	})

	started, err := svc.EnsureFresh(game.ID)
	if err != nil {
		t.Fatalf("ensure fresh: %v", err)
	}
	if !started {
		t.Fatal("stale metadata was not refreshed")
	}
	svc.wg.Wait()

	_, gets := provider.counts()
	if gets != 1 {
		t.Fatalf("gets = %d, want 1", gets)
	}

	started, err = svc.EnsureFresh(game.ID)
	if err != nil {
		t.Fatalf("second ensure fresh: %v", err)
	}
	if started {
		t.Fatal("second open retried a failed refresh instead of waiting out the retry window")
	}
	svc.wg.Wait()
	if _, again := provider.counts(); again != gets {
		t.Fatalf("gets = %d, want no extra provider call", again)
	}
}

func TestRateLimitPausesEveryProviderCall(t *testing.T) {
	provider := &fakeProvider{getErr: &RateLimitError{RetryAfter: 30 * time.Second}}
	svc, cat, _ := newTestService(t, provider)
	old := time.Now().Add(-defaultTTL - time.Hour)
	game := addGame(t, cat, catalog.Game{
		Title:             "Prey",
		ExternalIDs:       catalog.ExternalIDs{IGDB: "2657"},
		MetadataUpdatedAt: &old,
	})
	other := addGame(t, cat, catalog.Game{Title: "Dishonored", Provisional: true})

	if _, err := svc.EnsureFresh(game.ID); err != nil {
		t.Fatalf("ensure fresh: %v", err)
	}
	svc.wg.Wait()

	if _, err := svc.SearchCandidates("prey"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("search err = %v, want ErrRateLimited", err)
	}
	if _, err := svc.Refresh(game.ID); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("refresh err = %v, want ErrRateLimited", err)
	}
	accepted, err := svc.EnsureArt([]string{other.ID})
	if err != nil {
		t.Fatalf("ensure art during cooldown: %v", err)
	}
	if len(accepted) != 1 {
		t.Fatalf("accepted = %v, want the game taken and postponed", accepted)
	}
	svc.wg.Wait()

	if searches, gets := provider.counts(); searches != 0 || gets != 1 {
		t.Fatalf("provider calls = %d searches / %d gets, want 0 / 1", searches, gets)
	}

	_, _, _, pause := svc.budget.stats()
	if pause <= 0 || pause > 30*time.Second {
		t.Fatalf("cooldown = %v, want the retry-after hint", pause)
	}
}

// Длинная пауза обязана дойти до пользователя числом, а не пустым результатом:
// именно на этом ломался поиск метаданных из карточки игры.
func TestRateLimitTellsTheUserHowLongToWaitInsteadOfBlocking(t *testing.T) {
	provider := &fakeProvider{}
	svc, cat, _ := newTestService(t, provider)
	addGame(t, cat, catalog.Game{Title: "Prey"})
	svc.budget.penalize(time.Minute)

	done := make(chan error, 1)
	go func() { _, err := svc.SearchCandidates("prey"); done <- err }()

	select {
	case err := <-done:
		var limit *RateLimitError
		if !errors.As(err, &limit) {
			t.Fatalf("search err = %v, want a RateLimitError", err)
		}
		if limit.RetryAfter <= 0 {
			t.Fatalf("retry after = %v, want the remaining pause", limit.RetryAfter)
		}
	case <-time.After(userMaxWait):
		t.Fatal("user search sat through the whole cooldown instead of reporting it")
	}
	if searches, _ := provider.counts(); searches != 0 {
		t.Fatalf("searches = %d, want the paused budget to hold the request back", searches)
	}
}

func TestRateLimitWithoutHintPausesForOneTokenInterval(t *testing.T) {
	provider := &fakeProvider{searchErr: fmt.Errorf("%w: 429", ErrRateLimited)}
	svc, cat, _ := newTestService(t, provider)
	addGame(t, cat, catalog.Game{Title: "Prey"})
	resetBudget(svc)

	if _, err := svc.SearchCandidates("prey"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("search err = %v, want ErrRateLimited", err)
	}

	rate, _, _, pause := svc.budget.stats()
	if pause <= 0 || pause > userMaxWait {
		t.Fatalf("cooldown = %v, want one token interval at %v rps", pause, rate)
	}
}

type blockingProvider struct {
	mu      sync.Mutex
	current int
	peak    int
	entered chan struct{}
	proceed chan struct{}
}

func (p *blockingProvider) Name() string { return "blocking" }

func (p *blockingProvider) Search(_ context.Context, _ string, _ int) ([]Candidate, error) {
	p.mu.Lock()
	p.current++
	if p.current > p.peak {
		p.peak = p.current
	}
	p.mu.Unlock()

	p.entered <- struct{}{}
	<-p.proceed

	p.mu.Lock()
	p.current--
	p.mu.Unlock()
	return nil, ErrNoMatch
}

func (p *blockingProvider) Get(_ context.Context, _ string) (GameMetadata, error) {
	return GameMetadata{}, ErrNoMatch
}

func (p *blockingProvider) highWater() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.peak
}

func TestProviderCallsAreCapped(t *testing.T) {
	const workers = 8
	provider := &blockingProvider{
		entered: make(chan struct{}, workers),
		proceed: make(chan struct{}),
	}
	svc, _, _ := newTestService(t, provider)

	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			if _, err := svc.SearchCandidates(fmt.Sprintf("query-%d", n)); err == nil {
				t.Errorf("search %d: want an error from the stub provider", n)
			}
		}(i)
	}

	for range userSlots {
		<-provider.entered
	}
	close(provider.proceed)
	wg.Wait()

	if peak := provider.highWater(); peak > userSlots {
		t.Fatalf("concurrent provider calls = %d, want at most %d", peak, userSlots)
	}
}

func TestSlotsAreCapped(t *testing.T) {
	svc, _, _ := newTestService(t, &fakeProvider{})

	for i := range userSlots {
		if err := svc.budget.acquire(context.Background(), classUser); err != nil {
			t.Fatalf("slot %d: %v", i, err)
		}
	}

	full, cancel := context.WithCancel(context.Background())
	cancel()
	if err := svc.budget.acquire(full, classUser); err == nil {
		svc.budget.release(classUser)
		t.Fatalf("slot handed out beyond %d in flight", userSlots)
	}

	svc.budget.release(classUser)
	if err := svc.budget.acquire(context.Background(), classUser); err != nil {
		t.Fatalf("freed slot not reused: %v", err)
	}
	for range userSlots {
		svc.budget.release(classUser)
	}
}

// Фон не имеет права занимать слоты пользователя: карточка игры открывается,
// даже когда каталог целиком дозагружает обложки.
func TestBackgroundNeverTakesTheUserSlots(t *testing.T) {
	svc, _, _ := newTestService(t, &fakeProvider{})

	for i := range backSlots {
		if err := svc.budget.acquire(context.Background(), classBackground); err != nil {
			t.Fatalf("background slot %d: %v", i, err)
		}
	}
	for i := range userSlots {
		if err := svc.budget.acquire(context.Background(), classUser); err != nil {
			t.Fatalf("user slot %d starved by background work: %v", i, err)
		}
	}
	for range userSlots {
		svc.budget.release(classUser)
	}
	for range backSlots {
		svc.budget.release(classBackground)
	}
}

func TestCoverSourceURLReturnsRemoteURL(t *testing.T) {
	srv := imageServer(t)
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": fullMetadata(srv.URL)}}
	svc, cat, _ := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{Title: "Prey"})

	if _, err := svc.ApplyMatch(game.ID, "2657"); err != nil {
		t.Fatalf("apply match: %v", err)
	}

	url, err := svc.CoverSourceURL(game.ID)
	if err != nil {
		t.Fatalf("cover source: %v", err)
	}
	if !strings.HasPrefix(url, srv.URL) {
		t.Fatalf("cover source = %q, want remote url under %q", url, srv.URL)
	}
}

func TestCoverSourceURLWithoutAssets(t *testing.T) {
	svc, cat, _ := newTestService(t, &fakeProvider{})
	game := addGame(t, cat, catalog.Game{Title: "Prey"})

	url, err := svc.CoverSourceURL(game.ID)
	if err != nil {
		t.Fatalf("cover source: %v", err)
	}
	if url != "" {
		t.Fatalf("cover source = %q, want empty", url)
	}
}

func TestCoverSourceURLRejectsBadID(t *testing.T) {
	svc, _, _ := newTestService(t, &fakeProvider{})

	if _, err := svc.CoverSourceURL("  "); !errors.Is(err, errNoGameID) {
		t.Fatalf("empty id: %v", err)
	}
	if _, err := svc.CoverSourceURL("missing"); err == nil {
		t.Fatal("unknown game must be an error, not an empty cover")
	}
}

func TestGetViewReportsSearchingWhileTheLookupRuns(t *testing.T) {
	provider := &blockingProvider{entered: make(chan struct{}, 1), proceed: make(chan struct{})}
	svc, cat, _ := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{Title: "Prey", Provisional: true})

	if view, err := svc.GetView(game.ID); err != nil {
		t.Fatalf("view before the lookup: %v", err)
	} else if view.Match != MatchIdle {
		t.Fatalf("match = %q, want %q before anything started", view.Match, MatchIdle)
	}

	started, err := svc.EnsureFresh(game.ID)
	if err != nil {
		t.Fatalf("ensure fresh: %v", err)
	}
	if !started {
		t.Fatal("lookup was not started")
	}
	<-provider.entered

	view, err := svc.GetView(game.ID)
	if err != nil {
		t.Fatalf("view during the lookup: %v", err)
	}
	if view.Match != MatchSearching {
		t.Fatalf("match = %q, want %q while the provider is being asked", view.Match, MatchSearching)
	}

	close(provider.proceed)
	svc.wg.Wait()

	view, err = svc.GetView(game.ID)
	if err != nil {
		t.Fatalf("view after the lookup: %v", err)
	}
	if view.Match != MatchUnmatched {
		t.Fatalf("match = %q, want %q once the search came back empty", view.Match, MatchUnmatched)
	}
}

func TestViewReportsWhyTheCardIsEmpty(t *testing.T) {
	cases := []struct {
		name     string
		provider *fakeProvider
		want     MatchState
	}{
		{
			name: "кандидаты неоднозначны",
			provider: &fakeProvider{candidates: []Candidate{
				{ProviderID: "2657", Title: "Prey", ReleaseYear: 2017},
				{ProviderID: "7", Title: "Prey", ReleaseYear: 2006},
			}},
			want: MatchUnmatched,
		},
		{
			name:     "провайдер не ответил",
			provider: &fakeProvider{searchErr: errors.New("сервер прилёг")},
			want:     MatchFailed,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, cat, _ := newTestService(t, tc.provider)
			game := addGame(t, cat, catalog.Game{Title: "Prey", Provisional: true})

			if _, err := svc.EnsureFresh(game.ID); err != nil {
				t.Fatalf("ensure fresh: %v", err)
			}
			svc.wg.Wait()

			view, err := svc.GetView(game.ID)
			if err != nil {
				t.Fatalf("view: %v", err)
			}
			if view.Match != tc.want {
				t.Fatalf("match = %q, want %q", view.Match, tc.want)
			}
			if view.Resolved {
				t.Fatal("an unmatched game reported itself as resolved")
			}
		})
	}
}

func TestResolvedGameReportsIdle(t *testing.T) {
	srv := imageServer(t)
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": fullMetadata(srv.URL)}}
	svc, cat, _ := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{Title: "Prey", Provisional: true})

	if _, err := svc.ApplyMatch(game.ID, "2657"); err != nil {
		t.Fatalf("apply match: %v", err)
	}
	view, err := svc.GetView(game.ID)
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if view.Match != MatchIdle {
		t.Fatalf("match = %q, want %q for a matched game", view.Match, MatchIdle)
	}
}

func TestDismissMatchStopsAskingAndSurvivesARestart(t *testing.T) {
	provider := &fakeProvider{searchErr: errors.New("сервер прилёг")}
	svc, cat, dir := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{Title: "Prey", Provisional: true})

	view, err := svc.DismissMatch(game.ID)
	if err != nil {
		t.Fatalf("dismiss match: %v", err)
	}
	if view.Match != MatchSkipped {
		t.Fatalf("match = %q, want %q", view.Match, MatchSkipped)
	}

	searches, _ := provider.counts()
	started, err := svc.EnsureFresh(game.ID)
	if err != nil {
		t.Fatalf("ensure fresh: %v", err)
	}
	if started {
		t.Fatal("a dismissed game was searched for again")
	}
	svc.wg.Wait()
	if again, _ := provider.counts(); again != searches {
		t.Fatalf("searches = %d, want no extra provider call", again)
	}

	stored, err := newAttemptStore(dir, time.Now)
	if err != nil {
		t.Fatalf("reopen attempts: %v", err)
	}
	rec, ok := stored.state(game.ID)
	if !ok || !rec.Dismissed {
		t.Fatalf("record = %+v present = %v, want the dismissal on disk", rec, ok)
	}
}

func TestDismissMatchRejectsAnUnknownGame(t *testing.T) {
	cases := []struct {
		name   string
		gameID string
	}{
		{"пустой идентификатор", "   "},
		{"игры нет в каталоге", "нет-такой-игры"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, _ := newTestService(t, &fakeProvider{})
			if _, err := svc.DismissMatch(tc.gameID); err == nil {
				t.Fatal("dismiss reported success for a game it cannot know")
			}
		})
	}
}

func TestManualMatchClearsTheDismissal(t *testing.T) {
	srv := imageServer(t)
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": fullMetadata(srv.URL)}}
	svc, cat, _ := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{Title: "Prey", Provisional: true})

	if _, err := svc.DismissMatch(game.ID); err != nil {
		t.Fatalf("dismiss match: %v", err)
	}
	view, err := svc.ApplyMatch(game.ID, "2657")
	if err != nil {
		t.Fatalf("apply match: %v", err)
	}
	if view.Match != MatchIdle || !view.Resolved {
		t.Fatalf("view = %q resolved = %v, want a matched game", view.Match, view.Resolved)
	}
	if rec, ok := svc.attempts.state(game.ID); ok {
		t.Fatalf("record = %+v, want the dismissal gone after a manual match", rec)
	}
}
