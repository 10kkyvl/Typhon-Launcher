package updates

import (
	"testing"
	"time"
	"typhon/internal/library"
	"typhon/internal/sources"
)

func TestLegacyInstallFindsRevisionPublishedAfterInstallation(t *testing.T) {
	for _, version := range []string{"1.0", "special build"} {
		t.Run(version, func(t *testing.T) {
			uploaded := time.Unix(200, 0)
			candidate := release("r1", version, 100)
			candidate.UploadedAt = &uploaded
			game := library.Game{ID: "g", CanonicalGameID: canonical, SourceID: "src", DistributionID: "main", ReleaseID: "r1", Version: version, VersionSource: string(VersionSourceRelease), InstalledAt: time.Unix(100, 0)}
			svc, err := newServiceAt(t.TempDir(), nil)
			if err != nil {
				t.Fatal(err)
			}
			svc.library = &fakeLibrary{games: []library.Game{game}}
			bound := svc.bindLegacyDistribution(game, []sources.Release{candidate})
			if bound.ReleaseUploadedAt != nil {
				t.Fatal("migration mistook newer feed revision for installed revision")
			}
			result := ResolveUpdate(installedOf(bound), []sources.Release{candidate}, nil)
			if !result.Available || result.Kind != KindNewRelease {
				t.Fatalf("new revision not offered: %+v", result)
			}
			if result.InstalledReleaseUploadedAt != nil {
				t.Fatal("local installation time presented as provider metadata")
			}
			candidate.UploadedAt = &game.InstalledAt
			if got := ResolveUpdate(installedOf(game), []sources.Release{candidate}, nil); got.Available {
				t.Fatalf("equal-date revision offered: %+v", got)
			}
		})
	}
}

func TestLegacyRevisionBaselineUsesOnlyExactSavedRelease(t *testing.T) {
	oldDate, newDate := time.Unix(100, 0), time.Unix(200, 0)
	old := release("r1", "custom", 100)
	old.UploadedAt = &oldDate
	next := release("r2", "other", 100)
	next.UploadedAt = &newDate
	game := installedAt("r1", "custom")
	if result := ResolveUpdate(game, []sources.Release{old, next}, nil); !result.Available {
		t.Fatalf("known legacy baseline ignored: %+v", result)
	}
	old.SourceID = "foreign"
	if result := ResolveUpdate(game, []sources.Release{old, next}, nil); result.Available {
		t.Fatalf("foreign source supplied baseline: %+v", result)
	}
	game.ReleaseUploadedAt = &newDate
	old.SourceID = "src"
	if result := ResolveUpdate(game, []sources.Release{old, next}, nil); result.Available {
		t.Fatalf("saved upload baseline was overridden: %+v", result)
	}
}
