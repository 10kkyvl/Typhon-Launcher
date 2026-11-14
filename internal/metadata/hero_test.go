package metadata

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"typhon/internal/catalog"
	"typhon/internal/storage"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const heroPoster = "poster"

type heroCase struct {
	name     string
	noCover  bool
	shots    []string
	artworks []string
	// want is the source path of the hero, heroPoster for the cover, "" for none.
	want string
}

var heroCases = []heroCase{
	{name: "wide screenshot wins over artwork", shots: []string{"/1280x720-s0.png"}, artworks: []string{"/1920x1080-a0.png"}, want: "/1280x720-s0.png"},
	{name: "screenshot narrower than the band is skipped", shots: []string{"/1000x600-s0.png", "/1920x1080-s1.png"}, want: "/1920x1080-s1.png"},
	{name: "panorama screenshot is skipped", shots: []string{"/2800x1000-s0.png", "/1280x720-s1.png"}, want: "/1280x720-s1.png"},
	{name: "screenshot below the ratio is skipped", shots: []string{"/1280x1024-s0.png"}, artworks: []string{"/1280x720-a0.png"}, want: "/1280x720-a0.png"},
	{name: "ratio bounds are inclusive", shots: []string{"/1500x1000-s0.png", "/2600x1000-s1.png"}, want: "/1500x1000-s0.png"},
	{name: "upper ratio bound is inclusive", shots: []string{"/600x900-s0.png", "/2600x1000-s1.png"}, want: "/2600x1000-s1.png"},
	{name: "first fitting artwork when no screenshot fits", shots: []string{"/600x900-s0.png"}, artworks: []string{"/2800x1000-a0.png", "/1920x1080-a1.png"}, want: "/1920x1080-a1.png"},
	{name: "poster when no wide picture", shots: []string{"/1000x600-s0.png"}, artworks: []string{"/600x900-a0.png"}, want: heroPoster},
	{name: "poster without screenshots and artworks", want: heroPoster},
	{name: "nothing without a cover", noCover: true, shots: []string{"/600x900-s0.png"}, want: ""},
	{name: "several candidates: the first in response order", shots: []string{"/1920x1080-s0.png", "/1280x720-s1.png"}, artworks: []string{"/1280x720-a0.png"}, want: "/1920x1080-s0.png"},
}

func applyHeroCase(t *testing.T, tc heroCase) (*Service, *catalog.Service, catalog.Game, string) {
	t.Helper()
	srv, _ := sizedImageServer(t)
	meta := fullMetadata(srv.URL)
	meta.Cover = &ImageRef{URL: srv.URL + "/264x374-cover.png"}
	if tc.noCover {
		meta.Cover = nil
	}
	meta.Screenshots = refs(srv.URL, tc.shots...)
	meta.Artworks = refs(srv.URL, tc.artworks...)
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
	return svc, cat, stored, srv.URL
}

// sourceOf names the stored picture a local URL points at.
func sourceOf(t *testing.T, svc *Service, gameID, url string) string {
	t.Helper()
	for _, a := range svc.store.list(gameID) {
		if a.URL == url {
			return a.SourceURL
		}
	}
	t.Fatalf("no stored asset behind %q", url)
	return ""
}

func TestHeroFollowsTheChainInResponseOrder(t *testing.T) {
	for _, tc := range heroCases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, game, base := applyHeroCase(t, tc)
			view, err := svc.GetView(game.ID)
			if err != nil {
				t.Fatalf("view: %v", err)
			}
			switch tc.want {
			case "":
				if view.Hero != "" || view.HeroIsPoster {
					t.Fatalf("hero = %q poster=%v, want none", view.Hero, view.HeroIsPoster)
				}
			case heroPoster:
				if view.Hero == "" || view.Hero != view.Cover || !view.HeroIsPoster {
					t.Fatalf("hero = %q poster=%v, want the cover %q flagged as a poster", view.Hero, view.HeroIsPoster, view.Cover)
				}
			default:
				if view.Hero == "" || view.HeroIsPoster {
					t.Fatalf("hero = %q poster=%v, want %s", view.Hero, view.HeroIsPoster, tc.want)
				}
				if got := sourceOf(t, svc, game.ID, view.Hero); got != base+tc.want {
					t.Fatalf("hero is %s, want %s", got, base+tc.want)
				}
			}
		})
	}
}

func TestStoredHeroFromTheOldRuleSurvivesUntilAFullRefresh(t *testing.T) {
	srv, hits := sizedImageServer(t)
	dir := t.TempDir()
	cat, err := catalog.NewServiceAt(dir)
	if err != nil {
		t.Fatalf("catalog service: %v", err)
	}
	game := addGame(t, cat, catalog.Game{Title: "Prey"})

	// The state the launcher wrote before artworks: a cover and a screenshot
	// the old rule took as the hero (landscape, but narrower than the band).
	root := filepath.Join(dir, mediaDirName)
	stored := func(id string, kind AssetType, w, h int) MediaAsset {
		rel, err := writeAsset(root, game.ID, id, fetchedImage{Data: pngBytes(t, w, h), Format: "png", Width: w, Height: h})
		if err != nil {
			t.Fatalf("write asset: %v", err)
		}
		return MediaAsset{ID: id, GameID: game.ID, Type: kind, SourceURL: srv.URL + "/old.png", Path: rel, Width: w, Height: h, CreatedAt: time.Now()}
	}
	cover := stored("oldcover", AssetCover, 264, 374)
	shot := stored("oldshot", AssetScreenshot, 1000, 600)
	if err := storage.Save(filepath.Join(dir, assetsFileName), assetsVersion, []MediaAsset{cover, shot}); err != nil {
		t.Fatalf("save assets: %v", err)
	}
	if _, err := cat.ApplyMetadata(game.ID, catalog.MetadataPatch{
		Language: "en", IGDBID: "2657", Title: "Prey", CoverAssetID: cover.ID, HeroAssetID: shot.ID, UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("apply metadata: %v", err)
	}
	before := stateFiles(t, dir)

	meta := fullMetadata(srv.URL)
	meta.Cover = &ImageRef{URL: srv.URL + "/264x374-cover.png"}
	meta.Screenshots = refs(srv.URL, "/1000x600-s0.png")
	meta.Artworks = refs(srv.URL, "/1280x720-a0.png")
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": meta}}
	svc, err := NewServiceAt(dir, cat, provider)
	if err != nil {
		t.Fatalf("metadata service on the old state: %v", err)
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

	assertOldHero := func(stage string) {
		t.Helper()
		view, err := svc.GetView(game.ID)
		if err != nil {
			t.Fatalf("%s: view: %v", stage, err)
		}
		if view.Hero != "/"+path.Join(mediaDirName, shot.Path) || view.HeroIsPoster {
			t.Fatalf("%s: hero = %q poster=%v, want the stored screenshot", stage, view.Hero, view.HeroIsPoster)
		}
		art, err := svc.GetArt([]string{game.ID})
		if err != nil {
			t.Fatalf("%s: art: %v", stage, err)
		}
		if art[game.ID].Hero != view.Hero || art[game.ID].HeroIsPoster {
			t.Fatalf("%s: GetArt = %+v, want the view's hero", stage, art[game.ID])
		}
		files, err := svc.ArtFiles(game.ID)
		if err != nil {
			t.Fatalf("%s: art files: %v", stage, err)
		}
		if files.Hero != filepath.Join(root, filepath.FromSlash(shot.Path)) {
			t.Fatalf("%s: hero file = %q, want the stored screenshot", stage, files.Hero)
		}
	}
	loaded := stateFiles(t, dir)
	for _, name := range []string{assetsFileName, "catalog.json"} {
		if loaded[name] == "" || loaded[name] != before[name] {
			t.Fatalf("loading the old state changed %s", name)
		}
	}
	assertOldHero("old state")
	if changed := changedFiles(loaded, stateFiles(t, dir)); len(changed) > 0 {
		t.Fatalf("reading the old state rewrote %v", changed)
	}

	// The library's art refresh keeps a stored choice and downloads only the
	// cover, even though the answer lists a fitting artwork.
	images := hits.Load()
	svc.SetLanguage("ru")
	if _, err := svc.EnsureArt([]string{game.ID}); err != nil {
		t.Fatalf("ensure art: %v", err)
	}
	svc.wg.Wait()
	if updated, err := cat.GetGame(game.ID); err != nil || updated.MetadataLanguage != "ru" {
		t.Fatalf("art refresh did not run: %+v, %v", updated, err)
	}
	assertOldHero("after the art refresh")
	if got := hits.Load() - images; got != 1 {
		t.Fatalf("art refresh made %d image requests, want only the cover", got)
	}

	view, err := svc.Refresh(game.ID)
	if err != nil {
		t.Fatalf("full refresh: %v", err)
	}
	if view.HeroIsPoster || sourceOf(t, svc, game.ID, view.Hero) != srv.URL+"/1280x720-a0.png" {
		t.Fatalf("hero after a full refresh = %q poster=%v, want the artwork", view.Hero, view.HeroIsPoster)
	}
	updated, err := cat.GetGame(game.ID)
	if err != nil {
		t.Fatalf("reload game: %v", err)
	}
	if hero, ok := svc.store.find(updated.HeroAssetID); !ok || hero.URL != view.Hero {
		t.Fatalf("persisted hero %q does not match the view %q", updated.HeroAssetID, view.Hero)
	}
}

func TestHeroChoiceIsOffline(t *testing.T) {
	for _, tc := range []heroCase{heroCaseNamed(t, "wide screenshot wins over artwork"), heroCaseNamed(t, "poster when no wide picture")} {
		t.Run(tc.name, func(t *testing.T) {
			srv, hits := sizedImageServer(t)
			meta := fullMetadata(srv.URL)
			meta.Cover = &ImageRef{URL: srv.URL + "/264x374-cover.png"}
			meta.Screenshots = refs(srv.URL, tc.shots...)
			meta.Artworks = refs(srv.URL, tc.artworks...)
			provider := &fakeProvider{meta: map[string]GameMetadata{"2657": meta}}
			svc, cat, _ := newTestService(t, provider)
			game := addGame(t, cat, catalog.Game{Title: "Prey"})
			if _, err := svc.ApplyMatch(game.ID, "2657"); err != nil {
				t.Fatalf("apply match: %v", err)
			}
			images := hits.Load()
			searches, gets := provider.counts()
			srv.Close()

			files, err := svc.ArtFiles(game.ID)
			if err != nil {
				t.Fatalf("art files: %v", err)
			}
			chosen := files.Hero
			if tc.want == heroPoster {
				chosen = files.Cover
			}
			if err := os.Remove(chosen); err != nil {
				t.Fatalf("remove the chosen picture: %v", err)
			}

			view, err := svc.GetView(game.ID)
			if err != nil {
				t.Fatalf("view: %v", err)
			}
			art, err := svc.GetArt([]string{game.ID})
			if err != nil {
				t.Fatalf("art: %v", err)
			}
			files, err = svc.ArtFiles(game.ID)
			if err != nil {
				t.Fatalf("art files without the picture: %v", err)
			}
			wantPoster := tc.want == heroPoster
			if view.Hero == "" || view.HeroIsPoster != wantPoster || art[game.ID].Hero != view.Hero || art[game.ID].HeroIsPoster != wantPoster {
				t.Fatalf("view hero %q poster=%v, GetArt %+v; want the stored choice with poster=%v", view.Hero, view.HeroIsPoster, art[game.ID], wantPoster)
			}
			if files.Hero != "" {
				t.Fatalf("hero file = %q, want empty with the picture gone", files.Hero)
			}
			if got := hits.Load(); got != images {
				t.Fatalf("image requests while choosing: %d, want %d", got, images)
			}
			if s, g := provider.counts(); s != searches || g != gets {
				t.Fatalf("provider calls while choosing: %d/%d, want %d/%d", s, g, searches, gets)
			}
		})
	}
}

// stateFiles maps every state file in dir (the top level, where the services
// keep them) to its bytes.
func stateFiles(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("list state dir: %v", err)
	}
	out := map[string]string{}
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatalf("read state file: %v", err)
		}
		out[entry.Name()] = string(data)
	}
	return out
}

func changedFiles(before, after map[string]string) []string {
	var changed []string
	for name, data := range before {
		if other, ok := after[name]; !ok || other != data {
			changed = append(changed, name)
		}
	}
	for name := range after {
		if _, ok := before[name]; !ok {
			changed = append(changed, name)
		}
	}
	return changed
}

func heroCaseNamed(t *testing.T, name string) heroCase {
	t.Helper()
	for _, tc := range heroCases {
		if tc.name == name {
			return tc
		}
	}
	t.Fatalf("no hero case %q", name)
	return heroCase{}
}

// artRefresh runs the library's art refresh for a game the catalog knows by
// IGDB id and returns how many image requests it made beyond the cover.
func artRefresh(t *testing.T, svc *Service, cat *catalog.Service, gameID string, hits *atomic.Int64) (catalog.Game, int64) {
	t.Helper()
	images := hits.Load()
	accepted, err := svc.EnsureArt([]string{gameID})
	if err != nil || len(accepted) != 1 {
		t.Fatalf("ensure art = %v, %v", accepted, err)
	}
	svc.wg.Wait()
	updated, err := cat.GetGame(gameID)
	if err != nil {
		t.Fatalf("reload game: %v", err)
	}
	if updated.MetadataUpdatedAt == nil || updated.CoverAssetID == "" {
		t.Fatalf("art refresh did not apply: %+v", updated)
	}
	return updated, hits.Load() - images - 1
}

func TestArtRefreshFetchesOneHeroPictureWhenNoStoredOneFits(t *testing.T) {
	cases := []struct {
		name     string
		shots    []string
		artworks []string
		gone     bool
		extra    int64
		want     string
	}{
		{name: "a fitting artwork is listed", shots: []string{"/600x900-s0.png"}, artworks: []string{"/1920x1080-a0.png"}, extra: 1, want: "/1920x1080-a0.png"},
		{name: "nothing listed fits", shots: []string{"/1000x600-s0.png"}, artworks: []string{"/600x900-a0.png"}, extra: 0, want: heroPoster},
		{name: "several candidates: one download, screenshots first", shots: []string{"/1000x600-s0.png", "/1920x1080-s1.png", "/1280x720-s2.png"}, artworks: []string{"/1920x1080-a0.png", "/1280x720-a1.png"}, extra: 1, want: "/1920x1080-s1.png"},
		{name: "the download fails", gone: true, extra: 1, want: heroPoster},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, hits := sizedImageServer(t)
			meta := fullMetadata(srv.URL)
			meta.Cover = &ImageRef{URL: srv.URL + "/264x374-cover.png"}
			meta.Screenshots = refs(srv.URL, tc.shots...)
			meta.Artworks = refs(srv.URL, tc.artworks...)
			if tc.gone {
				meta.Artworks = []ImageRef{{URL: srv.URL + "/gone.png", Width: 1920, Height: 1080}}
			}
			provider := &fakeProvider{meta: map[string]GameMetadata{"2657": meta}}
			svc, cat, _ := newTestService(t, provider)
			game := addGame(t, cat, catalog.Game{Title: "Prey", ExternalIDs: catalog.ExternalIDs{IGDB: "2657"}})

			updated, extra := artRefresh(t, svc, cat, game.ID, hits)
			if extra != tc.extra {
				t.Fatalf("extra image requests = %d, want %d", extra, tc.extra)
			}
			if !updated.MetadataPartial {
				t.Fatal("art refresh cleared the partial flag; the page would skip the full refresh")
			}
			view, err := svc.GetView(game.ID)
			if err != nil {
				t.Fatalf("view: %v", err)
			}
			if tc.want == heroPoster {
				if !view.HeroIsPoster || view.Hero != view.Cover {
					t.Fatalf("hero = %q poster=%v, want the poster", view.Hero, view.HeroIsPoster)
				}
				return
			}
			if view.HeroIsPoster || sourceOf(t, svc, game.ID, view.Hero) != srv.URL+tc.want {
				t.Fatalf("hero = %q poster=%v, want %s", view.Hero, view.HeroIsPoster, tc.want)
			}
			if hero, ok := svc.store.find(updated.HeroAssetID); !ok || hero.URL != view.Hero {
				t.Fatalf("persisted hero %q does not match the view %q", updated.HeroAssetID, view.Hero)
			}
		})
	}
}

func TestArtRefreshFetchesNothingExtraWhenAStoredPictureFits(t *testing.T) {
	srv, hits := sizedImageServer(t)
	meta := fullMetadata(srv.URL)
	meta.Cover = &ImageRef{URL: srv.URL + "/264x374-cover.png"}
	meta.Screenshots = refs(srv.URL, "/1280x720-s0.png")
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": meta}}
	svc, cat, _ := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{Title: "Prey"})
	before, err := svc.ApplyMatch(game.ID, "2657")
	if err != nil {
		t.Fatalf("apply match: %v", err)
	}

	next := meta
	next.Screenshots = refs(srv.URL, "/1920x1080-s9.png")
	next.Artworks = refs(srv.URL, "/1920x1080-a9.png")
	provider.setMeta("2657", next)
	svc.SetLanguage("ru")
	if _, extra := artRefresh(t, svc, cat, game.ID, hits); extra != 0 {
		t.Fatalf("extra image requests = %d, want none while a stored picture fits", extra)
	}
	view, err := svc.GetView(game.ID)
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if view.Hero != before.Hero || view.HeroIsPoster {
		t.Fatalf("hero = %q poster=%v, want the stored %q", view.Hero, view.HeroIsPoster, before.Hero)
	}
}

func TestArtRefreshHeroScreenshotLeavesTheGalleryToTheFullRefresh(t *testing.T) {
	srv, hits := sizedImageServer(t)
	meta := fullMetadata(srv.URL)
	meta.Cover = &ImageRef{URL: srv.URL + "/264x374-cover.png"}
	meta.Screenshots = refs(srv.URL, "/1920x1080-s0.png", "/1920x1080-s1.png")
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": meta}}
	svc, cat, _ := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{Title: "Prey", ExternalIDs: catalog.ExternalIDs{IGDB: "2657"}})

	if _, extra := artRefresh(t, svc, cat, game.ID, hits); extra != 1 {
		t.Fatalf("first art refresh: extra image requests = %d, want 1", extra)
	}
	svc.SetLanguage("ru")
	updated, extra := artRefresh(t, svc, cat, game.ID, hits)
	if extra != 0 {
		t.Fatalf("second art refresh: extra image requests = %d, want none", extra)
	}
	if !updated.MetadataPartial {
		t.Fatal("the hero screenshot made the game look complete; its page would never fetch the gallery")
	}
	started, err := svc.EnsureFresh(game.ID)
	if err != nil || !started {
		t.Fatalf("ensure fresh = %v, %v; want the full refresh to start", started, err)
	}
	svc.wg.Wait()
	if got := len(svc.screenshots(game.ID)); got != 2 {
		t.Fatalf("gallery after the full refresh = %d screenshots, want 2", got)
	}
}

func TestArtRefreshDropsThePicturesOfAPreviousMatch(t *testing.T) {
	srv, _ := sizedImageServer(t)
	old := fullMetadata(srv.URL)
	old.Cover = &ImageRef{URL: srv.URL + "/264x374-oldcover.png"}
	old.Screenshots = refs(srv.URL, "/1280x720-olds0.png")
	old.Artworks = refs(srv.URL, "/1920x1080-olda0.png")
	other := fullMetadata(srv.URL)
	other.ProviderID = "7"
	other.Cover = &ImageRef{URL: srv.URL + "/264x374-newcover.png"}
	other.Screenshots = refs(srv.URL, "/1920x1080-news0.png", "/1920x1080-news1.png")
	other.Artworks = nil
	provider := &batchProvider{
		fakeProvider: &fakeProvider{meta: map[string]GameMetadata{"2657": old}},
		resolve:      map[string]GameMetadata{"Prey": other},
	}
	svc, cat, _ := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{Title: "Prey"})
	if _, err := svc.ApplyMatch(game.ID, "2657"); err != nil {
		t.Fatalf("first match: %v", err)
	}

	// The batch answers the same title with another game; the art refresh
	// takes it (sameGame accepts it by title).
	svc.SetLanguage("ru")
	if _, err := svc.EnsureArt([]string{game.ID}); err != nil {
		t.Fatalf("ensure art: %v", err)
	}
	svc.wg.Wait()
	updated, err := cat.GetGame(game.ID)
	if err != nil {
		t.Fatalf("reload game: %v", err)
	}
	if updated.ExternalIDs.IGDB != "7" {
		t.Fatalf("igdb id = %q, the batch answer was not applied", updated.ExternalIDs.IGDB)
	}
	for _, a := range svc.store.list(game.ID) {
		if strings.Contains(a.SourceURL, "-old") {
			t.Fatalf("stored %s %s from the previous match", a.Type, a.SourceURL)
		}
	}
	view, err := svc.GetView(game.ID)
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if view.HeroIsPoster || sourceOf(t, svc, game.ID, view.Hero) != srv.URL+"/1920x1080-news0.png" {
		t.Fatalf("hero = %q poster=%v, want the new game's first fitting screenshot", view.Hero, view.HeroIsPoster)
	}
	if hero, ok := svc.store.find(updated.HeroAssetID); !ok || hero.URL != view.Hero {
		t.Fatalf("persisted hero %q does not match the view %q", updated.HeroAssetID, view.Hero)
	}
	if !updated.MetadataPartial {
		t.Fatal("the game is not partial; its page would not fetch the new gallery")
	}
}
