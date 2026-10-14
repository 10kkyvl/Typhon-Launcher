package catalog

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type syncRemote struct {
	mu    sync.Mutex
	calls int
	build func(call int) GamePage
}

func (r *syncRemote) Browse(context.Context, GameQuery) (GamePage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	return r.build(r.calls), nil
}

func TestBrowseGetGameAndDiscoveryRunConcurrently(t *testing.T) {
	const catalogSize = 300
	s := parityService(t, catalogSize)
	var renamed atomic.Int64
	base := parityGames(catalogSize)
	remote := &syncRemote{build: func(int) GamePage {
		stamp := renamed.Add(1)
		page := GamePage{Total: 20, Page: 1, PageSize: 20, Revision: 1}
		for i := range 20 {
			g := base[(i*7)%catalogSize]
			g.ID = fmt.Sprintf("srv-%d", i)
			g.ServerID = ""
			g.AliasIDs = nil
			g.Title = fmt.Sprintf("Renamed %d %d", i, stamp)
			g.ExternalIDs = ExternalIDs{IGDB: fmt.Sprintf("i%d", i)}
			page.Items = append(page.Items, g)
		}
		return page
	}}
	s.SetRemoteCatalog(remote)

	const rounds = 150
	var wg sync.WaitGroup
	run := func(fn func(round int) error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for round := range rounds {
				if err := fn(round); err != nil {
					t.Errorf("round %d: %v", round, err)
					return
				}
			}
		}()
	}
	run(func(int) error {
		_, err := s.BrowseGames(context.Background(), GameQuery{Page: 1, PageSize: 20})
		return err
	})
	run(func(int) error {
		_, err := s.BrowseGames(context.Background(), GameQuery{Page: 1, PageSize: 20, Search: "other"})
		return err
	})
	run(func(round int) error {
		_, err := s.GetGame(fmt.Sprintf("g%03d", round%catalogSize))
		return err
	})
	run(func(int) error {
		s.GetDiscovery(context.Background(), DiscoveryQuery{})
		return nil
	})
	run(func(int) error {
		s.GetRecommendationProfile()
		return nil
	})
	run(func(round int) error {
		_, err := s.AddGame(Game{Title: fmt.Sprintf("Added %d", round), ExternalIDs: ExternalIDs{IGDB: fmt.Sprintf("a%d", round)}})
		return err
	})
	run(func(round int) error {
		return s.SetNotInterested(fmt.Sprintf("g%03d", round%catalogSize), round%2 == 0)
	})
	wg.Wait()

	s.mu.RLock()
	defer s.mu.RUnlock()
	assertRemoteMapsMatch(t, s.idx)
}

func TestCatalogPagesPrunedOncePerInterval(t *testing.T) {
	dir := t.TempDir()
	s := mustServiceAt(t, dir)
	now := time.Now()
	s.clock = func() time.Time { return now }
	s.SetRemoteCatalog(&remoteFixture{page: GamePage{Items: []Game{{ID: "srv", Title: "Page"}}, Total: 1}})

	pages := filepath.Join(dir, "catalog-pages")
	if err := os.MkdirAll(pages, 0o700); err != nil {
		t.Fatal(err)
	}
	stale := func(name string) string {
		path := filepath.Join(pages, name)
		if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
		old := time.Now().Add(-catalogPageCacheMaxAge - time.Hour)
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
		return path
	}
	exists := func(path string) bool {
		_, err := os.Stat(path)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			t.Fatal(err)
		}
		return err == nil
	}
	browse := func(search string) {
		if _, err := s.BrowseGames(context.Background(), GameQuery{Page: 1, Search: search}); err != nil {
			t.Fatal(err)
		}
	}

	first := stale("first.json")
	browse("one")
	if exists(first) {
		t.Fatal("the first page did not prune the stale cache file")
	}

	second := stale("second.json")
	now = now.Add(catalogPagePruneInterval - time.Minute)
	browse("two")
	if !exists(second) {
		t.Fatal("a second page inside the interval scanned the cache directory again")
	}

	now = now.Add(2 * time.Minute)
	browse("three")
	if exists(second) {
		t.Fatal("a page after the interval did not prune the stale cache file")
	}
}

func TestPruneCatalogPageCacheReportsUnreadableDirectory(t *testing.T) {
	err := pruneCatalogPageCache(filepath.Join(t.TempDir(), "missing"), time.Now())
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("prune of a missing directory = %v, want an error wrapping fs.ErrNotExist", err)
	}
}
