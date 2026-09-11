package sources

import (
	"errors"
	"testing"
	"time"

	"typhon/internal/catalog"
	"typhon/internal/settings"
	"typhon/internal/sources/feed"
)

func entry(title, magnet string, size int64) feed.Entry {
	return feed.Entry{Title: title, URIs: []string{magnet}, Size: size}
}

type stubMatcher struct {
	resolve   func([]catalog.Query) []catalog.Match
	provision func([]catalog.Query) (map[string]catalog.Game, error)
	epoch     uint64
	resolved  int
}

func (m *stubMatcher) ResolveAll(queries []catalog.Query) []catalog.Match {
	m.resolved += len(queries)
	return m.resolve(queries)
}

func (m *stubMatcher) Epoch() uint64 {
	if m.epoch == 0 {
		return 1
	}
	return m.epoch
}

func (m *stubMatcher) Provision(queries []catalog.Query) (map[string]catalog.Game, error) {
	return m.provision(queries)
}

func TestApplyMatchesNeverProvisions(t *testing.T) {
	now := time.Now()
	list := parseEntries("src", []feed.Entry{entry("Some Unmatched Game v1.0", magnetOf("11"), 10)}, now)
	list, _ = merge(nil, list, now, true)

	boom := errors.New("boom")
	m := &stubMatcher{
		resolve: func(qs []catalog.Query) []catalog.Match {
			out := make([]catalog.Match, len(qs))
			for i := range out {
				out[i] = catalog.Match{Status: catalog.StatusUnmatched}
			}
			return out
		},
		provision: func([]catalog.Query) (map[string]catalog.Game, error) { return nil, boom },
	}

	if err := applyMatches(m, list); err != nil {
		t.Fatal(err)
	}
	if list[0].CanonicalGameID != nil {
		t.Fatal("unmatched release attached")
	}
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

func TestParseEntriesDropsAmbiguousDistributionBinding(t *testing.T) {
	now := time.Now()
	first := entry("Game v2.0", magnetOf("11"), 100)
	first.DistributionID = "game-main"
	second := entry("Game v3.0", magnetOf("22"), 200)
	second.DistributionID = "game-main"

	list := parseEntries("src", []feed.Entry{first, second}, now)
	if len(list) != 2 {
		t.Fatalf("releases = %d, want both ambiguous alternatives", len(list))
	}
	for _, release := range list {
		if release.DistributionID != "" {
			t.Fatalf("ambiguous distribution binding survived: %+v", release)
		}
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

func TestMergeDistributionKeepsReleaseAcrossChangedMagnetAndTitle(t *testing.T) {
	now := time.Now()
	first := entry("Game v1.0 [FitGirl Repack]", magnetOf("11"), 10)
	first.DistributionID = "game-fitgirl"
	merged, _ := merge(nil, parseEntries("src", []feed.Entry{first}, now), now, true)
	merged[0].ID = "stable-release"
	oldHash := merged[0].InfoHash

	next := entry("Game v2.0 [FitGirl Repack]", magnetOf("22"), 20)
	next.DistributionID = "game-fitgirl"
	merged, summary := merge(merged, parseEntries("src", []feed.Entry{next}, now.Add(time.Hour)), now.Add(time.Hour), false)

	if len(merged) != 1 || merged[0].ID != "stable-release" || merged[0].Version != "2.0" {
		t.Fatalf("release continuity lost: %+v", merged)
	}
	if merged[0].InfoHash == oldHash {
		t.Fatal("test fixture did not change infohash")
	}
	if summary.Updated != 1 || summary.Added != 0 || summary.Removed != 0 {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestMergeAddsDistributionToExactLegacyTorrent(t *testing.T) {
	now := time.Now()
	legacy := parseEntries("src", []feed.Entry{entry("Game v1.0", magnetOf("11"), 10)}, now)
	legacy, _ = merge(nil, legacy, now, true)
	legacy[0].ID = "legacy-release"

	next := entry("Game v1.0", magnetOf("11"), 10)
	next.DistributionID = "game-main"
	merged, _ := merge(legacy, parseEntries("src", []feed.Entry{next}, now.Add(time.Hour)), now.Add(time.Hour), false)
	if len(merged) != 1 || merged[0].ID != "legacy-release" || merged[0].DistributionID != "game-main" {
		t.Fatalf("legacy binding was not migrated exactly: %+v", merged)
	}
}

func TestMergeDoesNotMigrateLegacyTorrentToAmbiguousIncomingDistributions(t *testing.T) {
	now := time.Now()
	legacy := parseEntries("src", []feed.Entry{entry("Game v1.0", magnetOf("11"), 10)}, now)
	legacy, _ = merge(nil, legacy, now, true)
	legacy[0].ID = "legacy-release"

	a := entry("Game A v1.0", magnetOf("11"), 10)
	a.DistributionID = "distribution-a"
	b := entry("Game B v1.0", magnetOf("11"), 10)
	b.DistributionID = "distribution-b"
	merged, summary := merge(legacy, parseEntries("src", []feed.Entry{a, b}, now.Add(time.Hour)), now.Add(time.Hour), false)

	if len(merged) != 3 || summary.Added != 2 || summary.Removed != 1 {
		t.Fatalf("ambiguous migration should preserve separate records: merged=%+v summary=%+v", merged, summary)
	}
	for _, release := range merged {
		if release.ID == "legacy-release" {
			if release.DistributionID != "" || release.Availability != AvailabilityRemoved {
				t.Fatalf("legacy record was rebound: %+v", release)
			}
		}
	}
}

func TestMergeDoesNotMigrateLegacyTorrentWhenIncomingSiblingHasNoDistribution(t *testing.T) {
	now := time.Now()
	legacy := parseEntries("src", []feed.Entry{entry("Game v1.0", magnetOf("11"), 10)}, now)
	legacy, _ = merge(nil, legacy, now, true)
	legacy[0].ID = "legacy-release"

	known := entry("Game A v1.0", magnetOf("11"), 10)
	known.DistributionID = "distribution-a"
	unknown := entry("Game B v1.0", magnetOf("11"), 10)
	merged, summary := merge(legacy, parseEntries("src", []feed.Entry{known, unknown}, now.Add(time.Hour)), now.Add(time.Hour), false)

	if len(merged) != 2 || summary.Added != 1 || summary.Updated != 1 {
		t.Fatalf("mixed incoming ambiguity should not migrate distribution: merged=%+v summary=%+v", merged, summary)
	}
	for _, release := range merged {
		if release.ID == "legacy-release" && release.DistributionID != "" {
			t.Fatalf("legacy record was rebound: %+v", release)
		}
	}
}

func TestMergeDoesNotRebindKnownDistributionThroughLegacyIdentity(t *testing.T) {
	now := time.Now()
	first := entry("Game v1.0", magnetOf("11"), 10)
	first.DistributionID = "distribution-a"
	merged, _ := merge(nil, parseEntries("src", []feed.Entry{first}, now), now, true)
	merged[0].ID = "release-a"

	foreign := entry("Game v1.0", magnetOf("11"), 10)
	foreign.DistributionID = "distribution-b"
	merged, summary := merge(merged, parseEntries("src", []feed.Entry{foreign}, now.Add(time.Hour)), now.Add(time.Hour), false)

	if len(merged) != 2 || summary.Added != 1 || summary.Removed != 1 {
		t.Fatalf("foreign distribution should be separate: merged=%+v summary=%+v", merged, summary)
	}
	for _, release := range merged {
		if release.ID == "release-a" && release.DistributionID != "distribution-a" {
			t.Fatalf("known distribution was rebound: %+v", release)
		}
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

func TestApplyMatchesLeavesUnmatchedInSources(t *testing.T) {
	cat := mustCatalog(t, t.TempDir())
	now := time.Now()
	list := parseEntries("src", []feed.Entry{entry("Hades.II.v0.9", magnetOf("11"), 10), entry("Hades II v1.0", magnetOf("22"), 20)}, now)
	list, _ = merge(nil, list, now, true)
	if err := applyMatches(cat, list); err != nil {
		t.Fatal(err)
	}
	for _, r := range list {
		if r.CanonicalGameID != nil || r.MatchStatus != catalog.StatusUnmatched {
			t.Fatalf("unexpected attachment: %+v", r)
		}
	}
	if len(cat.ListGames()) != 0 {
		t.Fatal("source import created games")
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
	if err := applyMatches(cat, list); err != nil {
		t.Fatalf("applyMatches: %v", err)
	}

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

	if err := applyMatches(cat, list); err != nil {
		t.Fatalf("applyMatches: %v", err)
	}

	if list[0].CanonicalGameID != nil {
		t.Fatal("stale alias match survived refresh")
	}
	if len(cat.ListGames()) != 1 {
		t.Fatal("refresh created a replacement game")
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

	if err := applyMatches(cat, list); err != nil {
		t.Fatalf("applyMatches: %v", err)
	}

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

	if err := applyMatches(cat, list); err != nil {
		t.Fatalf("applyMatches: %v", err)
	}

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

func TestParseEntriesCarriesTags(t *testing.T) {
	now := time.Now()
	list := parseEntries("src", []feed.Entry{
		entry("Hogwarts Legacy Digital Deluxe Edition [FitGirl Repack] + 5 DLCs", magnetOf("a1"), 10),
	}, now)
	if len(list) != 1 {
		t.Fatalf("releases = %d, want 1", len(list))
	}
	r := list[0]
	if r.Repacker != "fitgirl" {
		t.Fatalf("repacker = %q, want fitgirl", r.Repacker)
	}
	if r.DLCCount != 5 {
		t.Fatalf("dlcCount = %d, want 5", r.DLCCount)
	}
	found := false
	for _, tag := range r.Tags {
		if tag == "fitgirl" {
			found = true
		}
	}
	if !found {
		t.Fatalf("tags = %v, want to contain fitgirl", r.Tags)
	}
}

func TestMergeCarriesTags(t *testing.T) {
	now := time.Now()
	first := parseEntries("src", []feed.Entry{
		entry("Some Game v1.0", magnetOf("11"), 10),
	}, now)
	merged, _ := merge(nil, first, now, true)
	merged[0].ID = "id0"

	later := now.Add(time.Hour)
	second := parseEntries("src", []feed.Entry{
		entry("Some Game v1.0 [DODI Repack] + 3 DLCs", magnetOf("11"), 20),
	}, later)
	merged, _ = merge(merged, second, later, false)

	if merged[0].Repacker != "dodi" {
		t.Fatalf("repacker = %q, want dodi", merged[0].Repacker)
	}
	if merged[0].DLCCount != 3 {
		t.Fatalf("dlcCount = %d, want 3", merged[0].DLCCount)
	}
	found := false
	for _, tag := range merged[0].Tags {
		if tag == "dodi" {
			found = true
		}
	}
	if !found {
		t.Fatalf("tags = %v, want to contain dodi after update", merged[0].Tags)
	}
}

func TestChangedDetectsTags(t *testing.T) {
	current := &Release{RawTitle: "Game", Version: "1.0"}
	next := &Release{RawTitle: "Game", Version: "1.0"}
	if changed(current, next) {
		t.Fatal("identical releases should not be reported as changed")
	}
	next.Tags = []string{"fitgirl"}
	if !changed(current, next) {
		t.Fatal("tag change was not detected")
	}
	current.Tags = []string{"fitgirl"}
	if changed(current, next) {
		t.Fatal("equal tags should not be reported as changed")
	}
	next.Repacker = "fitgirl"
	if !changed(current, next) {
		t.Fatal("repacker change was not detected")
	}
	current.Repacker = "fitgirl"
	next.DLCCount = 5
	if !changed(current, next) {
		t.Fatal("dlcCount change was not detected")
	}
	current.DLCCount = 5
	next.SizeUnknown = true
	if !changed(current, next) {
		t.Fatal("sizeUnknown change was not detected")
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
