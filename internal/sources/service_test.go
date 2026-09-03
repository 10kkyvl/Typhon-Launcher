package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"typhon/internal/catalog"
	"typhon/internal/sources/feed"
)

func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	os.Exit(m.Run())
}

func magnetOf(seed string) string {
	hash := seed
	for len(hash) < 40 {
		hash += "0"
	}
	return "magnet:?xt=urn:btih:" + hash[:40] + "&dn=test"
}

type feedEntry struct {
	Title      string   `json:"title"`
	URIs       []string `json:"uris"`
	UploadDate string   `json:"uploadDate,omitempty"`
	FileSize   int64    `json:"fileSize"`
}

func feedBody(t *testing.T, name string, entries ...feedEntry) string {
	t.Helper()
	payload := map[string]any{"name": name, "version": 1, "downloads": entries}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

type feedServer struct {
	mu     sync.Mutex
	body   string
	etag   string
	status int
	hits   int
	server *httptest.Server
}

func newFeedServer(t *testing.T, body string) *feedServer {
	t.Helper()
	fs := &feedServer{body: body, etag: `"v1"`}
	fs.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fs.mu.Lock()
		body, etag, status := fs.body, fs.etag, fs.status
		fs.hits++
		fs.mu.Unlock()

		if status != 0 {
			http.Error(w, "boom", status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", etag)
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		fmt.Fprint(w, body)
	}))
	t.Cleanup(fs.server.Close)
	return fs
}

func (fs *feedServer) set(body, etag string) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.body = body
	fs.etag = etag
}

func (fs *feedServer) fail(status int) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.status = status
}

func (fs *feedServer) count() int {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.hits
}

func (fs *feedServer) url() string { return fs.server.URL + "/feed.json" }

func markStale(t *testing.T, s *Service, id string) {
	t.Helper()
	stale := time.Now().Add(-24 * time.Hour)
	s.mu.Lock()
	defer s.mu.Unlock()
	src := s.findLocked(id)
	if src == nil {
		t.Fatalf("source %s not found", id)
	}
	src.LastUpdatedAt = &stale
}

func mustCatalog(t testing.TB, dir string) *catalog.Service {
	t.Helper()
	cat, err := catalog.NewServiceAt(dir)
	if err != nil {
		t.Fatalf("new catalog service at %s: %v", dir, err)
	}
	return cat
}

func mustServiceAt(t testing.TB, dir string, cat *catalog.Service) *Service {
	t.Helper()
	s, err := newServiceAt(dir, nil, cat)
	if err != nil {
		t.Fatalf("new sources service at %s: %v", dir, err)
	}
	// The shipped client refuses loopback, which is where httptest listens.
	s.client = &http.Client{Timeout: feed.FetchTimeout}
	return s
}

func testService(t *testing.T) (*Service, *catalog.Service, string) {
	t.Helper()
	dir := t.TempDir()
	cat := mustCatalog(t, dir)
	return mustServiceAt(t, dir, cat), cat, dir
}

func addSource(t *testing.T, s *Service, url string) Source {
	t.Helper()
	src, err := s.AddSource(url)
	if err != nil {
		t.Fatalf("add source: %v", err)
	}
	return src
}

func releasesOf(t *testing.T, s *Service, sourceID, status string) []ReleaseView {
	t.Helper()
	page := s.QueryReleases(ReleaseQuery{SourceID: sourceID, Status: status, PageSize: maxPageSize})
	return page.Items
}

func TestAddSourceImportsReleases(t *testing.T) {
	s, _, _ := testService(t)
	server := newFeedServer(t, feedBody(t, "Example Source",
		feedEntry{Title: "Cyberpunk.2077.Ultimate.Edition.v2.31", URIs: []string{magnetOf("aa")}, FileSize: 82 << 30},
		feedEntry{Title: "The.Witcher.3.Wild.Hunt.Complete.Edition.v4.04", URIs: []string{magnetOf("bb")}, FileSize: 50 << 30},
	))

	src := addSource(t, s, server.url())
	if src.Name != "Example Source" {
		t.Fatalf("name = %q, want feed name", src.Name)
	}
	if src.Status != StatusActive || src.Health != HealthHealthy {
		t.Fatalf("status = %q health = %q", src.Status, src.Health)
	}
	if src.Entries != 2 || src.Matched != 2 {
		t.Fatalf("entries = %d matched = %d, want 2/2", src.Entries, src.Matched)
	}

	items := releasesOf(t, s, src.ID, "all")
	if len(items) != 2 {
		t.Fatalf("releases = %d, want 2", len(items))
	}
	if items[0].SourceName != "Example Source" || items[0].GameTitle == "" {
		t.Fatalf("release view = %+v", items[0])
	}
}

func TestAddSourceRejectsDuplicateURL(t *testing.T) {
	s, _, _ := testService(t)
	server := newFeedServer(t, feedBody(t, "Example", feedEntry{Title: "Some Game v1.0", URIs: []string{magnetOf("aa")}}))
	addSource(t, s, server.url())

	if _, err := s.AddSource(server.url()); err == nil {
		t.Fatal("expected duplicate url to be rejected")
	}
}

func TestAddSourceRejectsBadScheme(t *testing.T) {
	s, _, _ := testService(t)
	if _, err := s.AddSource("ftp://example.com/feed.json"); err == nil {
		t.Fatal("expected scheme rejection")
	}
	if len(s.ListSources()) != 0 {
		t.Fatal("source must not be stored")
	}
}

func TestObviousTitleMatchesCatalogGame(t *testing.T) {
	s, cat, _ := testService(t)
	game, err := cat.AddGame(catalog.Game{Title: "Cyberpunk 2077"})
	if err != nil {
		t.Fatal(err)
	}
	server := newFeedServer(t, feedBody(t, "Example",
		feedEntry{Title: "Cyberpunk.2077.Ultimate.Edition.v2.31.MULTi19", URIs: []string{magnetOf("aa")}},
	))

	src := addSource(t, s, server.url())
	items := releasesOf(t, s, src.ID, "all")
	if len(items) != 1 {
		t.Fatalf("releases = %d, want 1", len(items))
	}
	r := items[0].Release
	if r.CanonicalGameID == nil || *r.CanonicalGameID != game.ID {
		t.Fatalf("canonical game = %v, want %s", r.CanonicalGameID, game.ID)
	}
	if r.MatchStatus != catalog.StatusMatched || r.MatchConfidence < catalog.AutoThreshold {
		t.Fatalf("match = %q %f", r.MatchStatus, r.MatchConfidence)
	}
	if len(cat.ListGames()) != 1 {
		t.Fatalf("catalog games = %d, want no provisional duplicates", len(cat.ListGames()))
	}
}

func TestAmbiguousTitleGoesToReview(t *testing.T) {
	s, cat, _ := testService(t)
	first, second := 2006, 2017
	if _, err := cat.AddGame(catalog.Game{Title: "Prey", ReleaseYear: &first}); err != nil {
		t.Fatal(err)
	}
	if _, err := cat.AddGame(catalog.Game{Title: "Prey", ReleaseYear: &second}); err != nil {
		t.Fatal(err)
	}
	server := newFeedServer(t, feedBody(t, "Example",
		feedEntry{Title: "Prey.MULTi9.v1.0", URIs: []string{magnetOf("aa")}},
	))

	src := addSource(t, s, server.url())
	if src.Review != 1 {
		t.Fatalf("review = %d, want 1", src.Review)
	}
	items := releasesOf(t, s, src.ID, "review")
	if len(items) != 1 || items[0].Release.CanonicalGameID != nil {
		t.Fatalf("review release = %+v", items)
	}

	candidates, err := s.GetCandidates(items[0].Release.ID)
	if err != nil {
		t.Fatalf("candidates: %v", err)
	}
	if len(candidates) < 2 {
		t.Fatalf("candidates = %d, want at least 2", len(candidates))
	}
}

func TestConfirmMatchIsRememberedOnRefresh(t *testing.T) {
	s, cat, _ := testService(t)
	game, err := cat.AddGame(catalog.Game{Title: "Cyberpunk 2077"})
	if err != nil {
		t.Fatal(err)
	}
	server := newFeedServer(t, feedBody(t, "Example",
		feedEntry{Title: "CP2077 Ultimate v2.31", URIs: []string{magnetOf("aa")}},
	))

	src := addSource(t, s, server.url())
	items := releasesOf(t, s, src.ID, "all")
	if len(items) != 1 {
		t.Fatalf("releases = %d", len(items))
	}
	if items[0].Release.CanonicalGameID != nil && *items[0].Release.CanonicalGameID == game.ID {
		t.Fatal("release should not match automatically before manual confirmation")
	}

	if err := s.ConfirmMatch(items[0].Release.ID, game.ID); err != nil {
		t.Fatalf("confirm match: %v", err)
	}
	confirmed := releasesOf(t, s, src.ID, "all")[0].Release
	if confirmed.CanonicalGameID == nil || *confirmed.CanonicalGameID != game.ID || !confirmed.Locked {
		t.Fatalf("release after confirm = %+v", confirmed)
	}

	server.set(feedBody(t, "Example",
		feedEntry{Title: "CP2077 Ultimate v2.31", URIs: []string{magnetOf("aa")}},
		feedEntry{Title: "CP2077 Ultimate v2.31", URIs: []string{magnetOf("bb")}},
	), `"v2"`)
	if _, err := s.RefreshSource(src.ID); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	for _, item := range releasesOf(t, s, src.ID, "all") {
		if item.Release.CanonicalGameID == nil || *item.Release.CanonicalGameID != game.ID {
			t.Fatalf("release %q should be matched automatically: %+v", item.Release.RawTitle, item.Release)
		}
	}
}

func TestDuplicateInfoHashAcrossSources(t *testing.T) {
	s, _, _ := testService(t)
	shared := magnetOf("dd")
	first := newFeedServer(t, feedBody(t, "First", feedEntry{Title: "Shared Game v1.0", URIs: []string{shared}}))
	second := newFeedServer(t, feedBody(t, "Second", feedEntry{Title: "Shared Game v1.0", URIs: []string{shared}}))

	addSource(t, s, first.url())
	addSource(t, s, second.url())

	items := releasesOf(t, s, "", "all")
	if len(items) != 2 {
		t.Fatalf("releases = %d, want 2", len(items))
	}
	gameID := items[0].Release.CanonicalGameID
	if gameID == nil {
		t.Fatal("release without canonical game")
	}

	groups := s.GetReleasesForGame(*gameID)
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1 deduplicated group", len(groups))
	}
	if len(groups[0].Duplicates) != 1 {
		t.Fatalf("duplicates = %d, want 1", len(groups[0].Duplicates))
	}
	if groups[0].SourceName == groups[0].Duplicates[0].SourceName {
		t.Fatal("duplicate should come from the other source")
	}
}

func TestDifferentInfoHashesAreNotMerged(t *testing.T) {
	s, _, _ := testService(t)
	server := newFeedServer(t, feedBody(t, "Example",
		feedEntry{Title: "Shared Game v1.0", URIs: []string{magnetOf("aa")}},
		feedEntry{Title: "Shared Game v1.0", URIs: []string{magnetOf("bb")}},
	))
	src := addSource(t, s, server.url())

	items := releasesOf(t, s, src.ID, "all")
	if len(items) != 2 {
		t.Fatalf("releases = %d, want 2", len(items))
	}
	groups := s.GetReleasesForGame(*items[0].Release.CanonicalGameID)
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2 separate torrents", len(groups))
	}
}

func TestNewReleaseDetection(t *testing.T) {
	s, _, _ := testService(t)
	server := newFeedServer(t, feedBody(t, "Example",
		feedEntry{Title: "Game One v1.0", URIs: []string{magnetOf("aa")}},
	))
	src := addSource(t, s, server.url())

	for _, item := range releasesOf(t, s, src.ID, "all") {
		if item.Release.New {
			t.Fatal("initial import must not mark releases as new")
		}
	}

	server.set(feedBody(t, "Example",
		feedEntry{Title: "Game One v1.0", URIs: []string{magnetOf("aa")}},
		feedEntry{Title: "Game One v1.1", URIs: []string{magnetOf("bb")}},
	), `"v2"`)
	summary, err := s.RefreshSource(src.ID)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if summary.Added != 1 || summary.New != 1 {
		t.Fatalf("summary = %+v, want 1 added and 1 new", summary)
	}

	fresh := releasesOf(t, s, src.ID, "new")
	if len(fresh) != 1 || fresh[0].Release.RawTitle != "Game One v1.1" {
		t.Fatalf("new releases = %+v", fresh)
	}

	if err := s.AcknowledgeNew(src.ID); err != nil {
		t.Fatalf("acknowledge: %v", err)
	}
	if len(releasesOf(t, s, src.ID, "new")) != 0 {
		t.Fatal("new flag should be cleared")
	}
}

func TestRemovedReleaseIsKept(t *testing.T) {
	s, _, _ := testService(t)
	server := newFeedServer(t, feedBody(t, "Example",
		feedEntry{Title: "Game One v1.0", URIs: []string{magnetOf("aa")}},
		feedEntry{Title: "Game Two v1.0", URIs: []string{magnetOf("bb")}},
	))
	src := addSource(t, s, server.url())
	before := releasesOf(t, s, src.ID, "all")
	ids := map[string]string{}
	for _, item := range before {
		ids[item.Release.RawTitle] = item.Release.ID
	}

	server.set(feedBody(t, "Example",
		feedEntry{Title: "Game One v1.0", URIs: []string{magnetOf("aa")}},
	), `"v2"`)
	if _, err := s.RefreshSource(src.ID); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	removed := releasesOf(t, s, src.ID, "removed")
	if len(removed) != 1 {
		t.Fatalf("removed releases = %d, want 1", len(removed))
	}
	if removed[0].Release.ID != ids["Game Two v1.0"] {
		t.Fatal("removed release must keep its identifier")
	}
	if _, err := s.GetRelease(removed[0].Release.ID); err != nil {
		t.Fatalf("removed release must stay resolvable: %v", err)
	}
	all := releasesOf(t, s, src.ID, "all")
	if len(all) != 2 {
		t.Fatalf("all releases = %d, want 2 including the unavailable one", len(all))
	}
}

func TestConditionalRefreshSkipsReparse(t *testing.T) {
	s, _, _ := testService(t)
	server := newFeedServer(t, feedBody(t, "Example",
		feedEntry{Title: "Game One v1.0", URIs: []string{magnetOf("aa")}},
	))
	src := addSource(t, s, server.url())

	summary, err := s.RefreshSource(src.ID)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if !summary.NotModified {
		t.Fatalf("summary = %+v, want notModified", summary)
	}
	if summary.Added != 0 || summary.Removed != 0 {
		t.Fatalf("summary = %+v, want no changes", summary)
	}
	if len(releasesOf(t, s, src.ID, "all")) != 1 {
		t.Fatal("releases must survive a 304 refresh")
	}
}

func TestRefreshFailureKeepsReleases(t *testing.T) {
	s, _, _ := testService(t)
	server := newFeedServer(t, feedBody(t, "Example",
		feedEntry{Title: "Game One v1.0", URIs: []string{magnetOf("aa")}},
	))
	src := addSource(t, s, server.url())

	server.fail(http.StatusInternalServerError)
	if _, err := s.RefreshSource(src.ID); err == nil {
		t.Fatal("expected refresh error")
	}

	stored, err := s.GetSource(src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != StatusError || stored.Health != HealthError || stored.LastError == "" {
		t.Fatalf("source = %+v, want error state", stored)
	}
	if len(releasesOf(t, s, src.ID, "all")) != 1 {
		t.Fatal("releases must survive a failed refresh")
	}
}

func TestRefreshKeepsReleasesWhenSaveFails(t *testing.T) {
	dir := t.TempDir()
	cat := mustCatalog(t, dir)
	s := mustServiceAt(t, dir, cat)
	server := newFeedServer(t, feedBody(t, "Example",
		feedEntry{Title: "Game One v1.0", URIs: []string{magnetOf("aa")}},
	))
	src := addSource(t, s, server.url())

	releasesPath := s.store.releasesPath(src.ID)
	if err := os.Remove(releasesPath); err != nil {
		t.Fatalf("remove releases file: %v", err)
	}
	if err := os.Mkdir(releasesPath, 0o755); err != nil {
		t.Fatalf("block releases path: %v", err)
	}

	server.set(feedBody(t, "Example",
		feedEntry{Title: "Game One v1.0", URIs: []string{magnetOf("aa")}},
		feedEntry{Title: "Game Two v1.0", URIs: []string{magnetOf("bb")}},
	), `"v2"`)

	summary, err := s.RefreshSource(src.ID)
	if err == nil {
		t.Fatal("RefreshSource() error = nil, want the save failure")
	}
	if summary.Error == "" {
		t.Fatalf("summary = %+v, want it to carry the error", summary)
	}

	items := releasesOf(t, s, src.ID, "all")
	if len(items) != 1 {
		t.Fatalf("releases = %d, want the previous single release kept", len(items))
	}

	stored, statErr := s.GetSource(src.ID)
	if statErr != nil {
		t.Fatal(statErr)
	}
	if stored.Health != HealthError || stored.LastError == "" {
		t.Fatalf("source = %+v, want error health after the failed save", stored)
	}
}

func TestInvalidEntriesAreSkipped(t *testing.T) {
	s, _, _ := testService(t)
	body := `{"name":"Example","version":1,"downloads":[
		{"title":"Good Game v1.0","uris":["` + magnetOf("aa") + `"],"fileSize":100},
		{"title":"","uris":["` + magnetOf("bb") + `"]},
		{"title":"No links here","uris":[]},
		{"title":"Http only","uris":["https://example.com/file.zip"]}
	]}`
	server := newFeedServer(t, body)

	src := addSource(t, s, server.url())
	if src.Entries != 1 {
		t.Fatalf("entries = %d, want 1", src.Entries)
	}
	if src.Invalid == 0 {
		t.Fatal("invalid entries must be counted")
	}
	if src.Health != HealthWarning {
		t.Fatalf("health = %q, want warning", src.Health)
	}
	if src.Status != StatusActive {
		t.Fatalf("status = %q, want active", src.Status)
	}
}

func TestReleasesSurviveRestart(t *testing.T) {
	dir := t.TempDir()
	cat := mustCatalog(t, dir)
	s := mustServiceAt(t, dir, cat)
	server := newFeedServer(t, feedBody(t, "Example",
		feedEntry{Title: "Game One v1.0", URIs: []string{magnetOf("aa")}},
	))
	src := addSource(t, s, server.url())

	restarted := mustServiceAt(t, dir, mustCatalog(t, dir))
	sources := restarted.ListSources()
	if len(sources) != 1 || sources[0].ID != src.ID {
		t.Fatalf("sources after restart = %+v", sources)
	}
	if len(releasesOf(t, restarted, src.ID, "all")) != 1 {
		t.Fatal("releases must be restored from disk")
	}
}

func TestRemoveSourceDropsReleases(t *testing.T) {
	s, _, _ := testService(t)
	server := newFeedServer(t, feedBody(t, "Example",
		feedEntry{Title: "Game One v1.0", URIs: []string{magnetOf("aa")}},
	))
	src := addSource(t, s, server.url())

	if err := s.RemoveSource(src.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if len(s.ListSources()) != 0 {
		t.Fatal("source must be gone")
	}
	if len(releasesOf(t, s, "", "all")) != 0 {
		t.Fatal("releases must be gone")
	}
}

func TestQueryReleasesPaginationAndSearch(t *testing.T) {
	s, _, _ := testService(t)
	entries := make([]feedEntry, 0, 120)
	for i := 0; i < 120; i++ {
		entries = append(entries, feedEntry{
			Title:    fmt.Sprintf("Sample Game %03d v1.0", i),
			URIs:     []string{magnetOf(fmt.Sprintf("%04x", i))},
			FileSize: int64(i) << 20,
		})
	}
	server := newFeedServer(t, feedBody(t, "Example", entries...))
	src := addSource(t, s, server.url())

	page := s.QueryReleases(ReleaseQuery{SourceID: src.ID, Page: 2, PageSize: 50})
	if page.Total != 120 || len(page.Items) != 50 || page.Page != 2 {
		t.Fatalf("page = %+v", ReleasePage{Total: page.Total, Page: page.Page, PageSize: page.PageSize})
	}

	last := s.QueryReleases(ReleaseQuery{SourceID: src.ID, Page: 3, PageSize: 50})
	if len(last.Items) != 20 {
		t.Fatalf("last page items = %d, want 20", len(last.Items))
	}

	found := s.QueryReleases(ReleaseQuery{Search: "Sample Game 042"})
	if found.Total != 1 || !strings.Contains(found.Items[0].Release.RawTitle, "042") {
		t.Fatalf("search result = %+v", found)
	}

	byGame := s.QueryReleases(ReleaseQuery{Search: "sample game 007"})
	if byGame.Total != 1 {
		t.Fatalf("search by matched game title = %d, want 1", byGame.Total)
	}
}

func TestIgnoreRelease(t *testing.T) {
	s, _, _ := testService(t)
	server := newFeedServer(t, feedBody(t, "Example",
		feedEntry{Title: "Game One v1.0", URIs: []string{magnetOf("aa")}},
	))
	src := addSource(t, s, server.url())
	items := releasesOf(t, s, src.ID, "all")

	if err := s.IgnoreRelease(items[0].Release.ID, true); err != nil {
		t.Fatalf("ignore: %v", err)
	}
	if len(releasesOf(t, s, src.ID, "all")) != 0 {
		t.Fatal("ignored release must be hidden")
	}
	if len(releasesOf(t, s, src.ID, "ignored")) != 1 {
		t.Fatal("ignored release must be listed under its own filter")
	}
}

func TestPrepareDownloadCarriesProvenance(t *testing.T) {
	s, _, _ := testService(t)
	server := newFeedServer(t, feedBody(t, "Example",
		feedEntry{Title: "Game One v1.0", URIs: []string{magnetOf("aa")}},
	))
	src := addSource(t, s, server.url())
	release := releasesOf(t, s, src.ID, "all")[0].Release

	request, err := s.PrepareDownload(release.ID)
	if err != nil {
		t.Fatalf("prepare download: %v", err)
	}
	if request.URI != release.URIs[0] {
		t.Fatalf("uri = %q, want %q", request.URI, release.URIs[0])
	}
	if request.ReleaseID != release.ID || request.SourceID != src.ID {
		t.Fatalf("request = %+v", request)
	}
	if request.GameID == "" || release.CanonicalGameID == nil || request.GameID != *release.CanonicalGameID {
		t.Fatalf("game id = %q, want %v", request.GameID, release.CanonicalGameID)
	}
}

func TestLargeFeedImport(t *testing.T) {
	if testing.Short() {
		t.Skip("large feed import")
	}
	s, cat, _ := testService(t)
	const total = 10000
	entries := make([]feedEntry, 0, total)
	for i := 0; i < total; i++ {
		entries = append(entries, feedEntry{
			Title:    fmt.Sprintf("Bulk Game %05d v1.%d", i, i%9),
			URIs:     []string{magnetOf(fmt.Sprintf("%05x", i))},
			FileSize: int64(i) << 20,
		})
	}
	server := newFeedServer(t, feedBody(t, "Bulk", entries...))

	started := time.Now()
	src := addSource(t, s, server.url())
	elapsed := time.Since(started)

	if src.Entries != total {
		t.Fatalf("entries = %d, want %d", src.Entries, total)
	}
	if src.Matched != total {
		t.Fatalf("matched = %d, want %d", src.Matched, total)
	}
	if len(cat.ListGames()) != total {
		t.Fatalf("catalog games = %d, want %d", len(cat.ListGames()), total)
	}
	if elapsed > 60*time.Second {
		t.Fatalf("import took %s, too slow", elapsed)
	}

	page := s.QueryReleases(ReleaseQuery{SourceID: src.ID, Page: 1, PageSize: 50})
	if page.Total != total || len(page.Items) != 50 {
		t.Fatalf("page = total %d items %d", page.Total, len(page.Items))
	}

	refreshStarted := time.Now()
	if _, err := s.RefreshSource(src.ID); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if refreshElapsed := time.Since(refreshStarted); refreshElapsed > 30*time.Second {
		t.Fatalf("refresh took %s, too slow", refreshElapsed)
	}
}

func TestScheduledRefreshBacksOffAfterFailure(t *testing.T) {
	s, _, _ := testService(t)
	server := newFeedServer(t, feedBody(t, "Example",
		feedEntry{Title: "Game One v1.0", URIs: []string{magnetOf("aa")}},
	))
	src := addSource(t, s, server.url())
	markStale(t, s, src.ID)

	server.fail(http.StatusInternalServerError)
	before := server.count()

	s.refreshDue()
	if got := server.count(); got != before+1 {
		t.Fatalf("hits = %d, want %d", got, before+1)
	}

	s.refreshDue()
	s.refreshDue()
	if got := server.count(); got != before+1 {
		t.Fatalf("hits = %d, want %d: failing source must wait for its backoff", got, before+1)
	}
}

func TestScheduledRefreshResumesAfterBackoff(t *testing.T) {
	s, _, _ := testService(t)
	server := newFeedServer(t, feedBody(t, "Example",
		feedEntry{Title: "Game One v1.0", URIs: []string{magnetOf("aa")}},
	))
	src := addSource(t, s, server.url())
	markStale(t, s, src.ID)

	server.fail(http.StatusInternalServerError)
	s.refreshDue()

	s.mu.Lock()
	s.retryAt[src.ID] = time.Now().Add(-time.Second)
	s.mu.Unlock()

	server.fail(0)
	before := server.count()
	s.refreshDue()
	if got := server.count(); got != before+1 {
		t.Fatalf("hits = %d, want %d: expired backoff must allow a retry", got, before+1)
	}

	stored, err := s.GetSource(src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.LastError != "" || stored.Health == HealthError {
		t.Fatalf("source = %+v, want recovered state", stored)
	}

	s.mu.Lock()
	_, pending := s.retryAt[src.ID]
	failures := s.failures[src.ID]
	s.mu.Unlock()
	if pending || failures != 0 {
		t.Fatalf("backoff = %v, failures = %d, want cleared after success", pending, failures)
	}
}

func TestManualFailureArmsBackoff(t *testing.T) {
	s, _, _ := testService(t)
	server := newFeedServer(t, feedBody(t, "Example",
		feedEntry{Title: "Game One v1.0", URIs: []string{magnetOf("aa")}},
	))
	src := addSource(t, s, server.url())
	server.fail(http.StatusInternalServerError)

	if _, err := s.RefreshSource(src.ID); err == nil {
		t.Fatal("expected refresh error")
	}
	s.mu.Lock()
	failures := s.failures[src.ID]
	s.mu.Unlock()
	if failures != 1 {
		t.Fatalf("failures = %d, want 1", failures)
	}
}

func TestRetryDelayGrowsAndIsCapped(t *testing.T) {
	cases := []struct {
		failures int
		interval time.Duration
		want     time.Duration
	}{
		{1, 6 * time.Hour, time.Minute},
		{2, 6 * time.Hour, 2 * time.Minute},
		{4, 6 * time.Hour, 8 * time.Minute},
		{20, 6 * time.Hour, maxRetryDelay},
		{20, 10 * time.Minute, 10 * time.Minute},
		{1, 30 * time.Second, 30 * time.Second},
		{20, 0, maxRetryDelay},
	}
	for _, tc := range cases {
		if got := retryDelay(tc.failures, tc.interval); got != tc.want {
			t.Errorf("retryDelay(%d, %v) = %v, want %v", tc.failures, tc.interval, got, tc.want)
		}
	}
}

func TestURLSourceFetchPathsRejectLocalAddresses(t *testing.T) {
	dir := t.TempDir()
	cat := mustCatalog(t, dir)
	s, err := newServiceAt(dir, nil, cat)
	if err != nil {
		t.Fatalf("new sources service at %s: %v", dir, err)
	}
	fs := newFeedServer(t, feedBody(t, "Local", feedEntry{Title: "Game A", URIs: []string{magnetOf("a")}}))

	if _, err := s.TestSource(fs.url()); !errors.Is(err, feed.ErrBlockedAddress) {
		t.Fatalf("TestSource error = %v, want ErrBlockedAddress", err)
	}

	added, err := s.AddSource(fs.url())
	if !errors.Is(err, feed.ErrBlockedAddress) {
		t.Fatalf("AddSource error = %v, want ErrBlockedAddress", err)
	}
	if added.ID == "" {
		t.Fatal("AddSource returned no source to refresh")
	}

	if _, err := s.RefreshSource(added.ID); !errors.Is(err, feed.ErrBlockedAddress) {
		t.Fatalf("RefreshSource error = %v, want ErrBlockedAddress", err)
	}
	if _, err := s.refresh(context.Background(), added.ID, true); !errors.Is(err, feed.ErrBlockedAddress) {
		t.Fatalf("scheduled refresh error = %v, want ErrBlockedAddress", err)
	}
	if got := fs.count(); got != 0 {
		t.Fatalf("feed server saw %d requests, want 0", got)
	}
}

func TestSourceErrorHidesFeedURL(t *testing.T) {
	dir := t.TempDir()
	cat := mustCatalog(t, dir)
	s, err := newServiceAt(dir, nil, cat)
	if err != nil {
		t.Fatalf("new sources service at %s: %v", dir, err)
	}
	const raw = "http://127.0.0.1:9/feed.json?token=s3cret"
	if _, err := s.TestSource(raw); err == nil {
		t.Fatal("expected the fetch to be rejected")
	} else {
		for _, leak := range []string{"s3cret", "token", "/feed.json"} {
			if strings.Contains(err.Error(), leak) {
				t.Fatalf("error leaks %q: %v", leak, err)
			}
		}
	}
}
