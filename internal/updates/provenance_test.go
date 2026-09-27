package updates

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"typhon/internal/catalog"
	"typhon/internal/catalogmigration"
	"typhon/internal/download"
	"typhon/internal/library"
	"typhon/internal/sources"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestInstalledDistributionFlowImportsRegistersAndSelectsExactLine(t *testing.T) {
	root := t.TempDir()
	cat, err := catalog.NewServiceAt(filepath.Join(root, "catalog"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cat.AddGame(catalog.Game{Title: "Flow Game", ExternalIDs: catalog.ExternalIDs{IGDB: "123"}}); err != nil {
		t.Fatal(err)
	}
	releaseSource, err := sources.NewServiceAt(filepath.Join(root, "sources"), cat)
	if err != nil {
		t.Fatal(err)
	}
	if err := releaseSource.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := releaseSource.ServiceShutdown(); err != nil {
			t.Error(err)
		}
	})

	feedPath := filepath.Join(root, "feed.json")
	writeDistributionFeed(t, feedPath,
		feedRelease{DistributionID: "distribution-a", Title: "Flow Game v1.0 [FitGirl Repack]", Version: "1.0", Hash: "aa", UploadedAt: "2026-09-01T12:00:00Z"},
	)
	source, err := releaseSource.AddSourceFile(feedPath)
	if err != nil {
		t.Fatal(err)
	}
	page := releaseSource.QueryReleases(sources.ReleaseQuery{SourceID: source.ID, Status: "all", PageSize: 100})
	if len(page.Items) != 1 {
		t.Fatalf("initial imported releases = %d, want 1", len(page.Items))
	}
	installedRelease := page.Items[0].Release
	request, err := releaseSource.PrepareDownload(installedRelease.ID)
	if err != nil {
		t.Fatal(err)
	}
	if request.DistributionID != "distribution-a" || request.ReleaseUploadedAt == nil {
		t.Fatalf("download request lost source provenance: %+v", request)
	}

	installDir := filepath.Join(root, "installed", "Flow Game")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(installDir, "flow-game.exe")
	if err := os.WriteFile(executable, []byte("isolated fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	lib, err := library.NewServiceAt(filepath.Join(root, "library.json"))
	if err != nil {
		t.Fatal(err)
	}
	game, err := lib.RegisterInstalled(library.InstalledGame{
		Title:             "Flow Game",
		Executable:        executable,
		InstallDir:        installDir,
		Version:           request.Version,
		VersionSource:     string(VersionSourceRelease),
		ReleaseID:         request.ReleaseID,
		SourceID:          request.SourceID,
		DistributionID:    request.DistributionID,
		ReleaseUploadedAt: request.ReleaseUploadedAt,
		CanonicalGameID:   request.GameID,
	})
	if err != nil {
		t.Fatal(err)
	}

	// A later confirmed identity merge changes only catalog redirects. The
	// installed release and update line must survive that migration unchanged.
	if _, err = cat.AddGame(catalog.Game{ID: "canonical-target", Title: "Flow Game", CreatedAt: time.Unix(1, 0), ExternalIDs: catalog.ExternalIDs{IGDB: "123"}}); err != nil {
		t.Fatal(err)
	}
	if err = releaseSource.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
	report, err := catalogmigration.Plan(filepath.Join(root, "catalog"))
	if err != nil {
		t.Fatal(err)
	}
	if err = catalogmigration.Apply(filepath.Join(root, "catalog"), report); err != nil {
		t.Fatal(err)
	}
	cat, err = catalog.NewServiceAt(filepath.Join(root, "catalog"))
	if err != nil {
		t.Fatal(err)
	}
	lib.SetCanonicalIdentity(cat.SameGame)
	releaseSource, err = sources.NewServiceAt(filepath.Join(root, "sources"), cat)
	if err != nil {
		t.Fatal(err)
	}
	if err = releaseSource.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	if !cat.SameGame(game.CanonicalGameID, "canonical-target") || game.ReleaseID != installedRelease.ID {
		t.Fatal("migration lost installation identity")
	}
	if groups := releaseSource.GetReleasesForGame("canonical-target"); len(groups) != 1 {
		t.Fatalf("redirect lost release: %+v", groups)
	}

	updateService, err := newServiceAt(filepath.Join(root, "updates"), nil)
	if err != nil {
		t.Fatal(err)
	}
	updateService.library = lib
	updateService.releases = releaseSource
	updateService.ctx, updateService.cancel = context.WithCancel(context.Background())
	t.Cleanup(func() {
		updateService.cancel()
		updateService.wg.Wait()
	})

	writeDistributionFeed(t, feedPath,
		feedRelease{DistributionID: "distribution-a", Title: "Flow Game v1.0 [FitGirl Repack]", Version: "1.0", Hash: "aa", UploadedAt: "2026-09-01T12:00:00Z"},
		feedRelease{DistributionID: "distribution-b", Title: "Flow Game v9.0 [FitGirl Repack]", Version: "9.0", Hash: "bb", UploadedAt: "2026-09-10T12:00:00Z"},
	)
	if _, err := releaseSource.RefreshSource(source.ID); err != nil {
		t.Fatal(err)
	}
	if err := updateService.check(game); err != nil {
		t.Fatal(err)
	}
	foreignOnly, _ := updateService.snapshot(game.ID)
	if foreignOnly.Availability.Available {
		t.Fatalf("distribution B was offered for installed A: %+v", foreignOnly.Availability)
	}

	writeDistributionFeed(t, feedPath,
		feedRelease{DistributionID: "distribution-a", Title: "Flow Game v2.0 [FitGirl Repack]", Version: "2.0", Hash: "cc", UploadedAt: "2026-09-11T12:00:00Z"},
		feedRelease{DistributionID: "distribution-b", Title: "Flow Game v9.0 [FitGirl Repack]", Version: "9.0", Hash: "bb", UploadedAt: "2026-09-10T12:00:00Z"},
	)
	if _, err := releaseSource.RefreshSource(source.ID); err != nil {
		t.Fatal(err)
	}
	if err := updateService.check(game); err != nil {
		t.Fatal(err)
	}
	ownUpdate, _ := updateService.snapshot(game.ID)
	if !ownUpdate.Availability.Available || ownUpdate.Availability.TargetReleaseID != installedRelease.ID {
		t.Fatalf("distribution A update was not offered on its stable record: %+v", ownUpdate.Availability)
	}
	plan, err := updateService.buildPlan(context.Background(), game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if plan.TargetReleaseID != installedRelease.ID || plan.SourceID != source.ID || plan.DistributionID != "distribution-a" {
		t.Fatalf("plan escaped distribution A: %+v", plan)
	}
	if got := releaseSource.QueryReleases(sources.ReleaseQuery{SourceID: source.ID, Status: "all", PageSize: 100}).Total; got != 2 {
		t.Fatalf("catalog alternatives = %d, want 2", got)
	}
}

type feedRelease struct {
	DistributionID string
	Title          string
	Version        string
	Hash           string
	UploadedAt     string
}

func writeDistributionFeed(t *testing.T, path string, releases ...feedRelease) {
	t.Helper()
	body := `{"name":"Integration source","version":1,"downloads":[`
	for i, release := range releases {
		if i > 0 {
			body += ","
		}
		body += fmt.Sprintf(
			`{"distributionId":%q,"game":"Flow Game","title":%q,"version":%q,"uri":%q,"uploadDate":%q,"fileSize":1048576}`,
			release.DistributionID,
			release.Title,
			release.Version,
			"magnet:?xt=urn:btih:"+release.Hash+"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			release.UploadedAt,
		)
	}
	body += "]}"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyInstallBindsOnlyThroughExactSavedRelease(t *testing.T) {
	h := newHarness(t)
	h.library.games[0].DistributionID = ""
	h.releases.list = []sources.Release{release("r1", "1.0", 1), release("r2", "2.0", 2)}
	if err := h.service.check(h.library.games[0]); err != nil {
		t.Fatal(err)
	}
	if got := h.library.games[0].DistributionID; got != "main" {
		t.Fatalf("legacy distribution = %q, want main", got)
	}
	u, _ := h.service.snapshot("local-1")
	if !u.Availability.Available || u.Availability.TargetReleaseID != "r2" {
		t.Fatalf("availability after exact migration = %+v", u.Availability)
	}
}

func TestLegacyInstallBackfillsRevisionDateOnlyForExactInstalledVersion(t *testing.T) {
	h := newHarness(t)
	h.library.games[0].DistributionID = ""
	stamp := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	current := release("r1", "1.0", 1)
	current.UploadedAt = &stamp
	h.releases.list = []sources.Release{current}

	if err := h.service.check(h.library.games[0]); err != nil {
		t.Fatal(err)
	}
	bound := h.library.games[0]
	if bound.DistributionID != "main" || bound.ReleaseUploadedAt == nil || !bound.ReleaseUploadedAt.Equal(stamp) {
		t.Fatalf("exact legacy revision was not recovered: %+v", bound)
	}

	h = newHarness(t)
	h.library.games[0].DistributionID = ""
	changedScheme := release("r1", "build-20260910", 1)
	changedScheme.UploadedAt = &stamp
	h.releases.list = []sources.Release{changedScheme}
	if err := h.service.check(h.library.games[0]); err != nil {
		t.Fatal(err)
	}
	unknownRevision := h.library.games[0]
	if unknownRevision.DistributionID != "main" || unknownRevision.ReleaseUploadedAt != nil {
		t.Fatalf("legacy revision date was guessed after source mutation: %+v", unknownRevision)
	}
	u, _ := h.service.snapshot("local-1")
	if u.Availability.Available {
		t.Fatalf("unknown legacy revision was offered by date: %+v", u.Availability)
	}
}

func TestStartUpdateRejectsPlanWhoseTargetChangedDistribution(t *testing.T) {
	h := newHarness(t)
	plan := h.plan(t)
	for i := range h.releases.list {
		if h.releases.list[i].ID == plan.TargetReleaseID {
			h.releases.list[i].DistributionID = "foreign"
		}
	}
	if err := h.service.StartUpdate("local-1"); !errors.Is(err, errNoTarget) {
		t.Fatalf("StartUpdate = %v, want stale plan rejection", err)
	}
	if h.service.Busy("local-1") || len(h.downloads.requests) != 0 {
		t.Fatalf("stale plan started work: busy=%v downloads=%d", h.service.Busy("local-1"), len(h.downloads.requests))
	}
}

func TestForeignPrefetchTaskIsNotReused(t *testing.T) {
	h := newHarness(t)
	plan := h.plan(t)
	h.downloads.tasks["foreign-prefetch"] = &download.Download{
		ID: "foreign-prefetch", Status: download.StatusCompleted,
		Origin: download.Origin{
			ReleaseID: plan.TargetReleaseID, SourceID: plan.SourceID, DistributionID: "foreign",
			Purpose: download.PurposeUpdate, UpdatePlanID: plan.ID, LibraryID: plan.GameID,
		},
	}
	target, _ := h.releases.FindRelease(plan.TargetReleaseID)
	task, err := h.service.downloadRelease(context.Background(), plan, target.ID, h.service.config().DownloadsPath, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if task.ID == "foreign-prefetch" || len(h.downloads.requests) != 1 {
		t.Fatalf("foreign prefetch was reused: task=%+v requests=%d", task, len(h.downloads.requests))
	}
}

func TestPrefetchedRevisionIsReusedAfterPlanIsRecreated(t *testing.T) {
	h := newHarness(t)
	installedStamp := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	targetStamp := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	h.library.games[0].ReleaseUploadedAt = &installedStamp
	h.releases.list[1].UploadedAt = &targetStamp
	plan := h.plan(t)
	target, _ := h.releases.FindRelease(plan.TargetReleaseID)
	destination := h.service.config().DownloadsPath
	h.downloads.tasks["old-plan-prefetch"] = &download.Download{
		ID:          "old-plan-prefetch",
		Destination: destination,
		Status:      download.StatusCompleted,
		Origin: download.Origin{
			ReleaseID:         target.ID,
			SourceID:          target.SourceID,
			DistributionID:    target.DistributionID,
			ReleaseUploadedAt: target.UploadedAt,
			GameID:            canonical,
			Version:           releaseVersion(target),
			Purpose:           download.PurposeUpdate,
			UpdatePlanID:      "discarded-plan",
			LibraryID:         plan.GameID,
		},
	}

	plan.ID = "recreated-plan"
	task, err := h.service.downloadRelease(context.Background(), plan, target.ID, destination, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if task.ID != "old-plan-prefetch" || len(h.downloads.requests) != 0 {
		t.Fatalf("same downloaded revision was not reused: task=%+v requests=%d", task, len(h.downloads.requests))
	}
	state, _ := h.service.snapshot(plan.GameID)
	if state.DownloadID != task.ID {
		t.Fatalf("reused task was not attached to recreated plan: download=%q", state.DownloadID)
	}
}

func TestTargetRevisionDateChangeInvalidatesReadyPlan(t *testing.T) {
	h := newHarness(t)
	installedStamp := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	firstTargetStamp := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	secondTargetStamp := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	h.library.games[0].ReleaseUploadedAt = &installedStamp
	target := release("r2", "2.0", 2)
	target.UploadedAt = &firstTargetStamp
	h.releases.list = []sources.Release{target}

	plan := h.plan(t)
	h.service.mutate("local-1", func(u *Update) {
		u.Plan = &plan
		u.State = StateReady
		u.DownloadID = "old-download"
	})
	h.releases.list[0].UploadedAt = &secondTargetStamp
	if err := h.service.check(h.library.games[0]); err != nil {
		t.Fatal(err)
	}

	got, _ := h.service.snapshot("local-1")
	if got.Plan != nil || got.State != StateAvailable || got.DownloadID != "" {
		t.Fatalf("plan for the old target revision remained actionable: %+v", got)
	}
	if got.Availability.TargetReleaseUploadedAt == nil || !got.Availability.TargetReleaseUploadedAt.Equal(secondTargetStamp) {
		t.Fatalf("availability did not move to the new target revision: %+v", got.Availability)
	}
}

func TestSameReleaseIDVersionChangeInvalidatesReadyPlan(t *testing.T) {
	h := newHarness(t)
	h.releases.list = []sources.Release{release("r1", "2.0", 2)}
	if err := h.service.check(h.library.games[0]); err != nil {
		t.Fatal(err)
	}
	plan, err := h.service.buildPlan(context.Background(), "local-1")
	if err != nil {
		t.Fatal(err)
	}
	h.service.mutate("local-1", func(u *Update) {
		u.Plan = plan
		u.State = StateReady
		u.DownloadID = "old-download"
	})
	h.releases.list[0].Version = "3.0"
	if err := h.service.check(h.library.games[0]); err != nil {
		t.Fatal(err)
	}
	got, _ := h.service.snapshot("local-1")
	if got.Plan != nil || got.State != StateAvailable || got.DownloadID != "" || got.Availability.TargetVersion != "3.0" {
		t.Fatalf("stable-id plan was not invalidated: %+v", got)
	}
}

func TestServiceStartupDoesNotExposePersistedOfferOrPlan(t *testing.T) {
	dir := t.TempDir()
	svc, err := newServiceAt(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	stored := Update{
		GameID: "local-1", Title: "Game", State: StateReady,
		Availability: UpdateAvailability{Available: true, Kind: KindUpdate, GameID: "local-1", TargetReleaseID: "foreign"},
		Plan:         &UpdatePlan{ID: "stale", GameID: "local-1", TargetReleaseID: "foreign"},
	}
	if err := svc.store.saveUpdates([]Update{stored}); err != nil {
		t.Fatal(err)
	}
	if err := svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := svc.ServiceShutdown(); err != nil {
			t.Error(err)
		}
	})
	got, err := svc.GetUpdate("local-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != StateIdle || got.Availability.Available || got.Plan != nil {
		t.Fatalf("persisted offer survived restart: %+v", got)
	}
}
