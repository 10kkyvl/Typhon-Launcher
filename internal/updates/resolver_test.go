package updates

import (
	"reflect"
	"testing"
	"time"

	"typhon/internal/catalog"
	"typhon/internal/sources"
)

const canonical = "game-1"

func release(id, ver string, size int64) sources.Release {
	gameID := canonical
	return sources.Release{
		ID:              id,
		SourceID:        "src",
		DistributionID:  "main",
		Kind:            sources.KindRelease,
		RawTitle:        "Game " + ver,
		Title:           "Game",
		NormalizedTitle: "game",
		CanonicalGameID: &gameID,
		Version:         ver,
		Size:            size,
		URIs:            []string{"magnet:?xt=urn:btih:" + id},
		MatchStatus:     catalog.StatusMatched,
		MatchConfidence: 1,
		Availability:    sources.AvailabilityAvailable,
		FirstSeenAt:     time.Unix(1000, 0),
	}
}

func installedAt(releaseID, ver string) InstalledGame {
	return InstalledGame{
		GameID:          "local-1",
		CanonicalGameID: canonical,
		Title:           "Game",
		ReleaseID:       releaseID,
		SourceID:        "src",
		DistributionID:  "main",
		Version:         ver,
		VersionSource:   VersionSourceRelease,
	}
}

func patchRelease(id, from, to string, size int64) sources.Release {
	r := release(id, to, size)
	r.Kind = sources.KindPatch
	r.FromVersion = from
	r.ToVersion = to
	return r
}

func TestResolveUpdateAvailable(t *testing.T) {
	got := ResolveUpdate(installedAt("r1", "1.0"), []sources.Release{
		release("r1", "1.0", 10<<30),
		release("r2", "1.1", 12<<30),
	}, nil)
	if !got.Available || got.Kind != KindUpdate {
		t.Fatalf("availability = %+v", got)
	}
	if got.TargetReleaseID != "r2" || got.TargetVersion != "1.1" {
		t.Fatalf("target = %q %q", got.TargetReleaseID, got.TargetVersion)
	}
	if got.Strategy != StrategyFullRelease || !got.RequiresFullInstall {
		t.Fatalf("strategy = %q full = %v", got.Strategy, got.RequiresFullInstall)
	}
	if got.EstimatedDownloadBytes != 12<<30 {
		t.Fatalf("estimate = %d", got.EstimatedDownloadBytes)
	}
}

func TestResolveUpdateRejectsNewerReleaseFromAnotherSource(t *testing.T) {
	other := release("other", "9.0", 1)
	other.SourceID = "other-source"
	got := ResolveUpdate(installedAt("r1", "1.0"), []sources.Release{other}, nil)
	if got.Available {
		t.Fatalf("foreign source offered as update: %+v", got)
	}
}

func TestResolveUpdateRejectsSiblingDistributionFromSameRepacker(t *testing.T) {
	other := release("other", "9.0", 1)
	other.DistributionID = "sibling"
	other.Repacker = "fitgirl"
	installed := installedAt("r1", "1.0")
	got := ResolveUpdate(installed, []sources.Release{other}, nil)
	if got.Available {
		t.Fatalf("sibling distribution offered as update: %+v", got)
	}
}

func TestResolveUpdateAcceptsChangedTorrentOnConfirmedDistribution(t *testing.T) {
	target := release("new-record", "2.0", 1)
	target.InfoHash = "entirely-new-infohash"
	got := ResolveUpdate(installedAt("old-record", "1.0"), []sources.Release{target}, nil)
	if !got.Available || got.TargetReleaseID != "new-record" {
		t.Fatalf("confirmed distribution update rejected: %+v", got)
	}
}

func TestResolveUpdateAcceptsVersionChangeOnSameReleaseRecord(t *testing.T) {
	target := release("r1", "2.0", 1)
	got := ResolveUpdate(installedAt("r1", "1.0"), []sources.Release{target}, nil)
	if !got.Available || got.TargetReleaseID != "r1" || got.TargetVersion != "2.0" {
		t.Fatalf("updated stable record not offered: %+v", got)
	}
}

func TestResolveUpdateRejectsUnknownOrLostBinding(t *testing.T) {
	target := release("r2", "2.0", 1)
	cases := []InstalledGame{
		{GameID: "local", SourceID: "src", Version: "1.0"},
		{GameID: "local", ReleaseID: "r1", Version: "1.0"},
		{GameID: "local", ReleaseID: "missing", SourceID: "src", Version: "1.0"},
	}
	for _, installed := range cases {
		if got := ResolveUpdate(installed, []sources.Release{target}, nil); got.Available {
			t.Fatalf("unknown binding %+v offered %+v", installed, got)
		}
	}
}

func TestResolveUpdateRejectsForeignPatchesWithMatchingVersions(t *testing.T) {
	foreign := patch("foreign", "1.0", "2.0", 1)
	foreign.DistributionID = "sibling"
	got := ResolveUpdate(installedAt("r1", "1.0"), []sources.Release{release("r2", "2.0", 100)}, []Patch{foreign})
	if !got.Available || got.Strategy != StrategyFullRelease || got.PatchCount != 0 {
		t.Fatalf("foreign patch entered update strategy: %+v", got)
	}
}

func TestResolveUpdateLegacyBindingOnlyFollowsExactRecord(t *testing.T) {
	installed := installedAt("r1", "1.0")
	installed.DistributionID = ""
	exact := release("r1", "1.1", 1)
	exact.DistributionID = ""
	if got := ResolveUpdate(installed, []sources.Release{exact}, nil); !got.Available {
		t.Fatalf("exact legacy record not followed: %+v", got)
	}
	other := release("r2", "2.0", 1)
	other.DistributionID = ""
	if got := ResolveUpdate(installed, []sources.Release{other}, nil); got.Available {
		t.Fatalf("legacy metadata guess offered: %+v", got)
	}
}

func TestResolveUpdateInstalledIsNewest(t *testing.T) {
	got := ResolveUpdate(installedAt("r2", "1.1"), []sources.Release{
		release("r1", "1.0", 10<<30),
		release("r2", "1.1", 12<<30),
	}, nil)
	if got.Available || got.Kind != KindNone {
		t.Fatalf("availability = %+v", got)
	}
}

func TestResolveUpdateAmbiguousVersionIsNewRelease(t *testing.T) {
	newer := release("r2", "goty", 12<<30)
	installedStamp := time.Unix(1000, 0)
	stamp := time.Unix(5000, 0)
	newer.UploadedAt = &stamp
	installed := installedAt("r1", "1.0")
	installed.ReleaseUploadedAt = &installedStamp
	got := ResolveUpdate(installed, []sources.Release{
		release("r1", "1.0", 10<<30),
		newer,
	}, nil)
	if !got.Available {
		t.Fatal("expected the release to be surfaced")
	}
	if got.Kind != KindNewRelease {
		t.Fatalf("kind = %q, want %q", got.Kind, KindNewRelease)
	}
	if got.Reason == "" {
		t.Fatal("expected a reason for the weaker claim")
	}
}

func TestResolveUpdateUsesStoredRevisionDateWhenStableRecordChangesVersionScheme(t *testing.T) {
	installedStamp := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	targetStamp := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	installed := installedAt("r1", "1.0")
	installed.ReleaseUploadedAt = &installedStamp
	target := release("r1", "build-20260910", 1)
	target.UploadedAt = &targetStamp

	got := ResolveUpdate(installed, []sources.Release{target}, nil)
	if !got.Available || got.Kind != KindNewRelease || got.TargetReleaseID != "r1" {
		t.Fatalf("changed version scheme was not offered from immutable revision date: %+v", got)
	}
}

func TestResolveUpdateUsesStoredRevisionDateWhenStableRecordHasNoVersion(t *testing.T) {
	installedStamp := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	targetStamp := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	installed := installedAt("r1", "")
	installed.VersionSource = VersionSourceUnknown
	installed.ReleaseUploadedAt = &installedStamp
	target := release("r1", "", 1)
	target.InfoHash = "changed-infohash"
	target.UploadedAt = &targetStamp

	got := ResolveUpdate(installed, []sources.Release{target}, nil)
	if !got.Available || got.Kind != KindNewRelease || got.TargetReleaseID != "r1" {
		t.Fatalf("new revision without a version was not offered: %+v", got)
	}
}

func TestResolveUpdateUsesStoredRevisionDateWhenVersionIsUnchanged(t *testing.T) {
	installedStamp := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	targetStamp := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	installed := installedAt("r1", "1.0")
	installed.ReleaseUploadedAt = &installedStamp
	target := release("r1", "1.0", 1)
	target.UploadedAt = &targetStamp

	got := ResolveUpdate(installed, []sources.Release{target}, nil)
	if !got.Available || got.Kind != KindNewRelease || got.TargetReleaseID != "r1" {
		t.Fatalf("new revision with an unchanged version was not offered: %+v", got)
	}
	if got.Reason != "new_distribution_revision" {
		t.Fatalf("unchanged version must be explained as a new distribution revision: %q", got.Reason)
	}
}

func TestResolveUpdateDoesNotGuessRecencyWithoutInstalledRevisionDate(t *testing.T) {
	targetStamp := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	target := release("r1", "build-20260910", 1)
	target.UploadedAt = &targetStamp

	if got := ResolveUpdate(installedAt("r1", "1.0"), []sources.Release{target}, nil); got.Available {
		t.Fatalf("unknown installed revision date was guessed as older: %+v", got)
	}
}

func TestResolveUpdateUnknownInstalledVersion(t *testing.T) {
	installed := installedAt("r1", "")
	installed.VersionSource = VersionSourceUnknown
	got := ResolveUpdate(installed, []sources.Release{
		release("r1", "", 10<<30),
		release("r2", "1.1", 12<<30),
	}, nil)
	if got.Kind == KindUpdate {
		t.Fatalf("must not claim an update for an unknown installed version: %+v", got)
	}
}

func TestResolveUpdateIncompatibleEdition(t *testing.T) {
	installed := installedAt("r1", "1.0")
	installed.Edition = "Standard Edition"
	vr := release("r2", "1.1", 12<<30)
	vr.Edition = "VR Edition"
	got := ResolveUpdate(installed, []sources.Release{release("r1", "1.0", 10<<30), vr}, nil)
	if got.Available {
		t.Fatalf("incompatible edition must not be offered: %+v", got)
	}
}

func TestResolveUpdatePrefersPatchChain(t *testing.T) {
	patches := []Patch{
		patch("p1", "1.0", "1.1", 2<<30),
		patch("p2", "1.1", "1.2", 3<<30),
	}
	got := ResolveUpdate(installedAt("r1", "1.0"), []sources.Release{
		release("r1", "1.0", 40<<30),
		release("r2", "1.2", 40<<30),
	}, patches)
	if got.Strategy != StrategyPatchChain {
		t.Fatalf("strategy = %q", got.Strategy)
	}
	if got.PatchCount != 2 || got.EstimatedDownloadBytes != 5<<30 {
		t.Fatalf("patches = %d bytes = %d", got.PatchCount, got.EstimatedDownloadBytes)
	}
	if got.RequiresFullInstall {
		t.Fatal("patch chain must not require a full install")
	}
}

func TestResolveUpdateDeterministic(t *testing.T) {
	installed := installedAt("r1", "1.0")
	list := []sources.Release{
		release("r1", "1.0", 10<<30),
		release("r3", "1.2", 12<<30),
		release("r2", "1.1", 11<<30),
	}
	first := ResolveUpdate(installed, list, nil)
	shuffled := []sources.Release{list[2], list[0], list[1]}
	second := ResolveUpdate(installed, shuffled, nil)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("resolver is not deterministic:\n%+v\n%+v", first, second)
	}
	if first.TargetReleaseID != "r3" {
		t.Fatalf("target = %q, want r3", first.TargetReleaseID)
	}
}

func TestPatchesFromReleases(t *testing.T) {
	gameID := canonical
	list := []sources.Release{
		release("r1", "1.0", 1),
		{
			ID:              "p1",
			Kind:            sources.KindPatch,
			CanonicalGameID: &gameID,
			FromVersion:     "1.0",
			ToVersion:       "1.1",
			Size:            2 << 30,
			Availability:    sources.AvailabilityAvailable,
		},
		{
			ID:           "p2",
			Kind:         sources.KindPatch,
			FromVersion:  "1.1",
			Availability: sources.AvailabilityAvailable,
		},
	}
	patches := PatchesFrom(list)
	if len(patches) != 1 || patches[0].ID != "p1" || patches[0].GameID != canonical {
		t.Fatalf("patches = %+v", patches)
	}
}

// A source ships a compressed repack next to the portable build on the same day
// and neither carries a comparable version. The ids matter: sorted by id the
// repack lands before the installed build, which is how it was ever reached.
func TestResolveUpdateIgnoresSameDayReleaseWithoutVersion(t *testing.T) {
	stamp := time.Date(2026, 3, 25, 11, 9, 14, 0, time.UTC)
	installedRelease := release("r2-portable", "", 9790000000)
	installedRelease.UploadedAt = &stamp
	repack := release("r1-repack", "", 1820000000)
	repack.UploadedAt = &stamp

	installed := installedAt("r2-portable", "")
	installed.VersionSource = VersionSourceUnknown
	installed.ReleaseUploadedAt = &stamp
	got := ResolveUpdate(installed, []sources.Release{installedRelease, repack}, nil)

	if got.Available || got.Kind != KindNone {
		t.Fatalf("same-day release must not be offered: %+v", got)
	}
}

func TestResolveUpdateOffersStrictlyNewerReleaseWithoutVersion(t *testing.T) {
	older := time.Date(2026, 3, 25, 11, 9, 14, 0, time.UTC)
	newer := older.Add(48 * time.Hour)
	installedRelease := release("r-old", "", 9790000000)
	installedRelease.UploadedAt = &older
	candidate := release("r-new", "", 9800000000)
	candidate.UploadedAt = &newer

	installed := installedAt("r-old", "")
	installed.VersionSource = VersionSourceUnknown
	installed.ReleaseUploadedAt = &older
	got := ResolveUpdate(installed, []sources.Release{installedRelease, candidate}, nil)

	if !got.Available || got.Kind != KindNewRelease {
		t.Fatalf("a strictly newer release must still be offered: %+v", got)
	}
	if got.TargetReleaseID != "r-new" {
		t.Fatalf("target = %q, want r-new", got.TargetReleaseID)
	}
}
