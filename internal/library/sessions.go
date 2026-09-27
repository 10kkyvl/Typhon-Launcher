package library

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"time"

	"typhon/internal/procs"
	"typhon/internal/usagestats"
)

type SessionEvent struct {
	GameID          string `json:"gameId"`
	SessionSeconds  int64  `json:"sessionSeconds"`
	PlaytimeSeconds int64  `json:"playtimeSeconds"`
}

var (
	errSessionNotRunning       = errors.New("игра не запущена")
	errSessionCannotConfirm    = errors.New("не удалось подтвердить процесс игры")
	errSessionProcessGone      = errors.New("процесс игры больше не найден")
	errSessionIdentityMismatch = errors.New("процесс с этим pid принадлежит другой программе")
	errSessionIdentityUnknown  = errors.New("время запуска процесса неизвестно, подтверждение невозможно")
	errSessionLaunching        = errors.New("игра уже запускается")
	errSessionGameRemoved      = errors.New("игра запущена, но пока шёл запрос прав её убрали из библиотеки: время не учтено")
	errElevationDeclined       = errors.New("игре нужны права администратора: запрос Windows отклонён")
	errElevatedNoProcess       = errors.New("процесс игры, запущенной с правами администратора, не получен от системы")
)

func (s *Service) PlayGame(id string) error {
	s.mu.Lock()
	if _, ok := s.running[id]; ok {
		s.mu.Unlock()
		return errors.New("игра уже запущена")
	}
	if _, ok := s.launching[id]; ok {
		s.mu.Unlock()
		return errSessionLaunching
	}
	game := s.findLocked(id)
	if game == nil {
		s.mu.Unlock()
		return errors.New("игра не найдена")
	}
	if game.Uninstalled {
		s.mu.Unlock()
		return errors.New("игра не установлена")
	}
	if _, err := os.Stat(game.Executable); err != nil {
		s.mu.Unlock()
		return errors.New("исполняемый файл больше не существует")
	}

	exe := game.Executable
	args := slices.Clone(game.LaunchArgs)
	workDir, err := filepath.Abs(filepath.Dir(exe))
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("рабочая папка игры: %w", err)
	}

	cmd := exec.Command(game.Executable, game.LaunchArgs...)
	cmd.Dir = workDir
	startErr := cmd.Start()
	if startErr == nil {
		//nolint:gosec // G115: PID из os/exec укладывается в uint32 на Windows
		s.beginSessionLocked(id, game, launched{process: cmd.Process, pid: uint32(cmd.Process.Pid), wait: cmd.Wait})
		s.mu.Unlock()
		return nil
	}
	if !needsElevation(startErr) {
		s.mu.Unlock()
		slog.Error("launch game", "id", id, "executable", exe, "error", startErr)
		return fmt.Errorf("не удалось запустить игру: %w", startErr)
	}

	// Окно UAC ждёт ответа пользователя сколько угодно: место занимается в
	// launching, а мьютекс на время ожидания отпускается.
	s.launching[id] = struct{}{}
	elevate := s.elevate
	s.mu.Unlock()

	slog.Info("game requires elevation", "id", id, "executable", exe)
	proc, elevErr := elevate(exe, args, workDir)

	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.launching, id)
	if elevErr != nil {
		slog.Error("launch game elevated", "id", id, "executable", exe, "error", elevErr)
		return fmt.Errorf("не удалось запустить игру: %w", elevErr)
	}
	game = s.findLocked(id)
	if game == nil {
		s.watchProcess(id, proc)
		return errSessionGameRemoved
	}
	s.beginSessionLocked(id, game, proc)
	return nil
}

func (s *Service) beginSessionLocked(id string, game *Game, proc launched) {
	if current, ok := s.running[id]; ok {
		// Детект по ОС успел открыть сессию, пока пользователь отвечал на
		// UAC: событие старта уже ушло, осталось отдать сессии свой процесс.
		if current.process == nil && current.pid == proc.pid {
			current.process = proc.process
		}
		s.watchProcess(id, proc)
		return
	}

	startedAt := s.now()
	s.running[id] = &session{process: proc.process, pid: proc.pid, startedAt: startedAt, lastSeen: startedAt}
	slog.Info("game started", "id", id, "title", game.Title, "pid", proc.pid)
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
	s.watchProcess(id, proc)
}

func (s *Service) watchProcess(id string, proc launched) {
	s.sessionWG.Add(1)
	go func() {
		defer s.sessionWG.Done()
		if waitErr := proc.wait(); waitErr != nil {
			slog.Debug("game process exited", "id", id, "error", waitErr)
		}
		// Детект по ОС переживает лаунчер и сам решает, когда сессия
		// закончилась (см. detectTick); закрывать её здесь при активном
		// детекте — значит закрывать по смерти лаунчер-обёртки, а не игры.
		s.mu.Lock()
		closed := s.closed
		watching := s.watching
		current, ok := s.running[id]
		s.mu.Unlock()
		if closed || watching || !ok || current.pid != proc.pid {
			return
		}
		s.finishSession(id, current.startedAt)
	}()
}

// ServiceShutdown отменяет цикл детекта процессов и ждёт его завершения, а
// также короткие пост-сессионные горутины в wg. sessionWG — горутины
// ожидания cmd.Wait() дочерних процессов игр — сюда намеренно не входят:
// игра должна пережить закрытие лаунчера, а не быть убитой вместе с ним.
func (s *Service) ServiceShutdown() error {
	s.mu.Lock()
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	s.wg.Wait()
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	return nil
}

func (s *Service) StopGame(id string) error {
	s.mu.Lock()
	current, ok := s.running[id]
	ctx := s.ctx
	s.mu.Unlock()
	if !ok {
		return errSessionNotRunning
	}
	if current.process != nil {
		if err := current.process.Kill(); err != nil {
			return fmt.Errorf("остановить игру: %w", err)
		}
		return nil
	}

	// Сессия обнаружена по ОС: pid могла переиспользовать другая программа
	// с момента детекта, поэтому перед убийством личность подтверждается
	// свежим сканом и сверкой времени старта процесса.
	if ctx == nil {
		return fmt.Errorf("%w: сервис ещё не запущен", errSessionCannotConfirm)
	}
	list, err := s.scan(ctx)
	if err != nil {
		return fmt.Errorf("%w: %w", errSessionCannotConfirm, err)
	}
	var found *procs.Process
	for i := range list {
		if list[i].PID == current.pid {
			found = &list[i]
			break
		}
	}
	if found == nil {
		return errSessionProcessGone
	}
	if found.CreatedAtUnknown || current.createdAt.IsZero() {
		return errSessionIdentityUnknown
	}
	if !found.CreatedAt.Equal(current.createdAt) {
		return errSessionIdentityMismatch
	}

	proc, err := os.FindProcess(int(current.pid))
	if err != nil {
		return fmt.Errorf("найти процесс: %w", err)
	}
	if err := proc.Kill(); err != nil {
		return fmt.Errorf("остановить игру: %w", err)
	}
	return nil
}

func (s *Service) finishSession(id string, startedAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Проверка closed у вызывающего снимается до захвата мьютекса, поэтому
	// Shutdown может успеть пройти между ней и этим местом: после него
	// persist писать уже некуда.
	if s.closed {
		return
	}

	delete(s.running, id)
	for _, w := range s.watchers {
		w.SessionStopped(id)
	}
	seconds := int64(s.now().Sub(startedAt).Seconds())
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
	previousLastPlayed := game.LastPlayed
	previousPlaytime := game.PlaytimeSeconds
	now := s.now()
	game.LastPlayed = &now
	game.PlaytimeSeconds += seconds
	if err := s.persist(); err != nil {
		game.LastPlayed = previousLastPlayed
		game.PlaytimeSeconds = previousPlaytime
		// Вызывающие — cmd.Wait()-горутина и detect-цикл watch.go, вернуть
		// ошибку им наверх некому: это единственное место в пакете, где
		// логирование, а не return err, оправдано.
		slog.Error("persist session", "id", id, "error", err)
		return
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
