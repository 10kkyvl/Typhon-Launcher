package sources

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"typhon/internal/sources/feed"
)

func writeFeedFile(t *testing.T, path, body string) string {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write feed file: %v", err)
	}
	return path
}

func feedFile(t *testing.T, name string, entries ...feedEntry) string {
	t.Helper()
	return writeFeedFile(t, filepath.Join(t.TempDir(), "feed.json"), feedBody(t, name, entries...))
}

func addSourceFile(t *testing.T, s *Service, path string) Source {
	t.Helper()
	src, err := s.AddSourceFile(path)
	if err != nil {
		t.Fatalf("add source file: %v", err)
	}
	return src
}

func TestAddSourceFileImportsReleases(t *testing.T) {
	s, _, _ := testService(t)
	path := feedFile(t, "Local Source",
		feedEntry{Title: "Cyberpunk.2077.Ultimate.Edition.v2.31", URIs: []string{magnetOf("aa")}, FileSize: 82 << 30},
		feedEntry{Title: "The.Witcher.3.Wild.Hunt.Complete.Edition.v4.04", URIs: []string{magnetOf("bb")}, FileSize: 50 << 30},
	)

	src := addSourceFile(t, s, path)
	if src.Type != TypeFile {
		t.Fatalf("type = %q, want %q", src.Type, TypeFile)
	}
	if src.Path != path {
		t.Fatalf("path = %q, want %q", src.Path, path)
	}
	if src.URL != "" {
		t.Fatalf("url = %q, want empty", src.URL)
	}
	if src.Name != "Local Source" {
		t.Fatalf("name = %q, want feed name", src.Name)
	}
	if src.Status != StatusActive || src.Health != HealthHealthy {
		t.Fatalf("status = %q health = %q", src.Status, src.Health)
	}
	if src.Entries != 2 {
		t.Fatalf("entries = %d, want 2", src.Entries)
	}
	if items := releasesOf(t, s, src.ID, "all"); len(items) != 2 {
		t.Fatalf("releases = %d, want 2", len(items))
	}
}

func TestAddSourceFileNamesUnnamedFeedAfterFile(t *testing.T) {
	s, _, _ := testService(t)
	path := filepath.Join(t.TempDir(), "repacks.json")
	writeFeedFile(t, path, `{"version":1,"downloads":[{"title":"Some Game v1.0","uri":"`+magnetOf("aa")+`"}]}`)

	src := addSourceFile(t, s, path)
	if src.Name != "repacks.json" {
		t.Fatalf("name = %q, want file name", src.Name)
	}
}

func TestAddSourceFileRejectsDuplicate(t *testing.T) {
	s, _, _ := testService(t)
	path := feedFile(t, "Local", feedEntry{Title: "Some Game v1.0", URIs: []string{magnetOf("aa")}})
	addSourceFile(t, s, path)

	if _, err := s.AddSourceFile(path); !errors.Is(err, errSourceExists) {
		t.Fatalf("error = %v, want %v", err, errSourceExists)
	}
	if len(s.ListSources()) != 1 {
		t.Fatalf("sources = %d, want 1", len(s.ListSources()))
	}
}

func TestAddSourceFileRejectsBadPath(t *testing.T) {
	s, _, _ := testService(t)
	cases := []struct {
		name string
		path string
		want error
	}{
		{name: "empty", path: "", want: feed.ErrEmptyPath},
		{name: "relative", path: filepath.Join("feeds", "local.json"), want: feed.ErrRelativePath},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := s.AddSourceFile(c.path); !errors.Is(err, c.want) {
				t.Fatalf("error = %v, want %v", err, c.want)
			}
		})
	}
	if len(s.ListSources()) != 0 {
		t.Fatalf("sources = %d, want 0", len(s.ListSources()))
	}
}

func TestAddSourceFileKeepsSourceOnUnreadableFile(t *testing.T) {
	s, _, _ := testService(t)
	path := filepath.Join(t.TempDir(), "broken.json")
	writeFeedFile(t, path, "{not json")

	src, err := s.AddSourceFile(path)
	if err == nil {
		t.Fatal("expected error for broken feed file")
	}
	if src.Status != StatusError || src.LastError == "" {
		t.Fatalf("status = %q lastError = %q", src.Status, src.LastError)
	}
	list := s.ListSources()
	if len(list) != 1 || list[0].Type != TypeFile {
		t.Fatalf("sources = %+v", list)
	}
}

func TestRefreshFileSourceRereadsFile(t *testing.T) {
	s, _, _ := testService(t)
	path := feedFile(t, "Local", feedEntry{Title: "Some Game v1.0", URIs: []string{magnetOf("aa")}})
	src := addSourceFile(t, s, path)

	writeFeedFile(t, path, feedBody(t, "Local",
		feedEntry{Title: "Some Game v1.0", URIs: []string{magnetOf("aa")}},
		feedEntry{Title: "Another Game v2.0", URIs: []string{magnetOf("bb")}},
	))

	summary, err := s.RefreshSource(src.ID)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if summary.Added != 1 || summary.Entries != 2 {
		t.Fatalf("added = %d entries = %d, want 1/2", summary.Added, summary.Entries)
	}
	if summary.NotModified {
		t.Fatal("file source must not report NotModified")
	}
}

func TestRefreshFileSourceFailsWhenFileGone(t *testing.T) {
	s, _, _ := testService(t)
	path := feedFile(t, "Local", feedEntry{Title: "Some Game v1.0", URIs: []string{magnetOf("aa")}})
	src := addSourceFile(t, s, path)
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove feed file: %v", err)
	}

	if _, err := s.RefreshSource(src.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("error = %v, want %v", err, os.ErrNotExist)
	}
	current, err := s.GetSource(src.ID)
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	if current.Status != StatusError || current.Health != HealthError {
		t.Fatalf("status = %q health = %q", current.Status, current.Health)
	}
	if items := releasesOf(t, s, src.ID, "all"); len(items) != 1 {
		t.Fatalf("releases = %d, want kept 1", len(items))
	}
}

func TestFileSourceSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	cat := mustCatalog(t, dir)
	s := mustServiceAt(t, dir, cat)
	path := feedFile(t, "Local", feedEntry{Title: "Some Game v1.0", URIs: []string{magnetOf("aa")}})
	src := addSourceFile(t, s, path)

	restarted := mustServiceAt(t, dir, mustCatalog(t, dir))
	stored, err := restarted.GetSource(src.ID)
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	if stored.Type != TypeFile || stored.Path != path {
		t.Fatalf("stored = %+v", stored)
	}
	if _, err := restarted.RefreshSource(src.ID); err != nil {
		t.Fatalf("refresh after restart: %v", err)
	}
}

func TestLegacySourceLoadsAsURLType(t *testing.T) {
	dir := t.TempDir()
	legacy := `{"version":1,"data":[{"id":"src-1","name":"example.com","url":"https://example.com/feed.json","enabled":true,"status":"active","health":"healthy","createdAt":"2026-01-01T00:00:00Z"}]}`
	writeFeedFile(t, filepath.Join(dir, "sources.json"), legacy)

	s := mustServiceAt(t, dir, mustCatalog(t, dir))
	list := s.ListSources()
	if len(list) != 1 {
		t.Fatalf("sources = %d, want 1", len(list))
	}
	if list[0].Type != TypeURL {
		t.Fatalf("type = %q, want %q", list[0].Type, TypeURL)
	}
}

func TestTestSourceFilePreview(t *testing.T) {
	s, _, _ := testService(t)
	path := feedFile(t, "Local Source", feedEntry{Title: "Some Game v1.0", URIs: []string{magnetOf("aa")}})

	preview, err := s.TestSourceFile(path)
	if err != nil {
		t.Fatalf("test source file: %v", err)
	}
	if preview.Type != TypeFile || preview.Path != path || preview.URL != "" {
		t.Fatalf("preview = %+v", preview)
	}
	if preview.Name != "Local Source" || preview.Entries != 1 {
		t.Fatalf("preview = %+v", preview)
	}
	if preview.Duplicate {
		t.Fatal("preview must not be a duplicate before adding")
	}

	addSourceFile(t, s, path)
	again, err := s.TestSourceFile(strings.ToUpper(path))
	if err != nil {
		t.Fatalf("test source file: %v", err)
	}
	if !again.Duplicate {
		t.Fatal("preview must report duplicate after adding")
	}
}
