package sources

import (
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/catalog"
)

type versionedTestRemote struct {
	cachingTestRemote
	rules int
}

func (r *versionedTestRemote) MatchRulesVersion() int { return r.rules }

func rulesTestRemote(rules int) *versionedTestRemote {
	return &versionedTestRemote{
		rules: rules,
		cachingTestRemote: cachingTestRemote{games: map[string]catalog.Game{
			"Alpha Game": {ID: "srv-alpha", Title: "Alpha Game", ExternalIDs: catalog.ExternalIDs{IGDB: "1"}},
			"Beta Game":  {ID: "srv-beta", Title: "Beta Game", ExternalIDs: catalog.ExternalIDs{IGDB: "2"}},
		}},
	}
}

func refreshOnce(t *testing.T, s *Service, id string) {
	t.Helper()
	if _, err := s.RefreshSource(id); err != nil {
		t.Fatalf("refresh %s: %v", id, err)
	}
}

func TestSavedMatchIsAskedAgainWhenServerRulesMoveOn(t *testing.T) {
	tests := []struct {
		name        string
		first       int
		second      int
		wantRequery bool
	}{
		{"server reports newer rules", 1, 2, true},
		{"same rules", 2, 2, false},
		{"old server without a version", 0, 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, cat, _ := testService(t)
			a := addSource(t, s, newFeedServer(t, feedBody(t, "A",
				feedEntry{DistributionID: "a", Title: "Alpha Game v1.0", URIs: []string{magnetOf("aa")}})).url())
			remote := rulesTestRemote(tc.first)
			cat.SetRemoteCatalog(remote)
			refreshOnce(t, s, a.ID)
			if n := remote.queryCount(); n != 1 {
				t.Fatalf("first refresh queries = %d, want 1", n)
			}

			remote.rules = tc.second
			b := addSource(t, s, newFeedServer(t, feedBody(t, "B",
				feedEntry{DistributionID: "b", Title: "Beta Game v1.0", URIs: []string{magnetOf("bb")}})).url())
			refreshOnce(t, s, b.ID)
			base := remote.queryCount()

			refreshOnce(t, s, a.ID)
			got := remote.queryCount() - base
			if tc.wantRequery && got != 1 {
				t.Fatalf("outdated match was asked %d times, want 1", got)
			}
			if !tc.wantRequery && got != 0 {
				t.Fatalf("current match was asked again %d times", got)
			}
			if tc.wantRequery {
				refreshOnce(t, s, a.ID)
				if n := remote.queryCount() - base; n != 1 {
					t.Fatalf("match asked again after it was saved under the new rules: %d queries", n)
				}
			}
		})
	}
}

func TestRulesVersionSurvivesRestart(t *testing.T) {
	s, cat, dir := testService(t)
	a := addSource(t, s, newFeedServer(t, feedBody(t, "A",
		feedEntry{DistributionID: "a", Title: "Alpha Game v1.0", URIs: []string{magnetOf("aa")}})).url())
	remote := rulesTestRemote(2)
	cat.SetRemoteCatalog(remote)
	refreshOnce(t, s, a.ID)
	if got := s.knownRulesVersion(); got != 2 {
		t.Fatalf("known version = %d, want 2", got)
	}

	restarted := mustServiceAt(t, dir, cat)
	if got := restarted.knownRulesVersion(); got != 2 {
		t.Fatalf("known version after restart = %d, want 2", got)
	}
}

func TestBrokenMatchRulesFileStopsTheService(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "match-rules.json"), []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := newServiceAt(dir, nil, mustCatalog(t, t.TempDir())); err == nil {
		t.Fatal("service started over an unreadable match rules file")
	}
}
