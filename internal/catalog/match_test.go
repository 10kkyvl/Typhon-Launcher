package catalog

import "testing"

// Bug: a sequel whose title matches a catalog entry literally (exact_title,
// score 0.98) was sent to review whenever the catalog also held the
// prequel, because the prequel is always a decent fuzzy sibling (score
// capped at 0.95) and 0.98-0.95=0.03 < AmbiguityDelta (0.04). Ambiguity must
// only be judged among candidates that matched the same way; a literal
// title hit does not lose to a same-family fuzzy neighbor.
func TestResolveExactTitleBeatsFuzzySequelSibling(t *testing.T) {
	s := newTestService(t)
	games := seed(t, s,
		Game{Title: "Tormented Souls"},
		Game{Title: "Tormented Souls 2"},
	)

	match := s.Resolve(query("Tormented Souls 2"))
	if match.Status != StatusMatched {
		t.Fatalf("status = %s, want matched (match = %+v)", match.Status, match)
	}
	if match.GameID != games[1].ID {
		t.Fatalf("game = %s, want the sequel %s", match.GameID, games[1].ID)
	}
	if match.Method != MethodExactTitle {
		t.Fatalf("method = %s, want exact_title", match.Method)
	}
}

// Regression: two catalog entries that share the exact same title (the
// launcher's real catalog has this for Celeste, Hades, Hollow Knight) must
// still require manual review — real ambiguity within the same match class
// is not affected by the fix above.
func TestResolveDuplicateExactTitlesStayAmbiguous(t *testing.T) {
	s := newTestService(t)
	seed(t, s,
		Game{Title: "Hollow Knight"},
		Game{Title: "Hollow Knight"},
	)

	match := s.Resolve(query("Hollow Knight"))
	if match.Status != StatusReview {
		t.Fatalf("status = %s, want review (match = %+v)", match.Status, match)
	}
	if match.GameID != "" {
		t.Fatalf("game = %s, want empty for ambiguous match", match.GameID)
	}
}
