package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"typhon/internal/procs"
	"typhon/internal/uierr"
	"typhon/internal/usagestats"
)

type SessionEvent struct {
	GameID          string `json:"gameId"`
	SessionSeconds  int64  `json:"sessionSeconds"`
	PlaytimeSeconds int64  `json:"playtimeSeconds"`
}

var (
	errSessionNotRunning       = uierr.New("library.not_running", "игра не запущена")
	errSessionCannotConfirm    = uierr.New("library.cannot_confirm_process", "не удалось подтвердить процесс игры")
	errSessionProcessGone      = uierr.New("library.process_gone", "процесс игры больше не найден")
	errSessionIdentityMismatch = uierr.New("library.process_identity_mismatch", "процесс с этим pid принадлежит другой программе")
	errSessionIdentityUnknown  = uierr.New("library.process_identity_unknown", "время запуска процесса неизвестно, подтверждение невозможно")
)

func (s *Service) PlayGame(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return errSessionNotRunning
	}
	if _, ok := s.running[id]; ok || s.starting[id] != nil {
		return uierr.New("library.already_running", "игра уже запущена")
	}
	game := s.findLocked(id)
	if game == nil {
		return uierr.New("library.game_not_found", "игра не найдена")
	}
	if game.Uninstalled {
		return uierr.New("library.not_installed", "игра не установлена")
	}
	if _, err := os.Stat(game.Executable); err != nil {
		return uierr.New("library.executable_missing", "исполняемый файл больше не существует")
	}

	workDir, err := filepath.Abs(filepath.Dir(game.Executable))
	if err != nil {
		return fmt.Errorf("рабочая папка игры: %w", err)
	}
	copyGame := *game
	game = &copyGame
	req := launch{
		installDir: game.InstallDir,
		executable: game.Executable,
		args:       game.LaunchArgs,
		workDir:    workDir,
		shared:     game.UsesSharedBottle(),
	}
	ctx := s.ctx
	// Constructors are also used without startup by synchronous callers.
	// A missing context must not start platform work.
	if ctx == nil {
		return errSessionCannotConfirm
	}
	ctx, cancel := context.WithCancel(ctx)
	if s.starting == nil {
		s.starting = map[string]context.CancelFunc{}
	}
	s.starting[id] = cancel
	s.wg.Add(1)
	defer s.wg.Done()
	started := false
	defer func() {
		delete(s.starting, id)
		if !started {
			cancel()
		}
	}()
	s.mu.Unlock()
	err = s.prepare(ctx, req)
	s.mu.Lock()
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			return context.Canceled
		}
		slog.Error("prepare game runtime", "id", id, "installDir", game.InstallDir, "error", err)
		// Не поднявшееся окружение — такой же несостоявшийся запуск, как и не
		// стартовавший процесс. На macOS это вообще самая частая причина, по
		// которой игра не идёт, и журнал, молчащий о ней, оставляет
		// пользователя без единственной подсказки, которая у него была.
		s.noteLaunchFailureLocked(id, "library.runtime_failed", err.Error())
		return uierr.Wrap("library.runtime_failed", fmt.Errorf("не удалось подготовить окружение запуска: %w", err))
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Unlock()
	proc, err := s.start(ctx, req)
	s.mu.Lock()
	if err != nil {
		slog.Error("launch game", "id", id, "executable", game.Executable, "error", err)
		s.noteLaunchFailureLocked(id, "library.launch_failed", err.Error())
		return uierr.Wrap("library.launch_failed", fmt.Errorf("не удалось запустить игру: %w", err))
	}

	started = true
	//nolint:gosec // G115: PID из os/exec укладывается в uint32 на Windows
	pid := uint32(proc.pid())
	startedAt := s.now()
	s.running[id] = &session{process: proc, pid: pid, startedAt: startedAt, lastSeen: startedAt}
	slog.Info("game started", "id", id, "title", game.Title, "pid", proc.pid(),
		"executable", game.Executable, "workDir", workDir)
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

	executable := game.Executable
	s.sessionWG.Add(1)
	go func() {
		defer s.sessionWG.Done()
		defer cancel()
		waitErr := proc.wait()
		logExit(id, executable, s.now().Sub(startedAt), waitErr)
		// Детект по ОС переживает лаунчер и сам решает, когда сессия
		// закончилась (см. detectTick); закрывать её здесь при активном
		// детекте — значит закрывать по смерти лаунчер-обёртки, а не игры.
		s.mu.Lock()
		closed := s.closed
		watching := s.watching
		s.mu.Unlock()
		if closed || watching {
			return
		}
		s.finishSession(id, startedAt)
	}()
	return nil
}

// ServiceShutdown отменяет цикл детекта процессов и ждёт его завершения, а
// также короткие пост-сессионные горутины в wg. sessionWG — горутины
// ожидания cmd.Wait() дочерних процессов игр — сюда намеренно не входят:
// игра должна пережить закрытие лаунчера, а не быть убитой вместе с ним.
func (s *Service) ServiceShutdown() error {
	s.mu.Lock()
	s.closed = true
	cancel := s.cancel
	for _, stop := range s.starting {
		stop()
	}
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
	if cancel := s.starting[id]; cancel != nil {
		cancel()
		s.mu.Unlock()
		return nil
	}
	current, ok := s.running[id]
	ctx := s.ctx
	if ok {
		// Ставим до убийства: сессию закроет чужая горутина, и к тому
		// моменту отличить закрытие пользователем от падения будет нечем.
		current.stoppedByUser = true
	}
	s.mu.Unlock()
	if !ok {
		return errSessionNotRunning
	}
	if current.process != nil {
		if err := current.process.kill(); err != nil {
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
	list, _, err := s.scan(ctx)
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

// logExit — единственное место, где ОС говорит, почему игра закрылась. Без
// кода выхода журнал сообщает только «сессия длилась 2 секунды», и отличить
// не найденную библиотеку (0xC0000135) от вылета или от лаунчер-обёртки,
// которая отдала работу другому процессу и вышла сама, нечем.
func logExit(id, executable string, played time.Duration, err error) {
	code, known := exitCode(err)
	after := played.Round(time.Second)
	switch {
	case !known:
		slog.Warn("game process wait failed", "id", id, "executable", executable, "after", after, "error", err)
	case code != 0:
		slog.Warn("game process exited with an error", "id", id, "executable", executable,
			"after", after, "code", code, "codeHex", fmt.Sprintf("0x%08X", int64(code)&0xFFFFFFFF))
	default:
		slog.Info("game process exited", "id", id, "executable", executable, "after", after, "code", code)
	}
}

func exitCode(err error) (int, bool) {
	if err == nil {
		return 0, true
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode(), true
	}
	return 0, false
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

	stoppedByUser := false
	if current, ok := s.running[id]; ok {
		stoppedByUser = current.stoppedByUser
	}
	delete(s.running, id)
	for _, w := range s.watchers {
		w.SessionStopped(id)
	}
	endedAt := s.now()
	seconds := int64(endedAt.Sub(startedAt).Seconds())
	if s.onOutcome != nil {
		note := s.onOutcome
		played := endedAt.Sub(startedAt)
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			note(id, played, stoppedByUser)
		}()
	}
	if s.onSession != nil {
		notify := s.onSession
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			notify(id, seconds)
		}()
	}
	if s.playRecord != nil {
		record := s.playRecord
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			record(id, startedAt, endedAt)
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
		for i := range s.archived {
			if s.archived[i].ID == id {
				game = &s.archived[i]
				break
			}
		}
	}
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

// noteLaunchFailureLocked зовётся под мьютексом сервиса: PlayGame держит его
// на всё время запуска, а журнал совместимости пишется в своей горутине.
func (s *Service) noteLaunchFailureLocked(gameID, code, reason string) {
	if s.onLaunchFail == nil {
		return
	}
	note := s.onLaunchFail
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		note(gameID, code, reason)
	}()
}
