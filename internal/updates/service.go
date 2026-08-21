package updates

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"typhon/internal/download"
	"typhon/internal/install"
	"typhon/internal/library"
	"typhon/internal/settings"
	"typhon/internal/sources"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	eventAvailable = "update:available"
	eventStarted   = "update:started"
	eventUpdated   = "update:updated"
	eventCompleted = "update:completed"
	eventFailed    = "update:failed"
	eventRollback  = "update:rollback"

	eventVerifyStarted   = "verify:started"
	eventVerifyUpdated   = "verify:updated"
	eventVerifyCompleted = "verify:completed"

	eventRepairStarted   = "repair:started"
	eventRepairUpdated   = "repair:updated"
	eventRepairCompleted = "repair:completed"

	progressPeriodEvents  = 300 * time.Millisecond
	minSuccessfulSession  = 60 * time.Second
	previousKeepDuration  = 24 * time.Hour
	housekeepingInterval  = 15 * time.Minute
	stagingDirName        = ".staging"
	previousSuffix        = ".previous"
	replacedSuffix        = ".replaced"
	interruptedUpdateText = "обновление было прервано"
)

var pollInterval = time.Second

var (
	errNotTracked   = errors.New("для этой игры нет данных об обновлении")
	errNoPlan       = errors.New("сначала подготовьте план обновления")
	errGameRunning  = errors.New("игра запущена — закройте её перед обновлением")
	errBusy         = errors.New("операция уже выполняется")
	errNoRollback   = errors.New("предыдущая версия недоступна")
	errNoIdentity   = errors.New("проверка недоступна для этой установки")
	errNoDownloads  = errors.New("менеджер загрузок недоступен")
	errNoInstaller  = errors.New("установщик недоступен")
	errNoLibrary    = errors.New("библиотека недоступна")
	errUpdateFailed = errors.New("не удалось применить обновление")
)

type librarySource interface {
	GetInstalledGames() []library.Game
	GetRunningGames() []string
	ApplyInstalledUpdate(u library.InstalledUpdate) (library.Game, error)
}

type releaseSource interface {
	ReleasesFor(canonicalGameID, title string) []sources.Release
	FindRelease(id string) (sources.Release, bool)
}

type downloadSource interface {
	AddTask(ctx context.Context, req download.AddRequest) (download.Download, error)
	InspectReuse(ctx context.Context, req download.ReuseRequest, onProgress func(download.VerifyProgress)) (download.ReuseReport, error)
	Get(id string) (download.Download, error)
	Cancel(id string) error
	ByOrigin(gameID string, purpose download.Purpose) []download.Download
}

type installer interface {
	Start(downloadID string, opts install.StartOptions) (install.Installation, error)
}

type job struct {
	cancel context.CancelFunc
	done   chan struct{}
}

type Service struct {
	mu        sync.Mutex
	settings  *settings.Service
	library   librarySource
	releases  releaseSource
	downloads downloadSource
	installs  installer
	store     *store

	updates       map[string]*Update
	verifications map[string]*VerifyState
	rollbacks     map[string]*Rollback
	history       []UpdateHistory

	jobs    map[string]*job
	waiters map[string]chan install.Installation

	ctx     context.Context
	cancel  context.CancelFunc
	closing bool
	wg      sync.WaitGroup
}

func NewService(
	settingsService *settings.Service,
	lib *library.Service,
	releases *sources.Service,
	downloads *download.Manager,
	installs *install.Service,
) *Service {
	dir, err := settings.ConfigDir()
	if err != nil {
		slog.Error("resolve config dir", "error", err)
		dir = ""
	}
	s := newServiceAt(dir, settingsService)
	if lib != nil {
		s.library = lib
	}
	if releases != nil {
		s.releases = releases
	}
	if downloads != nil {
		s.downloads = downloads
	}
	if installs != nil {
		s.installs = installs
	}
	return s
}

func newServiceAt(dir string, settingsService *settings.Service) *Service {
	return &Service{
		settings:      settingsService,
		store:         newStore(dir),
		updates:       map[string]*Update{},
		verifications: map[string]*VerifyState{},
		rollbacks:     map[string]*Rollback{},
		jobs:          map[string]*job{},
		waiters:       map[string]chan install.Installation{},
	}
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
	for _, u := range s.store.loadUpdates() {
		item := u
		if item.State == StateUpdating || item.State == StateDownloading {
			item.State = StateFailed
			item.Error = interruptedUpdateText
			item.Step = ""
			item.Progress = 0
		}
		item.Planning = false
		s.updates[item.GameID] = &item
	}
	for _, v := range s.store.loadVerifications() {
		state := v
		state.Running = false
		state.Repairing = false
		s.verifications[state.GameID] = &state
	}
	for _, r := range s.store.loadRollbacks() {
		entry := r
		s.rollbacks[entry.GameID] = &entry
	}
	s.history = s.store.loadHistory()
	s.mu.Unlock()

	s.wg.Add(1)
	go s.housekeeping()

	if s.config().UpdateCheckAutomatically {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.checkAll(s.baseContext())
		}()
	}
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

func (s *Service) baseContext() context.Context {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx == nil {
		return context.Background()
	}
	return s.ctx
}

func (s *Service) persistLocked() {
	list := make([]Update, 0, len(s.updates))
	for _, u := range s.updates {
		list = append(list, *u)
	}
	sortUpdates(list)
	if err := s.store.saveUpdates(list); err != nil {
		slog.Error("persist updates", "error", err)
	}
}

func (s *Service) persistVerifyLocked() {
	list := make([]VerifyState, 0, len(s.verifications))
	for _, v := range s.verifications {
		list = append(list, *v)
	}
	if err := s.store.saveVerifications(list); err != nil {
		slog.Error("persist verifications", "error", err)
	}
}

func (s *Service) persistRollbacksLocked() {
	list := make([]Rollback, 0, len(s.rollbacks))
	for _, r := range s.rollbacks {
		list = append(list, *r)
	}
	if err := s.store.saveRollbacks(list); err != nil {
		slog.Error("persist rollbacks", "error", err)
	}
}

func sortUpdates(list []Update) {
	for i := 1; i < len(list); i++ {
		for j := i; j > 0 && list[j].GameID < list[j-1].GameID; j-- {
			list[j], list[j-1] = list[j-1], list[j]
		}
	}
}

func (s *Service) snapshot(gameID string) (Update, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.updates[gameID]
	if !ok {
		return Update{}, false
	}
	return *u, true
}

func (s *Service) mutate(gameID string, apply func(*Update)) (Update, bool) {
	s.mu.Lock()
	u, ok := s.updates[gameID]
	if !ok {
		s.mu.Unlock()
		return Update{}, false
	}
	apply(u)
	s.persistLocked()
	snap := *u
	s.mu.Unlock()
	emit(eventUpdated, snap)
	return snap, true
}

func (s *Service) GetUpdates() []Update {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := make([]Update, 0, len(s.updates))
	for _, u := range s.updates {
		list = append(list, *u)
	}
	sortUpdates(list)
	return list
}

func (s *Service) GetUpdate(gameID string) (Update, error) {
	u, ok := s.snapshot(gameID)
	if !ok {
		return Update{}, errNotTracked
	}
	return u, nil
}

func (s *Service) GetHistory(gameID string) []UpdateHistory {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]UpdateHistory, 0, len(s.history))
	for i := len(s.history) - 1; i >= 0; i-- {
		if gameID == "" || s.history[i].GameID == gameID {
			out = append(out, s.history[i])
		}
	}
	return out
}

func (s *Service) installedGame(gameID string) (library.Game, bool) {
	if s.library == nil {
		return library.Game{}, false
	}
	for _, g := range s.library.GetInstalledGames() {
		if g.ID == gameID {
			return g, true
		}
	}
	return library.Game{}, false
}

func (s *Service) running(gameID string) bool {
	if s.library == nil {
		return false
	}
	for _, id := range s.library.GetRunningGames() {
		if id == gameID {
			return true
		}
	}
	return false
}

func installedOf(g library.Game) InstalledGame {
	return InstalledGame{
		GameID:          g.ID,
		CanonicalGameID: g.CanonicalGameID,
		Title:           g.Title,
		InstallDir:      g.InstallDir,
		Executable:      g.Executable,
		ReleaseID:       g.ReleaseID,
		SourceID:        g.SourceID,
		Version:         g.Version,
		VersionSource:   versionSourceOf(g.VersionSource),
		SizeBytes:       g.SizeBytes,
	}
}

func versionSourceOf(raw string) VersionSource {
	switch raw {
	case string(VersionSourceRelease):
		return VersionSourceRelease
	case "pe_metadata", string(VersionSourceExecutable), "version_file":
		return VersionSourceExecutable
	case string(VersionSourceManifest):
		return VersionSourceManifest
	case string(VersionSourceManual):
		return VersionSourceManual
	default:
		return VersionSourceUnknown
	}
}

// CheckUpdates recomputes availability for every installed game.
func (s *Service) CheckUpdates() []Update {
	s.checkAll(s.baseContext())
	return s.GetUpdates()
}

func (s *Service) CheckGame(gameID string) (Update, error) {
	game, ok := s.installedGame(gameID)
	if !ok {
		return Update{}, errNotTracked
	}
	s.check(game)
	u, ok := s.snapshot(gameID)
	if !ok {
		return Update{}, errNotTracked
	}
	return u, nil
}

func (s *Service) checkAll(ctx context.Context) {
	if s.library == nil {
		return
	}
	for _, game := range s.library.GetInstalledGames() {
		if ctx.Err() != nil {
			return
		}
		s.check(game)
	}
}

func (s *Service) check(game library.Game) {
	if s.releases == nil {
		return
	}
	installed := installedOf(game)
	list := s.releases.ReleasesFor(game.CanonicalGameID, game.Title)
	availability := ResolveUpdate(installed, list, PatchesFrom(list))
	availability.GameID = game.ID

	s.mu.Lock()
	current, ok := s.updates[game.ID]
	if !ok {
		current = &Update{GameID: game.ID}
		s.updates[game.ID] = current
	}
	busy := current.State == StateDownloading || current.State == StateUpdating
	previous := current.Availability
	current.Title = game.Title
	current.CheckedAt = time.Now()
	current.CanRollback = s.rollbacks[game.ID] != nil
	if !busy {
		current.Availability = availability
		if previous.TargetReleaseID != availability.TargetReleaseID {
			current.Plan = nil
			current.Error = ""
		}
		switch {
		case availability.Available && current.State != StateReady:
			current.State = StateAvailable
		case !availability.Available:
			current.State = StateIdle
			current.Plan = nil
		}
	}
	s.persistLocked()
	snap := *current
	s.mu.Unlock()

	emit(eventUpdated, snap)
	if !busy && availability.Available && previous.TargetReleaseID != availability.TargetReleaseID {
		slog.Info("update available", "game", game.ID, "kind", availability.Kind,
			"from", availability.InstalledVersion, "to", availability.TargetVersion,
			"strategy", availability.Strategy, "confidence", availability.Confidence)
		emit(eventAvailable, snap)
	}
}

// HandleSourcesRefreshed re-resolves availability once new releases arrive.
//
//wails:ignore
func (s *Service) HandleSourcesRefreshed() {
	if !s.config().UpdateCheckAutomatically {
		return
	}
	go s.checkAll(s.baseContext())
}

// HandleInstallFinished routes installer results back to a waiting update job.
//
//wails:ignore
func (s *Service) HandleInstallFinished(item install.Installation) {
	s.mu.Lock()
	waiter, ok := s.waiters[item.DownloadID]
	if ok {
		delete(s.waiters, item.DownloadID)
	}
	s.mu.Unlock()
	if ok {
		waiter <- item
		close(waiter)
	}
}

// HandleSessionEnded drops the kept previous version after a real play session.
//
//wails:ignore
func (s *Service) HandleSessionEnded(gameID string, seconds int64) {
	if time.Duration(seconds)*time.Second < minSuccessfulSession {
		return
	}
	s.mu.Lock()
	entry, ok := s.rollbacks[gameID]
	if !ok || !entry.AwaitLaunch {
		s.mu.Unlock()
		return
	}
	path := entry.Path
	delete(s.rollbacks, gameID)
	s.persistRollbacksLocked()
	if u, tracked := s.updates[gameID]; tracked {
		u.CanRollback = false
	}
	s.persistLocked()
	s.mu.Unlock()

	slog.Info("previous version removed after a successful launch", "game", gameID, "path", path)
	removeTree(path)
}

func (s *Service) housekeeping() {
	defer s.wg.Done()
	ctx := s.baseContext()
	ticker := time.NewTicker(housekeepingInterval)
	defer ticker.Stop()
	s.sweepPrevious()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sweepPrevious()
		}
	}
}

func (s *Service) sweepPrevious() {
	now := time.Now()
	var stale []string
	s.mu.Lock()
	for id, entry := range s.rollbacks {
		if entry.KeepUntil == nil || now.Before(*entry.KeepUntil) {
			continue
		}
		stale = append(stale, entry.Path)
		delete(s.rollbacks, id)
		if u, ok := s.updates[id]; ok {
			u.CanRollback = false
		}
	}
	if len(stale) > 0 {
		s.persistRollbacksLocked()
		s.persistLocked()
	}
	s.mu.Unlock()

	for _, path := range stale {
		slog.Info("previous version expired", "path", path)
		removeTree(path)
	}
}

func (s *Service) beginJob(gameID string) (context.Context, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing || s.jobs[gameID] != nil {
		return nil, false
	}
	base := s.ctx
	if base == nil {
		base = context.Background()
	}
	ctx, cancel := context.WithCancel(base)
	s.jobs[gameID] = &job{cancel: cancel, done: make(chan struct{})}
	return ctx, true
}

func (s *Service) endJob(gameID string) {
	s.mu.Lock()
	j := s.jobs[gameID]
	delete(s.jobs, gameID)
	s.mu.Unlock()
	if j == nil {
		return
	}
	j.cancel()
	close(j.done)
}

func (s *Service) cancelJob(gameID string) {
	s.mu.Lock()
	j := s.jobs[gameID]
	s.mu.Unlock()
	if j != nil {
		j.cancel()
	}
}

func (s *Service) appendHistory(entry UpdateHistory) {
	s.mu.Lock()
	s.history = append(s.history, entry)
	if len(s.history) > maxHistory {
		s.history = s.history[len(s.history)-maxHistory:]
	}
	list := append([]UpdateHistory(nil), s.history...)
	s.mu.Unlock()
	if err := s.store.saveHistory(list); err != nil {
		slog.Error("persist update history", "error", err)
	}
}

func (s *Service) finishHistory(id, status, message string) {
	now := time.Now()
	s.mu.Lock()
	for i := range s.history {
		if s.history[i].ID != id {
			continue
		}
		s.history[i].Status = status
		s.history[i].Error = message
		s.history[i].CompletedAt = &now
		break
	}
	list := append([]UpdateHistory(nil), s.history...)
	s.mu.Unlock()
	if err := s.store.saveHistory(list); err != nil {
		slog.Error("persist update history", "error", err)
	}
}

func stagingDir(installDir, gameID string) string {
	return filepath.Join(filepath.Dir(installDir), stagingDirName, gameID)
}

func newID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("u%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
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

func relativeExecutable(installDir, executable string) string {
	if installDir == "" || executable == "" {
		return ""
	}
	rel, err := filepath.Rel(installDir, executable)
	if err != nil || strings.HasPrefix(rel, "..") {
		return ""
	}
	return rel
}
