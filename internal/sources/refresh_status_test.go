package sources

import (
	"path/filepath"
	"testing"
)

func TestRefreshRefusedByTheDiskDoesNotLeaveTheSourceUpdating(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		disabled    bool
		feedChanges bool
	}{
		{"feed fetch fails on an active source", false, false},
		{"feed fetch fails on a disabled source", true, false},
		{"new releases cannot be settled on an active source", false, true},
		{"new releases cannot be settled on a disabled source", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s, server, src, dir := blockedService(t)
			if tt.disabled {
				if err := s.SetSourceEnabled(src.ID, false); err != nil {
					t.Fatal(err)
				}
			}
			before, err := s.GetSource(src.ID)
			if err != nil {
				t.Fatal(err)
			}
			if before.Status == StatusUpdating {
				t.Fatalf("precondition: source is already %q", before.Status)
			}
			if tt.feedChanges {
				server.set(feedBody(t, "Feed",
					feedEntry{Title: "Game A", URIs: []string{magnetOf("a")}},
					feedEntry{Title: "Game B", URIs: []string{magnetOf("b")}},
				), `"v2"`)
			} else {
				server.fail(500)
			}
			replaceFileWithDir(t, filepath.Join(dir, "sources.json"))

			if _, err := s.RefreshSource(src.ID); err == nil {
				t.Fatal("expected the refresh to fail")
			}

			after, err := s.GetSource(src.ID)
			if err != nil {
				t.Fatal(err)
			}
			if after.Status != before.Status {
				t.Fatalf("status after the refused refresh = %q, want the previous %q", after.Status, before.Status)
			}
			s.mu.Lock()
			busy := s.refreshing[src.ID]
			s.mu.Unlock()
			if busy {
				t.Fatal("the source is still marked as refreshing")
			}
		})
	}
}
