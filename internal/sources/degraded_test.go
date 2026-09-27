package sources

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func skipOnWindowsPermissions(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("read-only directory permissions behave differently on windows")
	}
}

func chmodOrFatal(t *testing.T, dir string, mode os.FileMode) {
	t.Helper()
	if err := os.Chmod(dir, mode); err != nil {
		t.Fatal(err)
	}
}

// TestSettleRollsBackSourceMetadataWhenSourcesPersistFails covers settle's
// final saveSources call: releases.json for the source is written first and,
// on this failure, stays written (invariant I.4 treats each file as its own
// entity), while the Source counters computed from it roll back so
// sources.json is never asked to describe releases it does not agree with.
func TestSettleRollsBackSourceMetadataWhenSourcesPersistFails(t *testing.T) {
	skipOnWindowsPermissions(t)
	dir := t.TempDir()
	// The catalog gets its own directory: sharing dir with it would make
	// applyMatches's Provision() call fail too once dir is read-only, which
	// tests the wrong swallow (fail(), not settle's saveSources).
	cat := mustCatalog(t, t.TempDir())
	s := mustServiceAt(t, dir, cat)

	fs := newFeedServer(t, feedBody(t, "Feed", feedEntry{Title: "Game A", URIs: []string{magnetOf("a")}}))
	src := addSource(t, s, fs.url())
	before, err := s.GetSource(src.ID)
	if err != nil {
		t.Fatalf("get source: %v", err)
	}

	fs.set(feedBody(t, "Feed",
		feedEntry{Title: "Game A", URIs: []string{magnetOf("a")}},
		feedEntry{Title: "Game B", URIs: []string{magnetOf("b")}},
	), `"v2"`)

	chmodOrFatal(t, dir, 0o500)
	t.Cleanup(func() { chmodOrFatal(t, dir, 0o755) })

	if _, err := s.RefreshSource(src.ID); err == nil {
		t.Fatal("expected RefreshSource to report the sources.json persist failure")
	}

	after, err := s.GetSource(src.ID)
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	if after.Entries != before.Entries || after.Matched != before.Matched || after.Health != before.Health {
		t.Fatalf("source metadata = %+v, want rollback to %+v", after, before)
	}
	s.mu.Lock()
	status := s.status
	s.mu.Unlock()
	if !status.Degraded || status.Message == "" {
		t.Fatalf("status = %+v, want degraded", status)
	}

	releases, err := s.store.loadReleases(src.ID)
	if err != nil {
		t.Fatalf("load releases: %v", err)
	}
	if len(releases) != 2 {
		t.Fatalf("releases on disk = %d, want the merge already committed (2)", len(releases))
	}
}

// TestFailRollsBackWhenSourcesPersistFails covers the analogous swallow in
// fail(): a fetch failure that itself cannot be persisted must not leave the
// source's Health/LastError/backoff bookkeeping ahead of sources.json.
func TestFailRollsBackWhenSourcesPersistFails(t *testing.T) {
	skipOnWindowsPermissions(t)
	dir := t.TempDir()
	cat := mustCatalog(t, dir)
	s := mustServiceAt(t, dir, cat)

	fs := newFeedServer(t, feedBody(t, "Feed", feedEntry{Title: "Game A", URIs: []string{magnetOf("a")}}))
	src := addSource(t, s, fs.url())
	before, err := s.GetSource(src.ID)
	if err != nil {
		t.Fatalf("get source: %v", err)
	}

	fs.fail(500)
	chmodOrFatal(t, dir, 0o500)
	t.Cleanup(func() { chmodOrFatal(t, dir, 0o755) })

	if _, err := s.RefreshSource(src.ID); err == nil {
		t.Fatal("expected the feed fetch to fail")
	}

	after, err := s.GetSource(src.ID)
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	// Status flips to "updating" in memory (unpersisted) the moment refresh
	// starts, independently of fail(); that is not what this test covers, so
	// only the fields fail() itself set and rolled back are checked here.
	if after.Health != before.Health || after.LastError != before.LastError {
		t.Fatalf("source = %+v after a fetch failure whose persist also failed, want Health/LastError rolled back to %+v", after, before)
	}
	s.mu.Lock()
	_, hasFailure := s.failures[src.ID]
	_, hasRetry := s.retryAt[src.ID]
	status := s.status
	s.mu.Unlock()
	if hasFailure || hasRetry {
		t.Fatalf("backoff bookkeeping recorded despite a failed persist: failures=%v retry=%v", hasFailure, hasRetry)
	}
	if !status.Degraded {
		t.Fatalf("status = %+v, want degraded", status)
	}
}

// TestIgnoreReleaseRollsBackWhenReleasesPersistFails covers
// persistTouchedLocked: the mutated release must not stay flagged Ignored in
// memory once its own file failed to save.
func TestIgnoreReleaseRollsBackWhenReleasesPersistFails(t *testing.T) {
	skipOnWindowsPermissions(t)
	dir := t.TempDir()
	cat := mustCatalog(t, dir)
	s := mustServiceAt(t, dir, cat)

	fs := newFeedServer(t, feedBody(t, "Feed", feedEntry{Title: "Game A", URIs: []string{magnetOf("a")}}))
	src := addSource(t, s, fs.url())
	list := releasesOf(t, s, src.ID, "")
	if len(list) != 1 {
		t.Fatalf("releases = %+v, want 1", list)
	}
	releaseID := list[0].Release.ID

	releasesDir := filepath.Join(dir, "releases")
	chmodOrFatal(t, releasesDir, 0o500)
	t.Cleanup(func() { chmodOrFatal(t, releasesDir, 0o755) })

	if err := s.IgnoreRelease(releaseID, true); err == nil {
		t.Fatal("expected IgnoreRelease to report the releases persist failure")
	}

	got, err := s.GetRelease(releaseID)
	if err != nil {
		t.Fatalf("get release: %v", err)
	}
	if got.Release.Ignored {
		t.Fatal("release marked ignored in memory despite a failed persist")
	}
	s.mu.Lock()
	status := s.status
	s.mu.Unlock()
	if !status.Degraded {
		t.Fatalf("status = %+v, want degraded", status)
	}
}

// TestIgnoreReleaseRollsBackCountsWhenSourcesPersistFails covers
// recountLocked: the release save (a different file) already succeeded and
// must stay saved, but the Source counters computed from it roll back when
// sources.json cannot be written.
func TestIgnoreReleaseRollsBackCountsWhenSourcesPersistFails(t *testing.T) {
	skipOnWindowsPermissions(t)
	dir := t.TempDir()
	cat := mustCatalog(t, dir)
	s := mustServiceAt(t, dir, cat)

	fs := newFeedServer(t, feedBody(t, "Feed", feedEntry{Title: "Game A", URIs: []string{magnetOf("a")}}))
	src := addSource(t, s, fs.url())
	list := releasesOf(t, s, src.ID, "")
	if len(list) != 1 {
		t.Fatalf("releases = %+v, want 1", list)
	}
	releaseID := list[0].Release.ID

	before, err := s.GetSource(src.ID)
	if err != nil {
		t.Fatalf("get source: %v", err)
	}

	chmodOrFatal(t, dir, 0o500)
	t.Cleanup(func() { chmodOrFatal(t, dir, 0o755) })

	if err := s.IgnoreRelease(releaseID, true); err == nil {
		t.Fatal("expected IgnoreRelease to report the sources.json persist failure")
	}

	got, err := s.GetRelease(releaseID)
	if err != nil {
		t.Fatalf("get release: %v", err)
	}
	if !got.Release.Ignored {
		t.Fatal("the release save succeeded and must not be rolled back")
	}
	after, err := s.GetSource(src.ID)
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	if after.Matched != before.Matched || after.Review != before.Review || after.Unmatched != before.Unmatched {
		t.Fatalf("counts = %+v, want rollback to %+v", after, before)
	}
	s.mu.Lock()
	status := s.status
	s.mu.Unlock()
	if !status.Degraded {
		t.Fatalf("status = %+v, want degraded", status)
	}

	persisted, err := s.store.loadReleases(src.ID)
	if err != nil {
		t.Fatalf("load releases: %v", err)
	}
	if len(persisted) != 1 || !persisted[0].Ignored {
		t.Fatalf("persisted releases = %+v, want the ignore flag saved", persisted)
	}
}
