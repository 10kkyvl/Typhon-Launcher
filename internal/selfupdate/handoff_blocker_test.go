package selfupdate

import (
	"context"
	"os"
	"sync/atomic"
	"testing"

	"typhon/internal/uierr"
)

// While an install, a removal, a move or a game update runs, the old
// launcher's shutdown would wait for it behind a hidden window: the hand-over
// is refused before anything is parked, with its own code, not the one that
// sends the update screen to the ordinary restart (which would refuse too).
func TestBeginHandoffIsRefusedWhileWorkRuns(t *testing.T) {
	r := newHandoffRig(t, func(HandoffSpec, *fakeSuccessor) { t.Error("the successor started while work runs") })
	r.readyForHandoff(t)
	var asked atomic.Int32
	r.svc.SetHandoffBlocker(func() bool {
		asked.Add(1)
		if _, ok := r.journal(t); ok {
			t.Error("the blocker was asked after the journal was written")
		}
		return true
	})

	err := r.svc.BeginHandoff(1)
	if uierr.Code(err) != "selfupdate.work_running" {
		t.Fatalf("BeginHandoff() = %v, want selfupdate.work_running", err)
	}
	if asked.Load() == 0 {
		t.Fatal("the blocker was never asked")
	}
	if got := readFileString(t, r.disk.exe); got != oldBody {
		t.Fatalf("launcher = %q, want it untouched", got)
	}
	entries, rerr := os.ReadDir(r.disk.install)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if len(entries) != 1 {
		t.Fatalf("install dir holds %d entries, want only the launcher", len(entries))
	}
	if _, ok := r.journal(t); ok {
		t.Fatal("a journal was written for a refused hand-over")
	}
	if st := r.svc.GetStatus(); st.State != StateReady || st.ErrorCode != "" || r.svc.HandoffInProgress() {
		t.Fatalf("status after the refusal = %+v, want ready without an error", st)
	}
}

// With nothing running the blocker lets the hand-over go ahead.
func TestBeginHandoffGoesAheadWhenNoWorkRuns(t *testing.T) {
	r := newHandoffRig(t, func(HandoffSpec, *fakeSuccessor) {})
	r.readyForHandoff(t)
	r.svc.SetHandoffBlocker(func() bool { return false })
	if _, err := r.svc.prepareHandoff(); err != nil {
		t.Fatalf("prepareHandoff() = %v, want the hand-over allowed", err)
	}
}

// The ordinary update quits the launcher and waits, in its shutdown, for the
// same work: the window froze on "the launcher will restart" and the worker
// gave up. It is refused with the same code before the worker is staged.
func TestApplyUpdateIsRefusedWhileWorkRuns(t *testing.T) {
	r := newHandoffRig(t, func(HandoffSpec, *fakeSuccessor) { t.Error("a successor started from the ordinary update") })
	var started atomic.Int32
	prevStart := startWorker
	startWorker = func(string, string) error {
		started.Add(1)
		return nil
	}
	t.Cleanup(func() { startWorker = prevStart })
	r.svc.SetHandoffBlocker(func() bool { return true })

	err := r.svc.ApplyUpdate("en")
	if uierr.Code(err) != "selfupdate.work_running" {
		t.Fatalf("ApplyUpdate() = %v, want selfupdate.work_running", err)
	}
	if started.Load() != 0 {
		t.Fatal("the worker was started while work runs")
	}
	workerDir, err := WorkerDir(r.disk.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if exists(t, workerDir) {
		t.Fatal("the worker was staged for a refused update")
	}
	specPath, err := SpecPath(r.disk.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if exists(t, specPath) {
		t.Fatal("an update spec was written for a refused update")
	}
	if got := readFileString(t, r.disk.exe); got != oldBody {
		t.Fatalf("launcher = %q, want it untouched", got)
	}
	if st := r.svc.GetStatus(); st.State != StateReady || st.ErrorCode != "" {
		t.Fatalf("status after the refusal = %+v, want ready without an error", st)
	}

	// The refusal released the service: once the work is over the same
	// update goes ahead.
	r.svc.SetHandoffBlocker(func() bool { return false })
	if err := r.svc.ApplyUpdate("en"); err != nil {
		t.Fatalf("ApplyUpdate() after the work ended = %v", err)
	}
	if started.Load() != 1 {
		t.Fatalf("the worker started %d times after the work ended, want once", started.Load())
	}
}

func refusedStageLeftNothing(t *testing.T, configDir string) {
	t.Helper()
	if exists(t, mustWorkerDir(t, configDir)) {
		t.Fatal("the worker was staged for a refused start")
	}
	specPath, err := SpecPath(configDir)
	if err != nil {
		t.Fatal(err)
	}
	if exists(t, specPath) {
		t.Fatal("a spec was written for a refused start")
	}
}

// The launcher's own rollback quits it like an update does: with work running
// the shutdown waited for it behind a frozen window and the worker gave up
// after its timeout. It is refused with the update's code before the worker is
// staged, and the refusal leaves the service free.
func TestApplyRollbackIsRefusedWhileWorkRuns(t *testing.T) {
	f := newRollbackFixture(t)
	s := rollbackService(t, f, "1.3.0")
	started := captureWorkerStarts(t)
	s.SetHandoffBlocker(func() bool { return true })
	before := s.GetRollback()
	beforeStatus := s.GetStatus()

	for _, attempt := range []string{"first", "second"} {
		if err := s.ApplyRollback("en"); uierr.Code(err) != "selfupdate.work_running" {
			t.Fatalf("%s ApplyRollback() = %v, want selfupdate.work_running", attempt, err)
		}
	}
	if len(started.specs) != 0 {
		t.Fatalf("a refused rollback started %d workers", len(started.specs))
	}
	refusedStageLeftNothing(t, f.configDir)
	if got := readTestFile(t, f.target); got != f.newExe {
		t.Fatalf("launcher = %q, want it untouched", got)
	}
	if got := s.GetRollback(); got != before {
		t.Fatalf("rollback after the refusal = %+v, want %+v", got, before)
	}
	if st := s.GetStatus(); st != beforeStatus {
		t.Fatalf("status after the refusal = %+v, want it unchanged %+v", st, beforeStatus)
	}

	s.SetHandoffBlocker(func() bool { return false })
	if err := s.ApplyRollback("en"); err != nil {
		t.Fatalf("ApplyRollback() after the work ended = %v", err)
	}
	if len(started.specs) != 1 {
		t.Fatalf("workers started after the work ended = %d, want 1", len(started.specs))
	}
	if err := s.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
}

// "Install stable" used to download the whole release and only then hear the
// refusal, which it showed as a failed install. It asks first: nothing is
// fetched, the offer stays and the status carries no error.
func TestInstallStableIsRefusedBeforeItDownloadsWhileWorkRuns(t *testing.T) {
	st := newBetaStand(t, "1.2.0", "stable")
	started := captureWorkerStarts(t)
	s := st.service(t, t.TempDir(), "1.3.0-beta.1", nil)
	if _, err := s.CheckForUpdate(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := s.GetStatus()
	s.SetHandoffBlocker(func() bool { return true })

	for _, attempt := range []string{"first", "second"} {
		status, err := s.InstallStable(context.Background(), "en")
		if uierr.Code(err) != "selfupdate.work_running" {
			t.Fatalf("%s InstallStable() = %v, want selfupdate.work_running", attempt, err)
		}
		if status != before {
			t.Fatalf("%s refusal returned %+v, want the status unchanged %+v", attempt, status, before)
		}
	}
	st.mu.Lock()
	hits := st.hits
	st.mu.Unlock()
	if hits != 0 {
		t.Fatalf("%d artifact downloads for a refused install", hits)
	}
	if len(started.specs) != 0 {
		t.Fatalf("a refused install started %d workers", len(started.specs))
	}
	if got := s.GetStatus(); got != before || got.Error != "" || got.StableOffer != "1.2.0" {
		t.Fatalf("status after the refusal = %+v, want %+v with the offer and no error", got, before)
	}
	if _, err := s.DownloadUpdate(context.Background()); err == nil {
		t.Fatal("the refusal left the downgrade pending for the ordinary download")
	}

	s.SetHandoffBlocker(func() bool { return false })
	if _, err := s.InstallStable(context.Background(), "en"); err != nil {
		t.Fatalf("InstallStable() after the work ended = %v", err)
	}
	if len(started.specs) != 1 {
		t.Fatalf("workers started after the work ended = %d, want 1", len(started.specs))
	}
}

// When the offer is the saved snapshot, "install stable" is a rollback, and
// it is refused the same way without staging the worker.
func TestInstallStableRollbackIsRefusedWhileWorkRuns(t *testing.T) {
	st := newBetaStand(t, "1.2.0", "stable")
	f := newRollbackFixture(t)
	started := captureWorkerStarts(t)
	s := betaRollbackService(t, st, f)
	if _, err := s.CheckForUpdate(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := s.GetStatus()
	s.SetHandoffBlocker(func() bool { return true })

	for _, attempt := range []string{"first", "second"} {
		if _, err := s.InstallStable(context.Background(), "en"); uierr.Code(err) != "selfupdate.work_running" {
			t.Fatalf("%s InstallStable() = %v, want selfupdate.work_running", attempt, err)
		}
	}
	if len(started.specs) != 0 {
		t.Fatalf("a refused rollback started %d workers", len(started.specs))
	}
	refusedStageLeftNothing(t, f.configDir)
	if got := s.GetStatus(); got != before {
		t.Fatalf("status after the refusal = %+v, want %+v", got, before)
	}

	s.SetHandoffBlocker(func() bool { return false })
	if _, err := s.InstallStable(context.Background(), "en"); err != nil {
		t.Fatalf("InstallStable() after the work ended = %v", err)
	}
	if len(started.specs) != 1 {
		t.Fatalf("workers started after the work ended = %d, want 1", len(started.specs))
	}
	if err := s.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
}
