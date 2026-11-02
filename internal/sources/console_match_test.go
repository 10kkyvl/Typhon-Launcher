package sources

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"testing"
	"time"

	"typhon/internal/catalog"
	"typhon/internal/emulators"
	"typhon/internal/sources/feed"
	"typhon/internal/titles"
)

const pcName = "PC (Microsoft Windows)"

type consoleRemote struct {
	sourceMatchRemote
	answers map[string]catalog.ReleaseMatch
	asked   []catalog.ReleaseQuery
}

func consoleAskKey(title string, year int) string { return title + "|" + strconv.Itoa(year) }

func (r *consoleRemote) MatchReleases(_ context.Context, qs []catalog.ReleaseQuery) ([]catalog.ReleaseMatch, error) {
	r.asked = append(r.asked, qs...)
	out := make([]catalog.ReleaseMatch, len(qs))
	for i, q := range qs {
		if answer, ok := r.answers[consoleAskKey(q.Titles[0], q.Year)]; ok {
			out[i] = answer
		}
	}
	return out, nil
}

func consoleGame(id, title string, platforms ...string) *catalog.Game {
	return &catalog.Game{ID: id, Title: title, Platforms: platforms, ExternalIDs: catalog.ExternalIDs{IGDB: id}}
}

func TestConsoleIDsEqualEmulatorTable(t *testing.T) {
	table := make([]string, 0, len(emulators.Platforms()))
	for _, p := range emulators.Platforms() {
		table = append(table, p.ID)
	}
	if ids := titles.Consoles(); !slices.Equal(ids, table) {
		t.Fatalf("titles.Consoles() = %q, emulators table = %q", ids, table)
	}
}

func TestConsoleReleaseMatching(t *testing.T) {
	year := 2005
	cases := []struct {
		name       string
		answers    map[string]catalog.ReleaseMatch
		status     catalog.Status
		gameID     string
		reason     string
		candidates []string
		asks       int
	}{
		{
			name:    "chosen game on the console",
			answers: map[string]catalog.ReleaseMatch{"God of War|0": {Game: consoleGame("ps2-gow", "God of War", "PlayStation 2")}},
			status:  catalog.StatusMatched, gameID: "ps2-gow", asks: 1,
		},
		{
			name:    "chosen game on the PC only",
			answers: map[string]catalog.ReleaseMatch{"God of War|0": {Game: consoleGame("pc-gow", "God of War", pcName)}},
			status:  catalog.StatusUnmatched, reason: catalog.ReasonPlatform, asks: 1,
		},
		{
			name:    "chosen game on another console",
			answers: map[string]catalog.ReleaseMatch{"God of War|0": {Game: consoleGame("ps1-gow", "God of War", "PlayStation")}},
			status:  catalog.StatusUnmatched, reason: catalog.ReasonPlatform, asks: 1,
		},
		{
			name:    "chosen game without platforms",
			answers: map[string]catalog.ReleaseMatch{"God of War|0": {Game: consoleGame("x-gow", "God of War")}},
			status:  catalog.StatusUnmatched, reason: catalog.ReasonPlatform, asks: 1,
		},
		{
			name: "one console candidate becomes the game",
			answers: map[string]catalog.ReleaseMatch{
				"God of War|0": {Reason: catalog.ReasonAmbiguous, Candidates: []catalog.ReleaseCandidate{
					{ID: "pc-gow", Title: "God of War", ReleaseYear: 2018, Platforms: []string{pcName}},
					{ID: "ps2-gow", Title: "God of War", ReleaseYear: year, Platforms: []string{"PlayStation 2", "PlayStation 3"}},
				}},
				"God of War|2005": {Game: consoleGame("ps2-gow", "God of War", "PlayStation 2")},
			},
			status: catalog.StatusMatched, gameID: "ps2-gow", asks: 2,
		},
		{
			name: "one console candidate the server does not confirm stays for review",
			answers: map[string]catalog.ReleaseMatch{
				"God of War|0": {Reason: catalog.ReasonAmbiguous, Candidates: []catalog.ReleaseCandidate{
					{ID: "pc-gow", Title: "God of War", ReleaseYear: 2018, Platforms: []string{pcName}},
					{ID: "ps2-gow", Title: "God of War", ReleaseYear: year, Platforms: []string{"PlayStation 2"}},
				}},
				"God of War|2005": {Game: consoleGame("other", "God of War", "PlayStation 2")},
			},
			status: catalog.StatusReview, reason: catalog.ReasonAmbiguous, candidates: []string{"ps2-gow"}, asks: 2,
		},
		{
			name: "two console candidates stay for review without the PC one",
			answers: map[string]catalog.ReleaseMatch{"God of War|0": {Reason: catalog.ReasonAmbiguous, Candidates: []catalog.ReleaseCandidate{
				{ID: "pc-gow", Title: "God of War", Platforms: []string{pcName}},
				{ID: "ps2-a", Title: "God of War", Platforms: []string{"PlayStation 2"}},
				{ID: "ps2-b", Title: "God of War", Platforms: []string{"PlayStation 2"}},
			}}},
			status: catalog.StatusReview, reason: catalog.ReasonAmbiguous, candidates: []string{"ps2-a", "ps2-b"}, asks: 1,
		},
		{
			name: "no console candidate",
			answers: map[string]catalog.ReleaseMatch{"God of War|0": {Reason: catalog.ReasonAmbiguous, Candidates: []catalog.ReleaseCandidate{
				{ID: "pc-gow", Title: "God of War", Platforms: []string{pcName}},
				{ID: "x-gow", Title: "God of War"},
			}}},
			status: catalog.StatusUnmatched, reason: catalog.ReasonPlatform, asks: 1,
		},
		{
			name:    "unknown title",
			answers: map[string]catalog.ReleaseMatch{"God of War|0": {Reason: catalog.ReasonNone}},
			status:  catalog.StatusUnmatched, reason: catalog.ReasonNone, asks: 1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, cat, _ := testService(t)
			remote := &consoleRemote{answers: tc.answers}
			cat.SetRemoteCatalog(remote)
			server := newFeedServer(t, feedBody(t, "Source", feedEntry{Title: "God of War [PS2]", URIs: []string{magnetOf("a1")}}))
			src := addSource(t, s, server.url())

			items := releasesOf(t, s, src.ID, "")
			if len(items) != 1 {
				t.Fatalf("releases: %+v", items)
			}
			r := items[0].Release
			if r.Platform != titles.ConsolePS2 || r.Title != "God of War" {
				t.Fatalf("release: %+v", r)
			}
			if r.MatchStatus != tc.status {
				t.Fatalf("status = %q, want %q (%+v)", r.MatchStatus, tc.status, r.RemoteMatch)
			}
			got := ""
			if r.CanonicalGameID != nil {
				got = *r.CanonicalGameID
			}
			if got != tc.gameID {
				t.Fatalf("game = %q, want %q", got, tc.gameID)
			}
			if r.RemoteMatch == nil || r.RemoteMatch.Reason != tc.reason {
				t.Fatalf("remote match %+v, want reason %q", r.RemoteMatch, tc.reason)
			}
			var ids []string
			for _, c := range r.RemoteMatch.Candidates {
				ids = append(ids, c.ID)
			}
			if tc.status != catalog.StatusMatched && !slices.Equal(ids, tc.candidates) {
				t.Fatalf("candidates = %q, want %q", ids, tc.candidates)
			}
			if len(remote.asked) != tc.asks {
				t.Fatalf("%d queries, want %d: %+v", len(remote.asked), tc.asks, remote.asked)
			}
		})
	}
}

func TestConsoleQueryOnTheWireEqualsPCQuery(t *testing.T) {
	memo := newTitleMemo()
	pc := &Release{RawTitle: "God of War (2005)", Year: 2005}
	ps2 := &Release{RawTitle: "God of War [PS2] (2005)", Year: 2005, Platform: titles.ConsolePS2}
	_, queries, err := remoteReleaseQueries(context.Background(), memo, []*Release{pc, ps2})
	if err != nil {
		t.Fatal(err)
	}
	if len(queries) != 2 {
		t.Fatalf("one key for a PC and a console release: %+v", queries)
	}
	want := `{"titles":["God of War"],"year":2005}`
	for _, q := range queries {
		body, err := json.Marshal(q)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != want {
			t.Fatalf("wire body %s, want %s", body, want)
		}
	}
}

func TestPCAndConsoleReleasesOfOneTitleGetTheirOwnGames(t *testing.T) {
	s, cat, _ := testService(t)
	remote := &consoleRemote{answers: map[string]catalog.ReleaseMatch{
		"God of War|0": {Game: consoleGame("pc-gow", "God of War", pcName)},
	}}
	cat.SetRemoteCatalog(remote)
	server := newFeedServer(t, feedBody(t, "Source",
		feedEntry{Title: "God of War", URIs: []string{magnetOf("a1")}},
		feedEntry{Title: "God of War [PS2]", URIs: []string{magnetOf("a2")}},
	))
	addSource(t, s, server.url())
	groups := s.GetReleasesForGame("pc-gow")
	if len(groups) != 1 || groups[0].Release.Platform != "" {
		t.Fatalf("the PC card got: %+v", groups)
	}
}

func TestPCReleaseKeepsItsCacheKey(t *testing.T) {
	r := &Release{RawTitle: "God of War", Year: 2005}
	if key := remoteQueryKey(r); key != "God of War|2005" {
		t.Fatalf("key = %q", key)
	}
	r.Platform = titles.ConsolePS2
	if key := remoteQueryKey(r); key != "God of War|2005|ps2" {
		t.Fatalf("console key = %q", key)
	}
}

func TestSavedConsoleRowLeavesThePCCardOnRefresh(t *testing.T) {
	s, cat, _ := testService(t)
	remote := &consoleRemote{answers: map[string]catalog.ReleaseMatch{
		"God of War|0":     {Game: consoleGame("pc-gow", "God of War", pcName)},
		"God of War PS2|0": {Game: consoleGame("pc-gow", "God of War", pcName)},
	}}
	cat.SetRemoteCatalog(remote)
	server := newFeedServer(t, feedBody(t, "Source", feedEntry{Title: "God of War [PS2]", URIs: []string{magnetOf("a1")}}))
	src := addSource(t, s, server.url())

	id := "pc-gow"
	now := time.Now()
	s.mu.Lock()
	r := s.releases[src.ID][0]
	r.Platform = ""
	r.CanonicalGameID = &id
	r.MatchStatus = catalog.StatusMatched
	r.MatchMethod = string(catalog.MethodServerTitle)
	r.RemoteMatch = &RemoteMatch{Key: "God of War PS2|0", At: now, Status: catalog.StatusMatched, GameID: id, Rev: remoteMatchRev}
	s.mu.Unlock()
	remote.asked = nil

	summary, err := s.RefreshSource(src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !summary.NotModified {
		t.Fatalf("want the 304 path: %+v", summary)
	}
	if len(remote.asked) != 1 {
		t.Fatalf("queries after refresh: %+v", remote.asked)
	}
	if groups := s.GetReleasesForGame(id); len(groups) != 0 {
		t.Fatalf("the PC card kept: %+v", groups)
	}
	disk, err := s.store.loadReleases(src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if disk[0].Platform != titles.ConsolePS2 || disk[0].CanonicalGameID != nil {
		t.Fatalf("saved row: %+v", disk[0])
	}
}

func TestSavedReleaseWithoutPlatformLoads(t *testing.T) {
	var r Release
	if err := json.Unmarshal([]byte(`{"id":"a","rawTitle":"Game","title":"Game"}`), &r); err != nil {
		t.Fatal(err)
	}
	if r.Platform != "" {
		t.Fatalf("platform = %q", r.Platform)
	}
	body, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["platform"]; ok {
		t.Fatalf("a PC release writes the field: %s", body)
	}
}

func TestLocalMatchKeepsConsoleReleaseOffPCCard(t *testing.T) {
	cases := []struct {
		name      string
		platforms []string
		matched   bool
	}{
		{"pc game", []string{pcName}, false},
		{"no platforms", nil, false},
		{"same console", []string{"PlayStation 2"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, cat, _ := testService(t)
			game, err := cat.AddGame(catalog.Game{ID: "gow", Title: "God of War", Platforms: tc.platforms, ExternalIDs: catalog.ExternalIDs{IGDB: "1"}})
			if err != nil {
				t.Fatal(err)
			}
			list := parseEntries("src", []feed.Entry{{Title: "God of War [PS2]", URIs: []string{magnetOf("a1")}}}, time.Now())
			if err := applyMatches(cat, list); err != nil {
				t.Fatal(err)
			}
			got := list[0].CanonicalGameID != nil && *list[0].CanonicalGameID == game.ID
			if got != tc.matched {
				t.Fatalf("matched = %v, want %v: %+v", got, tc.matched, list[0])
			}
		})
	}
}

func TestLearnedMatchSkipsConsoleNamesake(t *testing.T) {
	s, cat, _ := testService(t)
	game, err := cat.AddGame(catalog.Game{ID: "gow", Title: "Some Other Title", ExternalIDs: catalog.ExternalIDs{IGDB: "1"}})
	if err != nil {
		t.Fatal(err)
	}
	server := newFeedServer(t, feedBody(t, "Source",
		feedEntry{Title: "God of War", URIs: []string{magnetOf("a1")}},
		feedEntry{Title: "God of War [PS2]", URIs: []string{magnetOf("a2")}},
	))
	src := addSource(t, s, server.url())
	var pc string
	for _, item := range releasesOf(t, s, src.ID, "") {
		if item.Release.Platform == "" {
			pc = item.Release.ID
		}
	}
	if err := s.ConfirmMatch(pc, game.ID); err != nil {
		t.Fatal(err)
	}
	groups := s.GetReleasesForGame(game.ID)
	if len(groups) != 1 || groups[0].Release.Platform != "" {
		t.Fatalf("groups: %+v", groups)
	}
}

func TestLocalMatchKeepsPCReleaseOffConsoleGame(t *testing.T) {
	cases := []struct {
		name      string
		platforms []string
		matched   bool
	}{
		{"console only", []string{"PlayStation 2"}, false},
		{"console and pc", []string{"PlayStation 2", pcName}, true},
		{"another console", []string{"Nintendo 64"}, true},
		{"no platforms", nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, cat, _ := testService(t)
			game, err := cat.AddGame(catalog.Game{ID: "gow", Title: "God of War", Platforms: tc.platforms, ExternalIDs: catalog.ExternalIDs{IGDB: "1"}})
			if err != nil {
				t.Fatal(err)
			}
			list := parseEntries("src", []feed.Entry{{Title: "God of War", URIs: []string{magnetOf("a1")}}}, time.Now())
			if err := applyMatches(cat, list); err != nil {
				t.Fatal(err)
			}
			got := list[0].CanonicalGameID != nil && *list[0].CanonicalGameID == game.ID
			if got != tc.matched {
				t.Fatalf("matched = %v, want %v: %+v", got, tc.matched, list[0])
			}
		})
	}
}

func TestConfirmedConsoleReleaseTeachesNoRuleForPC(t *testing.T) {
	s, cat, _ := testService(t)
	game, err := cat.AddGame(catalog.Game{ID: "ps2", Title: "Some Other Title", Platforms: []string{"PlayStation 2"}, ExternalIDs: catalog.ExternalIDs{IGDB: "1"}})
	if err != nil {
		t.Fatal(err)
	}
	server := newFeedServer(t, feedBody(t, "Source",
		feedEntry{Title: "God of War", URIs: []string{magnetOf("a1")}},
		feedEntry{Title: "God of War [PS2]", URIs: []string{magnetOf("a2")}},
	))
	src := addSource(t, s, server.url())
	var console string
	for _, item := range releasesOf(t, s, src.ID, "") {
		if item.Release.Platform != "" {
			console = item.Release.ID
		}
	}
	if err := s.ConfirmMatch(console, game.ID); err != nil {
		t.Fatal(err)
	}
	groups := s.GetReleasesForGame(game.ID)
	if len(groups) != 1 || groups[0].Release.ID != console {
		t.Fatalf("groups: %+v", groups)
	}
	if rules := cat.ListOverrides(); len(rules) != 0 {
		t.Fatalf("learned %+v", rules)
	}
	list := parseEntries("src", []feed.Entry{{Title: "God of War", URIs: []string{magnetOf("a1")}}}, time.Now())
	if err := applyMatches(cat, list); err != nil {
		t.Fatal(err)
	}
	if list[0].CanonicalGameID != nil {
		t.Fatalf("the PC release went to %q", *list[0].CanonicalGameID)
	}
}

func TestConsolePlatformReachesEveryReader(t *testing.T) {
	s, cat, _ := testService(t)
	remote := &consoleRemote{answers: map[string]catalog.ReleaseMatch{
		"God of War|0": {Game: consoleGame("ps2-gow", "God of War", "PlayStation 2")},
	}}
	cat.SetRemoteCatalog(remote)
	server := newFeedServer(t, feedBody(t, "Source", feedEntry{Title: "God of War [PS2]", URIs: []string{magnetOf("a1")}}))
	addSource(t, s, server.url())

	groups := s.GetReleasesForGame("ps2-gow")
	if len(groups) != 1 || groups[0].Release.Platform != titles.ConsolePS2 {
		t.Fatalf("GetReleasesForGame: %+v", groups)
	}
	byTitle := s.GetReleasesForTitle("God of War")
	if len(byTitle) != 1 || byTitle[0].Release.Platform != titles.ConsolePS2 {
		t.Fatalf("GetReleasesForTitle: %+v", byTitle)
	}
	list := s.ReleasesFor("ps2-gow", "")
	if len(list) != 1 || list[0].Platform != titles.ConsolePS2 {
		t.Fatalf("ReleasesFor: %+v", list)
	}
	body, err := json.Marshal(groups[0])
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Release struct {
			Platform string `json:"platform"`
		} `json:"release"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.Release.Platform != titles.ConsolePS2 {
		t.Fatalf("json: %s", body)
	}
}

func TestRelatedConsoleReleaseCarriesItsPlatform(t *testing.T) {
	s, cat, _ := testService(t)
	for _, g := range []catalog.Game{
		{ID: "pc", ServerID: "srv-pc", Title: "Game", ExternalIDs: catalog.ExternalIDs{IGDB: "1"}},
		{ID: "ps2", ServerID: "srv-ps2", Title: "Game Original", ExternalIDs: catalog.ExternalIDs{IGDB: "2"}},
	} {
		if _, err := cat.AddGame(g); err != nil {
			t.Fatal(err)
		}
	}
	cat.SetRemoteCatalog(&relatedFixture{items: []catalog.RelatedGame{
		{ID: "srv-ps2", Title: "Game Original", Relation: "port", Platforms: []string{"PlayStation 2"}},
	}})
	r := canonicalRelease("console", "h1", "ps2")
	r.Platform = titles.ConsolePS2
	s.mu.Lock()
	s.releases["src"] = []*Release{r}
	s.mu.Unlock()

	related, err := s.GetRelatedReleases(context.Background(), "pc")
	if err != nil {
		t.Fatal(err)
	}
	if len(related) != 1 || related[0].OnPC || len(related[0].Groups) != 1 || related[0].Groups[0].Release.Platform != titles.ConsolePS2 {
		t.Fatalf("related: %+v", related)
	}
}
