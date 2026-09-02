package install

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"typhon/internal/download"
	"typhon/internal/history"
	"typhon/internal/library"
	"typhon/internal/platform"
	"typhon/internal/settings"
	"typhon/internal/titles"
	"typhon/internal/usagestats"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	eventStarted   = "install:started"
	eventUpdated   = "install:updated"
	eventCompleted = "install:completed"
	eventFailed    = "install:failed"
	eventCancelled = "install:cancelled"
	eventRemoved   = "install:removed"

	partialSuffix    = ".partial"
	progressEpsilon  = 0.002
	rebootExitCode   = 3010
	maxSuffixAttempt = 50

	installPollInterval   = 2 * time.Second
	installerLogTailLimit = 8 << 10
	installerLogScanLimit = 256 << 10

	innoSuccessMarker = "Installation process succeeded."
)

const interruptedMessage = "установка была прервана"

var (
	errNotFound         = errors.New("установка не найдена")
	errNoDownloads      = errors.New("менеджер загрузок недоступен")
	errNotCompleted     = errors.New("загрузка ещё не завершена")
	errBusy             = errors.New("установка этой загрузки уже выполняется")
	errNoDestination    = errors.New("укажите папку установки")
	errDestNotEmpty     = errors.New("папка установки уже существует и не пуста")
	errUnknownType      = errors.New("тип пакета не распознан")
	errUnavailable      = errors.New("недоступно для этой установки")
	errExternalRuns     = errors.New("установщик запущен отдельно, дождитесь его завершения")
	errInstallerFail    = errors.New("установщик завершился с ошибкой")
	errNoExecutable     = errors.New("исполняемый файл не найден")
	errOutsideInstall   = errors.New("файл находится вне папки установки")
	errEmptyInstall     = errors.New("папка установки пуста")
	errNoLibrary        = errors.New("библиотека недоступна")
	errEmptyDestination = errors.New("каталог установки не задан")
	errNeedsUser        = errors.New("этот пакет требует участия пользователя")

	errInstallerNoOutput = errors.New("установщик не создал файлов в папке установки")

	// errInstallerNotConfirmedStopped значит, что процесс всё ещё может писать
	// в Destination: удалять каталог в этом случае нельзя (инвариант 9).
	errInstallerNotConfirmedStopped = errors.New("установщик не подтвердил остановку")
	errInstallerStillRunning        = errors.New("установка ещё идёт в фоне: установщик с правами администратора не завершился")
)

type downloadSource interface {
	Get(id string) (download.Download, error)
	DeleteData(id string) error
}

type registrar interface {
	RegisterInstalled(g library.InstalledGame) (library.Game, error)
	Find(id string) (library.Game, error)
	IsRunning(id string) bool
	RemoveGame(id string) error
	MarkUninstalled(id string) error
	CreateShortcut(id string) error
}

type job struct {
	cancel    context.CancelFunc
	cancelled bool
	external  bool
}

type Service struct {
	mu        sync.Mutex
	settings  *settings.Service
	downloads downloadSource
	library   registrar
	store     *store
	removals  *removalStore
	runner    runner

	items      []*Installation
	jobs       map[string]*job
	onFinished func(Installation)
	busy       func(gameID string) bool
	title      func(origin download.Origin) string
	usage      func(usagestats.Event)

	historyRecorder func(history.Record) error

	roots     []string
	freeSpace func(string) (platform.StorageInfo, error)

	ctx     context.Context
	cancel  context.CancelFunc
	closing bool
	wg      sync.WaitGroup
}

func NewService(settingsService *settings.Service, downloads *download.Manager, lib *library.Service) (*Service, error) {
	dir, err := settings.ConfigDir()
	if err != nil {
		return nil, fmt.Errorf("resolve config dir: %w", err)
	}
	s, err := newServiceAt(dir, settingsService)
	if err != nil {
		return nil, err
	}
	if downloads != nil {
		s.downloads = downloads
	}
	if lib != nil {
		s.library = lib
	}
	return s, nil
}

func newServiceAt(dir string, settingsService *settings.Service) (*Service, error) {
	if dir == "" {
		return nil, errors.New("installations path unavailable")
	}
	s := &Service{
		settings:  settingsService,
		store:     newStore(dir),
		removals:  newRemovalStore(dir),
		jobs:      map[string]*job{},
		freeSpace: platform.GetStorageInfo,
	}
	s.runner = newRunner(func() string { return s.config().GamesPath })
	return s, nil
}

//wails:ignore
func (s *Service) SetBusyCheck(fn func(gameID string) bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.busy = fn
}

//wails:ignore
func (s *Service) SetTitleResolver(fn func(origin download.Origin) string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.title = fn
}

//wails:ignore
func (s *Service) SetUsageRecorder(rec func(usagestats.Event)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.usage = rec
}

func (s *Service) recordUsage(ev usagestats.Event) {
	s.mu.Lock()
	rec := s.usage
	s.mu.Unlock()
	if rec == nil {
		return
	}
	rec(ev)
}

// installerType переводит внутренний Type в строку, ожидаемую usagestats:
// будущий Type, добавленный в model.go, обязан свестись к "unknown", а не
// молча провалить валидацию всего события в usagestats.
func installerType(t Type) string {
	switch t {
	case TypePortable, TypeArchiveZip, TypeArchive7z, TypeArchiveRar, TypeExeInstaller, TypeMsiInstaller:
		return string(t)
	default:
		return "unknown"
	}
}

func usageDurationSeconds(d time.Duration) int64 {
	if d < 0 {
		return 0
	}
	return int64(d.Seconds())
}

func (s *Service) titleOf(origin download.Origin) string {
	s.mu.Lock()
	resolve := s.title
	s.mu.Unlock()
	if resolve == nil {
		return ""
	}
	return strings.TrimSpace(resolve(origin))
}

func (s *Service) nameFor(d download.Download) string {
	if title := s.titleOf(d.Origin); title != "" {
		return title
	}
	return gameTitle(d.Name)
}

func (s *Service) config() settings.Settings {
	if s.settings == nil {
		return settings.Defaults()
	}
	return s.settings.GetSettings()
}

func emit(name string, data any) {
	if app := application.Get(); app != nil {
		app.Event.Emit(name, data)
	}
}

func (s *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.mu.Lock()
	s.ctx, s.cancel = context.WithCancel(ctx)
	stale := make([]string, 0, 4)
	stored, err := s.store.load()
	if err != nil {
		cancel := s.cancel
		s.cancel = nil
		s.mu.Unlock()
		cancel()
		return err
	}
	resume := make([]string, 0, 2)
	for _, rec := range stored {
		item := rec
		if transient(item.Status) {
			alive, done := s.transientWorkerStatus(item.ID)
			if alive || done {
				resume = append(resume, item.ID)
				slog.Info("installation worker still running, resuming", "id", item.ID, "name", item.Name)
			} else {
				item.Status = StatusInterrupted
				item.Error = interruptedMessage
				slog.Info("installation interrupted", "id", item.ID, "name", item.Name)
			}
		}
		if item.Destination != "" {
			stale = append(stale, item.Destination+partialSuffix)
		}
		s.items = append(s.items, &item)
	}
	s.persistLocked()
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		sweepPartial(stale)
		s.sweepRemovals()
	}()

	for _, id := range resume {
		//nolint:contextcheck // ctx унаследован от s.ctx (жизненный цикл сервиса, инварианты 19-20) через baseContext(), тот же приём, что spawnLocked уже использует для job-контекстов; contextcheck не видит связь через поле структуры
		s.spawnResumeWatcher(s.baseContext(), id)
	}
	return nil
}

// transientWorkerStatus решает судьбу записи, пережившей перезапуск лаунчера:
// os.FindProcess на Windows всегда "успешен" вне зависимости от того, жив ли
// процесс, поэтому единственный источник правды — файл состояния воркера
// (тот же, что пишет и читает runElevated) и workerProcessAlive по
// записанному в нём PID. Done=true значит, что воркер уже дописал итог
// (успех/провал/отмену) в файл до того, как лаунчер успел его прочитать —
// такую запись нельзя считать "мёртвой без результата" и подменять
// StatusInterrupted: итог должен дойти до записи так же, как если бы лаунчер
// был жив всё это время (finishResumed разбирает Done по существу). Ошибка
// чтения или проверки не превращается в "жив" по умолчанию — только
// подтверждённая жизнь или подтверждённый Done оставляют запись в работе.
func (s *Service) transientWorkerStatus(id string) (alive, done bool) {
	statePath := s.workerStatePath(id)
	state, found, err := readWorkerState(statePath)
	if err != nil {
		slog.Error("read worker state on startup", "id", id, "error", err)
		return false, false
	}
	if !found {
		return false, false
	}
	if state.Done {
		return false, true
	}
	alive, err = workerProcessAlive(state.PID)
	if err != nil {
		slog.Error("check worker process on startup", "id", id, "pid", state.PID, "error", err)
		return false, false
	}
	return alive, false
}

// var, не const: тесты укорачивают интервал, чтобы не ждать боевые тайминги.
var resumeWatchPollInterval = 2 * time.Second

// spawnResumeWatcher принимает ctx от жизни сервиса: s.cancel() при
// ServiceShutdown останавливает и наблюдателя, без чего s.wg.Wait() ждал бы
// его вечно (инвариант 19).
func (s *Service) spawnResumeWatcher(ctx context.Context, id string) {
	statePath := s.workerStatePath(id)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.watchResumedInstall(ctx, id, statePath)
	}()
}

// watchResumedInstall доводит до конца установку, чей воркер пережил
// перезапуск лаунчера: снимков before/beforeEntries/shell, взятых до старта
// установщика, восстановить неоткуда, поэтому этот путь не пытается их
// подделать — только ждёт Done и передаёт результат в finishResumed.
func (s *Service) watchResumedInstall(ctx context.Context, id, statePath string) {
	ticker := time.NewTicker(resumeWatchPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			state, found, err := readWorkerState(statePath)
			if err != nil {
				slog.Error("read worker state while resuming install", "id", id, "error", err)
				s.interruptResumed(id)
				return
			}
			if found && state.Done {
				s.finishResumed(ctx, id, state)
				return
			}
			if !found {
				s.interruptResumed(id)
				return
			}
			alive, err := workerProcessAlive(state.PID)
			if err != nil {
				slog.Error("check worker process while resuming install", "id", id, "pid", state.PID, "error", err)
				s.interruptResumed(id)
				return
			}
			if !alive {
				s.interruptResumed(id)
				return
			}
		}
	}
}

func (s *Service) interruptResumed(id string) {
	s.mu.Lock()
	item := s.findLocked(id)
	if item == nil || !active(item.Status) {
		s.mu.Unlock()
		return
	}
	item.Status = StatusInterrupted
	item.Error = interruptedMessage
	s.persistLocked()
	snap := snapshotOf(item)
	s.mu.Unlock()
	slog.Info("resumed installation interrupted", "id", id, "name", snap.Name)
	emit(eventUpdated, snap)
}

// finishResumed завершает установку по итогу воркера, обнаруженного живым на
// старте: происхождение каталога и запись деинсталлятора мы честно не
// знаем (их выясняет setRemoval по снимку до установки, а снимка нет),
// поэтому Owned=false и UninstallUnknown=true — явный неизвестный статус, а
// не унаследованный от нулевого значения. Уборка ярлыков здесь не
// выполняется: baseline ярлыков снимался до старта установщика и тоже не
// восстановим.
func (s *Service) finishResumed(ctx context.Context, id string, state workerState) {
	dropInstallerLog(s.installerLogPath(id))
	if state.Cancelled {
		// Cancel записал маркер и оставил статус рабочим именно ради этого
		// момента: воркер подтвердил отмену через Cancelled, а не через
		// текст ошибки, и запись должна дойти до "отменено", а не до "провал".
		s.cancelResumed(id)
		return
	}
	if state.Error != "" {
		slog.Warn("resumed installer worker failed", "id", id, "error", state.Error)
		s.fail(id, errors.New(state.Error))
		return
	}
	slog.Warn("installation resumed after launcher restart, uninstall origin unknown", "id", id)
	s.markResumedOwnership(id)
	if err := s.finalize(ctx, id); err != nil {
		s.fail(id, err)
	}
}

func (s *Service) cancelResumed(id string) {
	s.mu.Lock()
	item := s.findLocked(id)
	if item == nil {
		s.mu.Unlock()
		return
	}
	partial := partialPath(item)
	s.markCancelledLocked(item)
	snap := snapshotOf(item)
	s.mu.Unlock()
	go sweepPartial([]string{partial})
	s.notifyFinished(snap)
}

func (s *Service) markResumedOwnership(id string) {
	s.mu.Lock()
	item := s.findLocked(id)
	if item == nil {
		s.mu.Unlock()
		return
	}
	item.Owned = false
	item.UninstallUnknown = true
	item.Uninstall = library.Uninstall{}
	s.persistLocked()
	s.mu.Unlock()
}

func (s *Service) ServiceShutdown() error {
	s.mu.Lock()
	s.closing = true
	cancel := s.cancel
	for _, j := range s.jobs {
		j.cancel()
	}
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	s.wg.Wait()

	s.mu.Lock()
	s.persistLocked()
	s.mu.Unlock()
	return nil
}

func sweepPartial(paths []string) {
	for _, path := range paths {
		if !strings.HasSuffix(path, partialSuffix) {
			continue
		}
		if _, err := os.Stat(path); err != nil {
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			slog.Warn("remove partial install", "path", path, "error", err)
		} else {
			slog.Info("removed partial install", "path", path)
		}
	}
}

func (s *Service) findLocked(id string) *Installation {
	for _, item := range s.items {
		if item.ID == id {
			return item
		}
	}
	return nil
}

func (s *Service) persistLocked() {
	if err := s.persistNowLocked(); err != nil {
		slog.Error("persist installations", "error", err)
	}
}

func (s *Service) persistNowLocked() error {
	items := make([]Installation, 0, len(s.items))
	for _, item := range s.items {
		items = append(items, snapshotOf(item))
	}
	return s.store.save(items)
}

func (s *Service) snapshot(id string) (Installation, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.findLocked(id)
	if item == nil {
		return Installation{}, false
	}
	return snapshotOf(item), true
}

func (s *Service) isClosing() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closing
}

func (s *Service) List() []Installation {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Installation, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, snapshotOf(item))
	}
	return out
}

func (s *Service) InspectDownload(downloadID string) (PlanInfo, error) {
	d, err := s.completedDownload(downloadID)
	if err != nil {
		return PlanInfo{}, err
	}
	plan, err := Inspect(s.baseContext(), sourceDir(d))
	if err != nil {
		return PlanInfo{}, err
	}
	cfg := s.config()
	name := s.nameFor(d)
	if ownDestination(plan.Type, plan.Silent) || plan.Type == TypeUnknown {
		plan.Destination = s.proposeDestination(cfg.GamesPath, name)
	}
	free, err := s.freeBytes(volumeTarget(plan.Destination, cfg.GamesPath))
	if err != nil {
		return PlanInfo{}, err
	}
	return PlanInfo{
		Plan:          plan,
		DownloadID:    d.ID,
		Name:          name,
		RequiredBytes: requiredBytes(plan),
		FreeBytes:     free,
		Seeding:       d.Seeding,
	}, nil
}

func (s *Service) Start(downloadID string, opts StartOptions) (Installation, error) {
	d, err := s.completedDownload(downloadID)
	if err != nil {
		return Installation{}, err
	}

	s.mu.Lock()
	for _, item := range s.items {
		if item.DownloadID == downloadID && active(item.Status) {
			s.mu.Unlock()
			return Installation{}, errBusy
		}
	}
	s.mu.Unlock()

	plan, err := Inspect(s.baseContext(), sourceDir(d))
	if err != nil {
		return Installation{}, err
	}
	if opts.Type != "" {
		plan.Type = opts.Type
	}
	manual := false
	if installer := strings.TrimSpace(opts.InstallerPath); installer != "" && !samePath(installer, plan.InstallerPath) {
		info, err := os.Stat(installer)
		if err != nil || info.IsDir() {
			return Installation{}, errNoExecutable
		}
		kind := plan.Type
		if !external(kind) {
			kind = TypeExeInstaller
		}
		if err := fillInstallerPlan(&plan, kind, installer); err != nil {
			return Installation{}, err
		}
		manual = true
	}
	if plan.Type == TypeUnknown {
		return Installation{}, errUnknownType
	}
	if external(plan.Type) && plan.InstallerPath == "" {
		return Installation{}, errNoExecutable
	}

	item := &Installation{
		ID:              newID(),
		DownloadID:      d.ID,
		Name:            s.nameFor(d),
		Type:            plan.Type,
		Status:          StatusPending,
		SourcePath:      plan.SourcePath,
		ContentRoot:     plan.ContentRoot,
		InstallerPath:   plan.InstallerPath,
		ExtraInstallers: plan.ExtraInstallers,
		ManualInstaller: manual,
		WorkingDir:      plan.WorkingDir,
		Engine:          plan.Engine,
		Silent:          plan.Silent,
		ArchivePath:     plan.ArchivePath,
		BytesTotal:      plan.EstimatedSize,
		Origin:          d.Origin,
		Unattended:      opts.Unattended,
		SkipRegister:    opts.SkipRegister,
		StartedAt:       time.Now(),
	}

	if ownDestination(plan.Type, plan.Silent) {
		dest := strings.TrimSpace(opts.Destination)
		if dest == "" {
			return Installation{}, errNoDestination
		}
		if !filepath.IsAbs(dest) {
			return Installation{}, errRelativeDestination
		}
		if !destAvailable(dest) {
			return Installation{}, errDestNotEmpty
		}
		item.Destination = filepath.Clean(dest)
		item.Owned = controlled(plan.Type)
		if err := s.checkSpace(item.Destination, requiredBytes(plan)); err != nil {
			return Installation{}, err
		}
	}
	if plan.Type == TypePortable {
		item.Mode = installMode(opts.Mode, d.Seeding)
	}

	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		return Installation{}, errUnavailable
	}
	s.items = append(s.items, item)
	s.persistLocked()
	snap := snapshotOf(item)
	s.spawnLocked(item.ID)
	s.mu.Unlock()

	slog.Info("install started", "id", item.ID, "name", item.Name, "type", item.Type, "mode", item.Mode)
	emit(eventStarted, snap)
	s.recordUsage(usagestats.Event{
		Type:      usagestats.TypeInstallStarted,
		Timestamp: time.Now(),
		Properties: usagestats.Properties{
			GameID:        snap.Origin.GameID,
			InstallerType: installerType(snap.Type),
		},
	})
	emit(eventUpdated, snap)
	return snap, nil
}

func (s *Service) Cancel(id string) error {
	s.mu.Lock()
	item := s.findLocked(id)
	if item == nil {
		s.mu.Unlock()
		return errNotFound
	}
	if !active(item.Status) {
		s.mu.Unlock()
		return errUnavailable
	}
	j := s.jobs[id]
	if j != nil && j.external {
		s.mu.Unlock()
		return errExternalRuns
	}
	if j != nil {
		j.cancelled = true
		j.cancel()
		s.mu.Unlock()
		return nil
	}
	// Нет job — либо запись подхвачена после перезапуска (воркер жив, но
	// s.jobs для неё никогда не заводился), либо воркер уже мёртв. Проверка и
	// решение — под тем же захватом, что статус (инвариант 17): без этого UI
	// сказал бы "отменено" над установщиком, который продолжает писать.
	if alive, _ := s.transientWorkerStatus(id); alive {
		err := writeWorkerCancel(s.workerCancelPath(id))
		s.mu.Unlock()
		return err
	}

	partial := partialPath(item)
	s.markCancelledLocked(item)
	snap := snapshotOf(item)
	s.mu.Unlock()
	go sweepPartial([]string{partial})
	s.notifyFinished(snap)
	return nil
}

func (s *Service) ConfirmExecutable(id, executable string) error {
	executable = strings.TrimSpace(executable)
	s.mu.Lock()
	item := s.findLocked(id)
	if item == nil {
		s.mu.Unlock()
		return errNotFound
	}
	if item.Status != StatusWaitingForUser {
		s.mu.Unlock()
		return errUnavailable
	}
	destination := item.Destination
	kind := item.Type
	s.mu.Unlock()

	if err := validExecutable(executable, destination, kind); err != nil {
		return err
	}

	s.mu.Lock()
	item = s.findLocked(id)
	if item == nil {
		s.mu.Unlock()
		return errNotFound
	}
	item.Executable = executable
	if item.Destination == "" {
		item.Destination = filepath.Dir(executable)
	}
	s.persistLocked()
	s.mu.Unlock()

	if err := s.complete(id); err != nil {
		s.fail(id, err)
		return err
	}
	return nil
}

func (s *Service) Retry(id string) error {
	s.mu.Lock()
	item := s.findLocked(id)
	if item == nil {
		s.mu.Unlock()
		return errNotFound
	}
	if !retryable(item.Status) || s.jobs[id] != nil || s.closing {
		s.mu.Unlock()
		return errUnavailable
	}
	// Проверка и решение — под одним и тем же захватом (инвариант 17): статус
	// мог стать retryable по устаревшему прочтению (воркер, о котором
	// ServiceStartup не успел узнать, ещё пишет по тем же детерминированным
	// путям state/spec/cancel), и вызвать retry поверх него — поднять второго
	// воркера на то же место.
	if alive, _ := s.transientWorkerStatus(id); alive {
		s.mu.Unlock()
		return errInstallerStillRunning
	}
	downloadID := item.DownloadID
	kind := item.Type
	installer := item.InstallerPath
	manual := item.ManualInstaller
	destination := item.Destination
	title := item.Name
	s.mu.Unlock()

	d, err := s.completedDownload(downloadID)
	if err != nil {
		return err
	}
	plan, err := Inspect(s.baseContext(), sourceDir(d))
	if err != nil {
		return err
	}

	// Установщик, выбранный пользователем вручную, переживает повтор вместе со
	// своим движком; автоматический выбор берётся из свежего плана, иначе запись
	// прошлой версии лаунчера навсегда останется с неверным файлом.
	engine := plan.Engine
	extras := plan.ExtraInstallers
	if external(kind) && manual && installer != "" && !samePath(installer, plan.InstallerPath) {
		engine, err = DetectEngine(installer)
		if err != nil {
			return err
		}
		extras = nil
	} else if external(kind) {
		installer = plan.InstallerPath
	}
	if external(kind) && installer == "" {
		return errNoExecutable
	}
	if external(kind) && supportsSilent(engine) && destination == "" {
		destination = s.proposeDestination(s.config().GamesPath, title)
		if destination == "" {
			return errNoDestination
		}
	}

	s.mu.Lock()
	item = s.findLocked(id)
	if item == nil {
		s.mu.Unlock()
		return errNotFound
	}
	partial := partialPath(item)
	item.Status = StatusPending
	item.Error = ""
	item.Progress = 0
	item.BytesDone = 0
	item.CurrentFile = ""
	item.Executable = ""
	item.Candidates = nil
	item.CompletedAt = nil
	item.SourcePath = plan.SourcePath
	if controlled(item.Type) {
		item.ContentRoot = plan.ContentRoot
		item.ArchivePath = plan.ArchivePath
		item.BytesTotal = plan.EstimatedSize
		item.Mode = installMode(item.Mode, d.Seeding)
	}
	if external(item.Type) {
		item.InstallerPath = installer
		item.ExtraInstallers = extras
		item.WorkingDir = filepath.Dir(installer)
		item.Engine = engine
		item.Silent = supportsSilent(engine)
		item.BytesTotal = plan.EstimatedSize
		if item.Silent {
			item.Destination = destination
		}
	}
	s.persistLocked()
	snap := snapshotOf(item)
	s.mu.Unlock()

	sweepPartial([]string{partial})

	s.mu.Lock()
	s.spawnLocked(id)
	s.mu.Unlock()

	slog.Info("install retried", "id", id, "name", snap.Name)
	emit(eventUpdated, snap)
	return nil
}

func (s *Service) Dismiss(id string) error {
	s.mu.Lock()
	item := s.findLocked(id)
	if item == nil {
		s.mu.Unlock()
		return errNotFound
	}
	if active(item.Status) || s.jobs[id] != nil {
		s.mu.Unlock()
		return errUnavailable
	}
	partial := partialPath(item)
	for i, existing := range s.items {
		if existing.ID == id {
			s.items = append(s.items[:i], s.items[i+1:]...)
			break
		}
	}
	s.persistLocked()
	s.mu.Unlock()

	go sweepPartial([]string{partial})
	slog.Info("install dismissed", "id", id)
	emit(eventRemoved, RemovedEvent{ID: id})
	return nil
}

func (s *Service) DeleteDownloadData(downloadID string) error {
	if s.downloads == nil {
		return errNoDownloads
	}
	return s.downloads.DeleteData(downloadID)
}

//wails:ignore
func (s *Service) SetOnFinished(fn func(Installation)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onFinished = fn
}

func (s *Service) notifyFinished(item Installation) {
	s.mu.Lock()
	notify := s.onFinished
	s.mu.Unlock()
	if notify != nil {
		go notify(item)
	}
}

//wails:ignore
func (s *Service) HandleDownloadCompleted(d download.Download) {
	if d.Origin.Purpose != download.PurposeRelease {
		return
	}
	if !s.config().AutoInstall {
		return
	}
	info, err := s.InspectDownload(d.ID)
	if err != nil {
		slog.Warn("auto install inspect", "id", d.ID, "error", err)
		return
	}
	if !info.Plan.CanAutoInstall || !ownDestination(info.Plan.Type, info.Plan.Silent) {
		slog.Info("auto install skipped", "id", d.ID, "type", info.Plan.Type)
		return
	}
	if info.RequiredBytes > 0 && info.FreeBytes > 0 && info.FreeBytes < info.RequiredBytes {
		slog.Warn("auto install skipped, not enough space", "id", d.ID,
			"required", info.RequiredBytes, "free", info.FreeBytes)
		return
	}
	item, err := s.Start(d.ID, StartOptions{Destination: info.Plan.Destination, Type: info.Plan.Type})
	if err != nil {
		slog.Warn("auto install", "id", d.ID, "error", err)
		return
	}
	slog.Info("auto install started", "id", item.ID, "download", d.ID, "name", item.Name)
}

func (s *Service) completedDownload(id string) (download.Download, error) {
	if s.downloads == nil {
		return download.Download{}, errNoDownloads
	}
	d, err := s.downloads.Get(id)
	if err != nil {
		return download.Download{}, err
	}
	if d.Status != download.StatusCompleted {
		return download.Download{}, errNotCompleted
	}
	return d, nil
}

// baseLocked отдаёт контекст жизни сервиса; до ServiceStartup (и в тестах,
// которые его не вызывают) сервис живёт столько же, сколько процесс.
func (s *Service) baseLocked() context.Context {
	if s.ctx == nil {
		return context.Background()
	}
	return s.ctx
}

func (s *Service) baseContext() context.Context {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.baseLocked()
}

func (s *Service) spawnLocked(id string) {
	ctx, cancel := context.WithCancel(s.baseLocked())
	s.jobs[id] = &job{cancel: cancel}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer s.endJob(id)
		s.run(ctx, id)
	}()
}

func (s *Service) endJob(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j := s.jobs[id]; j != nil {
		j.cancel()
		delete(s.jobs, id)
	}
}

func (s *Service) setExternal(id string, running bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j := s.jobs[id]; j != nil {
		j.external = running
	}
}

func (s *Service) setStatus(id string, status Status) {
	s.mu.Lock()
	item := s.findLocked(id)
	if item == nil || item.Status == status {
		s.mu.Unlock()
		return
	}
	item.Status = status
	s.persistLocked()
	snap := snapshotOf(item)
	s.mu.Unlock()
	emit(eventUpdated, snap)
}

func (s *Service) updateProgress(id string, p Progress) {
	s.mu.Lock()
	item := s.findLocked(id)
	if item == nil {
		s.mu.Unlock()
		return
	}
	next := ratio(p.BytesDone, p.BytesTotal)
	moved := math.Abs(next-item.Progress) >= progressEpsilon ||
		p.CurrentFile != item.CurrentFile ||
		(p.BytesTotal <= 0 && p.BytesDone != item.BytesDone)
	if !moved {
		s.mu.Unlock()
		return
	}
	item.Progress = next
	item.BytesDone = p.BytesDone
	if p.BytesTotal > 0 {
		item.BytesTotal = p.BytesTotal
	}
	item.CurrentFile = p.CurrentFile
	snap := snapshotOf(item)
	s.mu.Unlock()
	emit(eventUpdated, snap)
}

func (s *Service) markCancelledLocked(item *Installation) {
	item.Status = StatusCancelled
	item.Error = ""
	item.Progress = 0
	item.CurrentFile = ""
	s.persistLocked()
	snap := snapshotOf(item)
	slog.Info("install cancelled", "id", item.ID, "name", item.Name)
	emit(eventCancelled, snap)
	emit(eventUpdated, snap)
}

func (s *Service) fail(id string, cause error) {
	s.mu.Lock()
	item := s.findLocked(id)
	if item == nil {
		s.mu.Unlock()
		return
	}
	j := s.jobs[id]
	// errInstallerNotConfirmedStopped переживает и j.cancelled, и s.closing:
	// установщик мог не остановиться именно потому, что пользователь отменил
	// или лаунчер закрывается, и в обоих случаях запись обязана остаться в
	// состоянии "каталог занят", а не превратиться в чистое "отменено, ничего
	// нет" — это и есть тот дефолт, который прячет живой процесс от UI.
	switch {
	case errors.Is(cause, errInstallerNotConfirmedStopped):
		item.Status = StatusFailed
		item.Error = cause.Error()
		s.persistLocked()
		snap := snapshotOf(item)
		s.mu.Unlock()
		slog.Error("install failed, installer still running", "id", id, "name", snap.Name, "error", cause)
		emit(eventFailed, snap)
		s.recordUsage(usagestats.Event{
			Type:      usagestats.TypeInstallFailed,
			Timestamp: time.Now(),
			Properties: usagestats.Properties{
				GameID:          snap.Origin.GameID,
				InstallerType:   installerType(snap.Type),
				DurationSeconds: usageDurationSeconds(time.Since(snap.StartedAt)),
				ErrorCode:       usagestats.Classify(cause),
			},
		})
		emit(eventUpdated, snap)
		s.notifyFinished(snap)
		s.recordInstallFailure(id, snap, cause)
	case j != nil && j.cancelled:
		// пользовательская отмена — не провал, событие install_failed не шлём
		s.markCancelledLocked(item)
		snap := snapshotOf(item)
		s.mu.Unlock()
		s.notifyFinished(snap)
	case s.closing || errors.Is(cause, context.Canceled):
		// лаунчер закрывается — событие всё равно не успеет улететь
		s.persistLocked()
		s.mu.Unlock()
		slog.Info("install interrupted", "id", id, "name", item.Name)
	default:
		item.Status = StatusFailed
		item.Error = cause.Error()
		s.persistLocked()
		snap := snapshotOf(item)
		s.mu.Unlock()
		slog.Error("install failed", "id", id, "name", snap.Name, "error", cause)
		emit(eventFailed, snap)
		s.recordUsage(usagestats.Event{
			Type:      usagestats.TypeInstallFailed,
			Timestamp: time.Now(),
			Properties: usagestats.Properties{
				GameID:          snap.Origin.GameID,
				InstallerType:   installerType(snap.Type),
				DurationSeconds: usageDurationSeconds(time.Since(snap.StartedAt)),
				ErrorCode:       usagestats.Classify(cause),
			},
		})
		emit(eventUpdated, snap)
		s.notifyFinished(snap)
		s.recordInstallFailure(id, snap, cause)
	}
}

// recordInstallFailure writes the terminal install_failed entry. fail() runs
// at the end of a background job (or of ConfirmExecutable) with no caller
// left to propagate a journal error to; Record already flipped history into
// Degraded and emitted history:degraded, so the user learns about the
// journal problem from the banner instead of a failed install turning into
// a failed-twice report.
func (s *Service) recordInstallFailure(id string, snap Installation, cause error) {
	if s.historyRecorder == nil {
		return
	}
	if err := s.historyRecorder(history.Record{
		Kind:   history.KindInstallFailed,
		GameID: snap.Origin.GameID,
		Title:  snap.Name,
		Detail: cause.Error(),
		RefID:  id,
	}); err != nil {
		slog.Error("record install history", "id", id, "error", err)
	}
}

func (s *Service) waitForUser(id string, candidates []Candidate) {
	s.mu.Lock()
	item := s.findLocked(id)
	if item == nil {
		s.mu.Unlock()
		return
	}
	item.Status = StatusWaitingForUser
	item.Candidates = candidates
	if len(candidates) > 0 && item.Executable == "" {
		item.Executable = candidates[0].Path
	}
	s.persistLocked()
	snap := snapshotOf(item)
	s.mu.Unlock()
	slog.Info("install waiting for user", "id", id, "candidates", len(candidates))
	emit(eventUpdated, snap)
}

func (s *Service) proposeDestination(gamesPath, name string) string {
	if gamesPath == "" {
		return ""
	}
	base := filepath.Join(gamesPath, sanitizeName(name))
	if destAvailable(base) {
		return base
	}
	for i := 2; i < maxSuffixAttempt; i++ {
		candidate := fmt.Sprintf("%s (%d)", base, i)
		if destAvailable(candidate) {
			return candidate
		}
	}
	return base
}

func (s *Service) freeBytes(path string) (int64, error) {
	if path == "" {
		return 0, errEmptyDestination
	}
	info, err := s.freeSpace(path)
	if err != nil {
		return 0, fmt.Errorf("свободное место %s: %w", path, err)
	}
	if info.FreeBytes > math.MaxInt64 {
		return math.MaxInt64, nil
	}
	return int64(info.FreeBytes), nil
}

func (s *Service) checkSpace(destination string, need int64) error {
	if need <= 0 {
		return nil
	}
	free, err := s.freeBytes(destination)
	if err != nil {
		return err
	}
	if free >= need {
		return nil
	}
	return fmt.Errorf("недостаточно места на диске: нужно %s, свободно %s", humanSize(need), humanSize(free))
}

func (s *Service) installRoots() []string {
	if len(s.roots) > 0 {
		return append([]string(nil), s.roots...)
	}
	paths := []string{s.config().GamesPath}
	for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)", "ProgramW6432"} {
		paths = append(paths, os.Getenv(env))
	}
	out := make([]string, 0, len(paths))
	seen := map[string]bool{}
	for _, path := range paths {
		if path == "" {
			continue
		}
		key := strings.ToLower(filepath.Clean(path))
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, filepath.Clean(path))
	}
	return out
}

func sourceDir(d download.Download) string {
	nested := filepath.Join(d.Destination, d.Name)
	if info, err := os.Stat(nested); err == nil && info.IsDir() {
		return nested
	}
	return d.Destination
}

func partialPath(item *Installation) string {
	if item.Destination == "" || !controlled(item.Type) {
		return ""
	}
	return item.Destination + partialSuffix
}

func installMode(mode string, seeding bool) string {
	if seeding {
		return ModeCopy
	}
	if mode == ModeMove {
		return ModeMove
	}
	return ModeCopy
}

func volumeTarget(destination, fallback string) string {
	if destination != "" {
		return destination
	}
	return fallback
}

func requiredBytes(p Plan) int64 {
	size := p.EstimatedSize
	if size <= 0 && p.CompressedSize > 0 {
		size = p.CompressedSize * 3
	}
	if size <= 0 {
		return 0
	}
	return size + size/20
}

func validExecutable(path, destination string, kind Type) error {
	if path == "" {
		return errNoExecutable
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return errNoExecutable
	}
	dir := filepath.Dir(path)
	if dir == filepath.VolumeName(dir)+string(filepath.Separator) {
		return errOutsideInstall
	}
	if controlled(kind) && destination != "" && !inside(destination, path) {
		return errOutsideInstall
	}
	return nil
}

func gameTitle(raw string) string {
	if base := titles.Parse(raw).Base; base != "" {
		return base
	}
	return strings.TrimSpace(raw)
}

func sanitizeName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r < 0x20, strings.ContainsRune(`<>:"/\|?*`, r):
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	cleaned := strings.Join(strings.Fields(b.String()), " ")
	cleaned = strings.TrimRight(cleaned, ". ")
	if cleaned == "" {
		return "Game"
	}
	return cleaned
}

func humanSize(bytes int64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1f ГБ", float64(bytes)/(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f МБ", float64(bytes)/(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1f КБ", float64(bytes)/(1<<10))
	default:
		return fmt.Sprintf("%d Б", bytes)
	}
}

func newID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("i%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}
