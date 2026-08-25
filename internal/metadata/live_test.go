package metadata_test

import (
	"context"
	"os"
	"testing"
	"time"

	"typhon/internal/catalog"
	"typhon/internal/metadata"
	"typhon/internal/metadata/typhonapi"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func liveService(t *testing.T) (*metadata.Service, *catalog.Service) {
	t.Helper()
	base := os.Getenv("TYPHON_METADATA_API")
	if base == "" {
		t.Skip("set TYPHON_METADATA_API to run against a live typhon-backend")
	}

	provider, err := typhonapi.New(base, func() (string, error) { return os.Getenv("TYPHON_METADATA_TOKEN"), nil })
	if err != nil {
		t.Fatalf("provider: %v", err)
	}

	dir := t.TempDir()
	cat, err := catalog.NewServiceAt(dir)
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	svc, err := metadata.NewServiceAt(dir, cat, provider)
	if err != nil {
		t.Fatalf("metadata service: %v", err)
	}
	if err := svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("startup: %v", err)
	}
	t.Cleanup(func() {
		if err := svc.ServiceShutdown(); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})
	return svc, cat
}

func waitResolved(t *testing.T, svc *metadata.Service, gameID string) metadata.View {
	t.Helper()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.After(90 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("background match did not finish in time")
		case <-ticker.C:
			view, err := svc.GetView(gameID)
			if err != nil {
				t.Fatalf("view: %v", err)
			}
			if view.Game.ExternalIDs.IGDB != "" {
				return view
			}
		}
	}
}

func TestLiveAutoMatchByYear(t *testing.T) {
	svc, cat := liveService(t)
	year := 2017
	game, err := cat.AddGame(catalog.Game{Title: "Prey", ReleaseYear: &year, Provisional: true})
	if err != nil {
		t.Fatalf("add game: %v", err)
	}

	view, err := svc.Refresh(game.ID)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if view.Game.ExternalIDs.IGDB == "" {
		t.Fatal("igdb id not stored")
	}
	if view.Game.Summary == "" {
		t.Fatal("summary not stored")
	}
	if view.Game.Developer == "" || view.Game.Publisher == "" {
		t.Fatalf("companies = %q / %q", view.Game.Developer, view.Game.Publisher)
	}
	if len(view.Game.Genres) == 0 || len(view.Game.Platforms) == 0 {
		t.Fatalf("lists = %+v", view.Game)
	}
	if view.Game.MetadataUpdatedAt == nil || time.Since(*view.Game.MetadataUpdatedAt) > time.Minute {
		t.Fatalf("timestamp = %v", view.Game.MetadataUpdatedAt)
	}
	t.Logf("matched igdb=%s cover=%q screenshots=%d", view.Game.ExternalIDs.IGDB, view.Cover, len(view.Screenshots))
}

func TestLiveAmbiguousStaysUnresolved(t *testing.T) {
	svc, cat := liveService(t)
	game, err := cat.AddGame(catalog.Game{Title: "Prey", Provisional: true})
	if err != nil {
		t.Fatalf("add game: %v", err)
	}

	if _, err := svc.Refresh(game.ID); err == nil {
		t.Fatal("refresh picked a match for an ambiguous title")
	}
	stored, err := cat.GetGame(game.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if stored.ExternalIDs.IGDB != "" {
		t.Fatalf("igdb id = %q, want empty", stored.ExternalIDs.IGDB)
	}
}

func TestLiveManualMatchThenRefreshUsesID(t *testing.T) {
	svc, cat := liveService(t)
	game, err := cat.AddGame(catalog.Game{Title: "какая-то игра", Provisional: true})
	if err != nil {
		t.Fatalf("add game: %v", err)
	}

	candidates, err := svc.SearchCandidates("Prey")
	if err != nil {
		t.Fatalf("search candidates: %v", err)
	}
	if len(candidates) == 0 {
		t.Fatal("no candidates returned")
	}

	view, err := svc.ApplyMatch(game.ID, candidates[0].ProviderID)
	if err != nil {
		t.Fatalf("apply match: %v", err)
	}
	if view.Game.ExternalIDs.IGDB != candidates[0].ProviderID {
		t.Fatalf("igdb id = %q, want %q", view.Game.ExternalIDs.IGDB, candidates[0].ProviderID)
	}
	if view.Game.Title != candidates[0].Title {
		t.Fatalf("provisional title not replaced: %q", view.Game.Title)
	}

	again, err := svc.Refresh(game.ID)
	if err != nil {
		t.Fatalf("second refresh: %v", err)
	}
	if again.Game.ExternalIDs.IGDB != candidates[0].ProviderID {
		t.Fatalf("refresh changed the match: %q", again.Game.ExternalIDs.IGDB)
	}
}

func TestLiveEnsureFreshResolvesRealTitles(t *testing.T) {
	svc, cat := liveService(t)
	for _, title := range []string{"TheoTown", "Workers & Resources Soviet Republic"} {
		t.Run(title, func(t *testing.T) {
			game, err := cat.AddGame(catalog.Game{Title: title, Provisional: true})
			if err != nil {
				t.Fatalf("add game: %v", err)
			}

			started, err := svc.EnsureFresh(game.ID)
			if err != nil {
				t.Fatalf("ensure fresh: %v", err)
			}
			if !started {
				t.Fatal("no background match was started")
			}
			view := waitResolved(t, svc, game.ID)
			if view.Game.ExternalIDs.IGDB == "" {
				t.Fatal("game stayed unresolved")
			}
			if view.Cover == "" {
				t.Fatal("no cover cached")
			}
			t.Logf("%s -> igdb=%s title=%q cover=%s screenshots=%d",
				title, view.Game.ExternalIDs.IGDB, view.Game.Title, view.Cover, len(view.Screenshots))
		})
	}
}
