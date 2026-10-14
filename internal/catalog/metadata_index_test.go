package catalog

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"
)

func indexMaps(idx *index) map[string]any {
	return map[string]any{
		"entries":        idx.entries,
		"byID":           idx.byID,
		"byTitle":        idx.byTitle,
		"byAlias":        idx.byAlias,
		"byToken":        idx.byToken,
		"byExternal":     idx.byExternal,
		"byServer":       idx.byServer,
		"byIGDB":         idx.byIGDB,
		"bySteam":        idx.bySteam,
		"byIdentity":     idx.byIdentity,
		"byLocal":        idx.byLocal,
		"redirectGroup":  idx.redirectGroup,
		"redirectTarget": idx.redirectTarget,
	}
}

// assertIndexEqualsRebuild swaps the live index for a rebuilt one, so the
// caller must not use the old pointer afterwards.
func assertIndexEqualsRebuild(t *testing.T, s *Service) {
	t.Helper()
	compareWithRebuild(t, s)
}

func compareWithRebuild(t *testing.T, s *Service) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	got := indexMaps(s.idx)
	s.rebuildLocked()
	want := indexMaps(s.idx)
	for name, g := range got {
		if !reflect.DeepEqual(g, want[name]) {
			t.Fatalf("%s drifted from a rebuild:\n got %v\nwant %v", name, g, want[name])
		}
	}
}

func metadataIndexService(t *testing.T, n int) *Service {
	t.Helper()
	s := newTestService(t)
	for i := range n {
		seed(t, s, Game{
			ID:       fmt.Sprintf("g%03d", i),
			Title:    fmt.Sprintf("Unique%03d Original", i),
			ServerID: fmt.Sprintf("srv%03d", i),
		})
	}
	return s
}

func titleHit(games []Game, id string) bool {
	for _, g := range games {
		if g.ID == id {
			return true
		}
	}
	return false
}

func TestApplyMetadataUpdatesIndexInPlace(t *testing.T) {
	cases := []struct {
		name  string
		patch func() MetadataPatch
		title string
		igdb  string
		steam string
	}{
		{
			name: "title and igdb id",
			patch: func() MetadataPatch {
				p := samplePatch()
				p.Title = "Zanzibar Chronicle"
				p.IGDBID = "7001"
				return p
			},
			title: "Zanzibar Chronicle",
			igdb:  "7001",
		},
		{
			name: "igdb and steam ids keep the title",
			patch: func() MetadataPatch {
				p := samplePatch()
				p.Title = ""
				p.IGDBID = "7002"
				p.SteamID = "9002"
				return p
			},
			title: "Unique040 Original",
			igdb:  "7002",
			steam: "9002",
		},
		{
			name: "steam id only",
			patch: func() MetadataPatch {
				p := samplePatch()
				p.Title = "Quokka Tales"
				p.IGDBID = ""
				p.SteamID = "9003"
				return p
			},
			title: "Quokka Tales",
			steam: "9003",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := metadataIndexService(t, 80)
			s.mu.RLock()
			idxBefore := s.idx
			epochBefore := s.epoch
			s.mu.RUnlock()

			updated, err := s.ApplyMetadata("g040", tc.patch())
			if err != nil {
				t.Fatalf("apply: %v", err)
			}

			s.mu.RLock()
			same := s.idx == idxBefore
			epochAfter := s.epoch
			s.mu.RUnlock()
			if !same {
				t.Fatal("ApplyMetadata replaced the whole index instead of updating one entry")
			}
			if epochAfter != epochBefore+1 {
				t.Fatalf("epoch = %d, want %d", epochAfter, epochBefore+1)
			}
			if updated.Title != tc.title {
				t.Fatalf("title = %q, want %q", updated.Title, tc.title)
			}
			if !titleHit(s.SearchGames(tc.title, 10), "g040") {
				t.Fatalf("game not found by its new title %q", tc.title)
			}
			if tc.title != "Unique040 Original" && titleHit(s.SearchGames("Unique040 Original", 10), "g040") {
				t.Fatal("stale title posting still returns the game")
			}
			if tc.igdb != "" {
				if got, ok := s.GameByIGDB(tc.igdb); !ok || got.ID != "g040" {
					t.Fatalf("GameByIGDB(%s) = %+v, %v", tc.igdb, got, ok)
				}
			}
			if tc.steam != "" {
				m := s.Resolve(Query{ExternalIDs: ExternalIDs{Steam: tc.steam}})
				if m.GameID != "g040" {
					t.Fatalf("resolve by steam id = %+v", m)
				}
			}
			assertIndexEqualsRebuild(t, s)
		})
	}
}

func TestApplyMetadataKeepsDuplicateProviderIDWinnerLikeRebuild(t *testing.T) {
	s := newTestService(t)
	seed(t, s,
		Game{ID: "a", Title: "Alpha", ServerID: "sa"},
		Game{ID: "b", Title: "Bravo", ServerID: "sb"},
		Game{ID: "c", Title: "Charlie", ServerID: "sc"},
	)
	steps := []struct {
		id    string
		title string
		igdb  string
	}{
		{"a", "Alpha", "5"},
		{"c", "Charlie", "5"},
		{"c", "Charlie", "6"},
		{"a", "Alpha Again", "5"},
		{"b", "Alpha Again", "5"},
	}
	for _, step := range steps {
		p := samplePatch()
		p.Title, p.IGDBID = step.title, step.igdb
		if _, err := s.ApplyMetadata(step.id, p); err != nil {
			t.Fatalf("apply %+v: %v", step, err)
		}
		s.mu.RLock()
		got := indexMaps(s.idx)
		fresh := buildIndex(s.games)
		s.mu.RUnlock()
		for _, name := range []string{"byTitle", "byToken", "byExternal", "byIGDB"} {
			if !reflect.DeepEqual(got[name], indexMaps(fresh)[name]) {
				t.Fatalf("after %+v: %s = %v, rebuild gives %v", step, name, got[name], indexMaps(fresh)[name])
			}
		}
	}
}

func TestApplyMetadataOnRedirectSourcePatchesTarget(t *testing.T) {
	s := newTestService(t)
	seed(t, s,
		Game{ID: "old", Title: "Merged Title", ServerID: "s-old"},
		Game{ID: "new", Title: "Merged Title", ServerID: "s-new"},
	)
	s.mu.Lock()
	s.redirects = map[string]string{"old": "new"}
	s.rebuildLocked()
	idxBefore := s.idx
	s.mu.Unlock()

	p := samplePatch()
	p.Title = "Merged Title Remastered"
	got, err := s.ApplyMetadata("old", p)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if got.ID != "new" {
		t.Fatalf("patched %q, want the redirect target", got.ID)
	}

	s.mu.RLock()
	if s.idx != idxBefore {
		t.Fatal("index replaced")
	}
	oldPos, newPos := -1, -1
	for i, g := range s.games {
		switch g.ID {
		case "old":
			oldPos = i
		case "new":
			newPos = i
		}
	}
	if s.idx.entries[oldPos].matchable {
		t.Fatal("redirect source became matchable again")
	}
	if !s.idx.entries[newPos].matchable {
		t.Fatal("redirect target is not matchable")
	}
	if s.idx.byID["old"] != newPos {
		t.Fatalf("byID[old] = %d, want target position %d", s.idx.byID["old"], newPos)
	}
	if s.games[oldPos].Title != "Merged Title" {
		t.Fatalf("redirect source was modified: %+v", s.games[oldPos])
	}
	s.mu.RUnlock()
	assertIndexEqualsRebuild(t, s)
}

func TestApplyMetadataRollbackRestoresIndex(t *testing.T) {
	s := metadataIndexService(t, 40)
	s.gamesPath = t.TempDir()
	p := samplePatch()
	p.Title = "Zanzibar Chronicle"
	p.IGDBID = "7001"
	if _, err := s.ApplyMetadata("g010", p); err == nil {
		t.Fatal("apply succeeded despite an unwritable catalog")
	}
	if titleHit(s.SearchGames("Zanzibar Chronicle", 10), "g010") {
		t.Fatal("rolled back title is still indexed")
	}
	if !titleHit(s.SearchGames("Unique010 Original", 10), "g010") {
		t.Fatal("original title lost after rollback")
	}
	if _, ok := s.GameByIGDB("7001"); ok {
		t.Fatal("rolled back provider id is still indexed")
	}
	assertIndexEqualsRebuild(t, s)
}

func TestPromoteDiscoveryGameAppendsToIndexInPlace(t *testing.T) {
	s := metadataIndexService(t, 60)
	s.rememberDiscoveryGames([]RecommendationItem{{Game: Game{
		ID: "pick", ServerID: "pick", Title: "Opened Pick", Genres: []string{"role-playing (rpg)"},
		ExternalIDs: ExternalIDs{IGDB: "4242"},
	}}})
	s.mu.RLock()
	idxBefore := s.idx
	epochBefore := s.epoch
	s.mu.RUnlock()

	got, err := s.GetGame("pick")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	s.mu.RLock()
	same := s.idx == idxBefore
	epochAfter := s.epoch
	s.mu.RUnlock()
	if !same {
		t.Fatal("promoting a discovery pick replaced the whole index")
	}
	if epochAfter != epochBefore+1 {
		t.Fatalf("epoch = %d, want %d", epochAfter, epochBefore+1)
	}
	if got.ID != "pick" {
		t.Fatalf("id = %q", got.ID)
	}
	if !titleHit(s.SearchGames("Opened Pick", 10), "pick") {
		t.Fatal("promoted game not searchable")
	}
	if g, ok := s.GameByIGDB("4242"); !ok || g.ID != "pick" {
		t.Fatalf("GameByIGDB = %+v, %v", g, ok)
	}
	assertIndexEqualsRebuild(t, s)
}

func TestApplyMetadataRacesWithReads(t *testing.T) {
	t.Run("search resolve and get", func(t *testing.T) { runMetadataRace(t, false) })
	t.Run("with durable browse", func(t *testing.T) { runMetadataRace(t, true) })
}

func runMetadataRace(t *testing.T, withBrowse bool) {
	t.Helper()
	const catalogSize = 300
	s := parityService(t, catalogSize)
	base := parityGames(catalogSize)
	var stamp int
	var stampMu sync.Mutex
	remote := &syncRemote{build: func(int) GamePage {
		stampMu.Lock()
		stamp++
		n := stamp
		stampMu.Unlock()
		page := GamePage{Total: 20, Page: 1, PageSize: 20, Revision: 1}
		for i := range 20 {
			g := base[(i*7)%catalogSize]
			g.ID = fmt.Sprintf("srv-%d", i)
			g.ServerID = ""
			g.AliasIDs = nil
			g.Title = fmt.Sprintf("Renamed %d %d", i, n)
			g.ExternalIDs = ExternalIDs{IGDB: fmt.Sprintf("r%d", i)}
			page.Items = append(page.Items, g)
		}
		return page
	}}
	s.SetRemoteCatalog(remote)

	const writers, rounds = 4, 60
	var wg sync.WaitGroup
	for w := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for round := range rounds {
				id := fmt.Sprintf("g%03d", (round*writers+w)%catalogSize)
				p := MetadataPatch{
					IGDBID:    fmt.Sprintf("i%d", (round+w)%(catalogSize/3)),
					Title:     fmt.Sprintf("Patched %d %d", w, round),
					UpdatedAt: time.Now(),
				}
				if _, err := s.ApplyMetadata(id, p); err != nil {
					t.Errorf("apply %s: %v", id, err)
					return
				}
			}
		}()
	}
	reader := func(fn func(round int) error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for round := range rounds * 2 {
				if err := fn(round); err != nil {
					t.Errorf("round %d: %v", round, err)
					return
				}
			}
		}()
	}
	if withBrowse {
		reader(func(int) error {
			_, err := s.BrowseGames(context.Background(), GameQuery{Page: 1, PageSize: 20})
			return err
		})
	}
	reader(func(round int) error {
		_, err := s.GetGame(fmt.Sprintf("g%03d", round%catalogSize))
		return err
	})
	reader(func(round int) error {
		s.SearchGames(fmt.Sprintf("Patched %d", round%writers), 10)
		s.Resolve(Query{Title: "Parity Game", Normalized: "parity game"})
		return nil
	})
	wg.Wait()

	if withBrowse {
		assertIndexEqualsRebuild(t, s)
		return
	}
	assertIndexEqualsRebuild(t, s)
}

func TestBrowseMergeKeepsRedirectSourceRedirected(t *testing.T) {
	s := newTestService(t)
	seed(t, s,
		Game{ID: "old", Title: "Merged Title", ServerID: "s-old", ExternalIDs: ExternalIDs{IGDB: "10"}},
		Game{ID: "new", Title: "Merged Title", ServerID: "s-new", ExternalIDs: ExternalIDs{IGDB: "10"}},
	)
	s.mu.Lock()
	s.redirects = map[string]string{"old": "new"}
	s.rebuildLocked()
	oldPos, newPos := -1, -1
	for i, g := range s.games {
		switch g.ID {
		case "old":
			oldPos = i
		case "new":
			newPos = i
		}
	}
	previous := s.games[oldPos]
	s.games[oldPos].Title = "Merged Title Repack"
	s.updateIndexAtLocked(oldPos, previous)
	if s.idx.byID["old"] != newPos {
		t.Fatalf("byID[old] = %d after a merge into the redirect source, want target %d", s.idx.byID["old"], newPos)
	}
	if s.idx.entries[oldPos].matchable {
		t.Fatal("redirect source became matchable again after a merge")
	}
	s.mu.Unlock()
	assertIndexEqualsRebuild(t, s)
}
