package catalog

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func oracleRemoteIndexes(games []Game) (byServer, byIGDB, bySteam map[string]int) {
	byServer = map[string]int{}
	byIGDB = map[string]int{}
	bySteam = map[string]int{}
	for pos, game := range games {
		if game.ServerID != "" {
			byServer[game.ServerID] = pos
		}
		if game.ExternalIDs.IGDB != "" {
			key := strings.ToLower(game.ExternalIDs.IGDB)
			if _, exists := byIGDB[key]; !exists {
				byIGDB[key] = pos
			}
		}
		if game.ExternalIDs.Steam != "" {
			key := strings.ToLower(game.ExternalIDs.Steam)
			if _, exists := bySteam[key]; !exists {
				bySteam[key] = pos
			}
		}
	}
	return byServer, byIGDB, bySteam
}

func oracleRemoteMatch(game Game, byServer, byIGDB, bySteam map[string]int) int {
	if game.ServerID != "" {
		if pos, ok := byServer[game.ServerID]; ok {
			return pos
		}
	}
	if game.ExternalIDs.IGDB != "" {
		if pos, ok := byIGDB[strings.ToLower(game.ExternalIDs.IGDB)]; ok {
			return pos
		}
	}
	if game.ExternalIDs.IGDB == "" && game.ExternalIDs.Steam != "" {
		if pos, ok := bySteam[strings.ToLower(game.ExternalIDs.Steam)]; ok {
			return pos
		}
	}
	return -1
}

func oracleIdentityKeys(game Game) []string {
	keys := []string{}
	for provider, id := range map[string]string{"igdb": game.ExternalIDs.IGDB, "steam": game.ExternalIDs.Steam} {
		if id != "" {
			keys = append(keys, provider+":"+strings.ToLower(id))
		}
		for _, linked := range game.ProviderLinks[provider] {
			if linked != "" {
				keys = append(keys, provider+":"+strings.ToLower(linked))
			}
		}
	}
	sort.Strings(keys)
	return keys
}

func oracleIdentityPostings(games []Game, local bool) map[string][]int {
	out := map[string][]int{}
	for pos, game := range games {
		var keys []string
		if local {
			keys = oracleIdentityKeys(Game{ExternalIDs: game.LocalExternalIDs})
		} else {
			keys = oracleIdentityKeys(game)
		}
		seen := map[string]bool{}
		for _, key := range keys {
			if !seen[key] {
				seen[key] = true
				out[key] = append(out[key], pos)
			}
		}
	}
	return out
}

func oracleReconcile(games []Game, currentGames []Game) ([]Game, bool) {
	out := append([]Game(nil), games...)
	claimed := map[string]bool{}
	igdbClaims := map[string]bool{}
	currentServers := map[string]bool{}
	currentIGDB := map[string]bool{}
	for _, current := range currentGames {
		if current.ServerID != "" {
			currentServers[current.ServerID] = true
		}
		if current.ExternalIDs.IGDB != "" {
			currentIGDB[current.ExternalIDs.IGDB] = true
		}
		if len(current.ProviderLinks["igdb"]) > 0 || current.ExternalIDs.IGDB != "" {
			for _, id := range current.ProviderLinks["igdb"] {
				igdbClaims[id] = true
			}
			if current.ExternalIDs.IGDB != "" {
				igdbClaims[current.ExternalIDs.IGDB] = true
			}
		}
		if len(current.ProviderLinks["steam"]) > 0 || current.ExternalIDs.Steam != "" {
			for _, id := range current.ProviderLinks["steam"] {
				claimed[id] = true
			}
			if current.ExternalIDs.Steam != "" {
				claimed[current.ExternalIDs.Steam] = true
			}
		}
	}
	changedAny := false
	for i, old := range out {
		if (old.ServerID != "" && currentServers[old.ServerID]) || old.ExternalIDs.IGDB == "" || currentIGDB[old.ExternalIDs.IGDB] {
			continue
		}
		kept := []string{}
		changed := false
		for _, id := range old.ProviderLinks["steam"] {
			if claimed[id] {
				changed = true
			} else {
				kept = append(kept, id)
			}
		}
		if claimed[old.ExternalIDs.Steam] {
			old.ExternalIDs.Steam = ""
			changed = true
		}
		if old.LocalExternalIDs.Steam != "" && claimed[old.LocalExternalIDs.Steam] {
			old.LocalExternalIDs.Steam = ""
			changed = true
		}
		keptIGDB := []string{}
		for _, id := range old.ProviderLinks["igdb"] {
			if igdbClaims[id] {
				changed = true
			} else {
				keptIGDB = append(keptIGDB, id)
			}
		}
		if old.LocalExternalIDs.IGDB != "" && igdbClaims[old.LocalExternalIDs.IGDB] {
			old.LocalExternalIDs.IGDB = ""
			changed = true
		}
		if !changed {
			continue
		}
		links := cloneProviderLinks(old.ProviderLinks)
		if links == nil {
			links = make(map[string][]string)
		}
		links["steam"] = kept
		links["igdb"] = keptIGDB
		old.ProviderLinks = links
		out[i] = old
		changedAny = true
	}
	return out, changedAny
}

func (s *Service) oracleAliases(items []Game) [][]string {
	byKey := map[string][]string{}
	for _, game := range s.games {
		id := s.resolveIDLocked(game.ID)
		if id == "" {
			continue
		}
		for _, key := range oracleIdentityKeys(game) {
			byKey[key] = append(byKey[key], id)
		}
	}
	aliases := make([][]string, len(items))
	for i, game := range items {
		seen := map[string]bool{}
		canonical := s.resolveIDLocked(game.ID)
		for _, key := range oracleIdentityKeys(game) {
			for _, id := range byKey[key] {
				if id != canonical && !seen[id] && s.sameGameLocked(canonical, id) {
					seen[id] = true
					aliases[i] = append(aliases[i], id)
				}
			}
		}
	}
	return aliases
}

func (s *Service) oracleGameAliases(game Game) []string {
	var ids []string
	for _, other := range s.games {
		if other.ID != game.ID && s.sameGameLocked(other.ID, game.ID) {
			ids = append(ids, other.ID)
		}
	}
	return ids
}

func parityValue(seed, i, pool int, prefix string) string {
	if benchPick(seed+91, i, 4) == 0 {
		return ""
	}
	if benchPick(seed+92, i, 5) == 0 {
		prefix = strings.ToUpper(prefix)
	}
	return fmt.Sprintf("%s%d", prefix, benchPick(seed, i, pool))
}

func parityLinks(seed, i, pool int, prefix string) []string {
	var out []string
	for k := range benchPick(seed+7, i, 3) {
		out = append(out, fmt.Sprintf("%s%d", prefix, benchPick(seed+k+1, i, pool)))
	}
	return out
}

// parityGames builds a catalog dense in shared ids: pools are smaller than the
// catalog, so duplicate ids, stale links and mixed-case ids are common.
func parityGames(n int) []Game {
	games := make([]Game, n)
	for i := range games {
		g := Game{
			ID:          fmt.Sprintf("g%03d", i),
			Title:       fmt.Sprintf("Parity Game %03d", i),
			SortTitle:   fmt.Sprintf("parity game %03d", i),
			ExternalIDs: ExternalIDs{IGDB: parityValue(1, i, n/3, "i"), Steam: parityValue(2, i, n/3, "s")},
		}
		if benchPick(3, i, 3) == 0 {
			g.ServerID = fmt.Sprintf("srv%03d", benchPick(4, i, n))
		}
		links := map[string][]string{}
		if v := parityLinks(5, i, n/3, "i"); len(v) > 0 {
			links["igdb"] = v
		}
		if v := parityLinks(6, i, n/3, "s"); len(v) > 0 {
			links["steam"] = v
		}
		if len(links) > 0 {
			g.ProviderLinks = links
		}
		if benchPick(8, i, 6) == 0 {
			g.LocalExternalIDs = ExternalIDs{IGDB: parityValue(9, i, n/3, "i"), Steam: parityValue(10, i, n/3, "s")}
		}
		games[i] = g
	}
	return games
}

func parityService(t *testing.T, n int) *Service {
	t.Helper()
	s, err := NewServiceAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.games = parityGames(n)
	s.redirects = map[string]string{}
	for i := range 5 {
		s.redirects[fmt.Sprintf("g%03d", i*3)] = fmt.Sprintf("g%03d", n/2+i*3)
	}
	s.redirects[fmt.Sprintf("g%03d", n/2)] = fmt.Sprintf("g%03d", n-1)
	s.rebuildLocked()
	return s
}

func positionsEqual(a, b map[string][]int) bool {
	if len(a) != len(b) {
		return false
	}
	for key, list := range a {
		if !reflect.DeepEqual(list, b[key]) {
			return false
		}
	}
	return true
}

func assertRemoteMapsMatch(t *testing.T, idx *index) {
	t.Helper()
	fresh := buildIndex(idx.games)
	for name, pair := range map[string][2]map[string][]int{
		"byServer":   {idx.byServer, fresh.byServer},
		"byIGDB":     {idx.byIGDB, fresh.byIGDB},
		"bySteam":    {idx.bySteam, fresh.bySteam},
		"byIdentity": {idx.byIdentity, fresh.byIdentity},
		"byLocal":    {idx.byLocal, fresh.byLocal},
	} {
		if !positionsEqual(pair[0], pair[1]) {
			t.Fatalf("%s drifted from a rebuild: got %v, want %v", name, pair[0], pair[1])
		}
	}
	byServer, byIGDB, bySteam := oracleRemoteIndexes(idx.games)
	got := map[string]map[string]int{"server": {}, "igdb": {}, "steam": {}}
	for key, list := range idx.byServer {
		got["server"][key] = list[len(list)-1]
	}
	for key, list := range idx.byIGDB {
		got["igdb"][key] = list[0]
	}
	for key, list := range idx.bySteam {
		got["steam"][key] = list[0]
	}
	for name, pair := range map[string][2]map[string]int{
		"server": {got["server"], byServer},
		"igdb":   {got["igdb"], byIGDB},
		"steam":  {got["steam"], bySteam},
	} {
		if !reflect.DeepEqual(pair[0], pair[1]) {
			t.Fatalf("%s winners differ from the full-pass maps: got %v, want %v", name, pair[0], pair[1])
		}
	}
	if want := oracleIdentityPostings(idx.games, false); !positionsEqual(idx.byIdentity, want) {
		t.Fatalf("byIdentity differs from the full pass: got %v, want %v", idx.byIdentity, want)
	}
	if want := oracleIdentityPostings(idx.games, true); !positionsEqual(idx.byLocal, want) {
		t.Fatalf("byLocal differs from the full pass: got %v, want %v", idx.byLocal, want)
	}
}

func TestIndexRemoteMapsFollowEveryMutation(t *testing.T) {
	s := parityService(t, 90)
	assertRemoteMapsMatch(t, s.idx)

	steps := []struct {
		name string
		act  func()
	}{
		{"update changes ids and links", func() {
			old := s.games[10]
			s.games[10].ExternalIDs = ExternalIDs{IGDB: "I5", Steam: "S9"}
			s.games[10].ServerID = "srv-moved"
			s.games[10].ProviderLinks = map[string][]string{"igdb": {"i1", "I2"}, "steam": {"s3"}}
			s.updateIndexAtLocked(10, old)
		}},
		{"update clears every id", func() {
			old := s.games[11]
			s.games[11].ExternalIDs = ExternalIDs{}
			s.games[11].ServerID = ""
			s.games[11].ProviderLinks = nil
			s.games[11].LocalExternalIDs = ExternalIDs{}
			s.updateIndexAtLocked(11, old)
		}},
		{"update gives two records one id", func() {
			old := s.games[40]
			s.games[40].ExternalIDs.IGDB = s.games[50].ExternalIDs.IGDB
			s.games[40].ServerID = s.games[50].ServerID
			s.updateIndexAtLocked(40, old)
		}},
		{"add registers a new record", func() {
			g := Game{ID: "added", Title: "Added", ServerID: "srv-added", ExternalIDs: ExternalIDs{IGDB: "I5", Steam: "S9"}, ProviderLinks: map[string][]string{"steam": {"s3"}}, LocalExternalIDs: ExternalIDs{IGDB: "i1"}}
			s.games = append(s.games, g)
			s.addToIndexLocked(g)
		}},
		{"rollback restores the old value", func() {
			current := s.games[10]
			s.games[10] = parityGames(90)[10]
			s.updateIndexAtLocked(10, current)
		}},
	}
	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			step.act()
			assertRemoteMapsMatch(t, s.idx)
		})
	}

	probes := []Game{
		{ServerID: "srv-moved"},
		{ServerID: "srv-added", ExternalIDs: ExternalIDs{IGDB: "nope"}},
		{ExternalIDs: ExternalIDs{IGDB: "i5"}},
		{ExternalIDs: ExternalIDs{IGDB: "I5", Steam: "s9"}},
		{ExternalIDs: ExternalIDs{Steam: "S9"}},
		{ExternalIDs: ExternalIDs{Steam: "none"}},
		{},
	}
	byServer, byIGDB, bySteam := oracleRemoteIndexes(s.games)
	for _, probe := range probes {
		if got, want := s.idx.matchRemote(probe), oracleRemoteMatch(probe, byServer, byIGDB, bySteam); got != want {
			t.Fatalf("matchRemote(%+v) = %d, full pass says %d", probe, got, want)
		}
	}
}

func TestReconcileUsesCandidatesAndMatchesFullPass(t *testing.T) {
	claimSets := [][]Game{
		{{ServerID: "srv-x", ExternalIDs: ExternalIDs{IGDB: "i1", Steam: "s2"}, ProviderLinks: map[string][]string{"steam": {"s2", "s3"}, "igdb": {"i1"}}}},
		{{ServerID: "srv001", ExternalIDs: ExternalIDs{IGDB: "I4"}, ProviderLinks: map[string][]string{"igdb": {"i7", "i8"}}}},
		{{ExternalIDs: ExternalIDs{IGDB: "i2", Steam: "S1"}, ProviderLinks: map[string][]string{"steam": {"s5", "S1"}}}, {ServerID: "srv003", ExternalIDs: ExternalIDs{IGDB: "i9"}}},
		{{ProviderLinks: map[string][]string{"steam": {""}}, ExternalIDs: ExternalIDs{IGDB: "i3"}}},
		{{ExternalIDs: ExternalIDs{Steam: "s6"}}},
	}
	moved := 0
	for n, claims := range claimSets {
		t.Run(fmt.Sprintf("claims%d", n), func(t *testing.T) {
			s := parityService(t, 120)
			cleaned := make([]Game, len(claims))
			for i, claim := range claims {
				cleaned[i] = claim
				cleaned[i].ProviderLinks = map[string][]string{}
				for provider, ids := range claim.ProviderLinks {
					cleaned[i].ProviderLinks[provider] = removeEmpty(ids)
				}
			}
			want, wantChanged := oracleReconcile(s.games, cleaned)
			before := append([]Game(nil), s.games...)
			epoch := s.epoch
			touched := map[int]Game{}
			s.mu.Lock()
			gotChanged := s.reconcileRemotePageLinksLocked(claims, touched)
			s.mu.Unlock()
			if gotChanged != wantChanged {
				t.Fatalf("changed = %v, full pass says %v", gotChanged, wantChanged)
			}
			if !reflect.DeepEqual(s.games, want) {
				for i := range want {
					if !reflect.DeepEqual(s.games[i], want[i]) {
						t.Fatalf("game %d = %+v, full pass leaves %+v", i, s.games[i], want[i])
					}
				}
			}
			for i := range want {
				_, isTouched := touched[i]
				if differs := !reflect.DeepEqual(before[i], want[i]); isTouched != differs {
					t.Fatalf("game %d touched = %v, but changed = %v", i, isTouched, differs)
				}
				if isTouched && !reflect.DeepEqual(touched[i], before[i]) {
					t.Fatalf("game %d rollback value = %+v, want %+v", i, touched[i], before[i])
				}
			}
			if gotChanged {
				moved++
				if s.epoch == epoch {
					t.Fatal("epoch did not move after a reconcile that changed games")
				}
			}
			assertRemoteMapsMatch(t, s.idx)
		})
	}
	if moved == 0 {
		t.Fatal("no claim set changed any record; the parity check proved nothing")
	}
}

func removeEmpty(ids []string) []string {
	var out []string
	for _, id := range ids {
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}

func TestReconcileClearsStaleClaimsOnGamesOutsideThePage(t *testing.T) {
	games := []Game{
		{ID: "steam-link", Title: "A", ExternalIDs: ExternalIDs{IGDB: "1"}, ProviderLinks: map[string][]string{"steam": {"11", "12"}}},
		{ID: "steam-id", Title: "B", ExternalIDs: ExternalIDs{IGDB: "3", Steam: "11"}},
		{ID: "steam-local", Title: "C", ExternalIDs: ExternalIDs{IGDB: "4"}, LocalExternalIDs: ExternalIDs{Steam: "11"}},
		{ID: "igdb-link", Title: "D", ExternalIDs: ExternalIDs{IGDB: "5"}, ProviderLinks: map[string][]string{"igdb": {"2", "5"}}},
		{ID: "igdb-local", Title: "E", ExternalIDs: ExternalIDs{IGDB: "6"}, LocalExternalIDs: ExternalIDs{IGDB: "2"}},
		{ID: "unrelated", Title: "F", ExternalIDs: ExternalIDs{IGDB: "7", Steam: "99"}, ProviderLinks: map[string][]string{"steam": {"99"}}},
		{ID: "no-igdb", Title: "G", ExternalIDs: ExternalIDs{Steam: "11"}},
		{ID: "same-server", Title: "H", ServerID: "srv-new", ExternalIDs: ExternalIDs{IGDB: "8"}, ProviderLinks: map[string][]string{"steam": {"11"}}},
		{ID: "same-igdb", Title: "I", ExternalIDs: ExternalIDs{IGDB: "2"}, ProviderLinks: map[string][]string{"steam": {"11"}}},
	}
	page := []Game{{ServerID: "srv-new", ExternalIDs: ExternalIDs{IGDB: "2", Steam: "11"}, ProviderLinks: map[string][]string{"steam": {"11"}}}}
	cases := []struct {
		id        string
		wantSteam []string
		wantIGDB  []string
		wantExt   ExternalIDs
		wantLocal ExternalIDs
		touched   bool
	}{
		{id: "steam-link", wantSteam: []string{"12"}, wantIGDB: []string{}, wantExt: ExternalIDs{IGDB: "1"}, touched: true},
		{id: "steam-id", wantSteam: []string{}, wantIGDB: []string{}, wantExt: ExternalIDs{IGDB: "3"}, touched: true},
		{id: "steam-local", wantSteam: []string{}, wantIGDB: []string{}, wantExt: ExternalIDs{IGDB: "4"}, touched: true},
		{id: "igdb-link", wantSteam: []string{}, wantIGDB: []string{"5"}, wantExt: ExternalIDs{IGDB: "5"}, touched: true},
		{id: "igdb-local", wantSteam: []string{}, wantIGDB: []string{}, wantExt: ExternalIDs{IGDB: "6"}, touched: true},
		{id: "unrelated", wantSteam: []string{"99"}, wantExt: ExternalIDs{IGDB: "7", Steam: "99"}},
		{id: "no-igdb", wantExt: ExternalIDs{Steam: "11"}},
		{id: "same-server", wantSteam: []string{"11"}, wantExt: ExternalIDs{IGDB: "8"}},
		{id: "same-igdb", wantSteam: []string{"11"}, wantExt: ExternalIDs{IGDB: "2"}},
	}
	s := newTestService(t)
	s.games = games
	s.rebuildLocked()
	touched := map[int]Game{}
	s.mu.Lock()
	changed := s.reconcileRemotePageLinksLocked(page, touched)
	s.mu.Unlock()
	if !changed {
		t.Fatal("reconcile reported no change")
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			got, ok := s.idx.game(tc.id)
			if !ok {
				t.Fatalf("game %q missing", tc.id)
			}
			if !reflect.DeepEqual(got.ProviderLinks["steam"], tc.wantSteam) || !reflect.DeepEqual(got.ProviderLinks["igdb"], tc.wantIGDB) {
				t.Fatalf("links = %v, want steam %v igdb %v", got.ProviderLinks, tc.wantSteam, tc.wantIGDB)
			}
			if got.ExternalIDs != tc.wantExt {
				t.Fatalf("external ids = %+v, want %+v", got.ExternalIDs, tc.wantExt)
			}
			if got.LocalExternalIDs != tc.wantLocal {
				t.Fatalf("local ids = %+v, want %+v", got.LocalExternalIDs, tc.wantLocal)
			}
			if _, isTouched := touched[s.idx.byID[tc.id]]; isTouched != tc.touched {
				t.Fatalf("touched = %v, want %v", isTouched, tc.touched)
			}
		})
	}
	assertRemoteMapsMatch(t, s.idx)
}

func TestPageAliasesMatchFullPass(t *testing.T) {
	s := parityService(t, 150)
	var items []Game
	for i := 0; i < len(s.games); i += 4 {
		items = append(items, s.games[i])
	}
	items = append(items,
		Game{ID: "fresh1", ExternalIDs: ExternalIDs{IGDB: "i1", Steam: "s2"}, ProviderLinks: map[string][]string{"steam": {"s3"}, "igdb": {"I4"}}},
		Game{ID: "fresh2", ExternalIDs: ExternalIDs{Steam: "S5"}},
		Game{ID: "fresh3"},
		Game{ID: s.games[3].ID},
	)
	s.mu.RLock()
	defer s.mu.RUnlock()
	want := s.oracleAliases(items)
	got := s.remotePageAliasesLocked(items)
	nonEmpty := 0
	for i := range items {
		if !reflect.DeepEqual(got[i], want[i]) {
			t.Fatalf("aliases of %s = %v, full pass says %v", items[i].ID, got[i], want[i])
		}
		if len(want[i]) > 0 {
			nonEmpty++
		}
	}
	if nonEmpty == 0 {
		t.Fatal("no item has aliases; the parity check proved nothing")
	}
}

func TestGetGameAliasesMatchFullPass(t *testing.T) {
	s := parityService(t, 150)
	withAliases, viaRedirect := 0, 0
	for _, g := range append([]Game(nil), s.games...) {
		got, err := s.GetGame(g.ID)
		if err != nil {
			t.Fatalf("GetGame(%s): %v", g.ID, err)
		}
		s.mu.RLock()
		base, _ := s.idx.game(g.ID)
		want := s.oracleGameAliases(base)
		s.mu.RUnlock()
		if !reflect.DeepEqual(got.AliasIDs, want) {
			t.Fatalf("aliases of %s = %v, full pass says %v", g.ID, got.AliasIDs, want)
		}
		if len(want) > 0 {
			withAliases++
		}
		if _, redirected := s.redirects[g.ID]; redirected && len(want) > 0 {
			viaRedirect++
		}
	}
	if withAliases == 0 || viaRedirect == 0 {
		t.Fatalf("fixture has %d games with aliases, %d through a redirect; the parity check proved nothing", withAliases, viaRedirect)
	}
}

func TestGetGameAliasesThroughRedirectOnly(t *testing.T) {
	s := newTestService(t)
	s.games = []Game{
		{ID: "old-a", Title: "Plain"},
		{ID: "old-b", Title: "Plain"},
		{ID: "target", Title: "Plain", ExternalIDs: ExternalIDs{IGDB: "9"}},
		{ID: "lonely", Title: "Lonely"},
	}
	s.redirects = map[string]string{"old-a": "old-b", "old-b": "target"}
	s.rebuildLocked()
	got, err := s.GetGame("target")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"old-a", "old-b"}; !reflect.DeepEqual(got.AliasIDs, want) {
		t.Fatalf("aliases = %v, want %v", got.AliasIDs, want)
	}
	if lonely, err := s.GetGame("lonely"); err != nil || len(lonely.AliasIDs) != 0 {
		t.Fatalf("a game without redirects or ids got aliases: %+v %v", lonely, err)
	}
}
