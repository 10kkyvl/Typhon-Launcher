package updates

import (
	"context"
	"errors"
	"testing"
	"time"

	"typhon/internal/download"
)

func (h *harness) awaitJob(t *testing.T, gameID string) {
	t.Helper()
	h.service.mu.Lock()
	j := h.service.jobs[gameID]
	h.service.mu.Unlock()
	if j == nil {
		return
	}
	select {
	case <-j.done:
	case <-time.After(30 * time.Second):
		t.Fatal("проверка не завершилась")
	}
}

func (h *harness) seedManifest(t *testing.T) {
	t.Helper()
	manifest, err := BuildManifest(context.Background(), h.installDir, nil)
	if err != nil {
		t.Fatalf("build manifest: %v", err)
	}
	manifest.GameID = "local-1"
	manifest.ReleaseID = "r1"
	manifest.Version = "1.0"
	if err := h.service.store.saveManifest(manifest); err != nil {
		t.Fatalf("save manifest: %v", err)
	}
}

func (h *harness) withTorrent(report download.ReuseReport) {
	h.releases.list[0].InfoHash = "hash-r1"
	h.downloads.reuseErr = nil
	h.downloads.reuse = report
}

func archiveReport() download.ReuseReport {
	return download.ReuseReport{
		InfoHash:     "hash-r1",
		Name:         "Game.v1.0.rar",
		Layout:       download.LayoutArchivePackage,
		Applicable:   false,
		TotalBytes:   157587818,
		MissingBytes: 157587818,
		MissingFiles: 1,
		TotalPieces:  1203,
		BadPieces:    1203,
		Files:        []download.ReuseFile{{Path: "Game.v1.0.rar", Size: 157587818, Missing: true}},
	}
}

// The release is a torrent holding an archive; the installation is what came
// out of that archive. Verifying one against the other reports the whole game
// as destroyed, which is what the stored manifest exists to prevent.
func TestVerifyGameFallsBackToManifestForArchiveRelease(t *testing.T) {
	h := newHarness(t)
	h.withTorrent(archiveReport())
	h.seedManifest(t)

	if err := h.service.VerifyGame("local-1"); err != nil {
		t.Fatalf("verify: %v", err)
	}
	h.awaitJob(t, "local-1")

	state, err := h.service.GetVerifyState("local-1")
	if err != nil {
		t.Fatalf("verify state: %v", err)
	}
	if state.Method != MethodManifest {
		t.Fatalf("method = %q, want %q", state.Method, MethodManifest)
	}
	if state.MissingFiles != 0 || state.CorruptedPieces != 0 {
		t.Fatalf("healthy install reported as damaged: missing=%d corrupted=%d",
			state.MissingFiles, state.CorruptedPieces)
	}
	if state.Repairable {
		t.Fatal("восстановление из архивного торрента писало бы архив в каталог игры")
	}
	if state.Ratio != 1 {
		t.Fatalf("ratio = %v, want 1", state.Ratio)
	}
}

func TestVerifyGameWithoutManifestReportsUnavailable(t *testing.T) {
	h := newHarness(t)
	h.withTorrent(archiveReport())

	if err := h.service.VerifyGame("local-1"); err != nil {
		t.Fatalf("verify: %v", err)
	}
	h.awaitJob(t, "local-1")

	state, err := h.service.GetVerifyState("local-1")
	if err != nil {
		t.Fatalf("verify state: %v", err)
	}
	if state.Method != MethodUnavailable {
		t.Fatalf("method = %q, want %q", state.Method, MethodUnavailable)
	}
	if state.MissingFiles != 0 || state.CorruptedPieces != 0 || state.Repairable {
		t.Fatalf("unverifiable install reported as damaged: %+v", state)
	}
}

func TestVerifyGameReportsTorrentDamage(t *testing.T) {
	h := newHarness(t)
	h.withTorrent(download.ReuseReport{
		InfoHash:     "hash-r1",
		Layout:       download.LayoutDirectFiles,
		Applicable:   true,
		PresentFiles: 3,
		Flat:         true,
		TotalBytes:   1000,
		MatchedBytes: 700,
		MissingBytes: 300,
		MissingFiles: 1,
		TotalPieces:  10,
		OkPieces:     7,
		BadPieces:    3,
	})

	if err := h.service.VerifyGame("local-1"); err != nil {
		t.Fatalf("verify: %v", err)
	}
	h.awaitJob(t, "local-1")

	state, err := h.service.GetVerifyState("local-1")
	if err != nil {
		t.Fatalf("verify state: %v", err)
	}
	if state.Method != MethodTorrent {
		t.Fatalf("method = %q, want %q", state.Method, MethodTorrent)
	}
	if state.MissingFiles != 1 || state.CorruptedPieces != 3 || !state.Repairable {
		t.Fatalf("real damage not reported: %+v", state)
	}
	if state.ReleaseID != "r1" || state.Version != "1.0" || state.InstallDir != h.installDir {
		t.Fatalf("result is not bound to the installation: %+v", state)
	}
}

func TestVerifyGameRefusesWhileGameRuns(t *testing.T) {
	h := newHarness(t)
	h.seedManifest(t)
	h.library.running = []string{"local-1"}

	if err := h.service.VerifyGame("local-1"); !errors.Is(err, errGameRunning) {
		t.Fatalf("err = %v, want %v", err, errGameRunning)
	}
}

// A result describes one release in one directory. After an update, a move or a
// re-link it says nothing about what is on disk now.
func TestGetVerifyStateDropsResultOfAnotherVersion(t *testing.T) {
	h := newHarness(t)
	h.withTorrent(download.ReuseReport{
		InfoHash:     "hash-r1",
		Layout:       download.LayoutDirectFiles,
		Applicable:   true,
		PresentFiles: 3,
		TotalBytes:   1000,
		MatchedBytes: 700,
		MissingFiles: 1,
		TotalPieces:  10,
		BadPieces:    3,
	})
	if err := h.service.VerifyGame("local-1"); err != nil {
		t.Fatalf("verify: %v", err)
	}
	h.awaitJob(t, "local-1")

	h.library.mu.Lock()
	h.library.games[0].Version = "1.1"
	h.library.mu.Unlock()

	state, err := h.service.GetVerifyState("local-1")
	if err != nil {
		t.Fatalf("verify state: %v", err)
	}
	if state.Method != MethodPending {
		t.Fatalf("method = %q, want %q", state.Method, MethodPending)
	}
	if state.MissingFiles != 0 || state.CorruptedPieces != 0 || state.Repairable {
		t.Fatalf("stale result served as current: %+v", state)
	}
}

// Sizing an update measures the installed files against the release the user
// does not have yet. Those differences are not damage and must never reach the
// integrity result.
func TestPreparePlanLeavesVerifyStateUntouched(t *testing.T) {
	h := newHarness(t)
	h.releases.list[1].InfoHash = "hash-r2"
	h.downloads.reuseErr = nil
	h.downloads.reuse = download.ReuseReport{
		InfoHash:     "hash-r2",
		Layout:       download.LayoutDirectFiles,
		Applicable:   true,
		PresentFiles: 3,
		TotalBytes:   1000,
		MatchedBytes: 400,
		MissingBytes: 600,
		MissingFiles: 12,
		TotalPieces:  10,
		BadPieces:    6,
	}

	h.service.check(h.library.games[0])
	if _, err := h.service.buildPlan(context.Background(), "local-1"); err != nil {
		t.Fatalf("build plan: %v", err)
	}

	state, err := h.service.GetVerifyState("local-1")
	if err != nil {
		t.Fatalf("verify state: %v", err)
	}
	if state.Method != MethodPending {
		t.Fatalf("method = %q, want %q", state.Method, MethodPending)
	}
	if state.MissingFiles != 0 || state.CorruptedPieces != 0 || state.CheckedAt != nil {
		t.Fatalf("update sizing leaked into the integrity result: %+v", state)
	}
}

func TestRepairRefusesWithoutTorrentVerdict(t *testing.T) {
	h := newHarness(t)
	h.withTorrent(archiveReport())
	h.seedManifest(t)
	if err := h.service.VerifyGame("local-1"); err != nil {
		t.Fatalf("verify: %v", err)
	}
	h.awaitJob(t, "local-1")

	if err := h.service.RepairGame("local-1"); !errors.Is(err, errRepairUnavailable) {
		t.Fatalf("err = %v, want %v", err, errRepairUnavailable)
	}
	h.downloads.mu.Lock()
	defer h.downloads.mu.Unlock()
	if len(h.downloads.requests) != 0 {
		t.Fatalf("repair queued a download into the install directory: %+v", h.downloads.requests)
	}
}
