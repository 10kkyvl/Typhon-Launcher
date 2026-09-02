package install

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestDiscardSilentKeepsDirectoryWhenInstallerNotConfirmedStopped закрывает
// КРИТ-1: errInstallerNotConfirmedStopped значит, что процесс, возможно, всё
// ещё пишет в Destination, и RemoveAll на него — гонка на единственной копии
// данных (инвариант 9).
func TestDiscardSilentKeepsDirectoryWhenInstallerNotConfirmedStopped(t *testing.T) {
	s, _, _ := newTestService(t)
	dest := filepath.Join(t.TempDir(), "Game")
	mkFile(t, filepath.Join(dest, "file.bin"), 16)

	cause := fmt.Errorf("%w: %w", errInstallerNotConfirmedStopped, context.Canceled)
	s.discardSilent(Installation{Destination: dest}, fsSnapshot{}, cause)

	if !exists(dest) {
		t.Fatal("destination removed even though the installer never confirmed it stopped")
	}
}

func TestDiscardSilentRemovesDirectoryForOrdinaryFailure(t *testing.T) {
	s, _, _ := newTestService(t)
	dest := filepath.Join(t.TempDir(), "Game")
	mkFile(t, filepath.Join(dest, "file.bin"), 16)

	s.discardSilent(Installation{Destination: dest}, fsSnapshot{}, errors.New("boom"))

	if exists(dest) {
		t.Fatal("destination should have been removed for an ordinary failure")
	}
}

// TestFailKeepsInstallerStillRunningAsFailedNotCancelled проверяет, что
// errInstallerNotConfirmedStopped переживает даже j.cancelled=true: до фикса
// Service.fail сначала смотрел j.cancelled и помечал запись чисто отменённой
// ("ничего не осталось"), хотя установщик мог остаться жив и писать дальше.
func TestFailKeepsInstallerStillRunningAsFailedNotCancelled(t *testing.T) {
	s, _, _ := newTestService(t)
	const id = "f1"
	s.mu.Lock()
	s.items = append(s.items, &Installation{ID: id, Name: "Game", Status: StatusInstalling})
	s.jobs[id] = &job{cancel: func() {}, cancelled: true}
	s.mu.Unlock()

	cause := fmt.Errorf("%w: %w", errInstallerNotConfirmedStopped, context.Canceled)
	s.fail(id, cause)

	got, ok := s.snapshot(id)
	if !ok {
		t.Fatal("record disappeared")
	}
	if got.Status != StatusFailed {
		t.Fatalf("status = %s, want %s even though the job was marked cancelled", got.Status, StatusFailed)
	}
	if got.Error == "" {
		t.Fatal("failed record must carry an error")
	}
}

func TestRetryProceedsWhenWorkerStateIsDone(t *testing.T) {
	s, downloads, _ := newTestService(t)
	root := t.TempDir()
	mkInstaller(t, filepath.Join(root, "setup.exe"), "Inno Setup Setup Data (5.6.2) (u)")
	downloads.add("d1", "Game", root)

	const id = "retry2"
	s.mu.Lock()
	s.items = append(s.items, &Installation{ID: id, DownloadID: "d1", Name: "Game", Type: TypeExeInstaller, Status: StatusFailed, Error: "boom"})
	s.mu.Unlock()

	statePath := s.workerStatePath(id)
	if err := writeWorkerState(statePath, workerState{PID: os.Getpid(), Done: true, Code: 0}); err != nil {
		t.Fatalf("write worker state: %v", err)
	}

	s.runner = &fakeRunner{act: func(runSpec) {}}
	if err := s.Retry(id); err != nil {
		t.Fatalf("Retry error = %v, want nil once the worker state is done", err)
	}
}
