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
		Version:         ver,
		VersionSource:   VersionSourceRelease,
	}
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
	stamp := time.Unix(5000, 0)
	newer.UploadedAt = &stamp
	got := ResolveUpdate(installedAt("r1", "1.0"), []sources.Release{
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
