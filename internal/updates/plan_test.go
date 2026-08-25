package updates

import (
	"context"
	"testing"
	"time"

	"typhon/internal/download"
	"typhon/internal/sources"
)

func planInputFor(reuse *download.ReuseReport) planInput {
	return planInput{
		Installed: InstalledGame{
			GameID:        "local-1",
			ReleaseID:     "r1",
			Version:       "1.0",
			VersionSource: VersionSourceRelease,
			InstallDir:    "C:\\Games\\Game",
		},
		Target: func() sources.Release {
			r := release("r2", "1.1", 100<<30)
			r.InfoHash = "abc"
			return r
		}(),
		Reuse: reuse,
	}
}

func TestTorrentReuseEstimate(t *testing.T) {
	report := &download.ReuseReport{
		InfoHash:     "abc",
		Layout:       download.LayoutDirectFiles,
		Flat:         true,
		TotalBytes:   100 << 30,
		MatchedBytes: 85 << 30,
		MissingBytes: 15 << 30,
	}
	in := planInputFor(report)
	strategy := torrentReuseStrategy{}
	if !strategy.CanHandle(context.Background(), in) {
		t.Fatal("direct file torrents with matching data must be reusable")
	}
	plan, err := strategy.Plan(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if plan.DownloadBytes != 15<<30 {
		t.Fatalf("download = %d, want %d", plan.DownloadBytes, int64(15)<<30)
	}
	if plan.ReusedBytes != 85<<30 {
		t.Fatalf("reused = %d, want %d", plan.ReusedBytes, int64(85)<<30)
	}
	if !plan.ReuseFlat {
		t.Fatal("plan must carry the chosen torrent mapping")
	}
}

func TestTorrentReuseRejectsArchivePackages(t *testing.T) {
	report := &download.ReuseReport{
		Layout:       download.LayoutArchivePackage,
		TotalBytes:   100 << 30,
		MatchedBytes: 90 << 30,
	}
	if (torrentReuseStrategy{}).CanHandle(context.Background(), planInputFor(report)) {
		t.Fatal("compressed installer packages must not claim reuse")
	}

	sparse := &download.ReuseReport{
		Layout:       download.LayoutDirectFiles,
		TotalBytes:   100 << 30,
		MatchedBytes: 2 << 30,
	}
	if (torrentReuseStrategy{}).CanHandle(context.Background(), planInputFor(sparse)) {
		t.Fatal("a negligible match must not claim reuse")
	}

	plan, err := fullReleaseStrategy{}.Plan(context.Background(), planInputFor(report))
	if err != nil {
		t.Fatal(err)
	}
	if plan.Strategy != StrategyFullRelease || plan.DownloadBytes != 100<<30 {
		t.Fatalf("fallback plan = %+v", plan)
	}
	if !plan.RollbackAvailable {
		t.Fatal("a staged full install must offer a rollback")
	}
}

func TestOrderReleasesPrefersVersionOverUploadDate(t *testing.T) {
	older := release("old", "1.2", 1)
	recent := time.Unix(9000, 0)
	older.UploadedAt = &recent

	newer := release("new", "1.10", 1)
	stale := time.Unix(100, 0)
	newer.UploadedAt = &stale

	ordered := OrderReleases([]sources.Release{older, newer})
	if ordered[0].ID != "new" {
		t.Fatalf("order = %q, %q; a newer upload must not outrank a newer version", ordered[0].ID, ordered[1].ID)
	}
}

func TestOrderReleasesFallsBackToUploadDate(t *testing.T) {
	first := release("a", "goty", 1)
	early := time.Unix(100, 0)
	first.UploadedAt = &early

	second := release("b", "repack", 1)
	late := time.Unix(9000, 0)
	second.UploadedAt = &late

	ordered := OrderReleases([]sources.Release{first, second})
	if ordered[0].ID != "b" {
		t.Fatalf("order = %q first, want b", ordered[0].ID)
	}
}

func TestOrderReleasesHonoursExplicitSequence(t *testing.T) {
	first := release("a", "1.5", 1)
	first.Sequence = 1
	second := release("b", "1.2", 1)
	second.Sequence = 7

	ordered := OrderReleases([]sources.Release{first, second})
	if ordered[0].ID != "b" {
		t.Fatalf("order = %q first, want b", ordered[0].ID)
	}
}

func TestCompatibleRejectsDifferentEdition(t *testing.T) {
	installed := InstalledGame{Edition: "Standard Edition"}
	vr := release("r2", "1.1", 1)
	vr.Edition = "VR Edition"
	if result := Compatible(installed, vr); result.Compatible {
		t.Fatalf("result = %+v", result)
	}

	same := release("r3", "1.1", 1)
	same.Edition = "standard edition"
	if result := Compatible(installed, same); !result.Compatible || result.Confidence < 1 {
		t.Fatalf("result = %+v", result)
	}
}

func TestCompatibleLowersConfidenceForForeignLanguage(t *testing.T) {
	installed := InstalledGame{Languages: []string{"RU"}}
	target := release("r2", "1.1", 1)
	target.Languages = []string{"JP"}
	result := Compatible(installed, target)
	if !result.Compatible || result.Confidence >= 1 {
		t.Fatalf("result = %+v", result)
	}
}
