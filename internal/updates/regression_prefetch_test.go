package updates

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"typhon/internal/download"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestPrefetchUpdate(t *testing.T) {
	t.Run("untracked game", func(t *testing.T) {
		h := newHarness(t)
		if err := h.service.PrefetchUpdate("nobody"); !errors.Is(err, errNotTracked) {
			t.Fatalf("err = %v, want %v", err, errNotTracked)
		}
	})
	t.Run("no prepared plan", func(t *testing.T) {
		h := newHarness(t)
		if err := h.service.check(h.library.games[0]); err != nil {
			t.Fatal(err)
		}
		if err := h.service.PrefetchUpdate("local-1"); !errors.Is(err, errNoPlan) {
			t.Fatalf("err = %v, want %v", err, errNoPlan)
		}
	})
	t.Run("torrent reuse writes into the installation and cannot be prefetched", func(t *testing.T) {
		h := newHarness(t)
		h.plan(t)
		h.service.mutate("local-1", func(u *Update) {
			reuse := *u.Plan
			reuse.Strategy = StrategyTorrentReuse
			u.Plan = &reuse
		})
		if err := h.service.PrefetchUpdate("local-1"); !errors.Is(err, errUnavailablePrefetch) {
			t.Fatalf("err = %v, want %v", err, errUnavailablePrefetch)
		}
		if len(h.downloads.requests) != 0 {
			t.Fatalf("a refused prefetch started downloads: %+v", h.downloads.requests)
		}
	})
	t.Run("stale plan releases the job slot", func(t *testing.T) {
		h := newHarness(t)
		h.plan(t)
		h.service.mutate("local-1", func(u *Update) {
			stale := *u.Plan
			stale.TargetReleaseID = "other-release"
			u.Plan = &stale
		})
		if err := h.service.PrefetchUpdate("local-1"); !errors.Is(err, errNoTarget) {
			t.Fatalf("err = %v, want %v", err, errNoTarget)
		}
		if h.service.Busy("local-1") {
			t.Fatal("a refused prefetch left the job slot taken")
		}
	})
	t.Run("data is downloaded and the update becomes ready", func(t *testing.T) {
		h := newHarness(t)
		h.plan(t)
		if err := h.service.PrefetchUpdate("local-1"); err != nil {
			t.Fatal(err)
		}
		got := h.waitState(t, StateReady)
		if got.Progress != 1 || got.Step != "" || got.Error != "" {
			t.Fatalf("ready update = %+v", got)
		}
		if len(h.downloads.requests) != 1 || h.downloads.requests[0].Origin.ReleaseID != "r2" || h.downloads.requests[0].Origin.Purpose != download.PurposeUpdate {
			t.Fatalf("download requests = %+v, want one update download of r2", h.downloads.requests)
		}
		if data, err := os.ReadFile(filepath.Join(h.installDir, "game.exe")); err != nil || string(data) != "old executable" {
			t.Fatalf("prefetch touched the installation: %q %v", data, err)
		}
		if _, err := os.Stat(h.installDir + previousSuffix); err == nil {
			t.Fatal("prefetch created a previous-version copy")
		}
	})
	t.Run("a failed download returns to available with the reason", func(t *testing.T) {
		h := newHarness(t)
		h.plan(t)
		h.downloads.failTask = true
		if err := h.service.PrefetchUpdate("local-1"); err != nil {
			t.Fatal(err)
		}
		got := waitUpdate(t, h.service, "local-1", func(u Update) bool {
			return u.State == StateAvailable && u.Error != "" && !h.service.Busy("local-1")
		})
		if got.Error != "сеть недоступна" || got.Step != "" || got.Progress != 0 {
			t.Fatalf("failed prefetch = %+v", got)
		}
	})
	t.Run("cancelling returns to available without an error", func(t *testing.T) {
		h := newHarness(t)
		hanging := &hangingDownloads{fakeDownloads: h.downloads, hang: "r2"}
		h.service.downloads = hanging
		h.plan(t)
		if err := h.service.PrefetchUpdate("local-1"); err != nil {
			t.Fatal(err)
		}
		waitUpdate(t, h.service, "local-1", func(u Update) bool { return u.State == StateDownloading && u.DownloadID != "" })

		if err := h.service.CancelUpdate("local-1"); err != nil {
			t.Fatal(err)
		}
		got := waitUpdate(t, h.service, "local-1", func(u Update) bool {
			return u.State == StateAvailable && !h.service.Busy("local-1")
		})
		if got.Error != "" || got.Progress != 0 || got.Step != "" {
			t.Fatalf("cancelled prefetch = %+v, want a clean available state", got)
		}
		if len(hanging.cancelledIDs()) != 1 {
			t.Fatalf("download cancellations = %v, want the running download stopped", hanging.cancelledIDs())
		}
	})
}

func TestPatchChainPrefetchDownloadsEveryPatchAndNotTheFullRelease(t *testing.T) {
	svc, _, _ := newPatchScenario(t, "")
	svc.releases = &fakeReleases{list: chainReleases()}
	prepareChain(t, svc)

	if err := svc.PrefetchUpdate("g1"); err != nil {
		t.Fatal(err)
	}
	waitUpdate(t, svc, "g1", func(u Update) bool { return u.State == StateReady && !svc.Busy("g1") })

	dl, ok := svc.downloads.(*patchDownloads)
	if !ok {
		t.Fatal("setup: unexpected downloads type")
	}
	dl.mu.Lock()
	defer dl.mu.Unlock()
	var got []string
	for _, req := range dl.requests {
		got = append(got, req.Origin.ReleaseID)
	}
	if !slices.Equal(got, []string{"p1", "p2"}) {
		t.Fatalf("downloaded %v, want the two patches in order and not the 100 MB release", got)
	}
}

func TestServiceShutdownDuringAnUpdateDownloadDoesNotHangOrTouchTheInstallation(t *testing.T) {
	h := newHarness(t)
	h.service.downloads = &hangingDownloads{fakeDownloads: h.downloads, hang: "r2"}
	h.plan(t)
	if err := h.service.StartUpdate("local-1"); err != nil {
		t.Fatal(err)
	}
	waitUpdate(t, h.service, "local-1", func(u Update) bool { return u.DownloadID != "" })

	done := make(chan error, 1)
	go func() { done <- h.service.ServiceShutdown() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ServiceShutdown: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("ServiceShutdown hung on a running update")
	}

	if data, err := os.ReadFile(filepath.Join(h.installDir, "game.exe")); err != nil || string(data) != "old executable" {
		t.Fatalf("installation touched: %q %v", data, err)
	}
	stored, err := h.service.store.loadUpdates()
	if err != nil || len(stored) != 1 {
		t.Fatalf("stored = %+v (%v)", stored, err)
	}
	if stored[0].State == StateUpdating || stored[0].State == StateDownloading {
		t.Fatalf("shutdown persisted %q: the next start would read a download that is not running", stored[0].State)
	}
	if h.service.Busy("local-1") {
		t.Fatal("job slot still taken after shutdown")
	}
}

func TestPersistedStateKeepsItsOnDiskContract(t *testing.T) {
	const journal = `{
  "previousRollback": {"gameId": "g1", "path": "/p/old", "installDir": "/g/Game", "executable": "/g/Game/game.exe", "version": "0.9", "releaseId": "r0", "sourceId": "src", "distributionId": "main", "releaseUploadedAt": "2026-07-01T10:00:00Z", "awaitLaunch": true, "keepUntil": "2026-09-02T10:00:00Z", "createdAt": "2026-09-01T10:00:00Z"},
  "rollback": {"gameId": "g1", "path": "/p/new", "installDir": "/g/Game", "executable": "/g/Game/game.exe", "version": "1.0", "releaseId": "r1", "sourceId": "src", "distributionId": "main", "releaseUploadedAt": "2026-08-01T10:00:00Z", "awaitLaunch": false, "keepUntil": "2026-09-05T10:00:00Z", "createdAt": "2026-09-04T10:00:00Z"},
  "retainedPrevious": "/g/Game.previous.retained",
  "original": {"id": "g1", "executable": "/g/Game/game.exe", "installDir": "/g/Game", "version": "1.0", "versionSource": "release_metadata", "releaseId": "r1", "sourceId": "src", "distributionId": "main", "releaseUploadedAt": "2026-08-01T10:00:00Z"},
  "gameId": "g1",
  "kind": "swap",
  "installDir": "/g/Game",
  "staging": "/g/.staging/g1",
  "previous": "/g/Game.previous",
  "version": "1.1",
  "patch": "p1",
  "startedAt": "2026-09-06T10:00:00Z"
}`
	const rollback = `{"gameId": "g1", "path": "/p/new", "installDir": "/g/Game", "executable": "/g/Game/game.exe", "version": "1.0", "releaseId": "r1", "sourceId": "src", "distributionId": "main", "releaseUploadedAt": "2026-08-01T10:00:00Z", "awaitLaunch": true, "keepUntil": "2026-09-05T10:00:00Z", "createdAt": "2026-09-04T10:00:00Z"}`
	const history = `{"id": "h1", "gameId": "g1", "fromVersion": "1.0", "toVersion": "1.1", "strategy": "patch_chain", "downloadBytes": 2048, "startedAt": "2026-09-06T10:00:00Z", "completedAt": "2026-09-06T10:05:00Z", "status": "failed", "error": "диск заполнен"}`
	const update = `{
  "gameId": "g1",
  "title": "Game",
  "state": "update_failed",
  "availability": {
    "available": true, "kind": "update", "gameId": "g1", "installedReleaseId": "r1", "targetReleaseId": "r2",
    "sourceId": "src", "distributionId": "main",
    "installedReleaseUploadedAt": "2026-08-01T10:00:00Z", "targetReleaseUploadedAt": "2026-09-01T10:00:00Z",
    "installedVersion": "1.0", "targetVersion": "1.1", "confidence": 0.9, "strategy": "patch_chain",
    "estimatedDownloadBytes": 2048, "requiresFullInstall": true, "patchCount": 1, "reason": "versions_not_comparable", "targetSize": 4096
  },
  "plan": {
    "id": "plan-1", "gameId": "g1", "installedReleaseId": "r1", "targetReleaseId": "r2", "sourceId": "src", "distributionId": "main",
    "installedReleaseUploadedAt": "2026-08-01T10:00:00Z", "targetReleaseUploadedAt": "2026-09-01T10:00:00Z",
    "installedVersion": "1.0", "targetVersion": "1.1", "strategy": "patch_chain",
    "steps": [{"kind": "apply_patch", "label": "Применение патча", "releaseId": "p1", "patchId": "p1", "bytes": 2048, "fromVersion": "1.0", "toVersion": "1.1"}],
    "downloadBytes": 2048, "reusedBytes": 1024, "requiredDiskBytes": 8192, "backupAvailable": true, "rollbackAvailable": true, "reuseFlat": true,
    "savesPath": "/saves", "patches": [{"id": "p1", "gameId": "canon", "fromVersion": "1.0", "toVersion": "1.1", "fromReleaseId": "r1", "toReleaseId": "r2", "releaseId": "p1", "sourceId": "src", "distributionId": "main", "uploadedAt": "2026-09-01T10:00:00Z", "title": "Patch", "size": 2048, "priority": 3, "createdAt": "2026-09-01T09:00:00Z"}],
    "confidence": 0.8, "createdAt": "2026-09-02T10:00:00Z"
  },
  "planning": true,
  "downloadId": "d1",
  "installId": "i1",
  "step": "apply_patch",
  "progress": 0.5,
  "message": "Применение патча",
  "error": "boom",
  "canRollback": true,
  "savesBackup": "/backup/1",
  "checkedAt": "2026-09-06T10:00:00Z"
}`
	tests := []struct {
		name string
		raw  string
		into func() any
	}{
		{"swap journal", journal, func() any { return new(SwapJournal) }},
		{"rollback", rollback, func() any { return new(Rollback) }},
		{"update history", history, func() any { return new(UpdateHistory) }},
		{"update with its offer and plan", update, func() any { return new(Update) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			value := tc.into()
			if err := json.Unmarshal([]byte(tc.raw), value); err != nil {
				t.Fatalf("a record written by an earlier release no longer parses: %v", err)
			}
			again, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			var want, got any
			if err := json.Unmarshal([]byte(tc.raw), &want); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(again, &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(want, got) {
				t.Fatalf("on-disk schema changed: recovery data from an earlier release no longer round-trips\nwant %s\ngot  %s", tc.raw, again)
			}
		})
	}
}

func TestSwapJournalFromAnEarlierReleaseIsStillRecovered(t *testing.T) {
	root := t.TempDir()
	installDir := filepath.Join(root, "Games", "Game")
	previous := installDir + previousSuffix
	writeFile(t, previous, "game.exe", "old executable")
	writeFile(t, installDir, "game.exe", "half-swapped new executable")

	dir := filepath.Join(root, "config")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := `{"version":1,"data":[{"gameId":"g1","kind":"swap","installDir":` + quoteJSON(t, installDir) +
		`,"previous":` + quoteJSON(t, previous) + `,"version":"1.1","startedAt":"2026-09-06T10:00:00Z"}]}`
	if err := os.WriteFile(filepath.Join(dir, "journal.json"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	lib := &fakeLibrary{}
	svc, err := newServiceAt(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.library = lib
	if err := svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := svc.ServiceShutdown(); err != nil {
			t.Error(err)
		}
	})

	if data, err := os.ReadFile(filepath.Join(installDir, "game.exe")); err != nil || string(data) != "old executable" {
		t.Fatalf("installation = %q (%v), want the previous version restored from the journal", data, err)
	}
	if svc.HasRollback("g1") {
		t.Fatal("a recovered journal is still pending")
	}
	journals, err := svc.store.loadJournals()
	if err != nil || len(journals) != 0 {
		t.Fatalf("journals after recovery = %+v (%v), want none", journals, err)
	}
}

func quoteJSON(t *testing.T, s string) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
