package catalog

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDiscoveryPreviewDoesNotWriteAndOpeningPickPersists(t *testing.T) {
	s := newTestService(t)
	seed(t, s, Game{ID: "local", Title: "Local"})
	before, err := os.ReadFile(s.gamesPath)
	if err != nil {
		t.Fatal(err)
	}
	remote := &recommendationRemote{page: GamePage{Items: []Game{{ID: "pick", Title: "Pick", Genres: []string{"Strategy"}}, {ID: "other", Title: "Other", Genres: []string{"Indie"}}}}}
	s.SetRemoteCatalog(remote)
	first := s.GetDiscovery(DiscoveryQuery{Limit: 1})
	second := s.GetDiscovery(DiscoveryQuery{Limit: 1, RefreshExcludeIDs: []string{first.Items[0].Game.ID}})
	if len(first.Items) != 1 || len(second.Items) != 1 || first.Items[0].Game.ID == second.Items[0].Game.ID || remote.calls != 1 {
		t.Fatalf("refresh=%+v calls=%d", second, remote.calls)
	}
	after, err := os.ReadFile(s.gamesPath)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("preview wrote catalog: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(s.gamesPath), "catalog-pages")); !os.IsNotExist(err) {
		t.Fatalf("preview wrote offline pages: %v", err)
	}
	if len(s.games) != 1 || len(s.browseSnapshots) != 0 {
		t.Fatalf("preview leaked persistent membership/snapshots: %d/%d", len(s.games), len(s.browseSnapshots))
	}
	if _, err := s.GetGame(first.Items[0].Game.ID); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewServiceAt(filepath.Dir(s.gamesPath))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.GetGame(first.Items[0].Game.ID); err != nil {
		t.Fatalf("opened pick missing after restart: %v", err)
	}
	if len(reopened.games) != 2 {
		t.Fatalf("persisted unvisited candidates: %d", len(reopened.games))
	}
}

func TestDiscoveryOfflineWorkingFilterAndRefreshBackfill(t *testing.T) {
	s := newTestService(t)
	seed(t, s, Game{ID: "works", Title: "Works", ExternalIDs: ExternalIDs{IGDB: "1"}, Genres: []string{"Indie"}}, Game{ID: "unknown", Title: "Unknown", Genres: []string{"Adventure"}}, Game{ID: "fails", Title: "Fails", ExternalIDs: ExternalIDs{IGDB: "2"}, Genres: []string{"Racing"}})
	s.SetCompatLookup(func(id string) (int, int, bool) {
		if id == "1" {
			return 4, 5, true
		}
		return 1, 5, true
	})
	result := s.GetDiscovery(DiscoveryQuery{GameQuery: GameQuery{Compat: CompatOnlyWorking}, RefreshExcludeIDs: []string{"works"}})
	if !result.Fallback || len(result.Items) != 1 || result.Items[0].Game.ID != "works" {
		t.Fatalf("offline working picks=%+v", result)
	}
}

func TestOpeningDiscoveryPickRollsBackFailedSave(t *testing.T) {
	s := newTestService(t)
	s.rememberDiscoveryGames([]RecommendationItem{{Game: Game{ID: "pick", Title: "Pick"}}})
	s.gamesPath = t.TempDir() // atomic rename over a directory must fail
	if _, err := s.GetGame("pick"); err == nil {
		t.Fatal("save unexpectedly succeeded")
	}
	if len(s.games) != 0 {
		t.Fatal("failed opening mutated catalog")
	}
	if _, ok := s.idx.game("pick"); ok {
		t.Fatal("failed opening mutated index")
	}
}

func TestOpeningPickReusesIdentityLearnedAfterPreview(t *testing.T) {
	s := newTestService(t)
	s.rememberDiscoveryGames([]RecommendationItem{{Game: Game{ID: "server", ServerID: "server", Title: "Pick", ExternalIDs: ExternalIDs{IGDB: "42"}}}})
	seed(t, s, Game{ID: "personal", Title: "Pick", ExternalIDs: ExternalIDs{IGDB: "42"}})
	got, err := s.GetGame("server")
	if err != nil || got.ID != "personal" || len(s.games) != 1 {
		t.Fatalf("duplicate identity after preview: %+v %v", got, err)
	}
}
