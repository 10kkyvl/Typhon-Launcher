package sources

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

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

func twoGameSource(t *testing.T, s *Service) Source {
	t.Helper()
	return addSource(t, s, newFeedServer(t, feedBody(t, "A",
		feedEntry{DistributionID: "a", Title: "Alpha Game v1.0", URIs: []string{magnetOf("aa")}},
		feedEntry{DistributionID: "b", Title: "Beta Game v1.0", URIs: []string{magnetOf("bb")}})).url())
}

func savedRules(t *testing.T, s *Service, sourceID string) map[string]int {
	t.Helper()
	out := map[string]int{}
	for _, it := range releasesOf(t, s, sourceID, "all") {
		if it.Release.RemoteMatch == nil {
			t.Fatalf("release %s has no saved match", it.Release.DistributionID)
		}
		out[it.Release.DistributionID] = it.Release.RemoteMatch.RulesVersion
	}
	return out
}

func TestFullyCachedSourceLearnsNewServerRules(t *testing.T) {
	tests := []struct {
		name        string
		serverRules int
		failed      string
		wait        time.Duration
		wantQueries int
		wantKnown   int
		wantSaved   map[string]int
	}{
		{"server moved on: all asked again", 4, "", rulesProbeEvery, 2, 4, map[string]int{"a": 4, "b": 4}},
		{"same rules: one probe only", 3, "", rulesProbeEvery, 1, 3, map[string]int{"a": 3, "b": 3}},
		{"probed recently: nothing asked", 4, "", time.Minute, 0, 3, map[string]int{"a": 3, "b": 3}},
		{"second batch fails: old match kept", 4, "Beta Game", rulesProbeEvery, 2, 4, map[string]int{"a": 4, "b": 3}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, cat, _ := testService(t)
			src := twoGameSource(t, s)
			remote := rulesTestRemote(3)
			cat.SetRemoteCatalog(remote)
			refreshOnce(t, s, src.ID)
			if got := s.knownRulesVersion(); got != 3 {
				t.Fatalf("known after first refresh = %d, want 3", got)
			}
			base := remote.queryCount()
			start := time.Now()
			s.now = func() time.Time { return start.Add(tc.wait) }

			remote.rules = tc.serverRules
			if tc.failed != "" {
				remote.fail = map[string]bool{tc.failed: true}
			}
			refreshOnce(t, s, src.ID)

			if got := remote.queryCount() - base; got != tc.wantQueries {
				t.Fatalf("queries in the pass = %d, want %d", got, tc.wantQueries)
			}
			if got := s.knownRulesVersion(); got != tc.wantKnown {
				t.Fatalf("known = %d, want %d", got, tc.wantKnown)
			}
			saved := savedRules(t, s, src.ID)
			for id, want := range tc.wantSaved {
				if saved[id] != want {
					t.Fatalf("release %s saved under rules %d, want %d (all: %v)", id, saved[id], want, saved)
				}
			}
			for _, it := range releasesOf(t, s, src.ID, "all") {
				if it.Release.RemoteMatch.GameID == "" {
					t.Fatalf("release %s lost its game", it.Release.DistributionID)
				}
			}
		})
	}
}

func TestRulesProbeStopsWhenContextIsCancelled(t *testing.T) {
	s, cat, _ := testService(t)
	src := twoGameSource(t, s)
	remote := rulesTestRemote(3)
	cat.SetRemoteCatalog(remote)
	refreshOnce(t, s, src.ID)
	base := remote.queryCount()
	remote.rules = 4

	s.mu.Lock()
	list := cloneReleases(s.releases[src.ID])
	s.mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	resolved, partial, err := s.resolveRemoteReleases(ctx, list, s.newRun(src.ID))
	if !errors.Is(err, context.Canceled) || resolved != nil || partial != nil {
		t.Fatalf("resolve = %v %v %v, want nothing and context.Canceled", resolved, partial, err)
	}
	if got := remote.queryCount() - base; got != 0 {
		t.Fatalf("queries after cancel = %d, want 0", got)
	}
	if got := s.knownRulesVersion(); got != 3 {
		t.Fatalf("known = %d, want 3", got)
	}
}
