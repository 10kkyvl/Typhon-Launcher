package download

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"typhon/internal/settings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestReloadRestoresEveryPersistedField(t *testing.T) {
	flag := true
	uploaded := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	added := time.Date(2026, 9, 1, 8, 30, 0, 0, time.UTC)
	completedAt := time.Date(2026, 9, 1, 9, 45, 0, 0, time.UTC)

	first := newTestManager(t, 1)
	dest := t.TempDir()
	first.mu.Lock()
	for i, status := range allStatuses {
		d := &Download{
			ID:          string(status),
			Name:        "Game " + string(status),
			Type:        TypeTorrent,
			Source:      "magnet:?xt=urn:btih:" + string(status),
			InfoHash:    "hash-" + string(status),
			Destination: filepath.Join(dest, string(status)),
			Status:      status,
			Downloaded:  40,
			Total:       100,
			Seeding:     status == StatusCompleted,
			Flat:        status == StatusPaused,
			InPlace:     status == StatusFailed,
			Origin: Origin{
				ReleaseID:         "rel-" + string(status),
				SourceID:          "src",
				DistributionID:    "dist",
				ReleaseUploadedAt: &uploaded,
				GameID:            "game",
				Version:           "1.2",
				Purpose:           PurposeUpdate,
				UpdatePlanID:      "plan",
				LibraryID:         "lib",
				AutoInstall:       &flag,
				ElevateAhead:      true,
			},
			AddedAt:    added.Add(time.Duration(i) * time.Minute),
			ETASeconds: 55,
			Seeders:    4,
			Peers:      9,
		}
		d.DownloadSpeed, d.UploadSpeed, d.Stalled = 1000, 500, true
		if status == StatusCompleted {
			d.CompletedAt = &completedAt
		}
		if status == StatusFailed {
			d.Error = "disk full"
		}
		first.items = append(first.items, d)
	}
	if err := first.persistLocked(); err != nil {
		first.mu.Unlock()
		t.Fatal(err)
	}
	first.mu.Unlock()

	second := reopen(t, first)

	got := second.List()
	if len(got) != len(allStatuses) {
		t.Fatalf("reloaded %d downloads, want %d", len(got), len(allStatuses))
	}
	for i, status := range allStatuses {
		d := got[i]
		if d.ID != string(status) {
			t.Fatalf("position %d holds %s, want %s: the queue order must survive a restart", i, d.ID, status)
		}
		if want := reloadedStatus(status); d.Status != want {
			t.Errorf("%s: status = %s, want %s", status, d.Status, want)
		}
		if d.Name != "Game "+string(status) || d.Type != TypeTorrent || d.Source != "magnet:?xt=urn:btih:"+string(status) ||
			d.InfoHash != "hash-"+string(status) || d.Destination != filepath.Join(dest, string(status)) {
			t.Errorf("%s: identity fields = %+v", status, d)
		}
		if d.Downloaded != 40 || d.Total != 100 || d.Progress != 0.4 {
			t.Errorf("%s: progress = %d/%d (%v)", status, d.Downloaded, d.Total, d.Progress)
		}
		if d.Seeding != (status == StatusCompleted) || d.Flat != (status == StatusPaused) || d.InPlace != (status == StatusFailed) {
			t.Errorf("%s: seeding=%v flat=%v inPlace=%v", status, d.Seeding, d.Flat, d.InPlace)
		}
		if wantErr := map[bool]string{true: "disk full", false: ""}[status == StatusFailed]; d.Error != wantErr {
			t.Errorf("%s: error = %q, want %q", status, d.Error, wantErr)
		}
		if !d.AddedAt.Equal(added.Add(time.Duration(i) * time.Minute)) {
			t.Errorf("%s: addedAt = %v", status, d.AddedAt)
		}
		if (status == StatusCompleted) != (d.CompletedAt != nil) || (d.CompletedAt != nil && !d.CompletedAt.Equal(completedAt)) {
			t.Errorf("%s: completedAt = %v", status, d.CompletedAt)
		}
		o := d.Origin
		if o.ReleaseID != "rel-"+string(status) || o.SourceID != "src" || o.DistributionID != "dist" || o.GameID != "game" ||
			o.Version != "1.2" || o.Purpose != PurposeUpdate || o.UpdatePlanID != "plan" || o.LibraryID != "lib" || !o.ElevateAhead {
			t.Errorf("%s: origin = %+v", status, o)
		}
		if o.AutoInstall == nil || !*o.AutoInstall {
			t.Errorf("%s: the auto-install choice made in the dialog was lost: %v", status, o.AutoInstall)
		}
		if o.ReleaseUploadedAt == nil || !o.ReleaseUploadedAt.Equal(uploaded) {
			t.Errorf("%s: release upload time = %v", status, o.ReleaseUploadedAt)
		}
		if d.ETASeconds != -1 || d.DownloadSpeed != 0 || d.UploadSpeed != 0 || d.Seeders != 0 || d.Peers != 0 || d.Stalled {
			t.Errorf("%s: live numbers survived a restart: %+v", status, d)
		}
	}
}

func TestReloadLeavesAnUnsetAutoInstallUnset(t *testing.T) {
	first := newTestManager(t, 1)
	first.addTestItem("a", StatusQueued)

	second := reopen(t, first)

	if got := mustGet(t, second, "a").Origin.AutoInstall; got != nil {
		t.Fatalf("auto-install = %v, want nil so that the global setting decides", *got)
	}
}

func TestReloadRebuildsTheSelectionFromTheCachedTorrent(t *testing.T) {
	mi, _ := buildTorrent(t, "Game", []tFile{{"a.bin", 20000}, {"b.bin", 30000}, {"c.bin", 10000}})
	hash := mi.HashInfoBytes().HexString()

	first := newTestManager(t, 1)
	if err := first.store.saveMetainfo(hash, mi); err != nil {
		t.Fatal(err)
	}
	first.addTestItem("a", StatusPaused)
	setItem(first, "a", func(d *Download) {
		d.InfoHash = hash
		d.Downloaded = 12345
		d.Files = []FileState{
			{Path: "Game/a.bin", Size: 20000},
			{Path: "Game/b.bin", Size: 30000, Selected: true},
			{Path: "Game/c.bin", Size: 10000},
		}
		d.Total = 30000
	})
	first.mu.Lock()
	if err := first.persistLocked(); err != nil {
		first.mu.Unlock()
		t.Fatal(err)
	}
	first.mu.Unlock()
	if got := persistedRecords(t, first)[0].Selected; len(got) != 1 || got[0] != 1 {
		t.Fatalf("persisted selection = %v, want [1]", got)
	}

	second := reopen(t, first)

	d := mustGet(t, second, "a")
	if len(d.Files) != 3 {
		t.Fatalf("files = %+v", d.Files)
	}
	for i, wantSelected := range []bool{false, true, false} {
		if d.Files[i].Selected != wantSelected {
			t.Errorf("file %d (%s) selected = %v, want %v", i, d.Files[i].Path, d.Files[i].Selected, wantSelected)
		}
	}
	if d.Total != 30000 {
		t.Errorf("total = %d, want the size of the selected file only", d.Total)
	}
	if d.Downloaded != 12345 || d.Progress != ratio(12345, 30000) {
		t.Errorf("progress = %d (%v)", d.Downloaded, d.Progress)
	}
	if d.Status != StatusPaused {
		t.Errorf("status = %s", d.Status)
	}
}

func TestReloadOfAnEmptyDirectoryStartsEmpty(t *testing.T) {
	m := newTestManager(t, 1)
	second := reopen(t, m)
	if got := second.List(); len(got) != 0 {
		t.Fatalf("downloads = %+v", got)
	}
}

type restartRow struct {
	id        string
	status    Status
	seeding   bool
	wantState Status
}

// A start retries a failed download; only a flap of the network leaves failed
// ones alone (see TestRouteBackBringsParkedDownloadsAndSkipsFailedOnes).
func TestStartupRestoresEachStatusOnARealClient(t *testing.T) {
	rows := []restartRow{
		{id: "done", status: StatusCompleted, wantState: StatusCompleted},
		{id: "seed", status: StatusCompleted, seeding: true, wantState: StatusCompleted},
		{id: "paused", status: StatusPaused, wantState: StatusPaused},
		{id: "failed", status: StatusFailed, wantState: StatusDownloading},
		{id: "queued", status: StatusQueued, wantState: StatusDownloading},
		{id: "downloading", status: StatusDownloading, wantState: StatusDownloading},
		{id: "verifying", status: StatusVerifying, wantState: StatusDownloading},
		{id: "metadata", status: StatusMetadata, wantState: StatusDownloading},
	}
	last := rows[len(rows)-1].id
	for _, seedAfter := range []bool{true, false} {
		name := map[bool]string{true: "seeding on", false: "seeding off"}[seedAfter]
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			first := mustManagerAt(t, dir)
			for _, row := range rows {
				realDownload(t, first, row.id, row.status, row.seeding)
				setItem(first, row.id, func(d *Download) {
					d.Destination = t.TempDir()
					d.InPlace = false
					d.Downloaded = 0
					if row.status == StatusFailed {
						d.Error = "disk full"
					}
				})
			}
			first.mu.Lock()
			if err := first.persistLocked(); err != nil {
				first.mu.Unlock()
				t.Fatal(err)
			}
			first.mu.Unlock()
			if err := first.pieceCompletion.Close(); err != nil {
				t.Fatal(err)
			}
			first.pieceCompletion = nil

			cfg := settings.Defaults()
			cfg.SeedAfterDownload = seedAfter
			cfg.MaxActiveDownloads = 10
			svc, err := settings.NewServiceAt(filepath.Join(t.TempDir(), "settings.json"))
			if err != nil {
				t.Fatal(err)
			}
			if err := svc.SaveSettings(cfg); err != nil {
				t.Fatal(err)
			}
			m, err := newManagerAt(dir, svc)
			if err != nil {
				t.Fatal(err)
			}
			closePieceCompletionOnCleanup(t, m)
			builds := &buildLog{}
			m.buildClient = builds.build
			if err := m.ServiceStartup(t.Context(), application.ServiceOptions{}); err != nil {
				t.Fatalf("startup: %v", err)
			}
			shutdown := sync.OnceFunc(func() {
				if err := m.ServiceShutdown(); err != nil {
					t.Errorf("shutdown: %v", err)
				}
			})
			t.Cleanup(shutdown)

			waitUntil(t, "the jobs of the restore pass to land", func() bool {
				return m.statusOf(t, last) == StatusDownloading && hasEngine(m, last)
			})
			for _, row := range rows {
				d := mustGet(t, m, row.id)
				wantSeed := row.seeding && seedAfter
				wantEngine := row.wantState != StatusCompleted || wantSeed
				if d.Status != row.wantState {
					t.Errorf("%s: status = %s, want %s", row.id, d.Status, row.wantState)
				}
				if d.Seeding != wantSeed {
					t.Errorf("%s: seeding = %v, want %v", row.id, d.Seeding, wantSeed)
				}
				if got := hasEngine(m, row.id); got != wantEngine {
					t.Errorf("%s: engine attached = %v, want %v", row.id, got, wantEngine)
				}
				if d.Error != "" {
					t.Errorf("%s: error = %q", row.id, d.Error)
				}
			}
			if n := builds.built(); n != 1 {
				t.Errorf("clients built = %d, want 1", n)
			}

			shutdown()
			again := reopen(t, m)
			for _, row := range rows {
				d := mustGet(t, again, row.id)
				if want := reloadedStatus(row.wantState); d.Status != want {
					t.Errorf("%s after a second restart: status = %s, want %s", row.id, d.Status, want)
				}
				if want := row.seeding && seedAfter; d.Seeding != want {
					t.Errorf("%s after a second restart: seeding = %v, want %v: the cleared wish to seed must reach the disk", row.id, d.Seeding, want)
				}
			}
		})
	}
}
