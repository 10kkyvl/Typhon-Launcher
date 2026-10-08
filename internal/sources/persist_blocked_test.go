package sources

import (
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/catalog"
)

// The tests in this file make a write fail by occupying a path instead of
// changing permissions, so they run on Windows as well as on Unix. A rename
// onto a directory fails everywhere, but Windows retries it for about half a
// second first, which is why the tests that depend on it run in parallel.

func occupyDir(t *testing.T, dir string) {
	t.Helper()
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func replaceFileWithDir(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func degradedOf(s *Service) degradedStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

func blockedService(t *testing.T) (*Service, *feedServer, Source, string) {
	t.Helper()
	dir := t.TempDir()
	s := mustServiceAt(t, dir, mustCatalog(t, t.TempDir()))
	server := newFeedServer(t, feedBody(t, "Feed", feedEntry{Title: "Game A", URIs: []string{magnetOf("a")}}))
	return s, server, addSource(t, s, server.url()), dir
}

func TestAddSourceRollsBackWhenItCannotBeSaved(t *testing.T) {
	dir := t.TempDir()
	s := mustServiceAt(t, dir, mustCatalog(t, t.TempDir()))
	server := newFeedServer(t, feedBody(t, "Feed", feedEntry{Title: "Game A", URIs: []string{magnetOf("a")}}))
	occupyDir(t, dir)

	if _, err := s.AddSource(server.url()); err == nil {
		t.Fatal("expected the save failure")
	}
	if got := s.ListSources(); len(got) != 0 {
		t.Fatalf("sources after a failed add = %+v", got)
	}
	if n := server.count(); n != 0 {
		t.Fatalf("a source that was never stored was fetched %d times", n)
	}
}

func TestFailRollsBackWhenTheStateFolderIsOccupied(t *testing.T) {
	s, server, src, dir := blockedService(t)
	before, err := s.GetSource(src.ID)
	if err != nil {
		t.Fatal(err)
	}
	server.fail(500)
	occupyDir(t, dir)

	if _, err := s.RefreshSource(src.ID); err == nil {
		t.Fatal("expected the feed fetch to fail")
	}
	after, err := s.GetSource(src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Health != before.Health || after.LastError != before.LastError {
		t.Fatalf("source = %+v, want Health/LastError rolled back to %+v", after, before)
	}
	s.mu.Lock()
	_, hasFailure := s.failures[src.ID]
	_, hasRetry := s.retryAt[src.ID]
	s.mu.Unlock()
	if hasFailure || hasRetry {
		t.Fatalf("backoff recorded for a failure nobody could persist: failures=%v retry=%v", hasFailure, hasRetry)
	}
	if !degradedOf(s).Degraded {
		t.Fatal("the service did not report itself degraded")
	}
}

func TestIgnoreReleaseRollsBackWhenTheStateFolderIsOccupied(t *testing.T) {
	s, _, src, dir := blockedService(t)
	list := releasesOf(t, s, src.ID, "")
	if len(list) != 1 {
		t.Fatalf("releases = %d, want 1", len(list))
	}
	releaseID := list[0].Release.ID
	wasNew := list[0].Release.New
	occupyDir(t, dir)

	if err := s.IgnoreRelease(releaseID, true); err == nil {
		t.Fatal("expected the save failure")
	}
	got, err := s.GetRelease(releaseID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Release.Ignored {
		t.Fatal("the release is ignored in memory but not on disk")
	}
	if got.Release.New != wasNew {
		t.Fatalf("New flag = %v, want the stored %v back", got.Release.New, wasNew)
	}
	if !degradedOf(s).Degraded {
		t.Fatal("the service did not report itself degraded")
	}
}

func TestAcknowledgeNewRollsBackWhenTheStateFolderIsOccupied(t *testing.T) {
	s, server, src, dir := blockedService(t)
	server.set(feedBody(t, "Feed",
		feedEntry{Title: "Game A", URIs: []string{magnetOf("a")}},
		feedEntry{Title: "Game B", URIs: []string{magnetOf("b")}},
	), `"v2"`)
	if _, err := s.RefreshSource(src.ID); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	newBefore := 0
	for _, item := range releasesOf(t, s, src.ID, "all") {
		if item.Release.New {
			newBefore++
		}
	}
	if newBefore == 0 {
		t.Fatal("the refresh produced no new release to acknowledge")
	}
	occupyDir(t, dir)

	if err := s.AcknowledgeNew(src.ID); err == nil {
		t.Fatal("expected the save failure")
	}
	newAfter := 0
	for _, item := range releasesOf(t, s, src.ID, "all") {
		if item.Release.New {
			newAfter++
		}
	}
	if newAfter != newBefore {
		t.Fatalf("new releases in memory = %d, on disk %d", newAfter, newBefore)
	}
}

func TestConfirmMatchRollsBackWhenTheStateFolderIsOccupied(t *testing.T) {
	dir := t.TempDir()
	cat := mustCatalog(t, t.TempDir())
	game, err := cat.AddGame(catalog.Game{Title: "Cyberpunk 2077"})
	if err != nil {
		t.Fatal(err)
	}
	s := mustServiceAt(t, dir, cat)
	server := newFeedServer(t, feedBody(t, "Feed", feedEntry{Title: "CP2077 Ultimate v2.31", URIs: []string{magnetOf("aa")}}))
	src := addSource(t, s, server.url())
	items := releasesOf(t, s, src.ID, "all")
	if len(items) != 1 {
		t.Fatalf("releases = %d", len(items))
	}
	releaseID := items[0].Release.ID
	occupyDir(t, dir)

	if err := s.ConfirmMatch(releaseID, game.ID); err == nil {
		t.Fatal("expected the save failure")
	}
	got, err := s.GetRelease(releaseID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Release.Locked || (got.Release.CanonicalGameID != nil && *got.Release.CanonicalGameID == game.ID) {
		t.Fatalf("the match is confirmed in memory but not on disk: %+v", got.Release)
	}
}

func TestSettleRollsBackWhenTheSourcesFileCannotBeReplaced(t *testing.T) {
	t.Parallel()
	s, server, src, dir := blockedService(t)
	before, err := s.GetSource(src.ID)
	if err != nil {
		t.Fatal(err)
	}
	server.set(feedBody(t, "Feed",
		feedEntry{Title: "Game A", URIs: []string{magnetOf("a")}},
		feedEntry{Title: "Game B", URIs: []string{magnetOf("b")}},
	), `"v2"`)
	replaceFileWithDir(t, filepath.Join(dir, "sources.json"))

	if _, err := s.RefreshSource(src.ID); err == nil {
		t.Fatal("expected RefreshSource to report the sources.json failure")
	}
	after, err := s.GetSource(src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Entries != before.Entries || after.Matched != before.Matched || after.Health != before.Health {
		t.Fatalf("source metadata = %+v, want rollback to %+v", after, before)
	}
	if !degradedOf(s).Degraded {
		t.Fatal("the service did not report itself degraded")
	}
	stored, err := s.store.loadReleases(src.ID)
	if err != nil {
		t.Fatalf("load releases: %v", err)
	}
	if len(stored) != 2 {
		t.Fatalf("releases on disk = %d, want the merge that was already committed (2)", len(stored))
	}
}

func TestIgnoreReleaseKeepsItsSaveWhenOnlyTheCountsCannotBeStored(t *testing.T) {
	t.Parallel()
	s, _, src, dir := blockedService(t)
	list := releasesOf(t, s, src.ID, "")
	releaseID := list[0].Release.ID
	before, err := s.GetSource(src.ID)
	if err != nil {
		t.Fatal(err)
	}
	replaceFileWithDir(t, filepath.Join(dir, "sources.json"))

	if err := s.IgnoreRelease(releaseID, true); err == nil {
		t.Fatal("expected the sources.json failure")
	}
	got, err := s.GetRelease(releaseID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Release.Ignored {
		t.Fatal("the release file was saved and must stay ignored")
	}
	after, err := s.GetSource(src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Matched != before.Matched || after.Review != before.Review || after.Unmatched != before.Unmatched {
		t.Fatalf("counts = %+v, want rollback to %+v", after, before)
	}
	if !degradedOf(s).Degraded {
		t.Fatal("the service did not report itself degraded")
	}
}
