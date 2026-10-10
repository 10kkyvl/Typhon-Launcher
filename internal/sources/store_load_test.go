package sources

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func fileBytes(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestMissingReleasesFileStartsWithNoReleases(t *testing.T) {
	dir := t.TempDir()
	st := newStore(dir)
	if err := st.saveSources([]Source{{ID: "src-1", Name: "Feed", URL: "https://example.test/feed.json"}}); err != nil {
		t.Fatal(err)
	}
	before := fileBytes(t, st.sourcesPath())

	s, err := newServiceAt(dir, nil, nil)
	if err != nil {
		t.Fatalf("a source without a releases file must still start: %v", err)
	}
	details, err := s.GetSourceDetails("src-1")
	if err != nil {
		t.Fatal(err)
	}
	if details.Total != 0 {
		t.Fatalf("releases = %d, want none", details.Total)
	}
	if got := fileBytes(t, st.sourcesPath()); got != before {
		t.Fatalf("starting rewrote sources.json: %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "releases")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("starting created the releases folder: %v", err)
	}
}

func TestUnreadableStatePathsFailStartup(t *testing.T) {
	cases := []struct {
		name string
		path func(*store) string
	}{
		{"sources file is a folder", func(st *store) string { return st.sourcesPath() }},
		{"releases file is a folder", func(st *store) string { return st.releasesPath("src-1") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			st := newStore(dir)
			if err := st.saveSources([]Source{{ID: "src-1", Name: "Feed", URL: "https://example.test/feed.json"}}); err != nil {
				t.Fatal(err)
			}
			if err := st.saveReleases("src-1", []*Release{{ID: "r1", SourceID: "src-1", RawTitle: "Game A"}}); err != nil {
				t.Fatal(err)
			}
			target := tc.path(st)
			if err := os.Remove(target); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(target, 0o755); err != nil {
				t.Fatal(err)
			}

			s, err := newServiceAt(dir, nil, nil)
			if err == nil {
				t.Fatalf("an unreadable state file was treated as an empty one: %+v", s.ListSources())
			}
			if errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("an unreadable state file was reported as missing: %v", err)
			}
		})
	}
}

func TestDamagedSourcesFileFailsStartupAndStaysUntouched(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"empty file", ""},
		{"nul bytes after a torn write", "\x00\x00\x00\x00\x00\x00\x00\x00"},
		{"utf-8 byte order mark", "\xef\xbb\xbf{\"version\":1,\"data\":[]}"},
		{"newer format than this build reads", `{"version":9,"data":[]}`},
		{"object where a list belongs", `{"version":1,"data":{"id":"src-1"}}`},
		{"trailing garbage", `{"version":1,"data":[]} tail`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "sources.json")
			if err := os.WriteFile(path, []byte(tc.raw), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := newServiceAt(dir, nil, nil); err == nil {
				t.Fatal("a damaged sources file must not produce a service")
			}
			if got := fileBytes(t, path); got != tc.raw {
				t.Fatalf("file rewritten: %q", got)
			}
		})
	}
}

func TestStartingFromGoodStateWritesNothing(t *testing.T) {
	dir := t.TempDir()
	s := mustServiceAt(t, dir, mustCatalog(t, t.TempDir()))
	server := newFeedServer(t, feedBody(t, "Feed",
		feedEntry{Title: "Game A", URIs: []string{magnetOf("a")}},
		feedEntry{Title: "Game B", URIs: []string{magnetOf("b")}},
	))
	src := addSource(t, s, server.url())

	sourcesFile := s.store.sourcesPath()
	releasesFile := s.store.releasesPath(src.ID)
	beforeSources, beforeReleases := fileBytes(t, sourcesFile), fileBytes(t, releasesFile)

	restarted := mustServiceAt(t, dir, nil)
	if got := len(restarted.ListSources()); got != 1 {
		t.Fatalf("sources after restart = %d", got)
	}
	if got := fileBytes(t, sourcesFile); got != beforeSources {
		t.Fatal("starting rewrote sources.json")
	}
	if got := fileBytes(t, releasesFile); got != beforeReleases {
		t.Fatal("starting rewrote the releases file")
	}
}

func TestSourceStateSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	cat := mustCatalog(t, t.TempDir())
	s := mustServiceAt(t, dir, cat)
	server := newFeedServer(t, feedBody(t, "Feed",
		feedEntry{Title: "Game A", URIs: []string{magnetOf("a")}},
		feedEntry{Title: "Game B", URIs: []string{magnetOf("b")}},
	))
	src := addSource(t, s, server.url())

	releases := releasesOf(t, s, src.ID, "all")
	if len(releases) != 2 {
		t.Fatalf("releases = %d", len(releases))
	}
	ignoredID := releases[0].Release.ID
	if err := s.IgnoreRelease(ignoredID, true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSourceEnabled(src.ID, false); err != nil {
		t.Fatal(err)
	}
	server.fail(500)
	if _, err := s.RefreshSource(src.ID); err == nil {
		t.Fatal("expected the refresh to fail")
	}
	want, err := s.GetSource(src.ID)
	if err != nil {
		t.Fatal(err)
	}

	restarted := mustServiceAt(t, dir, cat)
	got, err := restarted.GetSource(src.ID)
	if err != nil {
		t.Fatalf("source lost across a restart: %v", err)
	}
	if got.Enabled || got.Status != StatusDisabled {
		t.Fatalf("enabled=%v status=%q after restart, want the disabled state kept", got.Enabled, got.Status)
	}
	if got.Health != HealthError || got.LastError == "" {
		t.Fatalf("health=%q lastError=%q after restart, want the failure kept", got.Health, got.LastError)
	}
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("source after restart = %s, want %s", gotJSON, wantJSON)
	}
	release, err := restarted.GetRelease(ignoredID)
	if err != nil {
		t.Fatalf("release lost across a restart: %v", err)
	}
	if !release.Release.Ignored {
		t.Fatal("the ignore flag did not survive a restart")
	}
	details, err := restarted.GetSourceDetails(src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if details.Total != 2 || details.Ignored != 1 || details.Available != 1 {
		t.Fatalf("details after restart = %+v, want 2 releases of which 1 ignored", details)
	}
}

func TestRemovedSourceStaysGoneAfterRestart(t *testing.T) {
	dir := t.TempDir()
	s := mustServiceAt(t, dir, mustCatalog(t, t.TempDir()))
	server := newFeedServer(t, feedBody(t, "Feed", feedEntry{Title: "Game A", URIs: []string{magnetOf("a")}}))
	kept := addSource(t, s, server.url())
	other := newFeedServer(t, feedBody(t, "Other", feedEntry{Title: "Game B", URIs: []string{magnetOf("b")}}))
	removed := addSource(t, s, other.url())

	if err := s.RemoveSource(removed.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.store.releasesPath(removed.ID)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("releases file of the removed source is still there: %v", err)
	}

	restarted := mustServiceAt(t, dir, nil)
	list := restarted.ListSources()
	if len(list) != 1 || list[0].ID != kept.ID {
		t.Fatalf("sources after restart = %+v, want only the kept one", list)
	}
	if got := len(releasesOf(t, restarted, "", "all")); got != 1 {
		t.Fatalf("releases after restart = %d, want the kept source's one", got)
	}
}

func TestNewServiceAtBuildsAnIsolatedService(t *testing.T) {
	if _, err := NewServiceAt("", nil); err == nil {
		t.Fatal("an empty folder must not produce a service")
	}

	dir := t.TempDir()
	s, err := NewServiceAt(dir, nil)
	if err != nil {
		t.Fatalf("NewServiceAt: %v", err)
	}
	if got := s.ListSources(); len(got) != 0 {
		t.Fatalf("sources = %+v, want none", got)
	}
	if s.settings != nil {
		t.Fatal("an isolated service must not read the desktop settings")
	}
	if _, err := os.Stat(filepath.Join(dir, "sources.json")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("building the service wrote state: %v", err)
	}
}
