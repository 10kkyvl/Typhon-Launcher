package sources

import (
	"context"
	"errors"
	"os"
	"testing"

	"typhon/internal/catalog"
)

type sourceMatchRemote struct {
	callback func()
	err      error
	queries  []catalog.ReleaseQuery
	games    map[string]catalog.Game
}

func (r *sourceMatchRemote) Browse(context.Context, catalog.GameQuery) (catalog.GamePage, error) {
	return catalog.GamePage{}, nil
}
func (r *sourceMatchRemote) MatchReleases(_ context.Context, qs []catalog.ReleaseQuery) ([]catalog.ReleaseMatch, error) {
	if r.callback != nil {
		r.callback()
	}
	r.queries = qs
	if r.err != nil {
		return nil, r.err
	}
	out := make([]catalog.ReleaseMatch, len(qs))
	for i, q := range qs {
		if q.Titles[0] == "Grand Theft Auto V" {
			out[i].Game = &catalog.Game{ID: "server-gta", Title: "Grand Theft Auto V", ExternalIDs: catalog.ExternalIDs{IGDB: "1020"}}
		}
		for _, title := range q.Titles {
			if g, ok := r.games[title]; ok {
				out[i].Game = &g
				break
			}
		}
	}
	return out, nil
}

func TestSourceRefreshReparsesMetadataAndPersistsOfficialGameGroups(t *testing.T) {
	s, cat, dir := testService(t)
	cases := []struct {
		raw, title, id string
	}{
		{"ГТА 3 (GTA 3) — RePack от Igruha", "Grand Theft Auto III", "730"},
		{"GTA 4 / Grand Theft Auto IV (2010) RePack от xatab", "Grand Theft Auto IV", "731"},
		{"Little Nightmares III (2025/11/19) [Папка игры] (2025)", "Little Nightmares III", "264398"},
		{"TerraScape: Deluxe Edition – v2.1.0.3 + 2 DLCs/Bonuses", "TerraScape", "test-terrascape"},
	}
	entries := make([]feedEntry, 0, len(cases))
	remote := &sourceMatchRemote{games: map[string]catalog.Game{}}
	for _, tc := range cases {
		entries = append(entries, feedEntry{Title: tc.raw, DistributionID: tc.id, URIs: []string{magnetOf("ab")}})
		remote.games[tc.title] = catalog.Game{ID: "server-" + tc.id, Title: tc.title, ExternalIDs: catalog.ExternalIDs{IGDB: tc.id}}
	}
	server := newFeedServer(t, feedBody(t, "Metadata source", entries...))
	src := addSource(t, s, server.url())
	cat.SetRemoteCatalog(remote)
	summary, err := s.RefreshSource(src.ID)
	if err != nil || !summary.NotModified {
		t.Fatalf("cached source refresh: %+v %v", summary, err)
	}
	// The card reads these groups, not the transient match response. Reopen
	// both stores to ensure official identities and links survive a restart.
	reloaded := mustServiceAt(t, dir, mustCatalog(t, dir))
	for _, tc := range cases {
		groups := reloaded.GetReleasesForGame("server-" + tc.id)
		if len(groups) != 1 || groups[0].Release.DistributionID != tc.id || groups[0].Release.MatchMethod != string(catalog.MethodServerTitle) {
			t.Fatalf("%s missing its saved release: %+v", tc.title, groups)
		}
	}
}

func TestRemoteMatchingRepairsLegacyLinksOn304AndPreservesManualChoices(t *testing.T) {
	s, cat, _ := testService(t)
	raw := "Grand Theft Auto V / GTA 5 (v1.0.2802/1.64 Online, MULTi13) [FitGirl Repack]"
	server := newFeedServer(t, feedBody(t, "Source", feedEntry{Title: raw, URIs: []string{magnetOf("aa")}}, feedEntry{Title: "Unknown title v1.0", URIs: []string{magnetOf("bb")}}))
	src := addSource(t, s, server.url())
	old, err := cat.EnsureGame("Source garbage", 0)
	if err != nil {
		t.Fatal(err)
	}
	// Saved exact-title matches from older releases can point to source-created
	// personal cards. They must be rechecked against complete server membership.
	s.mu.Lock()
	for _, r := range s.releases[src.ID] {
		id := old.ID
		r.CanonicalGameID = &id
		r.MatchStatus = catalog.StatusMatched
		r.MatchMethod = string(catalog.MethodExactTitle)
	}
	locked := s.releases[src.ID][1]
	locked.Locked = true
	s.mu.Unlock()
	remote := &sourceMatchRemote{callback: func() { s.QueryReleases(ReleaseQuery{SourceID: src.ID}) }}
	cat.SetRemoteCatalog(remote)
	summary, err := s.RefreshSource(src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !summary.NotModified || len(remote.queries) != 1 {
		t.Fatalf("304/query dedup: %+v %v", summary, remote.queries)
	}
	groups := s.GetReleasesForGame("server-gta")
	if len(groups) != 1 || groups[0].Release.MatchMethod != string(catalog.MethodServerTitle) {
		t.Fatalf("official release missing: %+v", groups)
	}
	kept := s.GetReleasesForGame(old.ID)
	if len(kept) != 1 || !kept[0].Release.Locked {
		t.Fatalf("manual choice lost: %+v", kept)
	}
	disk, err := s.store.loadReleases(src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if *disk[0].CanonicalGameID != "server-gta" || disk[0].Title == "Source garbage" {
		t.Fatalf("rematch not durable: %+v", disk[0])
	}
	remote.err = errors.New("temporary server failure")
	if _, err := s.RefreshSource(src.ID); err == nil {
		t.Fatal("network failure hidden")
	}
	if len(s.GetReleasesForGame("server-gta")) != 1 {
		t.Fatal("network failure destroyed binding")
	}
	remote.err = nil
	if _, err := s.RefreshSource(src.ID); err != nil {
		t.Fatal(err)
	}
}

func TestRemoteMatchingPersistenceFailureRollsBackReleases(t *testing.T) {
	s, cat, _ := testService(t)
	server := newFeedServer(t, feedBody(t, "Source", feedEntry{Title: "Grand Theft Auto V v1.0", URIs: []string{magnetOf("aa")}}))
	src := addSource(t, s, server.url())
	before := *s.releases[src.ID][0]
	cat.SetRemoteCatalog(&sourceMatchRemote{})
	path := s.store.releasesPath(src.ID)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RefreshSource(src.ID); err == nil {
		t.Fatal("save failure hidden")
	}
	after := s.releases[src.ID][0]
	if after.CanonicalGameID != before.CanonicalGameID || after.MatchStatus != before.MatchStatus {
		t.Fatalf("memory-only rematch: %+v", after)
	}
}

func TestRemoteMatchingDeduplicatesVersions(t *testing.T) {
	s, cat, _ := testService(t)
	remote := &sourceMatchRemote{}
	cat.SetRemoteCatalog(remote)
	server := newFeedServer(t, feedBody(t, "Source", feedEntry{Title: "Grand Theft Auto V v1.0", URIs: []string{magnetOf("aa")}}, feedEntry{Title: "Grand Theft Auto V v2.0", URIs: []string{magnetOf("bb")}}))
	src := addSource(t, s, server.url())
	if len(remote.queries) != 1 || len(s.GetReleasesForGame("server-gta")) != 2 {
		t.Fatalf("queries=%d releases=%+v", len(remote.queries), s.QueryReleases(ReleaseQuery{SourceID: src.ID}))
	}
}

func TestLegacySourceRefetchesAndPersistsGameHint(t *testing.T) {
	s, cat, _ := testService(t)
	body := `{"name":"Hint source","version":1,"downloads":[{"title":"Distribution label v1.0","game":"Grand Theft Auto V","uris":["` + magnetOf("cc") + `"]}]}`
	server := newFeedServer(t, body)
	src := addSource(t, s, server.url())
	s.mu.Lock()
	s.findLocked(src.ID).ParseVersion = 0
	s.releases[src.ID][0].GameHint = ""
	s.mu.Unlock()
	remote := &sourceMatchRemote{}
	cat.SetRemoteCatalog(remote)
	summary, err := s.RefreshSource(src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if summary.NotModified || len(remote.queries) != 1 || remote.queries[0].Titles[0] != "Grand Theft Auto V" {
		t.Fatalf("legacy hint not refetched: %+v %v", summary, remote.queries)
	}
	if r := s.releases[src.ID][0]; r.GameHint != "Grand Theft Auto V" || r.CanonicalGameID == nil {
		t.Fatalf("game hint not saved: %+v", r)
	}
}

func TestSourcePreviewUsesServerCoverageWithoutCreatingLocalCards(t *testing.T) {
	s, cat, _ := testService(t)
	remote := &sourceMatchRemote{}
	cat.SetRemoteCatalog(remote)
	server := newFeedServer(t, feedBody(t, "Source", feedEntry{Title: "Grand Theft Auto V v1.0", URIs: []string{magnetOf("aa")}}, feedEntry{Title: "Grand Theft Auto V v2.0", URIs: []string{magnetOf("bb")}}, feedEntry{Title: "Unknown name", URIs: []string{magnetOf("cc")}}))
	p, err := s.TestSource(server.url())
	if err != nil {
		t.Fatal(err)
	}
	if p.Games != 2 || p.Known != 1 || p.Unknown != 1 {
		t.Fatalf("server preview: %+v", p)
	}
	if len(cat.ListGames()) != 0 {
		t.Fatal("preview created local games")
	}
}
