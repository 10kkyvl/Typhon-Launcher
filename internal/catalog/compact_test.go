package catalog

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"typhon/internal/storage"
)

func remoteGame(id string) Game {
	return Game{ID: id, ServerID: "srv-" + id, Title: "Game " + id}
}

func keptIDs(s *Service) []string {
	games := s.ListGames()
	ids := make([]string, 0, len(games))
	for _, g := range games {
		ids = append(ids, g.ID)
	}
	slices.Sort(ids)
	return ids
}

func seedExact(t *testing.T, s *Service, games ...Game) {
	t.Helper()
	for _, g := range games {
		if _, err := s.AddGame(g); err != nil {
			t.Fatalf("add game %q: %v", g.ID, err)
		}
	}
}

func setRedirects(s *Service, redirects map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.redirects = redirects
	s.rebuildLocked()
}

func TestCompactKeepRules(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name       string
		games      []Game
		referenced []string
		setup      func(s *Service)
		want       []string
	}{
		{
			name:  "unreferenced remote game is removed",
			games: []Game{remoteGame("cand"), remoteGame("victim")},
			want:  []string{},
		},
		{
			name:  "personal game without server id stays",
			games: []Game{{ID: "cand", Title: "Personal"}, remoteGame("victim")},
			want:  []string{"cand"},
		},
		{
			name: "game with downloaded metadata stays",
			games: []Game{
				{ID: "cand", ServerID: "srv-cand", Title: "Cached", MetadataUpdatedAt: &now},
				remoteGame("victim"),
			},
			want: []string{"cand"},
		},
		{
			name:       "referenced by id stays",
			games:      []Game{remoteGame("cand"), remoteGame("victim")},
			referenced: []string{"cand"},
			want:       []string{"cand"},
		},
		{
			name:       "referenced by server id stays",
			games:      []Game{remoteGame("cand"), remoteGame("victim")},
			referenced: []string{"srv-cand"},
			want:       []string{"cand"},
		},
		{
			name:  "redirect source and target stay",
			games: []Game{remoteGame("old"), remoteGame("mid"), remoteGame("new"), remoteGame("victim")},
			setup: func(s *Service) { setRedirects(s, map[string]string{"old": "mid", "mid": "new"}) },
			want:  []string{"mid", "new", "old"},
		},
		{
			name:  "override target stays",
			games: []Game{remoteGame("cand"), remoteGame("victim")},
			setup: func(s *Service) {
				s.mu.Lock()
				defer s.mu.Unlock()
				s.overrides = []MatchOverride{{Pattern: "some pattern", GameID: "cand", CreatedAt: now}}
				s.rebuildLocked()
			},
			want: []string{"cand"},
		},
		{
			name:  "not interested game stays",
			games: []Game{remoteGame("cand"), remoteGame("victim")},
			setup: func(s *Service) {
				s.mu.Lock()
				defer s.mu.Unlock()
				s.preferences.NotInterested = []string{"cand"}
			},
			want: []string{"cand"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestService(t)
			seedExact(t, s, tc.games...)
			if tc.setup != nil {
				tc.setup(s)
			}
			res, err := s.Compact(context.Background(), tc.referenced)
			if err != nil {
				t.Fatalf("compact: %v", err)
			}
			if got := keptIDs(s); !slices.Equal(got, tc.want) {
				t.Fatalf("kept = %v, want %v", got, tc.want)
			}
			if res.Kept != len(tc.want) || res.Removed != len(tc.games)-len(tc.want) {
				t.Fatalf("result = %+v, want kept %d removed %d", res, len(tc.want), len(tc.games)-len(tc.want))
			}
		})
	}
}

func TestCompactBackupIsWrittenOnceFromOriginalBytes(t *testing.T) {
	dir := t.TempDir()
	s := mustServiceAt(t, dir)
	seedExact(t, s, remoteGame("keep"), remoteGame("victim1"))
	path := filepath.Join(dir, "catalog.json")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	res, err := s.Compact(context.Background(), []string{"keep"})
	if err != nil {
		t.Fatalf("compact: %v", err)
	}
	if res.Removed != 1 || res.Backup != path+compactBackupSuffix {
		t.Fatalf("result = %+v, want one removal and backup at %s", res, path+compactBackupSuffix)
	}
	backup, err := os.ReadFile(res.Backup)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if !bytes.Equal(backup, original) {
		t.Fatalf("backup differs from the pre-compaction catalog")
	}

	marker := []byte("first backup marker")
	if err := os.WriteFile(res.Backup, marker, 0o600); err != nil {
		t.Fatal(err)
	}
	seedExact(t, s, remoteGame("victim2"))
	again, err := s.Compact(context.Background(), []string{"keep"})
	if err != nil {
		t.Fatalf("second compact: %v", err)
	}
	if again.Removed != 1 {
		t.Fatalf("second compact removed %d, want 1", again.Removed)
	}
	after, err := os.ReadFile(res.Backup)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, marker) {
		t.Fatalf("backup was rewritten by the second compaction: %q", after)
	}
}

func TestCompactWithNothingToRemoveTouchesNothing(t *testing.T) {
	dir := t.TempDir()
	s := mustServiceAt(t, dir)
	seedExact(t, s, remoteGame("a"), Game{ID: "personal", Title: "Personal"})
	path := filepath.Join(dir, "catalog.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	res, err := s.Compact(context.Background(), []string{"a"})
	if err != nil {
		t.Fatalf("compact: %v", err)
	}
	if res != (CompactResult{Kept: 2}) {
		t.Fatalf("result = %+v, want kept 2 only", res)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("catalog rewritten although nothing was removed")
	}
	if _, err := os.Stat(path + compactBackupSuffix); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("backup created although nothing was removed, err=%v", err)
	}
}

func TestCompactCancelledContextChangesNothing(t *testing.T) {
	dir := t.TempDir()
	s := mustServiceAt(t, dir)
	seedExact(t, s, remoteGame("victim"), remoteGame("other"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := s.Compact(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("compact err = %v, want context.Canceled", err)
	}
	if got := keptIDs(s); !slices.Equal(got, []string{"other", "victim"}) {
		t.Fatalf("games = %v, want both untouched", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "catalog.json"+compactBackupSuffix)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("backup created on a cancelled compaction, err=%v", err)
	}
}

func TestCompactRollsBackMemoryWhenPersistFails(t *testing.T) {
	dir := t.TempDir()
	s := mustServiceAt(t, dir)
	seedExact(t, s, remoteGame("keep"), remoteGame("victim"))
	path := filepath.Join(dir, "catalog.json")
	if err := os.WriteFile(path+compactBackupSuffix, []byte("existing backup"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Compact(context.Background(), []string{"keep"}); err == nil {
		t.Fatal("compact succeeded despite an unwritable catalog")
	}
	if got := keptIDs(s); !slices.Equal(got, []string{"keep", "victim"}) {
		t.Fatalf("games after failed compact = %v, want both restored", got)
	}
	if _, err := s.GetGame("victim"); err != nil {
		t.Fatalf("removal candidate unreachable after rollback: %v", err)
	}
}

func TestCompactedCatalogReopensWithRedirectsAndOverrides(t *testing.T) {
	dir := t.TempDir()
	s := mustServiceAt(t, dir)
	seedExact(t, s, remoteGame("old"), remoteGame("new"), remoteGame("target"), remoteGame("victim"))
	if err := storage.Save(filepath.Join(dir, "catalog-redirects.json"), 1, map[string]string{"old": "new"}); err != nil {
		t.Fatal(err)
	}
	s = mustServiceAt(t, dir)
	s.mu.Lock()
	s.overrides = []MatchOverride{{Pattern: "some pattern", GameID: "target", CreatedAt: time.Now()}}
	err := s.persistOverridesLocked()
	s.rebuildLocked()
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}

	res, err := s.Compact(context.Background(), nil)
	if err != nil {
		t.Fatalf("compact: %v", err)
	}
	if res.Removed != 1 {
		t.Fatalf("removed = %d, want 1", res.Removed)
	}

	reopened, err := NewServiceAt(dir)
	if err != nil {
		t.Fatalf("reopen after compact: %v", err)
	}
	if got := keptIDs(reopened); !slices.Equal(got, []string{"new", "old", "target"}) {
		t.Fatalf("reopened games = %v", got)
	}
	reopened.mu.RLock()
	defer reopened.mu.RUnlock()
	if reopened.redirects["old"] != "new" || reopened.overrideMap["some pattern"] != "target" {
		t.Fatalf("redirects = %v, overrides = %v", reopened.redirects, reopened.overrideMap)
	}
}
