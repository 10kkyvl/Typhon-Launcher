package catalog

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestRecommendationProfileRequiresMoreThanOneLaunch(t *testing.T) {
	s := newTestService(t)
	games := seed(t, s,
		Game{Title: "Action One", Genres: []string{"Action"}},
		Game{Title: "Action Two", Genres: []string{"Action"}},
	)
	s.SetRecommendationLibrarySource(func() []RecommendationLibraryItem {
		return []RecommendationLibraryItem{{CanonicalGameID: games[0].ID, PlaytimeSeconds: 45 * 60, Sessions: 1}}
	})
	profile := s.GetRecommendationProfile()
	if profile.EvidenceGames != 0 || profile.DefaultSort != "popular" {
		t.Fatalf("single launch became evidence: %+v", profile)
	}
	s.SetRecommendationLibrarySource(func() []RecommendationLibraryItem {
		return []RecommendationLibraryItem{{CanonicalGameID: games[0].ID, PlaytimeSeconds: 90 * 60, Sessions: 2}}
	})
	profile = s.GetRecommendationProfile()
	if profile.EvidenceGames != 1 || profile.DefaultSort != "popular" {
		t.Fatalf("one game should not be sufficient history: %+v", profile)
	}
	items := []RecommendationLibraryItem{
		{CanonicalGameID: games[0].ID, PlaytimeSeconds: 45 * 60, Sessions: 2},
		{CanonicalGameID: games[1].ID, PlaytimeSeconds: 60 * 60, Sessions: 2},
	}
	s.SetRecommendationLibrarySource(func() []RecommendationLibraryItem { return items })
	profile = s.GetRecommendationProfile()
	if profile.EvidenceGames != 2 || profile.DefaultSort == "for-you" {
		t.Fatalf("two games should remain below the confidence threshold: %+v", profile)
	}
}

func TestRecommendationRatingUsesReviewConfidence(t *testing.T) {
	lowReviews, highReviews := 3, 1000
	lowRating, highRating := 10.0, 8.5
	if got, want := qualityScore(Game{Rating: &lowRating, ReviewCount: &lowReviews}), qualityScore(Game{Rating: &highRating, ReviewCount: &highReviews}); got >= want {
		t.Fatalf("few enthusiastic reviews outranked reliable rating: %f >= %f", got, want)
	}
	if got := qualityScore(Game{}); got != 0 {
		t.Fatalf("missing rating became a score: %f", got)
	}
}

func TestDiscoveryExcludesLibraryDismissedAndKnownAddons(t *testing.T) {
	s := newTestService(t)
	games := seed(t, s,
		Game{Title: "Owned", Genres: []string{"Action"}},
		Game{Title: "Dismissed", Genres: []string{"Action"}},
		Game{Title: "Unknown Type", Genres: []string{"Puzzle"}},
		Game{Title: "DLC", Genres: []string{"Action"}, GameType: "DLC"},
		Game{Title: "Strategy", Genres: []string{"Strategy"}},
	)
	s.SetRecommendationLibrarySource(func() []RecommendationLibraryItem {
		return []RecommendationLibraryItem{{CanonicalGameID: games[0].ID, Sessions: 2, PlaytimeSeconds: 3600}}
	})
	if err := s.SetNotInterested(games[1].ID, true); err != nil {
		t.Fatal(err)
	}
	result := s.GetDiscovery(DiscoveryQuery{Limit: 5})
	if len(result.Items) == 0 {
		t.Fatal("discovery returned no eligible games")
	}
	for _, item := range result.Items {
		if item.Game.ID == games[0].ID || item.Game.ID == games[1].ID || item.Game.ID == games[3].ID {
			t.Fatalf("discovery included excluded game: %+v", item)
		}
	}
	foundUnknown := false
	for _, item := range result.Items {
		if item.Game.ID == games[2].ID {
			foundUnknown = true
		}
	}
	if !foundUnknown {
		t.Fatal("unknown game type was treated as an addon")
	}
}

func TestRecommendationPreferencesPersistAndUndo(t *testing.T) {
	dir := t.TempDir()
	s := mustServiceAt(t, dir)
	game := seed(t, s, Game{Title: "Dismiss Me"})[0]
	if err := s.SetNotInterested(game.ID, true); err != nil {
		t.Fatal(err)
	}
	reloaded := mustServiceAt(t, dir)
	prefs := reloaded.GetRecommendationPreferences()
	if len(prefs.NotInterested) != 1 || prefs.NotInterested[0] != game.ID {
		t.Fatalf("dismissal did not persist: %+v", prefs)
	}
	if err := reloaded.SetNotInterested(game.ID, false); err != nil {
		t.Fatal(err)
	}
	if got := mustServiceAt(t, dir).GetRecommendationPreferences().NotInterested; len(got) != 0 {
		t.Fatalf("undo did not persist: %v", got)
	}

	bad := mustServiceAt(t, filepath.Join(t.TempDir(), "bad"))
	if err := bad.SaveRecommendationPreferences(RecommendationPreferences{DefaultSort: "nope"}); !errors.Is(err, errInvalidRecommendationSort) {
		t.Fatalf("invalid sort error = %v", err)
	}
}

func TestLibraryRecommendationsHonorsHeroExclusion(t *testing.T) {
	s := newTestService(t)
	games := seed(t, s,
		Game{Title: "Hero", Genres: []string{"Action"}},
		Game{Title: "Unplayed", Genres: []string{"Action"}},
		Game{Title: "Return", Genres: []string{"Strategy"}},
	)
	s.SetRecommendationLibrarySource(func() []RecommendationLibraryItem {
		return []RecommendationLibraryItem{
			{LibraryID: "hero-library", CanonicalGameID: games[0].ID, Sessions: 2, PlaytimeSeconds: 3600, ContinuePlaying: true},
			{LibraryID: "unplayed-library", CanonicalGameID: games[1].ID, Installed: true},
			{LibraryID: "return-library", CanonicalGameID: games[2].ID, Sessions: 2, PlaytimeSeconds: 3600},
		}
	})
	result := s.GetLibraryRecommendations(LibraryRecommendationQuery{Limit: 2, ExcludeLibraryIDs: []string{"hero-library"}})
	for _, item := range result {
		if item.LibraryID == "hero-library" {
			t.Fatalf("hero was duplicated: %+v", result)
		}
	}
	if len(result) == 0 || result[0].Reason != "unplayed" {
		t.Fatalf("unplayed library recommendation missing: %+v", result)
	}
}
