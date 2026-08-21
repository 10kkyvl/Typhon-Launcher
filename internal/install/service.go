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
	"typhon/internal/library"
	"typhon/internal/platform"
	"typhon/internal/settings"

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
)

type downloadSource interface {
	Get(id string) (download.Download, error)
	DeleteData(id string) error
}

type registrar interface {
	RegisterInstalled(g library.InstalledGame) (library.Game, error)
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
	runner    runner

	items      []*Installation
	jobs       map[string]*job
	onFinished func(Installation)

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
	return &Service{
		settings:  settingsService,
		store:     newStore(dir),
		runner:    newRunner(),
		jobs:      map[string]*job{},
		freeSpace: platform.GetStorageInfo,
	}, nil
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
	for _, rec := range stored {
		item := rec
		if transient(item.Status) {
			item.Status = StatusInterrupted
			item.Error = interruptedMessage
			slog.Info("installation interrupted", "id", item.ID, "name", item.Name)
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
	}()
	return nil
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
	items := make([]Installation, 0, len(s.items))
	for _, item := range s.items {
		items = append(items, snapshotOf(item))
	}
	if err := s.store.save(items); err != nil {
		slog.Error("persist installations", "error", err)
	}
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
	plan, err := Inspect(sourceDir(d))
	if err != nil {
		return PlanInfo{}, err
	}
	cfg := s.config()
	if controlled(plan.Type) || plan.Type == TypeUnknown {
		plan.Destination = s.proposeDestination(cfg.GamesPath, d.Name)
	}
	return PlanInfo{
		Plan:          plan,
		DownloadID:    d.ID,
		Name:          d.Name,
		RequiredBytes: requiredBytes(plan),
		FreeBytes:     s.freeBytes(volumeTarget(plan.Destination, cfg.GamesPath)),
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

	plan, err := Inspect(sourceDir(d))
	if err != nil {
		return Installation{}, err
	}
	if opts.Type != "" {
		plan.Type = opts.Type
	}
	if installer := strings.TrimSpace(opts.InstallerPath); installer != "" {
		info, err := os.Stat(installer)
		if err != nil || info.IsDir() {
			return Installation{}, errNoExecutable
		}
		plan.InstallerPath = installer
		plan.WorkingDir = filepath.Dir(installer)
	}
	if plan.Type == TypeUnknown {
		return Installation{}, errUnknownType
	}
	if external(plan.Type) && plan.InstallerPath == "" {
		return Installation{}, errNoExecutable
	}

	item := &Installation{
		ID:            newID(),
		DownloadID:    d.ID,
		Name:          d.Name,
		Type:          plan.Type,
		Status:        StatusPending,
		SourcePath:    plan.SourcePath,
		ContentRoot:   plan.ContentRoot,
		InstallerPath: plan.InstallerPath,
		WorkingDir:    plan.WorkingDir,
		ArchivePath:   plan.ArchivePath,
		BytesTotal:    plan.EstimatedSize,
		Origin:        d.Origin,
		Unattended:    opts.Unattended,
		SkipRegister:  opts.SkipRegister,
		StartedAt:     time.Now(),
	}

	if controlled(plan.Type) {
		dest := strings.TrimSpace(opts.Destination)
		if dest == "" {
			return Installation{}, errNoDestination
		}
		if !destAvailable(dest) {
			return Installation{}, errDestNotEmpty
		}
		item.Destination = filepath.Clean(dest)
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
	downloadID := item.DownloadID
	s.mu.Unlock()

	d, err := s.completedDownload(downloadID)
	if err != nil {
		return err
	}
	plan, err := Inspect(sourceDir(d))
	if err != nil {
		return err
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
	if !info.Plan.CanAutoInstall || !controlled(info.Plan.Type) {
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

func (s *Service) spawnLocked(id string) {
	base := s.ctx
	if base == nil {
		base = context.Background()
	}
	ctx, cancel := context.WithCancel(base)
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
	switch {
	case j != nil && j.cancelled:
		s.markCancelledLocked(item)
		snap := snapshotOf(item)
		s.mu.Unlock()
		s.notifyFinished(snap)
	case s.closing || errors.Is(cause, context.Canceled):
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
		emit(eventUpdated, snap)
		s.notifyFinished(snap)
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

func (s *Service) freeBytes(path string) int64 {
	if path == "" {
		return 0
	}
	info, err := s.freeSpace(path)
	if err != nil {
		slog.Warn("check free space", "path", path, "error", err)
		return 0
	}
	if info.FreeBytes > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(info.FreeBytes)
}

func (s *Service) checkSpace(destination string, need int64) error {
	if need <= 0 || destination == "" {
		return nil
	}
	free := s.freeBytes(destination)
	if free <= 0 || free >= need {
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
