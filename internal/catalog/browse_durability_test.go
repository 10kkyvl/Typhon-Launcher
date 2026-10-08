package catalog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestBrowsePageDoesNotPersistOrGrowGamesForNewItems is the regression test
// for the bug this file's changes fix: browsing the public catalog used to
// write every previously-unseen item into s.games/catalog.json immediately,
// so scrolling through pages (never opening a single card) grew the on-disk
// catalog forever. A browsed page must only preview new items; GetGame is
// the first interaction that commits one.
func TestBrowsePageDoesNotPersistOrGrowGamesForNewItems(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewServiceAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	remote := &remoteFixture{page: GamePage{Items: []Game{
		{ID: "a", Title: "A"},
		{ID: "b", Title: "B"},
		{ID: "c", Title: "C"},
	}, Total: 3}}
	svc.SetRemoteCatalog(remote)

	page, err := svc.BrowseGames(context.Background(), GameQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 3 {
		t.Fatalf("page items = %d, want 3", len(page.Items))
	}
	if len(svc.games) != 0 {
		t.Fatalf("browsing a page grew s.games to %d, want 0", len(svc.games))
	}
	if _, statErr := os.Stat(svc.gamesPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("browsing a page wrote catalog.json: err=%v", statErr)
	}

	got, err := svc.GetGame("a")
	if err != nil {
		t.Fatalf("GetGame after browse-only: %v", err)
	}
	if got.Title != "A" {
		t.Fatalf("got = %+v, want title A", got)
	}
	if len(svc.games) != 1 {
		t.Fatalf("GetGame committed %d games, want exactly the one opened", len(svc.games))
	}
	if _, statErr := os.Stat(svc.gamesPath); statErr != nil {
		t.Fatalf("GetGame did not persist catalog.json: %v", statErr)
	}

	reopened, err := NewServiceAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(reopened.games) != 1 || reopened.games[0].ID != "a" {
		t.Fatalf("persisted games after restart = %+v, want only %q", reopened.games, "a")
	}
	if _, err := reopened.GetGame("b"); err == nil {
		t.Fatal("a game only ever browsed, never opened, survived as durable across restart")
	}
}

// TestGetGameAfterBrowseRollsBackOnPersistFailure exercises the same rollback
// invariant as TestOpeningDiscoveryPickRollsBackFailedSave, but feeds the
// pending cache through the durable BrowseGames path (rememberBrowsedGameLocked)
// instead of the discovery path (rememberDiscoveryGames), to prove the two
// entry points share promotion/rollback correctly.
func TestGetGameAfterBrowseRollsBackOnPersistFailure(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewServiceAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	remote := &remoteFixture{page: GamePage{Items: []Game{{ID: "pick", Title: "Pick"}}}}
	svc.SetRemoteCatalog(remote)
	if _, err := svc.BrowseGames(context.Background(), GameQuery{}); err != nil {
		t.Fatal(err)
	}
	if len(svc.games) != 0 {
		t.Fatalf("browse persisted before any GetGame: %+v", svc.games)
	}

	svc.gamesPath = t.TempDir() // atomic rename over a directory must fail
	if _, err := svc.GetGame("pick"); err == nil {
		t.Fatal("save unexpectedly succeeded")
	}
	if len(svc.games) != 0 {
		t.Fatal("failed promotion after browse mutated catalog in memory")
	}
	if _, ok := svc.idx.game("pick"); ok {
		t.Fatal("failed promotion after browse mutated the index")
	}
}

// TestBrowseGamesCanceledContextWritesNothing is the ctx-plumbing requirement:
// BrowseGames now takes the caller's ctx (Wails supplies the call's ctx
// automatically), and an already-canceled request must not silently fall
// back to serving/writing an offline cache — the caller asked to stop.
func TestBrowseGamesCanceledContextWritesNothing(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewServiceAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	remote := &ctxCheckingRemote{}
	svc.SetRemoteCatalog(remote)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = svc.BrowseGames(ctx, GameQuery{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if remote.calls != 1 {
		t.Fatalf("remote calls = %d, want 1", remote.calls)
	}
	if _, statErr := os.Stat(svc.gamesPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("canceled browse wrote catalog.json: err=%v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "catalog-pages")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("canceled browse wrote the offline page cache: err=%v", statErr)
	}
}

type ctxCheckingRemote struct{ calls int }

func (r *ctxCheckingRemote) Browse(ctx context.Context, _ GameQuery) (GamePage, error) {
	r.calls++
	if err := ctx.Err(); err != nil {
		return GamePage{}, err
	}
	return GamePage{Items: []Game{{ID: "x", Title: "X"}}}, nil
}

// TestBrowsedGameCacheIsBounded is the size-cap requirement for the shared
// temporary cache: dozens of browsed catalog pages (60 items each) must not
// grow this cache without bound the way catalog.json used to.
func TestBrowsedGameCacheIsBounded(t *testing.T) {
	svc := newTestService(t)
	base := time.Now()
	for i := 0; i < maxDiscoveryGamesCache+50; i++ {
		svc.rememberBrowsedGameLocked(Game{ID: fmt.Sprintf("g-%04d", i), Title: "G"}, base.Add(time.Duration(i)*time.Millisecond))
	}
	if len(svc.discoveryGames) != maxDiscoveryGamesCache {
		t.Fatalf("cache size = %d, want %d", len(svc.discoveryGames), maxDiscoveryGamesCache)
	}
	if _, ok := svc.discoveryGames["g-0000"]; ok {
		t.Fatal("oldest browsed entry should have been evicted")
	}
	newest := fmt.Sprintf("g-%04d", maxDiscoveryGamesCache+49)
	if _, ok := svc.discoveryGames[newest]; !ok {
		t.Fatal("newest browsed entry should still be cached")
	}
}
