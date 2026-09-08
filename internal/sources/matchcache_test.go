package sources

import (
	"testing"
	"time"

	"typhon/internal/catalog"
	"typhon/internal/sources/feed"
)

func unmatchedMatcher() *stubMatcher {
	return &stubMatcher{
		epoch: 7,
		resolve: func(qs []catalog.Query) []catalog.Match {
			out := make([]catalog.Match, len(qs))
			for i := range out {
				out[i] = catalog.Match{Status: catalog.StatusUnmatched}
			}
			return out
		},
		provision: func([]catalog.Query) (map[string]catalog.Game, error) {
			return map[string]catalog.Game{}, nil
		},
	}
}

func sampleList(t *testing.T, titles ...string) []*Release {
	t.Helper()
	now := time.Now()
	entries := make([]feed.Entry, 0, len(titles))
	for i, title := range titles {
		entries = append(entries, entry(title, magnetOf(string(rune('a'+i))+"1"), 10))
	}
	list := parseEntries("src", entries, now)
	list, _ = merge(nil, list, now, true)
	return list
}

func TestUnchangedReleasesAreNotRematchedOnTheSameEpoch(t *testing.T) {
	list := sampleList(t, "Some Unmatched Game v1.0", "Another Unmatched Game v2.0")
	m := unmatchedMatcher()

	if err := applyMatches(m, list); err != nil {
		t.Fatalf("first applyMatches() error = %v", err)
	}
	first := m.resolved
	if first == 0 {
		t.Fatal("the first pass resolved nothing")
	}

	if err := applyMatches(m, list); err != nil {
		t.Fatalf("second applyMatches() error = %v", err)
	}
	if m.resolved != first {
		t.Fatalf("resolved %d queries on the second pass, want none: the cache did not hold", m.resolved-first)
	}
}

func TestANewEpochForcesARematch(t *testing.T) {
	list := sampleList(t, "Some Unmatched Game v1.0")
	m := unmatchedMatcher()

	if err := applyMatches(m, list); err != nil {
		t.Fatalf("first applyMatches() error = %v", err)
	}
	first := m.resolved

	m.epoch = 8
	if err := applyMatches(m, list); err != nil {
		t.Fatalf("second applyMatches() error = %v", err)
	}
	if m.resolved == first {
		t.Fatal("a changed catalog did not force a rematch")
	}
}

func TestAChangedReleaseIsRematchedOnTheSameEpoch(t *testing.T) {
	now := time.Now()
	m := unmatchedMatcher()

	list := sampleList(t, "Some Game v1.0")
	if err := applyMatches(m, list); err != nil {
		t.Fatalf("first applyMatches() error = %v", err)
	}
	first := m.resolved

	// Тот же magnet, другая версия: identity держится на info hash, поэтому
	// запись обновляется, а не заводится заново.
	incoming := parseEntries("src", []feed.Entry{entry("Some Game v2.0", list[0].URIs[0], 20)}, now)
	merged, summary := merge(list, incoming, now, false)
	if summary.Updated != 1 {
		t.Fatalf("summary.Updated = %d, want 1: %+v", summary.Updated, summary)
	}

	if err := applyMatches(m, merged); err != nil {
		t.Fatalf("second applyMatches() error = %v", err)
	}
	if m.resolved == first {
		t.Fatal("a release whose feed data changed was served from the cache")
	}
}

// Точный матч переживает смену эпохи: каталог мог вырасти, но игра с таким
// названием в нём уже есть, и повторное сравнение ничего не изменит.
func TestStableMatchesSkipTheResolverAfterAnEpochChange(t *testing.T) {
	list := sampleList(t, "Some Game v1.0")
	gameID := "game-1"
	list[0].MatchStatus = catalog.StatusMatched
	list[0].CanonicalGameID = &gameID
	list[0].MatchMethod = string(catalog.MethodExactTitle)

	m := unmatchedMatcher()
	if err := applyMatches(m, list); err != nil {
		t.Fatalf("applyMatches() error = %v", err)
	}
	if m.resolved != 0 {
		t.Fatalf("resolved %d queries, want 0 for a stable match", m.resolved)
	}
	if list[0].MatchEpoch != m.Epoch() {
		t.Fatalf("MatchEpoch = %d, want %d", list[0].MatchEpoch, m.Epoch())
	}

	m.epoch = 9
	if err := applyMatches(m, list); err != nil {
		t.Fatalf("second applyMatches() error = %v", err)
	}
	if m.resolved != 0 {
		t.Fatalf("a stable match was re-resolved after the epoch moved: %d queries", m.resolved)
	}
}
