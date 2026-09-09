package sources

import (
	"testing"
	"time"
	"typhon/internal/catalog"
	"typhon/internal/sources/feed"
)

func TestRegressionRenamedReleaseKeepsWrongGame(t *testing.T) {
	list := sampleList(t, "Wrong Game v1.0")
	id := "wrong-game"
	list[0].CanonicalGameID = &id
	list[0].MatchStatus = catalog.StatusMatched
	list[0].MatchMethod = string(catalog.MethodExactTitle)
	list[0].MatchEpoch = 7
	m := unmatchedMatcher()
	incoming := parseEntries("src", []feed.Entry{entry("Corrected Other Game v1.0", list[0].URIs[0], 20)}, time.Now())
	merged, summary := merge(list, incoming, time.Now(), false)
	if summary.Updated != 1 {
		t.Fatal(summary)
	}
	if err := applyMatches(m, merged); err != nil {
		t.Fatal(err)
	}
	if m.resolved != 1 || merged[0].CanonicalGameID != nil {
		t.Fatal("not reproduced")
	}
	t.Log("Regression: corrected release title retains old game's exact match without resolving")
}
