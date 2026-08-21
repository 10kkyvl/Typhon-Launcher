package library

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"
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
	emit("game:started", SessionEvent{GameID: id})

	go func() {
		_ = cmd.Wait()
		s.finishSession(id, startedAt)
	}()
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
	seconds := int64(time.Since(startedAt).Seconds())
	if s.onSession != nil {
		notify := s.onSession
		go notify(id, seconds)
	}
	game := s.findLocked(id)
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
