package catalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCorruptRecommendationStateKeepsCatalogAvailableWithoutOverwriting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "recommendation.json")
	raw := []byte("broken preferences")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := NewServiceAt(dir)
	if err != nil {
		t.Fatalf("optional personalization stopped catalog startup: %v", err)
	}
	if s.GetRecommendationProfile().DefaultSort != "popular" {
		t.Fatal("corrupt personal state must use popular")
	}
	if err = s.SetNotInterested("test", true); err == nil {
		t.Fatal("corrupt dismissal file was replaced")
	}
	if err = s.SaveRecommendationPreferences(defaultRecommendationPreferences()); err == nil {
		t.Fatal("corrupt preference file was replaced")
	}
	bytes, err := os.ReadFile(path)
	if err != nil || string(bytes) != string(raw) {
		t.Fatal("original damaged state lost")
	}

	owned := "00000000-0000-0000-0000-000000000111"
	s.SetRecommendationLibrarySource(func() []RecommendationLibraryItem {
		return []RecommendationLibraryItem{{LibraryID: "owned", CanonicalGameID: owned, Favorite: true}}
	})
	if q := s.enrichRecommendationQuery(GameQuery{HideLibrary: true}); q.ExcludeLibrary != owned {
		t.Fatalf("damaged preferences disabled actual library exclusions: %+v", q)
	}
	if profile := s.GetRecommendationProfile(); profile.Confidence != 0 {
		t.Fatalf("damaged state personalized: %+v", profile)
	}
	if _, err = s.GetLibraryRecommendations(LibraryRecommendationQuery{}); err == nil {
		t.Fatalf("damaged recommendation state was reported as empty: %v", err)
	}
	s.SetRemoteCatalog(&remoteFixture{page: GamePage{Items: []Game{{ID: "official", Title: "Available game"}}, Total: 1}})
	page, err := s.BrowseGames(GameQuery{Sort: "auto", Page: 1})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("general catalog unavailable: %+v %v", page, err)
	}
}

func TestLibraryRecommendationsReportsMissingEvidenceSource(t *testing.T) {
	s, err := NewServiceAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.GetLibraryRecommendations(LibraryRecommendationQuery{}); err == nil {
		t.Fatalf("missing library source was reported as empty: %v", err)
	}
}

func TestDiscoveryExclusionsRemainWhenDismissedFilterIsDisabled(t *testing.T) {
	s, err := NewServiceAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pick := "00000000-0000-0000-0000-000000000111"
	q := s.enrichRecommendationQuery(GameQuery{HideNotInterested: false, ExcludeIDs: []string{pick}})
	if q.ExcludeNotInterested != pick {
		t.Fatalf("discovery card would repeat in main catalog: %+v", q)
	}
}

func TestLegacyRecommendationExclusionsUseProviderEvidence(t *testing.T) {
	s, err := NewServiceAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.AddGame(Game{ID: "local-game", Title: "Known library game", ExternalIDs: ExternalIDs{IGDB: "123"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := s.remoteRecommendationIDs([]string{"local-game", "unknown-file-game"}); len(got) != 1 || got[0] != "igdb:123" {
		t.Fatalf("invalid identity transport: %v", got)
	}
}

func TestUnmatchedUnplayedLibraryGameHasHonestRecommendation(t *testing.T) {
	s, err := NewServiceAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.SetRecommendationLibrarySource(func() []RecommendationLibraryItem {
		return []RecommendationLibraryItem{{LibraryID: "local", Title: "My local game", Installed: true}}
	})
	picks, err := s.GetLibraryRecommendations(LibraryRecommendationQuery{})
	if err != nil || len(picks) != 1 || picks[0].Reason != "unplayed" || picks[0].Game.Title != "My local game" {
		t.Fatalf("missing local-library suggestion: %+v", picks)
	}
	if err = s.SetNotInterested("local", true); err != nil {
		t.Fatal(err)
	}
	if got, err := s.GetLibraryRecommendations(LibraryRecommendationQuery{}); err != nil || len(got) != 0 {
		t.Fatalf("dismissed library game returned: %+v", got)
	}
}

func TestDiscoveryAlwaysExcludesOwnedAndRefreshBeforeFetching(t *testing.T) {
	s, err := NewServiceAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	owned := "00000000-0000-0000-0000-000000000111"
	skip := "00000000-0000-0000-0000-000000000222"
	s.SetRecommendationLibrarySource(func() []RecommendationLibraryItem {
		return []RecommendationLibraryItem{{LibraryID: "owned", CanonicalGameID: owned}}
	})
	remote := &recommendationRemote{page: GamePage{Items: []Game{{ID: "fresh", Title: "Fresh game", Genres: []string{"Strategy"}}}}}
	s.SetRemoteCatalog(remote)
	result := s.GetDiscovery(DiscoveryQuery{RefreshExcludeIDs: []string{skip}, Limit: 1})
	if len(result.Items) != 1 || !strings.Contains(remote.got.ExcludeLibrary, owned) || !strings.Contains(remote.got.ExcludeNotInterested, skip) {
		t.Fatalf("UI default query did not exclude before pagination: %+v %+v", result, remote.got)
	}
}

func TestMultipleReleasesDoNotMultiplyPreferenceEvidence(t *testing.T) {
	games := []Game{{ID: "one", Genres: []string{"RPG"}}}
	items := []RecommendationLibraryItem{
		{LibraryID: "a", CanonicalGameID: "one", Favorite: true, Sessions: 1, PlaytimeSeconds: 600},
		{LibraryID: "b", CanonicalGameID: "one", Favorite: true, Sessions: 1, PlaytimeSeconds: 600},
		{LibraryID: "c", CanonicalGameID: "one", Favorite: true, Sessions: 1, PlaytimeSeconds: 600},
	}
	evidence := buildEvidence(games, items)
	if evidence.Games != 1 || evidence.Playtime != 600 || profileFromEvidence(evidence, RecommendationPreferences{}).DefaultSort != "popular" {
		t.Fatalf("copies created false confidence: %+v", evidence)
	}
	for i := range items {
		items[i].Favorite = false
	}
	if evidence = buildEvidence(games, items); evidence.Games != 0 {
		t.Fatalf("one copied launch became interest: %+v", evidence)
	}
}
