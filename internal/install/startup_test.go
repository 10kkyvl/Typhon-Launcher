package install

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// TestServiceStartupResumesWorkerThatFinishedWhileLauncherWasDead закрывает
// finding 3: воркер, успевший записать Done в состояние ещё до того, как
// лаунчер перезапустился, не должен превращаться в StatusInterrupted — итог
// воркера (успех/провал/отмена) обязан дойти до записи так же, как если бы
// лаунчер был жив всё это время.
func TestServiceStartupResumesWorkerThatFinishedWhileLauncherWasDead(t *testing.T) {
	cases := []struct {
		name       string
		state      workerState
		wantStatus Status
		wantErr    string
	}{
		{"done clean", workerState{PID: 999999999, Done: true, Code: 0}, StatusCompleted, ""},
		{"done with error", workerState{PID: 999999999, Done: true, Error: "boom"}, StatusFailed, "boom"},
		{"done cancelled", workerState{PID: 999999999, Done: true, Cancelled: true, Error: "context canceled"}, StatusCancelled, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			s := mustServiceAt(t, dir)
			s.settings = newTestSettings(t)
			s.downloads = newFakeDownloads()
			registrar := &fakeRegistrar{}
			s.library = registrar

			dest := filepath.Join(t.TempDir(), "Game")
			exe := filepath.Join(dest, "Game.exe")
			mkFile(t, exe, 4096)

			const id = "resumed-done"
			item := Installation{
				ID: id, DownloadID: "d1", Name: "Game", Type: TypeExeInstaller,
				Status: StatusInstalling, Destination: dest, Executable: exe,
				Engine: EngineInno, Silent: true, StartedAt: time.Now(),
			}
			if err := s.store.save([]Installation{item}); err != nil {
				t.Fatalf("seed store: %v", err)
			}
			if err := writeWorkerState(s.workerStatePath(id), tc.state); err != nil {
				t.Fatalf("write worker state: %v", err)
			}

			restore := resumeWatchPollInterval
			resumeWatchPollInterval = 10 * time.Millisecond
			t.Cleanup(func() { resumeWatchPollInterval = restore })

			if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
				t.Fatalf("startup: %v", err)
			}
			t.Cleanup(func() {
				if err := s.ServiceShutdown(); err != nil {
					t.Errorf("shutdown: %v", err)
				}
			})

			got := s.waitStatus(t, id, tc.wantStatus)
			if got.Error != tc.wantErr {
				t.Fatalf("error = %q, want %q", got.Error, tc.wantErr)
			}
			if tc.wantStatus == StatusCompleted {
				if len(registrar.registered()) != 1 {
					t.Fatalf("registered games = %d, want 1", len(registrar.registered()))
				}
			}
		})
	}
}

// TestServiceStartupFinalizesControlledInstallCommittedBeforeCrash закрывает
// finding 4: launcher crashed after commit() renamed .partial into
// Destination but before the game was registered. .partial is gone,
// Destination holds the real files — ServiceStartup must finish the
// registration instead of reporting StatusInterrupted over an install that
// already succeeded.
func TestServiceStartupFinalizesControlledInstallCommittedBeforeCrash(t *testing.T) {
	dir := t.TempDir()
	s := mustServiceAt(t, dir)
	s.settings = newTestSettings(t)
	s.downloads = newFakeDownloads()
	registrar := &fakeRegistrar{}
	s.library = registrar

	dest := filepath.Join(t.TempDir(), "Game")
	mkFile(t, filepath.Join(dest, "Game.exe"), 4096)

	const id = "crash-committed"
	item := Installation{
		ID: id, DownloadID: "d1", Name: "Game", Type: TypeArchiveZip,
		Status: StatusExtracting, Destination: dest, StartedAt: time.Now(),
	}
	if err := s.store.save([]Installation{item}); err != nil {
		t.Fatalf("seed store: %v", err)
	}

	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("startup: %v", err)
	}
	t.Cleanup(func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})

	done := s.waitStatus(t, id, StatusCompleted)
	if done.GameID == "" {
		t.Fatal("GameID empty, want the install registered")
	}
	if len(registrar.registered()) != 1 {
		t.Fatalf("registered games = %d, want 1", len(registrar.registered()))
	}
}

// TestServiceStartupInterruptsControlledInstallWithEmptyDestination гарантирует,
// что фикс finding 4 не подхватывает записи, куда установщик так и не
// записал файлы: пустой (или отсутствующий) Destination — это настоящий
// прерванный процесс, а не пропущенная регистрация.
func TestServiceStartupInterruptsControlledInstallWithEmptyDestination(t *testing.T) {
	dir := t.TempDir()
	s := mustServiceAt(t, dir)
	s.settings = newTestSettings(t)

	dest := filepath.Join(t.TempDir(), "Game")
	const id = "crash-empty"
	item := Installation{
		ID: id, Name: "Game", Type: TypeArchiveZip,
		Status: StatusExtracting, Destination: dest, StartedAt: time.Now(),
	}
	if err := s.store.save([]Installation{item}); err != nil {
		t.Fatalf("seed store: %v", err)
	}

	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("startup: %v", err)
	}
	t.Cleanup(func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})

	got, ok := s.snapshot(id)
	if !ok || got.Status != StatusInterrupted || got.Error != interruptedMessage {
		t.Fatalf("status = %+v, want interrupted", got)
	}
}

// TestServiceStartupRecoversMoveModeCrashWithoutSource закрывает finding 5:
// MoveDir(ContentRoot -> .partial) уже удалил ContentRoot, когда лаунчер
// упал во время второго шага (commit() переносит .partial в Destination
// через свой собственный cross-device MoveDir). .partial теперь единственная
// копия данных пользователя, а Destination — недописанный кросс-девайсный
// перенос. sweepPartial не имеет права стереть .partial: она обязана
// стереть недописанный Destination и докатить перенос.
func TestServiceStartupRecoversMoveModeCrashWithoutSource(t *testing.T) {
	dir := t.TempDir()
	s := mustServiceAt(t, dir)
	s.settings = newTestSettings(t)
	s.downloads = newFakeDownloads()
	registrar := &fakeRegistrar{}
	s.library = registrar

	dest := filepath.Join(t.TempDir(), "Game")
	partial := dest + partialSuffix
	mkFile(t, filepath.Join(partial, "Game.exe"), 4096)
	mkFile(t, filepath.Join(partial, "data", "content.pak"), 2048)
	// недописанный кросс-девайсный перенос: меньше файлов, чем в partial
	mkFile(t, filepath.Join(dest, "Game.exe"), 100)

	contentRoot := filepath.Join(t.TempDir(), "already-gone")

	const id = "move-crash"
	item := Installation{
		ID: id, DownloadID: "d1", Name: "Game", Type: TypePortable, Mode: ModeMove,
		Status: StatusInstalling, Destination: dest, ContentRoot: contentRoot, StartedAt: time.Now(),
	}
	if err := s.store.save([]Installation{item}); err != nil {
		t.Fatalf("seed store: %v", err)
	}

	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("startup: %v", err)
	}
	t.Cleanup(func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})

	done := s.waitStatus(t, id, StatusCompleted)
	if done.GameID == "" {
		t.Fatal("GameID empty, want the install registered")
	}
	if exists(partial) {
		t.Fatal(".partial survived recovery, want it consumed by commit")
	}
	if got, err := os.ReadFile(filepath.Join(dest, "Game.exe")); err != nil || len(got) != 4096 {
		t.Fatalf("destination Game.exe = %d bytes, err=%v, want the full 4096-byte copy from .partial", len(got), err)
	}
	if !exists(filepath.Join(dest, "data", "content.pak")) {
		t.Fatal("destination missing data/content.pak from .partial")
	}
	if len(registrar.registered()) != 1 {
		t.Fatalf("registered games = %d, want 1", len(registrar.registered()))
	}
}

func TestServiceStartupKeepsCompleteMoveModeDestination(t *testing.T) {
	dir := t.TempDir()
	s := mustServiceAt(t, dir)
	s.settings = newTestSettings(t)
	s.downloads = newFakeDownloads()
	registrar := &fakeRegistrar{}
	s.library = registrar

	dest := filepath.Join(t.TempDir(), "Game")
	partial := dest + partialSuffix
	for _, root := range []string{partial, dest} {
		mkFile(t, filepath.Join(root, "Game.exe"), 4096)
		mkFile(t, filepath.Join(root, "data", "content.pak"), 2048)
	}

	const id = "move-crash-after-copy"
	item := Installation{
		ID: id, DownloadID: "d1", Name: "Game", Type: TypePortable, Mode: ModeMove,
		Status: StatusInstalling, Destination: dest, ContentRoot: filepath.Join(t.TempDir(), "gone"), StartedAt: time.Now(),
	}
	if err := s.store.save([]Installation{item}); err != nil {
		t.Fatalf("seed store: %v", err)
	}

	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("startup: %v", err)
	}
	t.Cleanup(func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})

	done := s.waitStatus(t, id, StatusCompleted)
	if done.GameID == "" {
		t.Fatal("GameID empty, want the install registered")
	}
	if exists(partial) {
		t.Fatal(".partial survived, want the verified duplicate removed")
	}
	if got, err := os.ReadFile(filepath.Join(dest, "Game.exe")); err != nil || len(got) != 4096 {
		t.Fatalf("destination Game.exe = %d bytes, err=%v, want the complete copy untouched", len(got), err)
	}
	if len(registrar.registered()) != 1 {
		t.Fatalf("registered games = %d, want 1", len(registrar.registered()))
	}
}

// TestServiceStartupSweepsPartialInCopyModeAsBefore проверяет, что фикс
// finding 5 не расширяется на Copy-режим: там ContentRoot — это скачанные
// данные, они не удаляются, и лишний .partial можно спокойно стереть и
// начать заново.
func TestServiceStartupSweepsPartialInCopyModeAsBefore(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(t.TempDir(), "Game")
	partial := dest + partialSuffix
	mkFile(t, filepath.Join(partial, "chunk.bin"), 16)
	contentRoot := t.TempDir()

	st := newStore(dir)
	if err := st.save([]Installation{
		{ID: "a", Name: "Game", Type: TypePortable, Mode: ModeCopy, Status: StatusInstalling, Destination: dest, ContentRoot: contentRoot},
	}); err != nil {
		t.Fatal(err)
	}

	s := mustServiceAt(t, dir)
	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("startup: %v", err)
	}
	t.Cleanup(func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})

	waitFor(t, "partial sweep", func() bool { return !exists(partial) })
}

// TestRetryFinalizesAlreadyCommittedControlledInstall закрывает finding 4 для
// Retry: перезапуск того же самого случая (.partial уже нет, Destination
// заполнен) не должен заново запускать распаковку в занятый каталог
// (errDestExists), а обязан просто дорегистрировать уже установленную игру.
func TestRetryFinalizesAlreadyCommittedControlledInstall(t *testing.T) {
	s, _, registrar := newTestService(t)

	dest := filepath.Join(t.TempDir(), "Game")
	mkFile(t, filepath.Join(dest, "Game.exe"), 4096)

	const id = "retry-committed"
	s.mu.Lock()
	s.items = append(s.items, &Installation{
		ID: id, DownloadID: "d1", Name: "Game", Type: TypeArchiveZip,
		Status: StatusFailed, Error: "boom", Destination: dest,
	})
	s.mu.Unlock()

	if err := s.Retry(id); err != nil {
		t.Fatalf("retry: %v", err)
	}

	done := s.waitStatus(t, id, StatusCompleted)
	if done.GameID == "" {
		t.Fatal("GameID empty, want the install registered")
	}
	if len(registrar.registered()) != 1 {
		t.Fatalf("registered games = %d, want 1", len(registrar.registered()))
	}
}
