package library

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"typhon/internal/uierr"
)

func TestConcurrentPlayGameStartsTheGameOnce(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	game, err := s.AddGame(tempGameExe(t), "Double click")
	if err != nil {
		t.Fatal(err)
	}
	var starts atomic.Int32
	proc := newRegProcess(900)
	release := make(chan struct{})
	var releaseOnce sync.Once
	letLaunchesFinish := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(func() {
		letLaunchesFinish()
		proc.exitNow()
	})
	s.start = func(context.Context, launch) (gameProcess, error) {
		starts.Add(1)
		<-release
		return proc, nil
	}

	const callers = 16
	errs := make(chan error, callers)
	var ready sync.WaitGroup
	ready.Add(callers)
	gate := make(chan struct{})
	for range callers {
		go func() {
			ready.Done()
			<-gate
			errs <- s.PlayGame(game.ID)
		}()
	}
	ready.Wait()
	close(gate)

	for range callers - 1 {
		select {
		case err := <-errs:
			if uierr.Code(err) != "library.already_running" {
				t.Fatalf("a concurrent PlayGame returned %v, want library.already_running while the first launch is in flight", err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("a concurrent PlayGame blocked behind the launch in flight")
		}
	}
	letLaunchesFinish()
	select {
	case err := <-errs:
		if err != nil {
			t.Fatalf("the winning PlayGame failed: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the winning PlayGame never returned")
	}
	if got := starts.Load(); got != 1 {
		t.Fatalf("the game was launched %d times, want once", got)
	}
	proc.exitNow()
	s.sessionWG.Wait()
}

func TestConcurrentSessionsOfDifferentGamesStayIndependent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)

	const games = 8
	byExecutable := map[string]*regProcess{}
	ids := make([]string, 0, games)
	for i := range games {
		game, err := s.AddGame(tempGameExe(t), fmt.Sprintf("Game %d", i))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, game.ID)
		byExecutable[game.Executable] = newRegProcess(1000 + i)
	}
	s.start = func(_ context.Context, req launch) (gameProcess, error) { return byExecutable[req.executable], nil }

	var recordMu sync.Mutex
	recorded := map[string]int{}
	s.SetPlayRecorder(func(id string, _, _ time.Time) {
		recordMu.Lock()
		defer recordMu.Unlock()
		recorded[id]++
	})

	stopReaders := make(chan struct{})
	var readers sync.WaitGroup
	for range 4 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stopReaders:
					return
				default:
				}
				s.GetGames()
				s.GetRunningGames()
				s.GetInstalledGames()
				for _, id := range ids {
					s.IsRunning(id)
				}
			}
		}()
	}

	var players sync.WaitGroup
	failures := make(chan error, games*2)
	for _, id := range ids {
		players.Add(1)
		go func() {
			defer players.Done()
			if err := s.PlayGame(id); err != nil {
				failures <- fmt.Errorf("PlayGame %s: %w", id, err)
				return
			}
			if err := s.StopGame(id); err != nil {
				failures <- fmt.Errorf("StopGame %s: %w", id, err)
			}
		}()
	}
	players.Wait()
	close(stopReaders)
	readers.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}

	s.sessionWG.Wait()
	s.wg.Wait()
	if running := s.GetRunningGames(); len(running) != 0 {
		t.Fatalf("running after every game was stopped: %v", running)
	}
	recordMu.Lock()
	defer recordMu.Unlock()
	for _, id := range ids {
		if recorded[id] != 1 {
			t.Errorf("game %s recorded %d play sessions, want exactly 1", id, recorded[id])
		}
		game, err := s.Find(id)
		if err != nil {
			t.Fatal(err)
		}
		if game.LastPlayed == nil {
			t.Errorf("game %s has no LastPlayed after its session closed", id)
		}
	}
	reloaded := mustServiceAt(t, path).GetGames()
	for _, g := range reloaded {
		if g.LastPlayed == nil {
			t.Errorf("game %s lost its LastPlayed on reload", g.ID)
		}
	}
}
