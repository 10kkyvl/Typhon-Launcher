package library

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// killFailProcess — процесс, который не даёт себя убить: kill возвращает
// ошибку, а завершается игра позже сама.
type killFailProcess struct {
	exitProcess
	killErr error
}

func (p *killFailProcess) kill() error { return p.killErr }

// Если убить игру не удалось, закрыл её не пользователь: последующее
// падение обязано остаться падением, а не стать «остановлено пользователем».
func TestFailedStopDoesNotHideALaterCrash(t *testing.T) {
	const dllNotFound = int(-1073741515)
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	game, err := s.AddGame(t.Context(), tempGameExe(t), "KillFails")
	if err != nil {
		t.Fatal(err)
	}
	clock := newRegClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	s.now = clock.now
	killErr := errors.New("access denied")
	proc := &killFailProcess{
		exitProcess: exitProcess{id: 6, exit: make(chan struct{}), err: codeError(dllNotFound)},
		killErr:     killErr,
	}
	s.start = func(_ context.Context, _ launch) (gameProcess, error) { return proc, nil }
	watcher := recordingWatcher{started: make(chan Game, 1), stopped: make(chan string, 1)}
	s.AddSessionWatcher(watcher)
	events := make(chan SessionEvent, 1)
	prev := emitStopped
	emitStopped = func(ev SessionEvent) { events <- ev }
	t.Cleanup(func() { emitStopped = prev })

	if err := s.PlayGame(game.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.StopGame(game.ID); !errors.Is(err, killErr) {
		t.Fatalf("StopGame error = %v, want the kill failure", err)
	}

	clock.set(clock.now().Add(2 * time.Second))
	close(proc.exit)
	waitSessionEnd(t, s, watcher)
	ev := <-events
	if !ev.Crashed || ev.ReasonCode != "dll_not_found" {
		t.Fatalf("event = %+v, want a crash: the stop failed, so the user did not close the game", ev)
	}
}
