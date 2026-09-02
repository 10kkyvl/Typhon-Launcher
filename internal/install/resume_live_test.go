//go:build windows || devmock

package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// TestServiceStartupResumesLiveWorkerAndFinalizes закрывает КРИТ-2: ловит
// самовоспроизводимую версию того, что произошло у пользователя — лаунчер
// перезапускается посреди тихой установки, воркер продолжает жить и
// дописывает игру, а запись должна не потеряться в StatusInterrupted, а
// дождаться Done и зарегистрироваться с честно неизвестным происхождением.
// Использует os.Getpid() как "живой" воркер, поэтому требует настоящего
// workerProcessAlive — на голом !windows-стенде без devmock он всегда
// сообщает "мёртв" (stubs_other.go / alive_other.go).
func TestServiceStartupResumesLiveWorkerAndFinalizes(t *testing.T) {
	dir := t.TempDir()
	s := mustServiceAt(t, dir)
	s.settings = newTestSettings(t)
	downloads := newFakeDownloads()
	registrar := &fakeRegistrar{}
	s.downloads = downloads
	s.library = registrar

	dest := filepath.Join(t.TempDir(), "Game")
	exe := filepath.Join(dest, "Game.exe")
	mkFile(t, exe, 4096)

	const id = "resume1"
	item := Installation{
		ID: id, DownloadID: "d1", Name: "Game", Type: TypeExeInstaller,
		Status: StatusInstalling, Destination: dest, Executable: exe,
		Engine: EngineInno, Silent: true, StartedAt: time.Now(),
	}
	if err := s.store.save([]Installation{item}); err != nil {
		t.Fatalf("seed store: %v", err)
	}

	statePath := s.workerStatePath(id)
	if err := writeWorkerState(statePath, workerState{PID: os.Getpid(), Phase: string(workerPhaseInstalling)}); err != nil {
		t.Fatalf("write worker state: %v", err)
	}

	restore := resumeWatchPollInterval
	resumeWatchPollInterval = 20 * time.Millisecond
	t.Cleanup(func() { resumeWatchPollInterval = restore })

	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("startup: %v", err)
	}
	t.Cleanup(func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})

	got, ok := s.snapshot(id)
	if !ok || got.Status != StatusInstalling {
		t.Fatalf("status after startup = %+v, want still installing: the worker (this test process) is alive", got)
	}

	if err := writeWorkerState(statePath, workerState{PID: os.Getpid(), Done: true, Code: 0}); err != nil {
		t.Fatalf("write final worker state: %v", err)
	}

	done := s.waitStatus(t, id, StatusCompleted)
	if done.Owned {
		t.Fatal("Owned = true, want false: origin of a resumed install is not known")
	}
	if !done.UninstallUnknown {
		t.Fatal("UninstallUnknown = false, want true: uninstall record for a resumed install is not known")
	}

	regs := registrar.registered()
	if len(regs) != 1 {
		t.Fatalf("registered games = %d, want 1", len(regs))
	}
	if regs[0].Owned || !regs[0].UninstallUnknown {
		t.Fatalf("registered = %+v, want Owned=false, UninstallUnknown=true", regs[0])
	}
}

// TestRetryRejectedWhileWorkerStillAlive закрывает п.3: статус мог стать
// retryable по устаревшему прочтению, и Retry не должен поднимать второго
// воркера поверх того, что уже пишет по тем же детерминированным путям
// state/spec/cancel. Требует настоящий workerProcessAlive(os.Getpid())==true.
func TestRetryRejectedWhileWorkerStillAlive(t *testing.T) {
	s, _, _ := newTestService(t)
	const id = "retry1"
	s.mu.Lock()
	s.items = append(s.items, &Installation{ID: id, DownloadID: "d1", Name: "Game", Type: TypeExeInstaller, Status: StatusFailed, Error: "boom"})
	s.mu.Unlock()

	statePath := s.workerStatePath(id)
	if err := writeWorkerState(statePath, workerState{PID: os.Getpid(), Done: false}); err != nil {
		t.Fatalf("write worker state: %v", err)
	}

	if err := s.Retry(id); !errors.Is(err, errInstallerStillRunning) {
		t.Fatalf("Retry error = %v, want errInstallerStillRunning", err)
	}
}

// TestCancelResumedInstallWritesMarkerAndWaitsForWorker закрывает последний
// хвост: Cancel() над записью, подхваченной после перезапуска (job для неё
// не заведён), не должен врать "отменено" над установщиком, который
// продолжает писать. Маркер создаётся, статус остаётся рабочим, и только
// когда воркер сам подтверждает Cancelled в состоянии, запись становится
// StatusCancelled через finishResumed -> cancelResumed.
func TestCancelResumedInstallWritesMarkerAndWaitsForWorker(t *testing.T) {
	dir := t.TempDir()
	s := mustServiceAt(t, dir)
	s.settings = newTestSettings(t)
	s.downloads = newFakeDownloads()
	s.library = &fakeRegistrar{}

	dest := filepath.Join(t.TempDir(), "Game")
	const id = "cancel1"
	item := Installation{
		ID: id, DownloadID: "d1", Name: "Game", Type: TypeExeInstaller,
		Status: StatusInstalling, Destination: dest, Engine: EngineInno, Silent: true,
		StartedAt: time.Now(),
	}
	if err := s.store.save([]Installation{item}); err != nil {
		t.Fatalf("seed store: %v", err)
	}

	statePath := s.workerStatePath(id)
	if err := writeWorkerState(statePath, workerState{PID: os.Getpid(), Phase: string(workerPhaseInstalling)}); err != nil {
		t.Fatalf("write worker state: %v", err)
	}

	restore := resumeWatchPollInterval
	resumeWatchPollInterval = 20 * time.Millisecond
	t.Cleanup(func() { resumeWatchPollInterval = restore })

	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("startup: %v", err)
	}
	t.Cleanup(func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})

	got, ok := s.snapshot(id)
	if !ok || got.Status != StatusInstalling {
		t.Fatalf("status after startup = %+v, want still installing", got)
	}

	if err := s.Cancel(id); err != nil {
		t.Fatalf("Cancel error = %v", err)
	}

	cancelPath := s.workerCancelPath(id)
	if !workerCancelRequested(cancelPath) {
		t.Fatal("cancel marker was not created")
	}

	got, ok = s.snapshot(id)
	if !ok || got.Status != StatusInstalling {
		t.Fatalf("status right after Cancel = %+v, want still installing (not cancelled yet)", got)
	}

	if err := writeWorkerState(statePath, workerState{PID: os.Getpid(), Done: true, Cancelled: true, Error: context.Canceled.Error()}); err != nil {
		t.Fatalf("write final worker state: %v", err)
	}

	done := s.waitStatus(t, id, StatusCancelled)
	if done.Error != "" {
		t.Fatalf("cancelled record carries error = %q, want empty", done.Error)
	}
}
