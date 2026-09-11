package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
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

func TestStopDuringPrepareReturnsCodedCancellation(t *testing.T) {
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
		return ctx.Err()
	}
	done := make(chan error, 1)
	go func() { done <- service.PlayGame(game.ID) }()
	<-entered
	if err := service.StopGame(game.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
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
