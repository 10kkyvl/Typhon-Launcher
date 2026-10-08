package sources

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func unblockSourcesFile(t *testing.T, dir string) {
	t.Helper()
	if err := os.Remove(filepath.Join(dir, "sources.json")); err != nil {
		t.Fatal(err)
	}
}

func storedSource(t *testing.T, s *Service, id string) (Source, bool) {
	t.Helper()
	list, err := s.store.loadSources()
	if err != nil {
		t.Fatalf("load sources: %v", err)
	}
	for _, src := range list {
		if src.ID == id {
			return src, true
		}
	}
	return Source{}, false
}

func TestRemoveSourceKeepsEverythingWhenItCannotBeSaved(t *testing.T) {
	t.Parallel()
	s, _, removed, dir := blockedService(t)
	otherServer := newFeedServer(t, feedBody(t, "Other", feedEntry{Title: "Game B", URIs: []string{magnetOf("b")}}))
	other := addSource(t, s, otherServer.url())
	retryAt := time.Now().Add(time.Hour)
	s.mu.Lock()
	s.failures[removed.ID] = 2
	s.retryAt[removed.ID] = retryAt
	s.mu.Unlock()
	wantReleases := len(releasesOf(t, s, removed.ID, "all"))
	if wantReleases == 0 {
		t.Fatal("the source has no releases to lose")
	}
	releasesFile := filepath.Join(dir, "releases", removed.ID+".json")
	replaceFileWithDir(t, filepath.Join(dir, "sources.json"))

	if err := s.RemoveSource(removed.ID); err == nil {
		t.Fatal("expected the save failure")
	}

	if _, err := s.GetSource(removed.ID); err != nil {
		t.Fatalf("the source is gone from memory although it is still on disk: %v", err)
	}
	if got := len(s.ListSources()); got != 2 {
		t.Fatalf("sources in memory = %d, want 2", got)
	}
	if got := len(releasesOf(t, s, removed.ID, "all")); got != wantReleases {
		t.Fatalf("releases in memory = %d, want %d", got, wantReleases)
	}
	if _, err := os.Stat(releasesFile); err != nil {
		t.Fatalf("the releases file was touched by a removal that was refused: %v", err)
	}
	s.mu.Lock()
	failures, hasFailures := s.failures[removed.ID]
	gotRetry, hasRetry := s.retryAt[removed.ID]
	s.mu.Unlock()
	if !hasFailures || failures != 2 || !hasRetry || !gotRetry.Equal(retryAt) {
		t.Fatalf("backoff = (%d, %v, %v), want it kept as (2, %v, true)", failures, gotRetry, hasRetry, retryAt)
	}

	unblockSourcesFile(t, dir)
	if err := s.SetSourceEnabled(other.ID, false); err != nil {
		t.Fatalf("unrelated save: %v", err)
	}
	if _, ok := storedSource(t, s, removed.ID); !ok {
		t.Fatal("the next successful save erased the source that was never removed")
	}
	if _, ok := storedSource(t, s, other.ID); !ok {
		t.Fatal("the neighbouring source is missing on disk")
	}
}

func TestRemoveSourceDropsItOnceTheSaveWorks(t *testing.T) {
	t.Parallel()
	s, _, src, dir := blockedService(t)
	releasesFile := filepath.Join(dir, "releases", src.ID+".json")

	if err := s.RemoveSource(src.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSource(src.ID); err == nil {
		t.Fatal("the removed source is still in memory")
	}
	if _, ok := storedSource(t, s, src.ID); ok {
		t.Fatal("the removed source is still on disk")
	}
	if _, err := os.Stat(releasesFile); !os.IsNotExist(err) {
		t.Fatalf("releases file stat = %v, want it removed", err)
	}
	s.mu.Lock()
	_, hasReleases := s.releases[src.ID]
	s.mu.Unlock()
	if hasReleases {
		t.Fatal("the removed source still has releases in memory")
	}
}

func TestSetSourceEnabledRollsBackWhenItCannotBeSaved(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		before bool
	}{
		{"disable an enabled source", true},
		{"enable a disabled source", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s, _, src, dir := blockedService(t)
			if !tt.before {
				if err := s.SetSourceEnabled(src.ID, false); err != nil {
					t.Fatal(err)
				}
			}
			was, err := s.GetSource(src.ID)
			if err != nil {
				t.Fatal(err)
			}
			replaceFileWithDir(t, filepath.Join(dir, "sources.json"))

			if err := s.SetSourceEnabled(src.ID, !tt.before); err == nil {
				t.Fatal("expected the save failure")
			}

			got, err := s.GetSource(src.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.Enabled != tt.before || got.Status != was.Status {
				t.Fatalf("source = (enabled %v, %q), want the stored (enabled %v, %q)", got.Enabled, got.Status, tt.before, was.Status)
			}
			unblockSourcesFile(t, dir)
			otherServer := newFeedServer(t, feedBody(t, "Other", feedEntry{Title: "Game B", URIs: []string{magnetOf("b")}}))
			addSource(t, s, otherServer.url())
			stored, ok := storedSource(t, s, src.ID)
			if !ok || stored.Enabled != tt.before {
				t.Fatalf("stored source = (%v, enabled %v), want enabled %v", ok, stored.Enabled, tt.before)
			}
		})
	}
}
