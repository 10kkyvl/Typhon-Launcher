package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/shortcut"
)

func syncFixture(t *testing.T) (*Service, ExternalGame) {
	t.Helper()
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	item, _ := externalFixture(t)
	if _, err := s.ApplyExternal(context.Background(), []ExternalGame{item}); err != nil {
		t.Fatal(err)
	}
	return s, item
}

func presentOf(provider string, appIDs ...string) map[string]map[string]bool {
	set := map[string]bool{}
	for _, id := range appIDs {
		set[id] = true
	}
	return map[string]map[string]bool{provider: set}
}

func TestSyncExternalMarksVanishedGames(t *testing.T) {
	tests := []struct {
		name          string
		present       map[string]map[string]bool
		running       bool
		wantMarked    int
		wantInstalled bool
	}{
		{"vanished from scanned provider", presentOf(ProviderSteam), false, 1, false},
		{"listed in present (skipped by scanner)", presentOf(ProviderSteam, "42"), false, 0, true},
		{"other provider scanned", presentOf(ProviderGOG), false, 0, true},
		{"provider not scanned", nil, false, 0, true},
		{"running game", presentOf(ProviderSteam), true, 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := syncFixture(t)
			id := s.GetGames()[0].ID
			if tc.running {
				s.mu.Lock()
				s.running[id] = &session{}
				s.mu.Unlock()
			}
			sum, err := s.SyncExternal(context.Background(), nil, tc.present)
			if err != nil {
				t.Fatal(err)
			}
			if sum.Marked != tc.wantMarked {
				t.Fatalf("marked = %d, want %d", sum.Marked, tc.wantMarked)
			}
			reloaded := mustServiceAt(t, s.path).GetGames()
			if len(reloaded) != 1 || reloaded[0].Uninstalled == tc.wantInstalled {
				t.Fatalf("persisted = %+v, wantInstalled = %v", reloaded, tc.wantInstalled)
			}
		})
	}
}

func TestSyncExternalKeepsPlaytimeOfMarkedGame(t *testing.T) {
	s, _ := syncFixture(t)
	s.mu.Lock()
	s.games[0].PlaytimeSeconds = 99
	s.mu.Unlock()
	if _, err := s.SyncExternal(context.Background(), nil, presentOf(ProviderSteam)); err != nil {
		t.Fatal(err)
	}
	g := s.GetGames()[0]
	if !g.Uninstalled || g.PlaytimeSeconds != 99 || g.External == nil || g.External.AppID != "42" {
		t.Fatalf("marked game = %+v", g)
	}
}

func TestSyncExternalBringsBackReturnedGame(t *testing.T) {
	s, item := syncFixture(t)
	if _, err := s.SyncExternal(context.Background(), nil, presentOf(ProviderSteam)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(item.InstallDir, "data.bin"), make([]byte, 1234), 0o600); err != nil {
		t.Fatal(err)
	}
	sum, err := s.SyncExternal(context.Background(), []ExternalGame{item}, presentOf(ProviderSteam, "42"))
	if err != nil {
		t.Fatal(err)
	}
	g := s.GetGames()[0]
	if sum.Updated != 1 || g.Uninstalled || g.SizeBytes != 1234 || g.SizeUnknown {
		t.Fatalf("summary = %+v, game = %+v", sum, g)
	}
}

func TestSyncExternalRollsBackWhenPersistFails(t *testing.T) {
	s, _ := syncFixture(t)
	blockPersist(t, s)
	other, _ := externalFixture(t)
	other.AppID = "43"
	if _, err := s.SyncExternal(context.Background(), []ExternalGame{other}, presentOf(ProviderSteam, "43")); err == nil {
		t.Fatal("expected persist failure")
	}
	games := s.GetGames()
	if len(games) != 1 || games[0].Uninstalled || games[0].External.AppID != "42" {
		t.Fatalf("memory after failed persist = %+v", games)
	}
}

func TestSyncExternalAddsAndMarksInOnePersist(t *testing.T) {
	s, _ := syncFixture(t)
	other, _ := externalFixture(t)
	other.AppID = "43"
	sum, err := s.SyncExternal(context.Background(), []ExternalGame{other}, presentOf(ProviderSteam, "43"))
	if err != nil {
		t.Fatal(err)
	}
	if sum.Added != 1 || sum.Marked != 1 {
		t.Fatalf("summary = %+v", sum)
	}
	byApp := map[string]Game{}
	for _, g := range mustServiceAt(t, s.path).GetGames() {
		byApp[g.External.AppID] = g
	}
	if !byApp["42"].Uninstalled || byApp["43"].Uninstalled {
		t.Fatalf("persisted = %+v", byApp)
	}
}

func TestExternalRefsListsEveryExternalGame(t *testing.T) {
	s, _ := syncFixture(t)
	if _, err := s.SyncExternal(context.Background(), nil, presentOf(ProviderSteam)); err != nil {
		t.Fatal(err)
	}
	refs := s.ExternalRefs()
	if len(refs) != 1 || refs[0] != (ExternalRef{Provider: ProviderSteam, AppID: "42"}) {
		t.Fatalf("refs = %+v", refs)
	}
}

func shortcutSyncFixture(t *testing.T) (*Service, *fakeShortcuts, string) {
	t.Helper()
	s, _ := syncFixture(t)
	fake := newFakeShortcuts(t.TempDir())
	s.shortcuts = fake
	link := filepath.Join(fake.desktop, "Game One.lnk")
	fake.created[link] = shortcut.Link{}
	s.mu.Lock()
	s.games[0].ShortcutPath = link
	s.mu.Unlock()
	return s, fake, link
}

func TestSyncExternalLeavesShortcutWhenPersistFails(t *testing.T) {
	s, fake, link := shortcutSyncFixture(t)
	blockPersist(t, s)
	if _, err := s.SyncExternal(context.Background(), nil, presentOf(ProviderSteam)); err == nil {
		t.Fatal("expected persist failure")
	}
	if _, ok := fake.created[link]; !ok || len(fake.removed) != 0 {
		t.Fatalf("shortcut removed although persist failed: removed = %v", fake.removed)
	}
	g := s.GetGames()[0]
	if g.ShortcutPath != link || g.Uninstalled {
		t.Fatalf("memory after failed persist = %+v", g)
	}
}

func TestSyncExternalShortcutOfVanishedGame(t *testing.T) {
	tests := []struct {
		name        string
		removeErr   error
		wantRemoved bool
		wantPath    bool
	}{
		{"removed", nil, true, false},
		{"removal fails, path kept", errors.New("denied"), false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, fake, link := shortcutSyncFixture(t)
			fake.removeErr = tc.removeErr
			if _, err := s.SyncExternal(context.Background(), nil, presentOf(ProviderSteam)); err != nil {
				t.Fatal(err)
			}
			if _, exists := fake.created[link]; exists == tc.wantRemoved {
				t.Fatalf("shortcut exists = %v, want removed = %v", exists, tc.wantRemoved)
			}
			persisted := mustServiceAt(t, s.path).GetGames()[0]
			if !persisted.Uninstalled || (persisted.ShortcutPath != "") != tc.wantPath {
				t.Fatalf("persisted = %+v", persisted)
			}
		})
	}
}
