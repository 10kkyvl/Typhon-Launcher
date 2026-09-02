package install

import (
	"context"
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
