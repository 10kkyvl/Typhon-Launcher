package library

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
	"typhon/internal/uierr"
)

func TestUnreadableLegacyMarkerDoesNotDropLocalOwnership(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	service := mustServiceAt(t, path)
	exe := tempGameExe(t)
	game, err := service.AddGame(exe, "Legacy")
	if err != nil {
		t.Fatal(err)
	}
	service.mu.Lock()
	service.games[0].Owned = true
	service.games[0].InstallType = ""
	service.games[0].CanonicalGameID = "catalog-1"
	if err := service.persist(); err != nil {
		service.mu.Unlock()
		t.Fatal(err)
	}
	service.mu.Unlock()
	if err := os.WriteFile(filepath.Join(filepath.Dir(exe), MarkerName), []byte("broken"), 0o600); err != nil {
		t.Fatal(err)
	}

	reloaded, err := NewServiceAt(path)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := reloaded.Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !stored.Owned {
		t.Fatalf("owned flag was dropped on marker parse error: %+v", stored)
	}
	if snapshot := reloaded.SyncSnapshot(); len(snapshot) != 1 || !snapshot[0].Owned {
		t.Fatalf("unknown ownership was downgraded during sync: %+v", snapshot)
	}
}

type lateProcess struct {
	killErr error
	exit    chan struct{}
	killed  bool
	waited  bool
}

func (p *lateProcess) pid() int { return 4242 }

func (p *lateProcess) kill() error {
	p.killed = true
	if p.killErr != nil {
		return p.killErr
	}
	close(p.exit)
	return nil
}

func (p *lateProcess) wait() error {
	<-p.exit
	p.waited = true
	return nil
}

func TestStopDuringStartStopsTheLateGame(t *testing.T) {
	tests := []struct {
		name        string
		killErr     error
		wantErr     bool
		wantRunning bool
	}{
		{name: "stopped", wantErr: true},
		{name: "kill fails", killErr: errors.New("access denied"), wantRunning: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
			game, err := service.AddGame(tempGameExe(t), "Game")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			service.ctx = ctx
			service.prepare = func(context.Context, launch) error { return nil }
			proc := &lateProcess{killErr: tc.killErr, exit: make(chan struct{})}
			watcher := recordingWatcher{started: make(chan Game, 1), stopped: make(chan string, 1)}
			service.AddSessionWatcher(watcher)
			entered := make(chan struct{})
			service.start = func(ctx context.Context, _ launch) (gameProcess, error) {
				close(entered)
				<-ctx.Done()
				return proc, nil
			}
			done := make(chan error, 1)
			go func() { done <- service.PlayGame(game.ID) }()
			<-entered
			if err := service.StopGame(game.ID); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if tc.wantErr && (!errors.Is(err, context.Canceled) || uierr.Code(err) != "library.launch_cancelled") {
					t.Fatalf("error = %v, want coded cancellation", err)
				}
				if !tc.wantErr && err != nil {
					t.Fatalf("error = %v, want the running game to be tracked", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("PlayGame did not return after cancellation")
			}
			if !proc.killed {
				t.Fatal("a game started after Stop was left running")
			}
			if tc.killErr == nil && !proc.waited {
				t.Fatal("stopped game was not waited for")
			}
			if got := service.IsRunning(game.ID); got != tc.wantRunning {
				t.Fatalf("IsRunning = %v, want %v", got, tc.wantRunning)
			}
			if !tc.wantRunning {
				return
			}
			close(proc.exit)
			select {
			case <-watcher.stopped:
			case <-time.After(5 * time.Second):
				t.Fatal("tracked game session did not finish after the game exited")
			}
		})
	}
}

func TestStopDuringPrepareReturnsCodedCancellation(t *testing.T) {
	for _, returnCancellation := range []bool{true, false} {
		t.Run(fmt.Sprint(returnCancellation), func(t *testing.T) {

			service := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
			exe := tempGameExe(t)
			game, err := service.AddGame(exe, "Game")
			if err != nil {
				t.Fatal(err)
			}
			service.ctx = context.Background()
			failures := make(chan string, 1)
			service.SetLaunchFailureRecorder(func(_, code, _ string) { failures <- code })
			entered := make(chan struct{})
			service.prepare = func(ctx context.Context, _ launch) error {
				close(entered)
				<-ctx.Done()
				if returnCancellation {
					return ctx.Err()
				}
				return nil
			}
			done := make(chan error, 1)
			go func() { done <- service.PlayGame(game.ID) }()
			<-entered
			if err := service.StopGame(game.ID); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) || uierr.Code(err) != "library.launch_cancelled" {
					t.Fatalf("error = %v, want context cancellation", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("PlayGame did not return after cancellation")
			}
			select {
			case code := <-failures:
				t.Fatalf("canceled prepare recorded launch failure %q", code)
			default:
			}

		})
	}
}

func TestApplyInstalledUpdateRetriesAfterExecutableRepoint(t *testing.T) {
	service := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	dir := t.TempDir()
	oldExe := filepath.Join(dir, "old.exe")
	newExe := filepath.Join(dir, "new.exe")
	if err := os.WriteFile(oldExe, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newExe, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	game, err := service.RegisterInstalled(InstalledGame{Title: "Game", InstallDir: dir, Executable: oldExe})
	if err != nil {
		t.Fatal(err)
	}
	oldMeasure := measureInstall
	t.Cleanup(func() { measureInstall = oldMeasure })
	entered := make(chan struct{})
	release := make(chan struct{})
	var mu sync.Mutex
	calls := 0
	measureInstall = func(id, path string) (int64, bool) {
		mu.Lock()
		calls++
		first := calls == 1
		mu.Unlock()
		if first {
			close(entered)
			<-release
		}
		return 99, false
	}
	done := make(chan struct {
		game Game
		err  error
	}, 1)
	go func() {
		// The update was resolved against oldExe. The user changes the
		// executable while its size is being measured; the retry must keep
		// the newer user choice instead of writing oldExe back.
		updated, err := service.ApplyInstalledUpdate(InstalledUpdate{ID: game.ID, Version: "2", Executable: oldExe})
		done <- struct {
			game Game
			err  error
		}{updated, err}
	}()
	<-entered
	if _, err := service.SetExecutable(game.ID, newExe); err != nil {
		t.Fatal(err)
	}
	close(release)
	result := <-done
	if result.err != nil {
		t.Fatal(result.err)
	}
	if result.game.Executable != newExe || result.game.SizeBytes != 99 {
		t.Fatalf("updated game = %+v, want new executable and retried measurement", result.game)
	}
	stored, err := service.Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Executable != newExe {
		t.Fatalf("persisted executable = %q, want user-selected %q", stored.Executable, newExe)
	}
}
