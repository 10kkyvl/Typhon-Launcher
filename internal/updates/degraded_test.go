package updates

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// makeDirUnwritable simulates a full disk or an antivirus holding the
// directory: os.CreateTemp inside dir starts failing with a permission
// error, which is exactly what storage.WriteAtomic sees in that situation.
// Permissions are windows-specific enough that read-only directories behave
// differently there, so callers skip on windows the same way
// TestApplyFullReleaseJournalPersistFailureAbortsBeforeRename already does.
func makeDirUnwritable(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil { //nolint:gosec // G302: временно закрываем права каталога, чтобы смоделировать сбой persist (инвариант 5)
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(dir, 0o755); err != nil { //nolint:gosec // G302: возврат прав, выставленных выше по той же причине (инвариант 5)
			t.Errorf("restore permissions on %s: %v", dir, err)
		}
	})
}

func skipOnWindowsPermissions(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("read-only directory permissions behave differently on windows")
	}
}

func TestMutateRollsBackAndDegradesOnPersistFailure(t *testing.T) {
	skipOnWindowsPermissions(t)
	h := newHarness(t)
	if err := h.service.check(h.library.games[0]); err != nil {
		t.Fatalf("seed check: %v", err)
	}
	before, ok := h.service.snapshot("local-1")
	if !ok {
		t.Fatal("expected local-1 to be tracked")
	}

	makeDirUnwritable(t, h.service.store.dir)

	snap, ok := h.service.mutate("local-1", func(u *Update) { u.Message = "changed" })
	if !ok {
		t.Fatal("mutate reported the game as untracked")
	}
	if snap != before {
		t.Fatalf("mutate returned %+v on a persist failure, want the unchanged %+v", snap, before)
	}
	if after, ok := h.service.snapshot("local-1"); !ok || after != before {
		t.Fatalf("in-memory state = %+v (ok=%v) after a persist failure, want rollback to %+v", after, ok, before)
	}
	h.service.mu.Lock()
	status := h.service.status
	h.service.mu.Unlock()
	if !status.Degraded || status.Message == "" {
		t.Fatalf("status = %+v, want degraded", status)
	}

	if err := os.Chmod(h.service.store.dir, 0o755); err != nil { //nolint:gosec // G302: возврат прав, выставленных выше по той же причине (инвариант 5)
		t.Fatal(err)
	}
	if _, ok := h.service.mutate("local-1", func(u *Update) { u.Message = "changed" }); !ok {
		t.Fatal("mutate reported the game as untracked")
	}
	h.service.mu.Lock()
	status = h.service.status
	h.service.mu.Unlock()
	if status.Degraded {
		t.Fatalf("status = %+v, want cleared after a successful persist", status)
	}
}

func TestCheckRollsBackNewEntryOnPersistFailure(t *testing.T) {
	skipOnWindowsPermissions(t)
	h := newHarness(t)
	if _, ok := h.service.snapshot("local-1"); ok {
		t.Fatal("expected local-1 not to be tracked yet")
	}
	makeDirUnwritable(t, h.service.store.dir)

	if err := h.service.check(h.library.games[0]); err == nil {
		t.Fatal("expected check to report the persist failure")
	}
	if _, ok := h.service.snapshot("local-1"); ok {
		t.Fatal("a failed first check must not leave a partially tracked entry behind")
	}
	h.service.mu.Lock()
	status := h.service.status
	h.service.mu.Unlock()
	if !status.Degraded {
		t.Fatalf("status = %+v, want degraded", status)
	}
}

func TestCheckRollsBackExistingEntryOnPersistFailure(t *testing.T) {
	skipOnWindowsPermissions(t)
	h := newHarness(t)
	if err := h.service.check(h.library.games[0]); err != nil {
		t.Fatalf("seed check: %v", err)
	}
	before, ok := h.service.snapshot("local-1")
	if !ok {
		t.Fatal("expected local-1 to be tracked")
	}

	makeDirUnwritable(t, h.service.store.dir)

	if err := h.service.check(h.library.games[0]); err == nil {
		t.Fatal("expected check to report the persist failure")
	}
	if after, ok := h.service.snapshot("local-1"); !ok || after != before {
		t.Fatalf("state = %+v (ok=%v), want rollback to %+v", after, ok, before)
	}
}

func TestCheckGamePropagatesPersistFailure(t *testing.T) {
	skipOnWindowsPermissions(t)
	h := newHarness(t)
	if err := h.service.check(h.library.games[0]); err != nil {
		t.Fatalf("seed check: %v", err)
	}
	makeDirUnwritable(t, h.service.store.dir)

	if _, err := h.service.CheckGame("local-1"); err == nil {
		t.Fatal("expected CheckGame to surface the persist failure to its caller")
	}
}

func TestPruneRollsBackOnPersistFailure(t *testing.T) {
	skipOnWindowsPermissions(t)
	h := newHarness(t)
	if err := h.service.check(h.library.games[0]); err != nil {
		t.Fatalf("seed check: %v", err)
	}
	before, ok := h.service.snapshot("local-1")
	if !ok {
		t.Fatal("expected local-1 to be tracked")
	}

	makeDirUnwritable(t, h.service.store.dir)

	h.service.prune(map[string]bool{})

	after, ok := h.service.snapshot("local-1")
	if !ok || after != before {
		t.Fatalf("prune changed the entry despite a failed persist: %+v (ok=%v), want %+v", after, ok, before)
	}
	h.service.mu.Lock()
	status := h.service.status
	h.service.mu.Unlock()
	if !status.Degraded {
		t.Fatalf("status = %+v, want degraded", status)
	}
}

func TestHandleSessionEndedRollsBackOnPersistFailure(t *testing.T) {
	skipOnWindowsPermissions(t)
	h := newHarness(t)
	if err := h.service.check(h.library.games[0]); err != nil {
		t.Fatalf("seed check: %v", err)
	}
	previousPath := filepath.Join(t.TempDir(), "previous")
	if err := os.MkdirAll(previousPath, 0o755); err != nil {
		t.Fatal(err)
	}

	h.service.mu.Lock()
	h.service.rollbacks["local-1"] = &Rollback{GameID: "local-1", Path: previousPath, AwaitLaunch: true}
	if u, ok := h.service.updates["local-1"]; ok {
		u.CanRollback = true
	}
	if err := h.service.persistRollbacksLocked(); err != nil {
		h.service.mu.Unlock()
		t.Fatalf("seed rollbacks: %v", err)
	}
	if err := h.service.persistLocked(); err != nil {
		h.service.mu.Unlock()
		t.Fatalf("seed updates: %v", err)
	}
	h.service.mu.Unlock()

	makeDirUnwritable(t, h.service.store.dir)

	h.service.HandleSessionEnded("local-1", 600)

	h.service.mu.Lock()
	_, stillHasRollback := h.service.rollbacks["local-1"]
	canRollback := h.service.updates["local-1"].CanRollback
	status := h.service.status
	h.service.mu.Unlock()

	if !stillHasRollback {
		t.Fatal("rollback entry removed in memory despite a failed persist")
	}
	if !canRollback {
		t.Fatal("CanRollback cleared in memory despite a failed persist")
	}
	if !status.Degraded {
		t.Fatalf("status = %+v, want degraded", status)
	}
	if _, err := os.Stat(previousPath); err != nil {
		t.Fatalf("previous version directory removed despite a failed persist: %v", err)
	}
}

func TestSweepPreviousRollsBackOnPersistFailure(t *testing.T) {
	skipOnWindowsPermissions(t)
	h := newHarness(t)
	if err := h.service.check(h.library.games[0]); err != nil {
		t.Fatalf("seed check: %v", err)
	}
	previousPath := filepath.Join(t.TempDir(), "previous")
	if err := os.MkdirAll(previousPath, 0o755); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour)

	h.service.mu.Lock()
	h.service.rollbacks["local-1"] = &Rollback{GameID: "local-1", Path: previousPath, KeepUntil: &past}
	if u, ok := h.service.updates["local-1"]; ok {
		u.CanRollback = true
	}
	if err := h.service.persistRollbacksLocked(); err != nil {
		h.service.mu.Unlock()
		t.Fatalf("seed rollbacks: %v", err)
	}
	if err := h.service.persistLocked(); err != nil {
		h.service.mu.Unlock()
		t.Fatalf("seed updates: %v", err)
	}
	h.service.mu.Unlock()

	makeDirUnwritable(t, h.service.store.dir)

	h.service.sweepPrevious()

	h.service.mu.Lock()
	_, stillHasRollback := h.service.rollbacks["local-1"]
	canRollback := h.service.updates["local-1"].CanRollback
	status := h.service.status
	h.service.mu.Unlock()

	if !stillHasRollback || !canRollback {
		t.Fatalf("sweep dropped state despite a failed persist: hasRollback=%v canRollback=%v", stillHasRollback, canRollback)
	}
	if !status.Degraded {
		t.Fatalf("status = %+v, want degraded", status)
	}
	if _, err := os.Stat(previousPath); err != nil {
		t.Fatalf("previous version directory removed despite a failed persist: %v", err)
	}
}

func TestAppendHistoryRollsBackOnPersistFailure(t *testing.T) {
	skipOnWindowsPermissions(t)
	h := newHarness(t)
	makeDirUnwritable(t, h.service.store.dir)

	h.service.appendHistory(UpdateHistory{ID: "h1", GameID: "local-1", Status: HistoryRunning})

	h.service.mu.Lock()
	historyLen := len(h.service.history)
	status := h.service.status
	h.service.mu.Unlock()

	if historyLen != 0 {
		t.Fatalf("history = %d entries, want rollback to empty", historyLen)
	}
	if !status.Degraded {
		t.Fatalf("status = %+v, want degraded", status)
	}
}

func TestFinishHistoryRollsBackOnPersistFailure(t *testing.T) {
	skipOnWindowsPermissions(t)
	h := newHarness(t)
	h.service.appendHistory(UpdateHistory{ID: "h1", GameID: "local-1", Status: HistoryRunning})
	h.service.mu.Lock()
	before := append([]UpdateHistory(nil), h.service.history...)
	h.service.mu.Unlock()

	makeDirUnwritable(t, h.service.store.dir)

	h.service.finishHistory("h1", HistoryCompleted, "")

	h.service.mu.Lock()
	after := append([]UpdateHistory(nil), h.service.history...)
	status := h.service.status
	h.service.mu.Unlock()

	if len(after) != len(before) || after[0] != before[0] {
		t.Fatalf("history = %+v, want rollback to %+v", after, before)
	}
	if !status.Degraded {
		t.Fatalf("status = %+v, want degraded", status)
	}
}

func TestEmitVerifyRollsBackOnPersistFailure(t *testing.T) {
	skipOnWindowsPermissions(t)
	h := newHarness(t)
	h.service.emitVerify("local-1", eventVerifyStarted, func(v *VerifyState) {
		*v = VerifyState{GameID: "local-1", Method: MethodManifest, Running: true}
	})
	h.service.mu.Lock()
	before := *h.service.verifications["local-1"]
	h.service.mu.Unlock()

	makeDirUnwritable(t, h.service.store.dir)

	snap := h.service.emitVerify("local-1", eventVerifyCompleted, func(v *VerifyState) {
		v.Running = false
		v.Error = "boom"
	})
	if snap.Error != "" || snap.Running != before.Running {
		t.Fatalf("emitVerify returned %+v on a persist failure, want the unchanged %+v", snap, before)
	}
	h.service.mu.Lock()
	after := *h.service.verifications["local-1"]
	status := h.service.status
	h.service.mu.Unlock()
	if after.Error != "" || after.Running != before.Running {
		t.Fatalf("in-memory verify state = %+v after a persist failure, want rollback to %+v", after, before)
	}
	if !status.Degraded {
		t.Fatalf("status = %+v, want degraded", status)
	}
}

func TestRememberPreviousRollsBackOnPersistFailure(t *testing.T) {
	skipOnWindowsPermissions(t)
	h := newHarness(t)
	if err := h.service.check(h.library.games[0]); err != nil {
		t.Fatalf("seed check: %v", err)
	}
	game := h.library.games[0]

	makeDirUnwritable(t, h.service.store.dir)

	h.service.rememberPrevious(game, filepath.Join(t.TempDir(), "previous"))

	h.service.mu.Lock()
	_, hasRollback := h.service.rollbacks[game.ID]
	canRollback := h.service.updates[game.ID].CanRollback
	status := h.service.status
	h.service.mu.Unlock()

	if hasRollback {
		t.Fatal("rollback entry recorded in memory despite a failed persist")
	}
	if canRollback {
		t.Fatal("CanRollback set in memory despite a failed persist")
	}
	if !status.Degraded {
		t.Fatalf("status = %+v, want degraded", status)
	}
}

func TestForgetPreviousRollsBackOnPersistFailure(t *testing.T) {
	skipOnWindowsPermissions(t)
	h := newHarness(t)
	if err := h.service.check(h.library.games[0]); err != nil {
		t.Fatalf("seed check: %v", err)
	}
	h.service.mu.Lock()
	h.service.rollbacks["local-1"] = &Rollback{GameID: "local-1", Path: filepath.Join(t.TempDir(), "previous")}
	if u, ok := h.service.updates["local-1"]; ok {
		u.CanRollback = true
	}
	if err := h.service.persistRollbacksLocked(); err != nil {
		h.service.mu.Unlock()
		t.Fatalf("seed rollbacks: %v", err)
	}
	if err := h.service.persistLocked(); err != nil {
		h.service.mu.Unlock()
		t.Fatalf("seed updates: %v", err)
	}
	h.service.mu.Unlock()

	makeDirUnwritable(t, h.service.store.dir)

	h.service.forgetPrevious("local-1")

	h.service.mu.Lock()
	_, stillHasRollback := h.service.rollbacks["local-1"]
	canRollback := h.service.updates["local-1"].CanRollback
	status := h.service.status
	h.service.mu.Unlock()

	if !stillHasRollback || !canRollback {
		t.Fatalf("forgetPrevious dropped state despite a failed persist: hasRollback=%v canRollback=%v", stillHasRollback, canRollback)
	}
	if !status.Degraded {
		t.Fatalf("status = %+v, want degraded", status)
	}
}
