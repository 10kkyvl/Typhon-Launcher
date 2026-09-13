package catalog

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type recommendationRemote struct {
	page GamePage
	got  GameQuery
}

func (r *recommendationRemote) Browse(_ context.Context, q GameQuery) (GamePage, error) {
	r.got = q
	return r.page, nil
}

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

func TestRecommendationConfigUsesValidatedDefaults(t *testing.T) {
	defaults, err := parseRecommendationConfig("")
	if err != nil || defaults.MinMeaningfulSeconds != 30*60 || defaults.MeaningfulSessions != 2 || defaults.ReturnDays != 90 || defaults.InstalledBoost != 0.05 {
		t.Fatalf("defaults = %+v, err=%v", defaults, err)
	}
	custom, err := parseRecommendationConfig(`{"minMeaningfulSeconds":3600,"meaningfulSessions":3,"returnDays":120,"genreWeight":0.6}`)
	if err != nil || custom.MinMeaningfulSeconds != 3600 || custom.MeaningfulSessions != 3 || custom.ReturnDays != 120 || custom.GenreWeight != 0.6 {
		t.Fatalf("custom config = %+v, err=%v", custom, err)
	}
	invalid, err := parseRecommendationConfig(`{"returnDays":0}`)
	if err == nil || invalid != defaults {
		t.Fatalf("invalid config = %+v, err=%v", invalid, err)
	}
}

func TestRecommendationRatingUsesReviewConfidence(t *testing.T) {
	lowReviews, highReviews := 3, 1000
	lowRating, highRating := 10.0, 8.5
	if got, want := qualityScore(Game{Rating: &lowRating, RatingCount: &lowReviews}), qualityScore(Game{Rating: &highRating, RatingCount: &highReviews}); got >= want {
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
		Game{Title: "No Metadata"},
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
			if item.Reason != "category" || item.ReasonGenre != "Puzzle" {
				t.Fatalf("unknown game got fabricated preference reason: %+v", item)
			}
		}
		if item.Game.Title == "No Metadata" {
			t.Fatalf("candidate without a signal or genre was recommended: %+v", item)
		}
	}
	if !foundUnknown {
		t.Fatal("unknown game type was treated as an addon")
	}
}

func TestDiscoveryContentKindsMatchBackendGroups(t *testing.T) {
	s := newTestService(t)
	games := seed(t, s,
		Game{Title: "Main", Genres: []string{"Action"}, GameType: "main game"},
		Game{Title: "DLC", Genres: []string{"Adventure"}, GameType: "DLC"},
		Game{Title: "Addon", Genres: []string{"Strategy"}, GameType: "DLC / Addon"},
		Game{Title: "Expansion", Genres: []string{"RPG"}, GameType: "Expansion"},
		Game{Title: "Demo", Genres: []string{"Shooter"}, GameType: "Demo"},
		Game{Title: "Soundtrack", Genres: []string{"Indie"}, GameType: "Soundtrack"},
		Game{Title: "Music", Genres: []string{"Platform"}, GameType: "Music"},
		Game{Title: "Unknown type", Genres: []string{"Puzzle"}, GameType: "mystery"},
		Game{Title: "Tool", Genres: []string{"Action"}, GameType: "Tool"},
	)
	ids := func(result DiscoveryResult) map[string]bool {
		got := make(map[string]bool, len(result.Items))
		for _, item := range result.Items {
			got[item.Game.ID] = true
		}
		return got
	}

	all := ids(s.GetDiscovery(DiscoveryQuery{GameQuery: GameQuery{Kind: "all"}, Limit: 5}))
	for _, i := range []int{1, 4} {
		if !all[games[i].ID] {
			t.Fatalf("kind=all omitted %q: %+v", games[i].Title, all)
		}
	}
	allSoundtrackService := newTestService(t)
	soundtrackGames := seed(t, allSoundtrackService,
		Game{Title: "Soundtrack", Genres: []string{"Indie"}, GameType: "Soundtrack"},
		Game{Title: "Tool", Genres: []string{"Action"}, GameType: "Tool"},
	)
	allSoundtrack := ids(allSoundtrackService.GetDiscovery(DiscoveryQuery{GameQuery: GameQuery{Kind: "all"}, Limit: 5}))
	if !allSoundtrack[soundtrackGames[0].ID] || allSoundtrack[soundtrackGames[1].ID] {
		t.Fatalf("kind=all soundtrack grouping = %+v", allSoundtrack)
	}
	if all[games[8].ID] {
		t.Fatal("kind=all included software-like content")
	}

	dlc := ids(s.GetDiscovery(DiscoveryQuery{GameQuery: GameQuery{Kind: "dlc"}, Limit: 5}))
	for _, i := range []int{1, 2, 3} {
		if !dlc[games[i].ID] {
			t.Fatalf("kind=dlc omitted %q: %+v", games[i].Title, dlc)
		}
	}
	if dlc[games[4].ID] || dlc[games[5].ID] {
		t.Fatalf("kind=dlc leaked another content group: %+v", dlc)
	}

	demo := ids(s.GetDiscovery(DiscoveryQuery{GameQuery: GameQuery{Kind: "demo"}, Limit: 5}))
	if !demo[games[4].ID] || len(demo) != 1 {
		t.Fatalf("kind=demo = %+v", demo)
	}
	soundtrack := ids(s.GetDiscovery(DiscoveryQuery{GameQuery: GameQuery{Kind: "soundtrack"}, Limit: 5}))
	if !soundtrack[games[5].ID] || !soundtrack[games[6].ID] || len(soundtrack) != 2 {
		t.Fatalf("kind=soundtrack = %+v", soundtrack)
	}

	defaultKinds := ids(s.GetDiscovery(DiscoveryQuery{Limit: 5}))
	if defaultKinds[games[1].ID] || defaultKinds[games[4].ID] || defaultKinds[games[5].ID] {
		t.Fatalf("default discovery exposed add-on content: %+v", defaultKinds)
	}
	unknownService := newTestService(t)
	unknownGames := seed(t, unknownService,
		Game{Title: "Unknown type", Genres: []string{"Puzzle"}, GameType: "mystery"},
		Game{Title: "DLC", Genres: []string{"Action"}, GameType: "DLC"},
	)
	unknownDefault := ids(unknownService.GetDiscovery(DiscoveryQuery{Limit: 1}))
	if !unknownDefault[unknownGames[0].ID] {
		t.Fatal("unknown content type was treated as an add-on")
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

func TestSavingStalePreferencesKeepsDismissalsAndUndoResolvesAlias(t *testing.T) {
	dir := t.TempDir()
	s := mustServiceAt(t, dir)
	game := seed(t, s, Game{ID: "local-game", ServerID: "server-game", Title: "Dismiss Me"})[0]
	if err := s.SetNotInterested("server-game", true); err != nil {
		t.Fatal(err)
	}
	stale := s.GetRecommendationPreferences()
	stale.NotInterested = nil
	stale.DefaultSort = "popular"
	if err := s.SaveRecommendationPreferences(stale); err != nil {
		t.Fatal(err)
	}
	reloaded := mustServiceAt(t, dir)
	if got := reloaded.GetRecommendationPreferences().NotInterested; len(got) != 1 || got[0] != game.ID {
		t.Fatalf("stale save erased dismissal: %v", got)
	}

	// Simulate an older persisted alias. Undo through the canonical local ID
	// must remove it as well.
	reloaded.mu.Lock()
	reloaded.preferences.NotInterested = []string{"server-game"}
	reloaded.mu.Unlock()
	if err := reloaded.SetNotInterested(game.ID, false); err != nil {
		t.Fatal(err)
	}
	if got := reloaded.GetRecommendationPreferences().NotInterested; len(got) != 0 {
		t.Fatalf("alias dismissal survived undo: %v", got)
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

func TestLibraryRecommendationsDoNotReviveShortAbandonedLaunches(t *testing.T) {
	s := newTestService(t)
	games := seed(t, s,
		Game{Title: "Liked Action", Genres: []string{"Action"}},
		Game{Title: "Short Puzzle", Genres: []string{"Puzzle"}},
		Game{Title: "Action Return", Genres: []string{"Action"}},
	)
	old := time.Now().Add(-100 * 24 * time.Hour)
	s.SetRecommendationLibrarySource(func() []RecommendationLibraryItem {
		return []RecommendationLibraryItem{
			{LibraryID: "liked", CanonicalGameID: games[0].ID, Favorite: true, Sessions: 2, PlaytimeSeconds: 3600},
			{LibraryID: "short", CanonicalGameID: games[1].ID, Sessions: 1, PlaytimeSeconds: 10 * 60, LastPlayed: &old},
			{LibraryID: "return", CanonicalGameID: games[2].ID, Sessions: 1, PlaytimeSeconds: 10 * 60, LastPlayed: &old},
		}
	})
	result := s.GetLibraryRecommendations(LibraryRecommendationQuery{Limit: 5})
	for _, item := range result {
		if item.LibraryID == "short" {
			t.Fatalf("short abandoned launch was recommended: %+v", result)
		}
	}
	foundReturn := false
	for _, item := range result {
		if item.LibraryID == "return" {
			foundReturn = true
			if item.Reason != "return" {
				t.Fatalf("return recommendation reason = %q", item.Reason)
			}
		}
	}
	if !foundReturn {
		t.Fatalf("affinity-backed return recommendation missing: %+v", result)
	}
}

func TestDiscoveryUsesRemoteCandidatesBeforeLocalFallback(t *testing.T) {
	s := newTestService(t)
	owned := seed(t, s, Game{ID: "00000000-0000-0000-0000-000000000111", Title: "Owned", Genres: []string{"Action"}})[0]
	local := seed(t, s, Game{ID: "local-candidate", ServerID: "server-candidate", Title: "Cached Candidate", Genres: []string{"Strategy"}})[0]
	remote := &recommendationRemote{page: GamePage{Items: []Game{{ID: "server-candidate", Title: "Remote Candidate", SortTitle: "remote", Genres: []string{"Strategy"}}}}}
	s.SetRemoteCatalog(remote)
	s.SetRecommendationLibrarySource(func() []RecommendationLibraryItem {
		return []RecommendationLibraryItem{{CanonicalGameID: owned.ID, LibraryID: "owned"}}
	})
	result := s.GetDiscovery(DiscoveryQuery{GameQuery: GameQuery{HideLibrary: true, HideNotInterested: true, ExcludeIDs: []string{"00000000-0000-0000-0000-000000000222"}}, Limit: 1})
	if result.Fallback || len(result.Items) != 1 || result.Items[0].Game.ID != local.ID {
		t.Fatalf("remote discovery = %+v, fallback=%v", result.Items, result.Fallback)
	}
	if remote.got.Sort != "popular" || remote.got.Page != 1 || remote.got.PageSize != 60 {
		t.Fatalf("remote query = %+v", remote.got)
	}
	if remote.got.ExcludeNotInterested != "00000000-0000-0000-0000-000000000222" || remote.got.ExcludeLibrary != owned.ID {
		t.Fatalf("remote exclusions = %+v", remote.got)
	}
}

type pagedRecommendationRemote struct {
	pages   map[int][]Game
	queries []GameQuery
}

func (r *pagedRecommendationRemote) Browse(_ context.Context, q GameQuery) (GamePage, error) {
	r.queries = append(r.queries, q)
	return GamePage{Items: r.pages[q.Page], Total: 120, Page: q.Page, PageSize: 60, Revision: 7}, nil
}

func TestDiscoveryFetchesBoundedContinuationForExplainableCandidates(t *testing.T) {
	s := newTestService(t)
	unknown := make([]Game, 60)
	for i := range unknown {
		unknown[i] = Game{ID: fmt.Sprintf("unknown-%02d", i), Title: fmt.Sprintf("Unknown %02d", i)}
	}
	remote := &pagedRecommendationRemote{pages: map[int][]Game{
		1: unknown,
		2: {{ID: "suitable", Title: "Suitable", Genres: []string{"Strategy"}}},
	}}
	s.SetRemoteCatalog(remote)
	result := s.GetDiscovery(DiscoveryQuery{Limit: 1})
	if result.Fallback || len(result.Items) != 1 || result.Items[0].Game.ID != "suitable" {
		t.Fatalf("bounded remote discovery = %+v, fallback=%v", result.Items, result.Fallback)
	}
	if len(remote.queries) != 2 || remote.queries[0].Page != 1 || remote.queries[1].Page != 2 {
		t.Fatalf("continuation queries = %+v", remote.queries)
	}
	if remote.queries[1].Revision != 7 {
		t.Fatalf("continuation lost frozen revision: %+v", remote.queries[1])
	}
}

func TestDiscoveryPassesLargeLibraryAndRefreshExclusionsToRemote(t *testing.T) {
	s := newTestService(t)
	remote := &recommendationRemote{page: GamePage{Items: []Game{{ID: "next-page", Title: "Next Page Candidate", Genres: []string{"Strategy"}}}}}
	s.SetRemoteCatalog(remote)
	owned := make([]RecommendationLibraryItem, 65)
	for i := range owned {
		owned[i] = RecommendationLibraryItem{CanonicalGameID: fmt.Sprintf("00000000-0000-0000-0000-%012d", i+1000)}
	}
	s.SetRecommendationLibrarySource(func() []RecommendationLibraryItem { return owned })
	result := s.GetDiscovery(DiscoveryQuery{
		GameQuery:         GameQuery{HideLibrary: true, HideNotInterested: true},
		RefreshExcludeIDs: []string{"00000000-0000-0000-0000-000000000333"},
		Limit:             1,
	})
	if result.Fallback || len(result.Items) != 1 || result.Items[0].Game.ID != "next-page" {
		t.Fatalf("discovery after large exclusions = %+v, fallback=%v", result.Items, result.Fallback)
	}
	if got := len(strings.Split(remote.got.ExcludeLibrary, ",")); got != len(owned) {
		t.Fatalf("library exclusions = %d, want %d", got, len(owned))
	}
	if !strings.Contains(remote.got.ExcludeLibrary, "00000000-0000-0000-0000-000000001064") || remote.got.ExcludeNotInterested != "00000000-0000-0000-0000-000000000333" {
		t.Fatalf("remote exclusions = %+v", remote.got)
	}
}
