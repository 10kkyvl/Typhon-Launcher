package updates

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"typhon/internal/sources"
)

func waitUpdate(t *testing.T, svc *Service, gameID string, cond func(Update) bool) Update {
	t.Helper()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.After(10 * time.Second)
	for {
		if u, ok := svc.snapshot(gameID); ok && cond(u) {
			return u
		}
		select {
		case <-ticker.C:
		case <-timeout:
			u, _ := svc.snapshot(gameID)
			t.Fatalf("condition not reached for %s: %+v", gameID, u)
			return Update{}
		}
	}
}

func TestRollbackOnlyStateOffersNoNewRelease(t *testing.T) {
	h := newHarness(t)
	h.plan(t)
	if err := h.service.StartUpdate("local-1"); err != nil {
		t.Fatal(err)
	}
	h.waitState(t, StateIdle)

	got, err := h.service.CheckGame("local-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Availability.Available || got.Availability.Kind != KindNone {
		t.Fatalf("availability = %+v, want no offer: the newest release is the one installed", got.Availability)
	}
	if got.State != StateIdle || got.Plan != nil {
		t.Fatalf("state = %q plan = %v, want an idle card without a plan", got.State, got.Plan)
	}
	if !got.CanRollback || !h.service.HasRollback("local-1") {
		t.Fatal("the way back to the previous version disappeared")
	}
	if got.Availability.InstalledVersion != "1.1" {
		t.Fatalf("InstalledVersion = %q, want the version the update installed", got.Availability.InstalledVersion)
	}

	h.releases.list = append(h.releases.list, release("r3", "1.2", 13<<20))
	got, err = h.service.CheckGame("local-1")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Availability.Available || got.Availability.Kind != KindUpdate || got.Availability.TargetVersion != "1.2" || got.State != StateAvailable {
		t.Fatalf("a genuinely newer release was not offered: %+v", got)
	}
	if !got.CanRollback {
		t.Fatal("offering a new release dropped the rollback of the previous update")
	}
}

func TestCheckDoesNotDisturbARunningUpdate(t *testing.T) {
	h := newHarness(t)
	h.plan(t)
	h.service.mutate("local-1", func(u *Update) {
		u.State = StateUpdating
		u.Step = StepDownload
		u.Progress = 0.4
	})
	before, _ := h.service.snapshot("local-1")

	h.releases.list = append(h.releases.list, release("r3", "1.2", 13<<20))
	if err := h.service.check(h.library.games[0]); err != nil {
		t.Fatal(err)
	}

	got, _ := h.service.snapshot("local-1")
	if got.State != StateUpdating || got.Step != StepDownload || got.Progress != 0.4 {
		t.Fatalf("a check changed the state of a running update: %+v", got)
	}
	if got.Availability.TargetReleaseID != before.Availability.TargetReleaseID || got.Plan == nil {
		t.Fatalf("a check replaced the offer or plan of a running update: %+v", got)
	}
}

func TestCheckKeepsAPreparedPlanUntilTheOfferChanges(t *testing.T) {
	h := newHarness(t)
	h.plan(t)
	h.service.mutate("local-1", func(u *Update) {
		u.State = StateReady
		u.DownloadID = "task-r2"
	})

	if err := h.service.check(h.library.games[0]); err != nil {
		t.Fatal(err)
	}
	got, _ := h.service.snapshot("local-1")
	if got.State != StateReady || got.Plan == nil || got.DownloadID != "task-r2" {
		t.Fatalf("an unchanged offer lost its prepared plan or download: %+v", got)
	}

	h.releases.list = append(h.releases.list, release("r3", "1.2", 13<<20))
	if err := h.service.check(h.library.games[0]); err != nil {
		t.Fatal(err)
	}
	got, _ = h.service.snapshot("local-1")
	if got.Plan != nil || got.DownloadID != "" || got.Error != "" || got.State != StateAvailable {
		t.Fatalf("a changed offer kept the plan for the old target: %+v", got)
	}
	if got.Availability.TargetVersion != "1.2" {
		t.Fatalf("TargetVersion = %q, want 1.2", got.Availability.TargetVersion)
	}
}

func TestCheckClearsTheOfferWhenTheReleaseDisappears(t *testing.T) {
	h := newHarness(t)
	h.plan(t)

	h.releases.list = []sources.Release{release("r1", "1.0", 10<<20)}
	if err := h.service.check(h.library.games[0]); err != nil {
		t.Fatal(err)
	}
	got, _ := h.service.snapshot("local-1")
	if got.Availability.Available || got.State != StateIdle || got.Plan != nil {
		t.Fatalf("a withdrawn release is still offered: %+v", got)
	}
}

func TestPreparePlanFlow(t *testing.T) {
	t.Run("untracked game", func(t *testing.T) {
		h := newHarness(t)
		if err := h.service.PreparePlan("nobody"); !errors.Is(err, errNotTracked) {
			t.Fatalf("err = %v, want %v", err, errNotTracked)
		}
	})
	t.Run("nothing to update to", func(t *testing.T) {
		h := newHarness(t)
		h.releases.list = []sources.Release{release("r1", "1.0", 10<<20)}
		if err := h.service.check(h.library.games[0]); err != nil {
			t.Fatal(err)
		}
		if err := h.service.PreparePlan("local-1"); !errors.Is(err, errNoTarget) {
			t.Fatalf("err = %v, want %v", err, errNoTarget)
		}
		if h.service.Busy("local-1") {
			t.Fatal("a refused plan left the job slot taken")
		}
	})
	t.Run("another operation holds the game", func(t *testing.T) {
		h := newHarness(t)
		if err := h.service.check(h.library.games[0]); err != nil {
			t.Fatal(err)
		}
		if _, ok := h.service.beginJob("local-1"); !ok {
			t.Fatal("setup: job slot unavailable")
		}
		if err := h.service.PreparePlan("local-1"); !errors.Is(err, errBusy) {
			t.Fatalf("err = %v, want %v", err, errBusy)
		}
		h.service.endJob("local-1")
	})
	t.Run("plan is built and stored", func(t *testing.T) {
		h := newHarness(t)
		if err := h.service.check(h.library.games[0]); err != nil {
			t.Fatal(err)
		}
		if err := h.service.PreparePlan("local-1"); err != nil {
			t.Fatal(err)
		}
		got := waitUpdate(t, h.service, "local-1", func(u Update) bool {
			return u.Plan != nil && !u.Planning && !h.service.Busy("local-1")
		})
		if got.Plan.Strategy != StrategyFullRelease || got.Plan.TargetVersion != "1.1" || got.Plan.InstalledVersion != "1.0" {
			t.Fatalf("plan = %+v", got.Plan)
		}
		if got.Availability.Strategy != StrategyFullRelease || got.Availability.EstimatedDownloadBytes != got.Plan.DownloadBytes {
			t.Fatalf("availability not refreshed from the plan: %+v", got.Availability)
		}
		if got.Error != "" || got.State != StateAvailable {
			t.Fatalf("state = %q error = %q after planning", got.State, got.Error)
		}
		stored, err := h.service.store.loadUpdates()
		if err != nil || len(stored) != 1 || stored[0].Plan == nil || stored[0].Planning {
			t.Fatalf("stored = %+v (%v), want the finished plan persisted and planning cleared", stored, err)
		}
	})
	t.Run("offer vanishes while planning", func(t *testing.T) {
		h := newHarness(t)
		if err := h.service.check(h.library.games[0]); err != nil {
			t.Fatal(err)
		}
		h.releases.list = []sources.Release{release("r1", "1.0", 10<<20)}
		if err := h.service.PreparePlan("local-1"); err != nil {
			t.Fatal(err)
		}
		got := waitUpdate(t, h.service, "local-1", func(u Update) bool {
			return !u.Planning && u.Error != "" && !h.service.Busy("local-1")
		})
		if got.Plan != nil {
			t.Fatalf("a failed planning stored a plan: %+v", got.Plan)
		}
		if !strings.Contains(got.Error, "updates.no_target") {
			t.Fatalf("error = %q, want the coded updates.no_target reason", got.Error)
		}
	})
}

func TestStartUpdateRefusalsLeaveEverythingAlone(t *testing.T) {
	tests := []struct {
		name    string
		arrange func(t *testing.T, h *harness)
		id      string
		want    error
	}{
		{"untracked game", func(*testing.T, *harness) {}, "nobody", errNotTracked},
		{"no prepared plan", func(_ *testing.T, h *harness) {
			h.service.mutate("local-1", func(u *Update) { u.Plan = nil })
		}, "local-1", errNoPlan},
		{"downloads unavailable", func(_ *testing.T, h *harness) { h.service.downloads = nil }, "local-1", errNoDownloads},
		{"library unavailable", func(_ *testing.T, h *harness) { h.service.library = nil }, "local-1", errNoLibrary},
		{"another operation holds the game", func(t *testing.T, h *harness) {
			if _, ok := h.service.beginJob("local-1"); !ok {
				t.Fatal("setup: job slot unavailable")
			}
		}, "local-1", errBusy},
		{"plan no longer matches the offer", func(_ *testing.T, h *harness) {
			h.service.mutate("local-1", func(u *Update) {
				stale := *u.Plan
				stale.TargetReleaseID = "other-release"
				u.Plan = &stale
			})
		}, "local-1", errNoTarget},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			h.plan(t)
			tc.arrange(t, h)

			err := h.service.StartUpdate(tc.id)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if !errors.Is(tc.want, errBusy) && h.service.Busy("local-1") {
				t.Fatal("a refused update left the job slot taken, blocking every later operation")
			}
			if history := h.service.GetHistory(""); len(history) != 0 {
				t.Fatalf("a refused update wrote history: %+v", history)
			}
			if len(h.downloads.requests) != 0 {
				t.Fatalf("a refused update started a download: %+v", h.downloads.requests)
			}
			if data, err := os.ReadFile(filepath.Join(h.installDir, "game.exe")); err != nil || string(data) != "old executable" {
				t.Fatalf("installation touched: %q %v", data, err)
			}
		})
	}
}

func TestVersionSourceOfMapsStoredNames(t *testing.T) {
	tests := map[string]VersionSource{
		"release_metadata":    VersionSourceRelease,
		"pe_metadata":         VersionSourceExecutable,
		"executable_metadata": VersionSourceExecutable,
		"version_file":        VersionSourceExecutable,
		"manifest":            VersionSourceManifest,
		"manual":              VersionSourceManual,
		"":                    VersionSourceUnknown,
		"something new":       VersionSourceUnknown,
	}
	for raw, want := range tests {
		if got := versionSourceOf(raw); got != want {
			t.Errorf("versionSourceOf(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestPatchChainIsFoundWhateverTheSpellingOfTheVersions(t *testing.T) {
	spellings := []struct {
		name                         string
		installed, fromTag, toTag    string
		releaseVersion, patchVersion string
	}{
		{"identical", "1.0", "1.0", "1.1", "1.1", "1.1"},
		{"v prefix on the patch", "1.0", "v1.0", "v1.1", "1.1", "v1.1"},
		{"trailing zero on the installed version", "1.0.0", "1.0", "1.1", "1.1", "1.1"},
		{"upper case prefix and extra zero", "v1.0", "V1.0.0", "V1.1.0", "1.1", "V1.1.0"},
		{"padded by the feed", " 1.0 ", "1.0", " 1.1 ", "1.1", "1.1"},
	}
	for _, tc := range spellings {
		t.Run(tc.name, func(t *testing.T) {
			p := patchRelease("p1", tc.fromTag, tc.toTag, 1<<20)
			p.ToVersion = tc.patchVersion
			got := ResolveUpdate(installedAt("r1", tc.installed), []sources.Release{
				release("r1", "1.0", 50<<20),
				release("r2", tc.releaseVersion, 100<<20),
				p,
			}, PatchesFrom([]sources.Release{p}))
			if !got.Available || got.Kind != KindUpdate {
				t.Fatalf("availability = %+v, want an update", got)
			}
			if got.Strategy != StrategyPatchChain || got.PatchCount != 1 || got.RequiresFullInstall {
				t.Fatalf("strategy = %q patches = %d full = %v: the patch was not connected to the installed version", got.Strategy, got.PatchCount, got.RequiresFullInstall)
			}
		})
	}
}

func TestVersionKeyIsOneNormalisationForEveryComparison(t *testing.T) {
	same := [][]string{
		{"1.0", "1.0.0", "v1.0", "V1.0.0", " 1.0 "},
		{"1.10", "v1.10.0"},
	}
	for _, group := range same {
		want := VersionKey(group[0])
		if want == "" {
			t.Fatalf("VersionKey(%q) is empty", group[0])
		}
		for _, raw := range group[1:] {
			if got := VersionKey(raw); got != want {
				t.Errorf("VersionKey(%q) = %q, want it to equal VersionKey(%q) = %q", raw, got, group[0], want)
			}
		}
	}
	if VersionKey("1.1") == VersionKey("1.10") {
		t.Error("1.1 and 1.10 are different versions")
	}
	if VersionKey("") != "" || VersionKey("   ") != "" {
		t.Error("an empty version must have an empty key, never a key that matches other empties")
	}
	if a, b := VersionKey("Build ALPHA"), VersionKey("  build alpha "); a == "" || a != b {
		t.Errorf("a version that is not a number must still compare case- and space-insensitively: %q vs %q", a, b)
	}
}
