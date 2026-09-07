package install

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"typhon/internal/download"
)

var (
	errBrokerNoDirPath     = errors.New("каталог брокера установки недоступен")
	errBrokerNoContentRoot = errors.New("загрузка без каталога назначения")
	errBrokerNoLibraryRoot = errors.New("папка игр не задана")
	errBrokerClosing       = errors.New("сервис установки выключается")

	// Подменяются в тестах, чтобы не ждать боевые тайминги и не зависеть от
	// платформы, на которой идёт прогон.
	elevationAvailable = elevationSupported
	brokerStopWait     = 10 * time.Second
)

type broker struct {
	dir  string
	proc workerHandle
	gone chan struct{}
	dead bool
}

func (s *Service) brokerDir(downloadID string) string {
	if s.store == nil || s.store.dir == "" {
		return ""
	}
	return filepath.Join(s.store.dir, "broker-"+downloadID)
}

func selectedPaths(d download.Download) []string {
	paths := make([]string, 0, len(d.Files))
	for _, f := range d.Files {
		if f.Selected {
			paths = append(paths, f.Path)
		}
	}
	return paths
}

// HandleDownloadStarted поднимает повышенного брокера, пока пользователь ещё
// перед экраном. Отказ здесь не отменяет установку: без брокера она позже
// сама попросит права через runElevated, и пользователь увидит обычный запрос
// UAC — это единственная деградация, и она в безопасную сторону.
//
//wails:ignore
func (s *Service) HandleDownloadStarted(d download.Download) {
	if d.Origin.Purpose != download.PurposeRelease || !d.Origin.ElevateAhead {
		return
	}
	if !elevationAvailable() {
		return
	}
	if !autoInstallFor(d, s.config().AutoInstall) {
		return
	}
	if !InstallerLikely(selectedPaths(d)) {
		return
	}
	if err := s.startBroker(d); err != nil {
		slog.Warn("pre-elevation broker", "download", d.ID, "error", err)
	}
}

func (s *Service) startBroker(d download.Download) error {
	dir := s.brokerDir(d.ID)
	if dir == "" {
		return errBrokerNoDirPath
	}
	if d.Destination == "" {
		return fmt.Errorf("%w: %s", errBrokerNoContentRoot, d.ID)
	}
	gamesPath := s.config().GamesPath
	if gamesPath == "" {
		return errBrokerNoLibraryRoot
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("prepare broker dir %s: %w", dir, err)
	}
	if err := removeBrokerFile(brokerAbortPath(dir)); err != nil {
		return err
	}
	if err := removeBrokerFile(brokerSpecPath(dir)); err != nil {
		return err
	}
	pin := brokerPin{
		DownloadID:  d.ID,
		ContentRoot: d.Destination,
		LibraryRoot: gamesPath,
		StateDir:    s.store.dir,
	}
	if err := writeBrokerPin(brokerPinPath(dir), pin); err != nil {
		return err
	}
	if err := touchBrokerAlive(brokerAlivePath(dir)); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("путь к лаунчеру: %w", err)
	}
	proc, err := startElevatedWorker(runSpec{
		Path:      exe,
		Args:      []string{installBrokerFlag, dir},
		Hidden:    true,
		ID:        d.ID,
		StatePath: brokerStateAnchor(dir),
	})
	if err != nil {
		return workerStartError(exe, err)
	}

	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		askBrokerToExit(dir)
		proc.close()
		return errBrokerClosing
	}
	if s.brokers == nil {
		s.brokers = map[string]*broker{}
	}
	replaced := s.brokers[d.ID]
	b := &broker{dir: dir, proc: proc, gone: make(chan struct{})}
	s.brokers[d.ID] = b
	base, baseErr := s.baseLocked()
	if baseErr != nil {
		delete(s.brokers, d.ID)
		s.mu.Unlock()
		askBrokerToExit(dir)
		proc.close()
		return baseErr
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.tendBroker(base, d.ID, b)
	}()
	s.mu.Unlock()

	if replaced != nil {
		askBrokerToExit(replaced.dir)
	}
	slog.Info("pre-elevation broker started", "download", d.ID, "dir", dir)
	return nil
}

// tendBroker держит хартбит и снимает брокера, как только он больше не нужен.
// Мёртвый брокер помечается сразу: runElevated обязан узнать об этом до того,
// как решит отдать ему установку, иначе задание уйдёт в никуда.
func (s *Service) tendBroker(ctx context.Context, downloadID string, b *broker) {
	go func() {
		defer close(b.gone)
		// Хэндл закрывает только тот, кто его ждёт: CloseHandle под висящим
		// WaitForSingleObject — это ожидание на переиспользованном значении,
		// а ждём мы здесь часами, пока качается торрент.
		defer b.proc.close()
		if _, err := b.proc.wait(); err != nil {
			slog.Warn("wait for pre-elevation broker", "download", downloadID, "error", err)
		}
	}()

	ticker := time.NewTicker(brokerHeartbeatPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-b.gone:
			s.markBrokerDead(downloadID)
			return
		case <-ctx.Done():
			askBrokerToExit(b.dir)
			// Ждём выхода ограниченно: заклинивший процесс с правами
			// администратора не должен держать выключение лаунчера. Он всё
			// равно уйдёт сам — хартбит перестал обновляться в этот момент.
			select {
			case <-b.gone:
			case <-time.After(brokerStopWait):
				slog.Warn("pre-elevation broker did not exit in time", "download", downloadID, "dir", b.dir)
			}
			s.markBrokerDead(downloadID)
			return
		case <-ticker.C:
			if err := touchBrokerAlive(brokerAlivePath(b.dir)); err != nil {
				slog.Warn("broker heartbeat", "download", downloadID, "error", err)
			}
		}
	}
}

func (s *Service) markBrokerDead(downloadID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b := s.brokers[downloadID]; b != nil {
		b.dead = true
	}
}

// brokerFor отдаёт живого брокера этой загрузки. nil значит «повышаться
// придётся самим»: брокера не просили, он не поднялся или уже умер, и тогда
// установка идёт обычным путём с запросом UAC в свой момент.
func (s *Service) brokerFor(downloadID string) *brokerHandoff {
	if downloadID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.brokers[downloadID]
	if b == nil || b.dead {
		return nil
	}
	return &brokerHandoff{Dir: b.dir, Gone: b.gone}
}

// DropBroker снимает брокера загрузки: она установлена, отменена или удалена,
// и держать ради неё процесс с правами администратора больше не за что.
//
//wails:ignore
func (s *Service) DropBroker(downloadID string) {
	s.mu.Lock()
	b := s.brokers[downloadID]
	delete(s.brokers, downloadID)
	s.mu.Unlock()
	if b == nil {
		return
	}
	askBrokerToExit(b.dir)
}

// askBrokerToExit только выставляет метку: хэндл процесса принадлежит
// горутине ожидания в tendBroker, и закрывать его здесь нельзя.
func askBrokerToExit(dir string) {
	if err := writeWorkerFile(brokerAbortPath(dir), []byte{}); err != nil {
		slog.Warn("ask pre-elevation broker to exit", "dir", dir, "error", err)
	}
}

func removeBrokerFile(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}
