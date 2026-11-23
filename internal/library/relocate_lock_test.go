package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func TestRelocateReportsMarkerWriteFailure(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	game, _ := addRelocateGame(t, s)

	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	newDir := filepath.Join(blocker, "new")

	if _, err := s.Relocate(game.ID, newDir); err == nil {
		t.Fatal("a failed install marker write was swallowed")
	}

	stored, err := s.Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.InstallDir != newDir {
		t.Fatalf("memory and disk disagree after the marker failure: installDir = %q", stored.InstallDir)
	}
	reloaded := mustServiceAt(t, s.path)
	onDisk, err := reloaded.Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if onDisk.InstallDir != stored.InstallDir {
		t.Fatalf("disk installDir = %q, memory = %q", onDisk.InstallDir, stored.InstallDir)
	}
}

func TestCheckRelocatable(t *testing.T) {
	cases := []struct {
		name    string
		outside bool
		id      func(Game) string
		want    error
	}{
		{"executable inside", false, func(g Game) string { return g.ID }, nil},
		{"executable outside", true, func(g Game) string { return g.ID }, ErrExecutableOutside},
		{"unknown game", false, func(Game) string { return "missing" }, errNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
			game, _ := addRelocateGame(t, s)
			if tc.outside {
				outside := filepath.Join(t.TempDir(), "game.exe")
				if err := os.WriteFile(outside, []byte("stub"), 0o755); err != nil {
					t.Fatal(err)
				}
				s.mu.Lock()
				s.games[0].Executable = outside
				s.mu.Unlock()
			}
			if err := s.CheckRelocatable(tc.id(game)); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

// moveReservations are the two ways a move reserves a game: a new move, and
// the recovery of one a killed process left.
var moveReservations = []struct {
	name  string
	begin func(s *Service, game Game) (func(), error)
}{
	{"new move", func(s *Service, game Game) (func(), error) { return s.BeginMove(game.ID) }},
	{"recovery", func(s *Service, game Game) (func(), error) {
		return s.BeginRecovery(game.ID, game.InstallDir, game.InstallDir+"-moved")
	}},
}

func TestMoveReservationBlocksLaunch(t *testing.T) {
	for _, r := range moveReservations {
		t.Run(r.name, func(t *testing.T) {
			s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
			game, _ := addRelocateGame(t, s)
			s.prepare = func(context.Context, launch) error { return errors.New("stop before real launch") }

			release, err := r.begin(s, game)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.PlayGame(game.ID); !errors.Is(err, ErrGameRelocating) {
				t.Fatalf("PlayGame while relocating = %v, want ErrGameRelocating", err)
			}
			for _, again := range moveReservations {
				if _, err := again.begin(s, game); !errors.Is(err, ErrGameRelocating) {
					t.Fatalf("%s during a %s = %v, want ErrGameRelocating", again.name, r.name, err)
				}
			}
			release()
			release()
			if err := s.PlayGame(game.ID); errors.Is(err, ErrGameRelocating) {
				t.Fatal("launch still refused after release")
			}
		})
	}
}

func TestMoveReservationRefusesAStartingGame(t *testing.T) {
	for _, r := range moveReservations {
		t.Run(r.name, func(t *testing.T) {
			s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
			game, _ := addRelocateGame(t, s)
			entered := make(chan struct{})
			proceed := make(chan struct{})
			s.prepare = func(context.Context, launch) error {
				close(entered)
				<-proceed
				return errors.New("stop before real launch")
			}
			done := make(chan error, 1)
			go func() { done <- s.PlayGame(game.ID) }()
			<-entered

			if _, err := r.begin(s, game); !errors.Is(err, ErrRelocateGameRunning) {
				t.Fatalf("%s during a launch = %v, want ErrRelocateGameRunning", r.name, err)
			}
			close(proceed)
			if err := <-done; err == nil {
				t.Fatal("expected the injected preparation failure")
			}
		})
	}
}

func TestMoveReservationRefusesUnknownGame(t *testing.T) {
	for _, r := range moveReservations {
		t.Run(r.name, func(t *testing.T) {
			s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
			missing := Game{ID: "missing", InstallDir: filepath.Join(t.TempDir(), "missing")}
			if _, err := r.begin(s, missing); !errors.Is(err, errNotFound) {
				t.Fatalf("err = %v, want errNotFound", err)
			}
		})
	}
}

func TestMoveReservationAndLaunchNeverOverlap(t *testing.T) {
	for _, r := range moveReservations {
		t.Run(r.name, func(t *testing.T) {
			s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
			game, _ := addRelocateGame(t, s)
			var holding atomic.Bool
			var overlap atomic.Bool
			s.prepare = func(context.Context, launch) error {
				if holding.Load() {
					overlap.Store(true)
				}
				return errors.New("stop before real launch")
			}

			for range 200 {
				var wg sync.WaitGroup
				wg.Add(2)
				go func() {
					defer wg.Done()
					if err := s.PlayGame(game.ID); err == nil {
						overlap.Store(true)
					}
				}()
				go func() {
					defer wg.Done()
					release, err := r.begin(s, game)
					if err != nil {
						return
					}
					holding.Store(true)
					if err := s.PlayGame(game.ID); !errors.Is(err, ErrGameRelocating) {
						overlap.Store(true)
					}
					holding.Store(false)
					release()
				}()
				wg.Wait()
			}
			if overlap.Load() {
				t.Fatal("a launch ran while the game was locked for a move")
			}
		})
	}
}
