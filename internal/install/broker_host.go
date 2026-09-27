package install

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
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

	// askBrokerToExit пробует записать маркер выхода в течение
	// brokerAskRetryWindow, повторяя каждые brokerAskRetryInterval: одиночный
	// транзиентный отказ (антивирус держит хэндл, EACCES) не должен навсегда
	// оставить брокера с правами администратора не узнавшим, что его просили
	// выйти.
	brokerAskRetryInterval = 200 * time.Millisecond
	brokerAskRetryWindow   = 3 * time.Second
)

type broker struct {
	key  ed25519.PrivateKey
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
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	proc, err := startElevatedWorker(runSpec{
		Path:      exe,
		Args:      []string{installBrokerFlag, dir, base64.StdEncoding.EncodeToString(public)},
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
		if err := askBrokerToExit(dir); err != nil {
			slog.Warn("pre-elevation broker exit request never got through", "download", d.ID, "dir", dir, "error", err)
		}
		proc.close()
		return errBrokerClosing
	}
	if s.brokers == nil {
		s.brokers = map[string]*broker{}
	}
	replaced := s.brokers[d.ID]
	b := &broker{key: private, dir: dir, proc: proc, gone: make(chan struct{})}
	s.brokers[d.ID] = b
	base, baseErr := s.baseLocked()
	if baseErr != nil {
		delete(s.brokers, d.ID)
		s.mu.Unlock()
		if err := askBrokerToExit(dir); err != nil {
			slog.Warn("pre-elevation broker exit request never got through", "download", d.ID, "dir", dir, "error", err)
		}
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
		if err := askBrokerToExit(replaced.dir); err != nil {
			slog.Warn("pre-elevation broker exit request never got through", "download", d.ID, "dir", replaced.dir, "error", err)
		}
	}
	slog.Info("pre-elevation broker started", "download", d.ID, "dir", dir)
	return nil
}

// tendBroker держит хартбит и снимает брокера, как только он больше не нужен.
// Мёртвый брокер помечается сразу: runElevated обязан узнать об этом до того,
// как решит отдать ему установку, иначе задание уйдёт в никуда.
func (s *Service) tendBroker(ctx context.Context, downloadID string, b *broker) {
	// Учтена в том же s.wg, что и сама tendBroker: без этого внешняя
	// горутина снимала свой wg.Done() по таймауту brokerStopWait независимо
	// от того, вышел ли настоящий процесс, и эта горутина оставалась висеть
	// на wait() невидимой для ServiceShutdown (инвариант 19).
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
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
			if err := askBrokerToExit(b.dir); err != nil {
				slog.Warn("pre-elevation broker exit request never got through", "download", downloadID, "dir", b.dir, "error", err)
			}
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
	return &brokerHandoff{Dir: b.dir, Gone: b.gone, Key: b.key}
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
	if err := askBrokerToExit(b.dir); err != nil {
		slog.Warn("pre-elevation broker exit request never got through", "download", downloadID, "dir", b.dir, "error", err)
	}
}

// askBrokerToExit только выставляет метку: хэндл процесса принадлежит
// горутине ожидания в tendBroker, и закрывать его здесь нельзя. Раньше
// единственная неудачная запись (антивирус держит хэндл, EACCES) молча
// считалась успехом всеми четырьмя вызывающими в этом файле — брокер с
// правами администратора никогда не узнавал, что его просили выйти, и жил
// до brokerMaxLifetime (12 часов). Теперь запись повторяется, пока есть
// время (brokerAskRetryWindow), и вызывающий может узнать об окончательной
// неудаче через возвращённую ошибку.
func askBrokerToExit(dir string) error {
	deadline := time.Now().Add(brokerAskRetryWindow)
	for {
		err := writeWorkerFile(brokerAbortPath(dir), []byte{})
		if err == nil {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("ask pre-elevation broker to exit %s: %w", dir, err)
		}
		slog.Warn("ask pre-elevation broker to exit, retrying", "dir", dir, "error", err)
		<-time.After(brokerAskRetryInterval)
	}
}

func removeBrokerFile(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}
