package sources

import (
	"testing"
	"time"

	"typhon/internal/catalog"
	"typhon/internal/settings"
	"typhon/internal/sources/feed"
)

func entry(title, magnet string, size int64) feed.Entry {
	return feed.Entry{Title: title, URIs: []string{magnet}, Size: size}
}

func TestParseEntriesExtractsMetadata(t *testing.T) {
	now := time.Now()
	list := parseEntries("src", []feed.Entry{
		entry("Cyberpunk.2077.Ultimate.Edition.v2.31.MULTi19.x64", magnetOf("a1"), 82<<30),
	}, now)

	if len(list) != 1 {
		t.Fatalf("releases = %d, want 1", len(list))
	}
	r := list[0]
	if r.Title != "Cyberpunk 2077" {
		t.Fatalf("title = %q, want Cyberpunk 2077", r.Title)
	}
	if r.Version != "2.31" {
		t.Fatalf("version = %q, want 2.31", r.Version)
	}
	if r.Edition != "Ultimate Edition" {
		t.Fatalf("edition = %q, want Ultimate Edition", r.Edition)
	}
	if r.InfoHash == "" {
		t.Fatal("infohash not extracted")
	}
	if r.Availability != AvailabilityAvailable {
		t.Fatalf("availability = %q", r.Availability)
	}
}

func TestParseEntriesDropsDuplicates(t *testing.T) {
	now := time.Now()
	list := parseEntries("src", []feed.Entry{
		entry("Some Game v1.0", magnetOf("b2"), 100),
		entry("Some Game v1.0 mirror", magnetOf("b2"), 100),
		entry("Other Game v1.0", magnetOf("c3"), 100),
	}, now)
	if len(list) != 2 {
		t.Fatalf("releases = %d, want 2", len(list))
	}
}

func TestMergeTracksLifecycle(t *testing.T) {
	now := time.Now()
	first := parseEntries("src", []feed.Entry{
		entry("Game One v1.0", magnetOf("11"), 10),
		entry("Game Two v1.0", magnetOf("22"), 20),
	}, now)

	merged, summary := merge(nil, first, now, true)
	if summary.Added != 2 || summary.New != 0 {
		t.Fatalf("initial summary = %+v, want 2 added and 0 new", summary)
	}
	for i := range merged {
		merged[i].ID = "id" + string(rune('a'+i))
	}

	later := now.Add(time.Hour)
	second := parseEntries("src", []feed.Entry{
		entry("Game One v1.1", magnetOf("11"), 15),
		entry("Game Three v1.0", magnetOf("33"), 30),
	}, later)

	merged, summary = merge(merged, second, later, false)
	if summary.Added != 1 || summary.New != 1 {
		t.Fatalf("summary = %+v, want 1 added and 1 new", summary)
	}
	if summary.Removed != 1 {
		t.Fatalf("removed = %d, want 1", summary.Removed)
	}
	if summary.Updated != 1 {
		t.Fatalf("updated = %d, want 1", summary.Updated)
	}
	if len(merged) != 3 {
		t.Fatalf("releases = %d, want 3", len(merged))
	}

	byTitle := map[string]*Release{}
	for _, r := range merged {
		byTitle[r.RawTitle] = r
	}
	if got := byTitle["Game One v1.1"]; got == nil || got.ID != "ida" || got.Size != 15 {
		t.Fatalf("updated release = %+v, want preserved id and new size", got)
	}
	if got := byTitle["Game Two v1.0"]; got == nil || got.Availability != AvailabilityRemoved {
		t.Fatalf("missing release should be marked removed, got %+v", got)
	}
}

func TestMergeRestoresRemovedRelease(t *testing.T) {
	now := time.Now()
	list := parseEntries("src", []feed.Entry{entry("Game One v1.0", magnetOf("11"), 10)}, now)
	list, _ = merge(nil, list, now, true)
	list, _ = merge(list, nil, now, false)
	if list[0].Availability != AvailabilityRemoved {
		t.Fatal("release should be removed")
	}

	again := parseEntries("src", []feed.Entry{entry("Game One v1.0", magnetOf("11"), 10)}, now)
	list, summary := merge(list, again, now, false)
	if summary.Restored != 1 {
		t.Fatalf("restored = %d, want 1", summary.Restored)
	}
	if list[0].Availability != AvailabilityAvailable {
		t.Fatalf("availability = %q, want available", list[0].Availability)
	}
}

func TestApplyMatchesProvisionsUnmatched(t *testing.T) {
	cat := mustCatalog(t, t.TempDir())
	now := time.Now()
	list := parseEntries("src", []feed.Entry{
		entry("Hades.II.v0.9", magnetOf("11"), 10),
		entry("Hades II v1.0", magnetOf("22"), 20),
	}, now)
	list, _ = merge(nil, list, now, true)

	applyMatches(cat, list)

	if list[0].CanonicalGameID == nil || list[1].CanonicalGameID == nil {
		t.Fatal("both releases should be attached to a canonical game")
	}
	if *list[0].CanonicalGameID != *list[1].CanonicalGameID {
		t.Fatal("releases of the same game must share the canonical game")
	}
	if list[0].MatchStatus != catalog.StatusMatched {
		t.Fatalf("status = %q, want matched", list[0].MatchStatus)
	}
	if len(cat.ListGames()) != 1 {
		t.Fatalf("catalog games = %d, want 1", len(cat.ListGames()))
	}
}

func TestApplyMatchesKeepsAmbiguousForReview(t *testing.T) {
	cat := mustCatalog(t, t.TempDir())
	prey2006 := 2006
	prey2017 := 2017
	if _, err := cat.AddGame(catalog.Game{Title: "Prey", ReleaseYear: &prey2006}); err != nil {
		t.Fatal(err)
	}
	if _, err := cat.AddGame(catalog.Game{Title: "Prey", ReleaseYear: &prey2017}); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	list := parseEntries("src", []feed.Entry{entry("Prey.MULTi9.v1.0", magnetOf("11"), 10)}, now)
	list, _ = merge(nil, list, now, true)
	applyMatches(cat, list)

	if list[0].MatchStatus != catalog.StatusReview {
		t.Fatalf("status = %q, want review", list[0].MatchStatus)
	}
	if list[0].CanonicalGameID != nil {
		t.Fatal("ambiguous release must not be attached to a game")
	}
	if len(cat.ListGames()) != 2 {
		t.Fatalf("catalog games = %d, want 2 (no provisioning for ambiguous titles)", len(cat.ListGames()))
	}
}

func TestApplyMatchesRecomputesAliasMatch(t *testing.T) {
	cat := mustCatalog(t, t.TempDir())
	wrong, err := cat.AddGame(catalog.Game{Title: "The Incredible Adventures of Van Helsing"})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	list := parseEntries("src", []feed.Entry{entry("Adorable Adventures", magnetOf("11"), 10)}, now)
	list, _ = merge(nil, list, now, true)
	stale := wrong.ID
	list[0].CanonicalGameID = &stale
	list[0].MatchStatus = catalog.StatusMatched
	list[0].MatchMethod = string(catalog.MethodAlias)
	list[0].MatchConfidence = 0.97

	applyMatches(cat, list)

	if list[0].CanonicalGameID == nil {
		t.Fatal("release lost its game instead of getting a fresh one")
	}
	if *list[0].CanonicalGameID == wrong.ID {
		t.Fatal("stale alias match survived the refresh")
	}
	if list[0].MatchMethod != string(catalog.MethodProvisional) {
		t.Fatalf("method = %q, want a freshly provisioned game", list[0].MatchMethod)
	}
}

func TestApplyMatchesKeepsExactMatch(t *testing.T) {
	cat := mustCatalog(t, t.TempDir())
	game, err := cat.AddGame(catalog.Game{Title: "Some Game"})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	list := parseEntries("src", []feed.Entry{entry("Some Game v1.0", magnetOf("11"), 10)}, now)
	list, _ = merge(nil, list, now, true)
	id := game.ID
	list[0].CanonicalGameID = &id
	list[0].MatchStatus = catalog.StatusMatched
	list[0].MatchMethod = string(catalog.MethodExactTitle)
	list[0].MatchConfidence = 0.98

	applyMatches(cat, list)

	if list[0].MatchConfidence != 0.98 || list[0].CanonicalGameID == nil || *list[0].CanonicalGameID != game.ID {
		t.Fatalf("exact match was recomputed: %+v", list[0])
	}
}

func TestApplyMatchesSkipsLocked(t *testing.T) {
	cat := mustCatalog(t, t.TempDir())
	now := time.Now()
	list := parseEntries("src", []feed.Entry{entry("Some Game v1.0", magnetOf("11"), 10)}, now)
	list, _ = merge(nil, list, now, true)
	manual := "manual-game"
	list[0].Locked = true
	list[0].CanonicalGameID = &manual
	list[0].MatchStatus = catalog.StatusMatched

	applyMatches(cat, list)

	if list[0].CanonicalGameID == nil || *list[0].CanonicalGameID != manual {
		t.Fatal("locked release must keep its manual match")
	}
	if len(cat.ListGames()) != 0 {
		t.Fatalf("catalog games = %d, want 0", len(cat.ListGames()))
	}
}

func TestRefreshIntervalMapping(t *testing.T) {
	cases := map[string]time.Duration{
		settings.RefreshManual:   0,
		settings.RefreshHourly:   time.Hour,
		settings.RefreshSixHours: 6 * time.Hour,
		settings.RefreshHalfDay:  12 * time.Hour,
		settings.RefreshDaily:    24 * time.Hour,
		"garbage":                6 * time.Hour,
	}
	for value, want := range cases {
		if got := refreshInterval(settings.Settings{SourceRefreshInterval: value}); got != want {
			t.Fatalf("interval for %q = %s, want %s", value, got, want)
		}
	}
}

func TestParseEntriesMapsPatchMetadata(t *testing.T) {
	now := time.Now()
	list := parseEntries("src", []feed.Entry{
		{
			Title:       "Cyberpunk 2077 Update 2.21 to 2.31",
			Game:        "Cyberpunk 2077",
			Type:        feed.TypePatch,
			FromVersion: "2.21",
			ToVersion:   "2.31",
			Sequence:    4,
			URIs:        []string{magnetOf("b1")},
			Size:        6 << 30,
		},
	}, now)

	if len(list) != 1 {
		t.Fatalf("releases = %d, want 1", len(list))
	}
	r := list[0]
	if r.Kind != KindPatch {
		t.Fatalf("kind = %q, want %q", r.Kind, KindPatch)
	}
	if r.Title != "Cyberpunk 2077" || r.NormalizedTitle != "cyberpunk 2077" {
		t.Fatalf("title = %q normalized = %q", r.Title, r.NormalizedTitle)
	}
	if r.FromVersion != "2.21" || r.ToVersion != "2.31" || r.Version != "2.31" {
		t.Fatalf("versions = %q -> %q (%q)", r.FromVersion, r.ToVersion, r.Version)
	}
	if r.Sequence != 4 {
		t.Fatalf("sequence = %d, want 4", r.Sequence)
	}
}
