package metadata

import (
	"errors"
	"os"
	"path"
	"path/filepath"
	"testing"

	"typhon/internal/catalog"
)

func matchedGame(t *testing.T) (*Service, catalog.Game, string) {
	t.Helper()
	srv := imageServer(t)
	provider := &fakeProvider{meta: map[string]GameMetadata{"2657": fullMetadata(srv.URL)}}
	svc, cat, dir := newTestService(t, provider)
	game := addGame(t, cat, catalog.Game{Title: "Prey", Provisional: true})
	if _, err := svc.ApplyMatch(game.ID, "2657"); err != nil {
		t.Fatalf("apply match: %v", err)
	}
	stored, err := cat.GetGame(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	return svc, stored, dir
}

func TestArtFilesPointsAtTheSameImagesAsGetArt(t *testing.T) {
	svc, game, dir := matchedGame(t)

	got, err := svc.ArtFiles(game.ID)
	if err != nil {
		t.Fatalf("ArtFiles: %v", err)
	}
	art, err := svc.GetArt([]string{game.ID})
	if err != nil {
		t.Fatal(err)
	}
	want := art[game.ID]
	if want.Cover == "" || want.Hero == "" {
		t.Fatalf("GetArt = %+v, fixture lost its images", want)
	}
	for name, pair := range map[string][2]string{"cover": {got.Cover, want.Cover}, "hero": {got.Hero, want.Hero}} {
		file, url := pair[0], pair[1]
		if !filepath.IsAbs(file) {
			t.Fatalf("%s path %q is not absolute", name, file)
		}
		if filepath.Base(file) != filepath.Base(url) {
			t.Fatalf("%s file %q differs from the url %q", name, file, url)
		}
		if _, err := os.Stat(file); err != nil {
			t.Fatalf("%s file: %v", name, err)
		}
		rel, err := filepath.Rel(filepath.Join(dir, mediaDirName), file)
		if err != nil || !filepath.IsLocal(rel) {
			t.Fatalf("%s file %q is outside the media root (%v)", name, file, err)
		}
	}

	for _, tc := range heroCases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, game, _ := applyHeroCase(t, tc)
			view, err := svc.GetView(game.ID)
			if err != nil {
				t.Fatalf("view: %v", err)
			}
			art, err := svc.GetArt([]string{game.ID})
			if err != nil {
				t.Fatalf("art: %v", err)
			}
			if got, want := art[game.ID], (Art{Cover: view.Cover, Hero: view.Hero, HeroIsPoster: view.HeroIsPoster}); got != want {
				t.Fatalf("GetArt = %+v, view says %+v", got, want)
			}
			files, err := svc.ArtFiles(game.ID)
			if err != nil {
				t.Fatalf("ArtFiles: %v", err)
			}
			if filepath.Base(files.Cover) != path.Base(view.Cover) {
				t.Fatalf("cover file %q, view cover %q", files.Cover, view.Cover)
			}
			switch {
			case tc.want == heroPoster:
				// The Steam grid keeps the cover in its portrait slot; a tall
				// picture is not a hero there.
				if !view.HeroIsPoster || view.Hero != view.Cover || files.Hero != "" {
					t.Fatalf("view hero %q poster=%v, hero file %q; want the cover as a poster and no hero file", view.Hero, view.HeroIsPoster, files.Hero)
				}
			case view.Hero == "":
				if files.Hero != "" {
					t.Fatalf("hero file = %q, want none", files.Hero)
				}
			default:
				if files.Hero == "" || filepath.Base(files.Hero) != path.Base(view.Hero) {
					t.Fatalf("hero file %q, view hero %q", files.Hero, view.Hero)
				}
			}
		})
	}
}

func TestArtFilesEdgeCases(t *testing.T) {
	t.Run("game outside the catalog", func(t *testing.T) {
		svc, _, _ := newTestService(t, &fakeProvider{})
		got, err := svc.ArtFiles("no-such-game")
		if err != nil || got != (LocalArt{}) {
			t.Fatalf("ArtFiles = %+v, %v", got, err)
		}
	})

	t.Run("empty id", func(t *testing.T) {
		svc, _, _ := newTestService(t, &fakeProvider{})
		if _, err := svc.ArtFiles("  "); !errors.Is(err, errNoGameID) {
			t.Fatalf("err = %v, want errNoGameID", err)
		}
	})

	t.Run("game without assets", func(t *testing.T) {
		svc, cat, _ := newTestService(t, &fakeProvider{})
		game := addGame(t, cat, catalog.Game{Title: "Bare"})
		got, err := svc.ArtFiles(game.ID)
		if err != nil || got != (LocalArt{}) {
			t.Fatalf("ArtFiles = %+v, %v", got, err)
		}
	})

	t.Run("file removed from disk", func(t *testing.T) {
		svc, game, _ := matchedGame(t)
		full, err := svc.ArtFiles(game.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(full.Cover); err != nil {
			t.Fatal(err)
		}
		got, err := svc.ArtFiles(game.ID)
		if err != nil {
			t.Fatalf("ArtFiles: %v", err)
		}
		if got.Cover != "" || got.Hero != full.Hero {
			t.Fatalf("ArtFiles = %+v, want only the hero left", got)
		}
	})

	t.Run("recorded path escapes the media root", func(t *testing.T) {
		svc, game, _ := matchedGame(t)
		svc.store.mu.Lock()
		for i := range svc.store.assets {
			if svc.store.assets[i].ID == game.CoverAssetID {
				svc.store.assets[i].Path = "../outside.png"
			}
		}
		svc.store.mu.Unlock()
		if _, err := svc.ArtFiles(game.ID); !errors.Is(err, errAssetPath) {
			t.Fatalf("err = %v, want errAssetPath", err)
		}
	})
}
