package library

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"typhon/internal/usagestats"
)

type SessionEvent struct {
	GameID          string `json:"gameId"`
	SessionSeconds  int64  `json:"sessionSeconds"`
	PlaytimeSeconds int64  `json:"playtimeSeconds"`
}

func (s *Service) PlayGame(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.running[id]; ok {
		return errors.New("игра уже запущена")
	}
	game := s.findLocked(id)
	if game == nil {
		return errors.New("игра не найдена")
	}
	if game.Uninstalled {
		return errors.New("игра не установлена")
	}
	if _, err := os.Stat(game.Executable); err != nil {
		return errors.New("исполняемый файл больше не существует")
	}

	workDir, err := filepath.Abs(filepath.Dir(game.Executable))
	if err != nil {
		return fmt.Errorf("рабочая папка игры: %w", err)
	}

	cmd := exec.Command(game.Executable, game.LaunchArgs...)
	cmd.Dir = workDir
	if err := cmd.Start(); err != nil {
		slog.Error("launch game", "id", id, "executable", game.Executable, "error", err)
		return fmt.Errorf("не удалось запустить игру: %w", err)
	}

	startedAt := time.Now()
	s.running[id] = &session{process: cmd.Process, startedAt: startedAt}
	slog.Info("game started", "id", id, "title", game.Title, "pid", cmd.Process.Pid)
	for _, w := range s.watchers {
		w.SessionStarted(*game)
	}
	s.recordUsage(usagestats.Event{
		Type:      usagestats.TypeGameStarted,
		Timestamp: time.Now(),
		Properties: usagestats.Properties{
			GameID: game.CanonicalGameID,
		},
	})
	emit("game:started", SessionEvent{GameID: id})

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		_ = cmd.Wait()
		s.finishSession(id, startedAt)
	}()
	return nil
}

// ServiceShutdown waits for the session watchers and the session-finished
// callbacks to return. Without it those goroutines outlive the service and
// can run their persist after shutdown, against a state directory the owner
// already considers closed.
func (s *Service) ServiceShutdown() error {
	s.wg.Wait()
	return nil
}

func (s *Service) StopGame(id string) error {
	s.mu.Lock()
	current, ok := s.running[id]
	s.mu.Unlock()
	if !ok {
		return errors.New("игра не запущена")
	}
	return current.process.Kill()
}

func (s *Service) finishSession(id string, startedAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.running, id)
	for _, w := range s.watchers {
		w.SessionStopped(id)
	}
	seconds := int64(time.Since(startedAt).Seconds())
	if s.onSession != nil {
		notify := s.onSession
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			notify(id, seconds)
		}()
	}
	game := s.findLocked(id)
	usageGameID := ""
	if game != nil {
		usageGameID = game.CanonicalGameID
	}
	s.recordUsage(usagestats.Event{
		Type:      usagestats.TypeGameStopped,
		Timestamp: time.Now(),
		Properties: usagestats.Properties{
			GameID:          usageGameID,
			DurationSeconds: seconds,
		},
	})
	if game == nil {
		emit("game:stopped", SessionEvent{GameID: id, SessionSeconds: seconds})
		return
	}
	now := time.Now()
	game.LastPlayed = &now
	game.PlaytimeSeconds += seconds
	if err := s.persist(); err != nil {
		slog.Error("persist session", "id", id, "error", err)
	}
	slog.Info("game stopped", "id", id, "title", game.Title, "sessionSeconds", seconds)
	emit("game:stopped", SessionEvent{GameID: id, SessionSeconds: seconds, PlaytimeSeconds: game.PlaytimeSeconds})
	s.emitUpdated()
}

func (s *Service) findLocked(id string) *Game {
	for i := range s.games {
		if s.games[i].ID == id {
			return &s.games[i]
		}
	}
	return nil
}
