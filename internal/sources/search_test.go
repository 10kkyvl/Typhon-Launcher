package sources

import (
	"testing"
	"time"

	"typhon/internal/catalog"
)

func gameRef(id string) *string {
	return &id
}

func searchService(t *testing.T, srcs []*Source, releases map[string][]*Release) *Service {
	t.Helper()
	s := mustServiceAt(t, t.TempDir(), nil)
	s.sources = srcs
	s.releases = releases
	return s
}

func availableRelease(id, sourceID, rawTitle, normalized, gameID, version string) *Release {
	r := &Release{
		ID:              id,
		SourceID:        sourceID,
		RawTitle:        rawTitle,
		Title:           rawTitle,
		NormalizedTitle: normalized,
		Version:         version,
		Availability:    AvailabilityAvailable,
		MatchStatus:     catalog.StatusUnmatched,
	}
	if gameID != "" {
		r.CanonicalGameID = gameRef(gameID)
		r.MatchStatus = catalog.StatusMatched
	}
	return r
}

func TestSearchReleaseMatchesAggregatesByGame(t *testing.T) {
	s := searchService(t,
		[]*Source{
			{ID: "src-1", Name: "Feed A", Enabled: true},
			{ID: "src-2", Name: "Feed B", Enabled: true},
		},
		map[string][]*Release{
			"src-1": {
				availableRelease("r1", "src-1", "Cyberpunk.2077.Ultimate.Edition.v2.31", "cyberpunk 2077 ultimate edition", "game-1", "2.31"),
				availableRelease("r2", "src-1", "Cyberpunk 2077 v2.2 [Repack]", "cyberpunk 2077", "game-1", "2.2"),
			},
			"src-2": {
				availableRelease("r3", "src-2", "Cyberpunk.2077-RUNE", "cyberpunk 2077", "game-1", "2.12"),
			},
		},
	)

	matches := s.SearchReleaseMatches("Cyberpunk", nil, 5)

	if len(matches.Games) != 1 {
		t.Fatalf("expected one aggregated game, got %d", len(matches.Games))
	}
	info := matches.Games["game-1"]
	if info.Releases != 3 {
		t.Fatalf("expected 3 releases, got %d", info.Releases)
	}
	if info.Sources != 2 {
		t.Fatalf("expected 2 distinct sources, got %d", info.Sources)
	}
	if info.LatestVersion != "2.31" {
		t.Fatalf("expected latest version 2.31, got %q", info.LatestVersion)
	}
	if len(matches.Unmatched) != 0 {
		t.Fatalf("matched releases must not appear as unmatched, got %d", len(matches.Unmatched))
	}
}

func TestSearchReleaseMatchesCountsGamesFoundByCanonicalID(t *testing.T) {
	s := searchService(t,
		[]*Source{{ID: "src-1", Name: "Feed A", Enabled: true}},
		map[string][]*Release{
			"src-1": {availableRelease("r1", "src-1", "CP2077-RUNE", "cp2077 rune", "game-1", "2.31")},
		},
	)

	matches := s.SearchReleaseMatches("Cyberpunk", []string{"game-1"}, 5)

	if matches.Games["game-1"].Releases != 1 {
		t.Fatalf("expected release counted through canonical id, got %+v", matches.Games)
	}
	if len(matches.Unmatched) != 0 {
		t.Fatalf("expected no unmatched hits, got %d", len(matches.Unmatched))
	}
}

func TestSearchReleaseMatchesSkipsDisabledSources(t *testing.T) {
	s := searchService(t,
		[]*Source{
			{ID: "src-1", Name: "Feed A", Enabled: true},
			{ID: "src-2", Name: "Feed B", Enabled: false, Status: StatusDisabled},
		},
		map[string][]*Release{
			"src-1": {availableRelease("r1", "src-1", "TheoTown v1.12", "theotown", "", "1.12")},
			"src-2": {
				availableRelease("r2", "src-2", "TheoTown v1.13", "theotown", "", "1.13"),
				availableRelease("r3", "src-2", "TheoTown Deluxe", "theotown deluxe", "game-1", ""),
			},
		},
	)

	matches := s.SearchReleaseMatches("theotown", []string{"game-1"}, 5)

	if len(matches.Unmatched) != 1 || matches.Unmatched[0].Release.ID != "r1" {
		t.Fatalf("expected only the enabled source release, got %+v", matches.Unmatched)
	}
	if len(matches.Games) != 0 {
		t.Fatalf("disabled source must not contribute matched releases, got %+v", matches.Games)
	}
}

func TestSearchReleaseMatchesSkipsUnavailableAndIgnored(t *testing.T) {
	removed := availableRelease("r2", "src-1", "TheoTown v1.13", "theotown", "", "1.13")
	removed.Availability = AvailabilityRemoved
	ignored := availableRelease("r3", "src-1", "TheoTown v1.14", "theotown", "", "1.14")
	ignored.Ignored = true

	s := searchService(t,
		[]*Source{{ID: "src-1", Name: "Feed A", Enabled: true}},
		map[string][]*Release{
			"src-1": {
				availableRelease("r1", "src-1", "TheoTown v1.12", "theotown", "", "1.12"),
				removed,
				ignored,
			},
		},
	)

	matches := s.SearchReleaseMatches("theotown", nil, 5)

	if len(matches.Unmatched) != 1 || matches.Unmatched[0].Release.ID != "r1" {
		t.Fatalf("expected only the available release, got %+v", matches.Unmatched)
	}
}

func TestSearchReleaseMatchesLimitsUnmatched(t *testing.T) {
	list := make([]*Release, 0, 8)
	for i := range 8 {
		list = append(list, availableRelease(string(rune('a'+i)), "src-1", "Some Game Build 123", "some game build 123", "", ""))
	}
	s := searchService(t,
		[]*Source{{ID: "src-1", Name: "Feed A", Enabled: true}},
		map[string][]*Release{"src-1": list},
	)

	matches := s.SearchReleaseMatches("some game", nil, 5)

	if len(matches.Unmatched) != 5 {
		t.Fatalf("expected 5 unmatched hits, got %d", len(matches.Unmatched))
	}
	if matches.MoreUnmatched != 3 {
		t.Fatalf("expected 3 more, got %d", matches.MoreUnmatched)
	}
	if matches.Unmatched[0].SourceName != "Feed A" {
		t.Fatalf("expected source name resolved, got %q", matches.Unmatched[0].SourceName)
	}
}

func TestSearchReleaseMatchesEmptyQuery(t *testing.T) {
	s := searchService(t,
		[]*Source{{ID: "src-1", Name: "Feed A", Enabled: true}},
		map[string][]*Release{
			"src-1": {availableRelease("r1", "src-1", "TheoTown", "theotown", "game-1", "1.12")},
		},
	)

	matches := s.SearchReleaseMatches("  ", []string{"game-1"}, 5)

	if len(matches.Games) != 0 || len(matches.Unmatched) != 0 {
		t.Fatalf("empty query must return nothing, got %+v", matches)
	}
}

func TestLatestVersionPrefersNewestComparable(t *testing.T) {
	older := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		refs []versionRef
		want string
	}{
		{"comparable picks highest", []versionRef{
			{raw: "1.2", at: &newer, id: "b"},
			{raw: "2.31", at: &older, id: "a"},
		}, "2.31"},
		{"incomparable falls back to newest upload", []versionRef{
			{raw: "build 1234", at: &older, id: "a"},
			{raw: "2024.11.02", at: &newer, id: "b"},
		}, "2024.11.02"},
		{"missing versions ignored", []versionRef{
			{raw: "", at: &newer, id: "a"},
			{raw: "1.5", at: &older, id: "b"},
		}, "1.5"},
		{"no versions", []versionRef{{raw: "", at: nil, id: "a"}}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := latestVersion(tc.refs); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestSearchReleaseMatchesIsDeterministicOverManyReleases(t *testing.T) {
	const total = 10000
	list := make([]*Release, 0, total)
	for i := range total {
		gameID := ""
		if i%2 == 0 {
			gameID = "game-1"
		}
		list = append(list, availableRelease(
			"r"+string(rune('a'+i%26))+string(rune('a'+i/26%26))+string(rune('a'+i/676%26)),
			"src-1",
			"Some Game Build 1234",
			"some game build 1234",
			gameID,
			"1.0",
		))
	}
	s := searchService(t,
		[]*Source{{ID: "src-1", Name: "Feed A", Enabled: true}},
		map[string][]*Release{"src-1": list},
	)

	first := s.SearchReleaseMatches("some game", nil, 5)
	for range 5 {
		next := s.SearchReleaseMatches("some game", nil, 5)
		if next.Games["game-1"] != first.Games["game-1"] {
			t.Fatalf("aggregate is not deterministic: %+v vs %+v", next.Games, first.Games)
		}
		if len(next.Unmatched) != len(first.Unmatched) {
			t.Fatalf("unmatched count changed: %d vs %d", len(next.Unmatched), len(first.Unmatched))
		}
		for i := range next.Unmatched {
			if next.Unmatched[i].Release.ID != first.Unmatched[i].Release.ID {
				t.Fatalf("unmatched order changed at %d", i)
			}
		}
	}
	if first.Games["game-1"].Releases != total/2 {
		t.Fatalf("expected %d matched releases, got %d", total/2, first.Games["game-1"].Releases)
	}
}

func BenchmarkSearchReleaseMatches(b *testing.B) {
	list := make([]*Release, 0, 10000)
	for i := range 10000 {
		gameID := ""
		if i%3 == 0 {
			gameID = "game-1"
		}
		list = append(list, availableRelease("r"+string(rune(i)), "src-1", "Some Game Build 1234", "some game build 1234", gameID, "1.0"))
	}
	s := mustServiceAt(b, b.TempDir(), nil)
	s.sources = []*Source{{ID: "src-1", Name: "Feed A", Enabled: true}}
	s.releases = map[string][]*Release{"src-1": list}

	b.ResetTimer()
	for range b.N {
		s.SearchReleaseMatches("cyberpunk", nil, 5)
	}
}
