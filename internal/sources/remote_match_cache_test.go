package sources

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"typhon/internal/catalog"
)

type cachingTestRemote struct {
	mu      sync.Mutex
	queries []catalog.ReleaseQuery
	fail    map[string]bool
	games   map[string]catalog.Game
}

func (r *cachingTestRemote) Browse(context.Context, catalog.GameQuery) (catalog.GamePage, error) {
	return catalog.GamePage{}, nil
}

func (r *cachingTestRemote) MatchReleases(_ context.Context, qs []catalog.ReleaseQuery) ([]catalog.ReleaseMatch, error) {
	r.mu.Lock()
	r.queries = append(r.queries, qs...)
	r.mu.Unlock()

	out := make([]catalog.ReleaseMatch, len(qs))
	var failed []int
	for i, q := range qs {
		title := q.Titles[0]
		if r.fail[title] {
			failed = append(failed, i)
			continue
		}
		if g, ok := r.games[title]; ok {
			game := g
			out[i].Game = &game
		}
	}
	if len(failed) == 0 {
		return out, nil
	}
	return out, &catalog.PartialMatchError{Failed: failed, Total: len(qs), Err: errors.New("boom")}
}

func (r *cachingTestRemote) queryCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.queries)
}

func TestPartialRefreshKeepsPreviousMatchForFailedReleaseAndSavesSuccesses(t *testing.T) {
	s, cat, _ := testService(t)
	server := newFeedServer(t, feedBody(t, "Source",
		feedEntry{DistributionID: "good", Title: "Good Game v1.0", URIs: []string{magnetOf("aa")}},
		feedEntry{DistributionID: "bad", Title: "Bad Game v1.0", URIs: []string{magnetOf("bb")}},
	))
	src := addSource(t, s, server.url())
	remote := &cachingTestRemote{
		fail: map[string]bool{"Bad Game": true},
		games: map[string]catalog.Game{
			"Good Game": {ID: "server-good", Title: "Good Game", ExternalIDs: catalog.ExternalIDs{IGDB: "1"}},
		},
	}
	cat.SetRemoteCatalog(remote)

	if _, err := s.RefreshSource(src.ID); err != nil {
		t.Fatalf("partial refresh must not surface as an error: %v", err)
	}

	items := releasesOf(t, s, src.ID, "all")
	var good, bad *ReleaseView
	for i := range items {
		switch items[i].Release.DistributionID {
		case "good":
			good = &items[i]
		case "bad":
			bad = &items[i]
		}
	}
	if good == nil || bad == nil {
		t.Fatalf("releases missing: %+v", items)
	}
	if good.Release.MatchStatus != catalog.StatusMatched || good.Release.CanonicalGameID == nil || *good.Release.CanonicalGameID != "server-good" {
		t.Fatalf("good release = %+v, want the server match saved", good.Release)
	}
	if good.Release.RemoteMatch == nil || good.Release.RemoteMatch.GameID != "server-good" {
		t.Fatalf("good release RemoteMatch = %+v, want it cached", good.Release.RemoteMatch)
	}
	if bad.Release.CanonicalGameID != nil {
		t.Fatalf("bad release = %+v, want no canonical id (kept unmatched, not the good release's match)", bad.Release)
	}
	if bad.Release.RemoteMatch != nil {
		t.Fatalf("bad release RemoteMatch = %+v, want no cache entry for a failed index", bad.Release.RemoteMatch)
	}

	stored, err := s.GetSource(src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Health != HealthWarning {
		t.Fatalf("health = %s, want warning", stored.Health)
	}
	if stored.LastError == "" {
		t.Fatal("want a LastError describing the partial match failure")
	}
}

func TestSecondRefreshOnlyQueriesNewReleases(t *testing.T) {
	s, cat, _ := testService(t)
	server := newFeedServer(t, feedBody(t, "Source",
		feedEntry{DistributionID: "a", Title: "Alpha Game v1.0", URIs: []string{magnetOf("aa")}},
		feedEntry{DistributionID: "b", Title: "Beta Game v1.0", URIs: []string{magnetOf("bb")}},
	))
	src := addSource(t, s, server.url())
	remote := &cachingTestRemote{games: map[string]catalog.Game{
		"Alpha Game": {ID: "server-alpha", Title: "Alpha Game", ExternalIDs: catalog.ExternalIDs{IGDB: "1"}},
		"Beta Game":  {ID: "server-beta", Title: "Beta Game", ExternalIDs: catalog.ExternalIDs{IGDB: "2"}},
	}}
	cat.SetRemoteCatalog(remote)

	if _, err := s.RefreshSource(src.ID); err != nil {
		t.Fatalf("first refresh: %v", err)
	}
	if n := remote.queryCount(); n != 2 {
		t.Fatalf("first refresh sent %d queries, want 2", n)
	}

	remote.games["New Game"] = catalog.Game{ID: "server-new", Title: "New Game", ExternalIDs: catalog.ExternalIDs{IGDB: "3"}}
	server.set(feedBody(t, "Source",
		feedEntry{DistributionID: "a", Title: "Alpha Game v1.0", URIs: []string{magnetOf("aa")}},
		feedEntry{DistributionID: "b", Title: "Beta Game v1.0", URIs: []string{magnetOf("bb")}},
		feedEntry{DistributionID: "c", Title: "New Game v1.0", URIs: []string{magnetOf("cc")}},
	), `"v2"`)

	if _, err := s.RefreshSource(src.ID); err != nil {
		t.Fatalf("second refresh: %v", err)
	}
	if n := remote.queryCount(); n != 3 {
		t.Fatalf("cumulative queries after second refresh = %d, want 3 (only the new release added)", n)
	}
}

func TestExpiredRemoteMatchCacheReQueries(t *testing.T) {
	s, cat, _ := testService(t)
	server := newFeedServer(t, feedBody(t, "Source", feedEntry{DistributionID: "a", Title: "Alpha Game v1.0", URIs: []string{magnetOf("aa")}}))
	src := addSource(t, s, server.url())
	remote := &cachingTestRemote{games: map[string]catalog.Game{
		"Alpha Game": {ID: "server-alpha", Title: "Alpha Game", ExternalIDs: catalog.ExternalIDs{IGDB: "1"}},
	}}
	cat.SetRemoteCatalog(remote)

	if _, err := s.RefreshSource(src.ID); err != nil {
		t.Fatalf("first refresh: %v", err)
	}
	if n := remote.queryCount(); n != 1 {
		t.Fatalf("first refresh sent %d queries, want 1", n)
	}

	s.mu.Lock()
	for _, r := range s.releases[src.ID] {
		if r.RemoteMatch == nil {
			t.Fatal("release has no cached match to expire")
		}
		r.RemoteMatch.At = time.Now().Add(-48 * time.Hour)
	}
	s.mu.Unlock()

	if _, err := s.RefreshSource(src.ID); err != nil {
		t.Fatalf("second refresh: %v", err)
	}
	if n := remote.queryCount(); n != 2 {
		t.Fatalf("cumulative queries after expiry = %d, want 2", n)
	}
}

func TestCachedMatchWithMissingCatalogGameReQueries(t *testing.T) {
	s, cat, _ := testService(t)
	server := newFeedServer(t, feedBody(t, "Source", feedEntry{DistributionID: "a", Title: "Alpha Game v1.0", URIs: []string{magnetOf("aa")}}))
	src := addSource(t, s, server.url())
	remote := &cachingTestRemote{games: map[string]catalog.Game{
		"Alpha Game": {ID: "server-alpha", Title: "Alpha Game", ExternalIDs: catalog.ExternalIDs{IGDB: "1"}},
	}}
	cat.SetRemoteCatalog(remote)

	s.mu.Lock()
	for _, r := range s.releases[src.ID] {
		r.RemoteMatch = &RemoteMatch{
			Key:    remoteQueryKey(r),
			At:     time.Now(),
			Status: catalog.StatusMatched,
			GameID: "ghost-game-id",
			Method: string(catalog.MethodServerTitle),
		}
	}
	s.mu.Unlock()

	if _, err := s.RefreshSource(src.ID); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if n := remote.queryCount(); n != 1 {
		t.Fatalf("queries = %d, want 1: a cached match pointing at a missing catalog game must be re-verified", n)
	}
	items := releasesOf(t, s, src.ID, "all")
	if len(items) != 1 || items[0].Release.CanonicalGameID == nil || *items[0].Release.CanonicalGameID != "server-alpha" {
		t.Fatalf("release = %+v, want the freshly re-queried match applied", items)
	}
}

func TestChangedTitleReQueries(t *testing.T) {
	s, cat, _ := testService(t)
	server := newFeedServer(t, feedBody(t, "Source", feedEntry{DistributionID: "a", Title: "Old Title v1.0", URIs: []string{magnetOf("aa")}}))
	src := addSource(t, s, server.url())
	remote := &cachingTestRemote{games: map[string]catalog.Game{
		"Old Title": {ID: "server-old", Title: "Old Title", ExternalIDs: catalog.ExternalIDs{IGDB: "1"}},
		"New Title": {ID: "server-new", Title: "New Title", ExternalIDs: catalog.ExternalIDs{IGDB: "2"}},
	}}
	cat.SetRemoteCatalog(remote)

	if _, err := s.RefreshSource(src.ID); err != nil {
		t.Fatalf("first refresh: %v", err)
	}
	if n := remote.queryCount(); n != 1 {
		t.Fatalf("first refresh sent %d queries, want 1", n)
	}

	server.set(feedBody(t, "Source", feedEntry{DistributionID: "a", Title: "New Title v1.0", URIs: []string{magnetOf("aa")}}), `"v2"`)
	if _, err := s.RefreshSource(src.ID); err != nil {
		t.Fatalf("second refresh: %v", err)
	}
	if n := remote.queryCount(); n != 2 {
		t.Fatalf("cumulative queries after title change = %d, want 2", n)
	}

	items := releasesOf(t, s, src.ID, "all")
	if len(items) != 1 || items[0].Release.CanonicalGameID == nil || *items[0].Release.CanonicalGameID != "server-new" {
		t.Fatalf("release = %+v, want the new title's match", items)
	}
}

func TestTwinReleaseCacheServesSharedKey(t *testing.T) {
	s, cat, _ := testService(t)
	server := newFeedServer(t, feedBody(t, "Source",
		feedEntry{DistributionID: "a", Title: "Alpha Game v1.0", URIs: []string{magnetOf("aa")}},
		feedEntry{DistributionID: "b", Title: "Alpha Game v1.1", URIs: []string{magnetOf("bb")}},
	))
	src := addSource(t, s, server.url())
	remote := &cachingTestRemote{games: map[string]catalog.Game{
		"Alpha Game": {ID: "server-alpha", Title: "Alpha Game", ExternalIDs: catalog.ExternalIDs{IGDB: "1"}},
	}}
	cat.SetRemoteCatalog(remote)

	if _, err := s.RefreshSource(src.ID); err != nil {
		t.Fatalf("first refresh: %v", err)
	}
	if n := remote.queryCount(); n != 1 {
		t.Fatalf("first refresh sent %d queries, want 1 for two releases sharing a key", n)
	}

	s.mu.Lock()
	list := s.releases[src.ID]
	if len(list) != 2 || list[0].RemoteMatch == nil || list[1].RemoteMatch == nil {
		s.mu.Unlock()
		t.Fatalf("want both releases cached after the first refresh")
	}
	list[0].RemoteMatch = nil
	s.mu.Unlock()

	if _, err := s.RefreshSource(src.ID); err != nil {
		t.Fatalf("second refresh: %v", err)
	}
	if n := remote.queryCount(); n != 1 {
		t.Fatalf("cumulative queries = %d, want 1: the twin's fresh cache covers the shared key", n)
	}
}
