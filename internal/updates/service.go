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
	"typhon/internal/history"
	"typhon/internal/install"
	"typhon/internal/library"
	"typhon/internal/settings"
	"typhon/internal/sources"
	"typhon/internal/uierr"
	"typhon/internal/usagestats"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	eventAvailable = "update:available"
	eventStarted   = "update:started"
	eventUpdated   = "update:updated"
	eventCompleted = "update:completed"
	eventFailed    = "update:failed"
	eventRollback  = "update:rollback"
	eventDegraded  = "update:degraded"

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
	patchBackupSuffix     = ".patch-backup"
	interruptedUpdateText = "обновление было прервано"
)

var pollInterval = time.Second

var (
	errNotTracked = uierr.New("updates.not_tracked", "для этой игры нет данных об обновлении")

	errEmptyInstallDir = uierr.New("updates.no_install_dir", "каталог установки не задан")
	errNoPlan          = uierr.New("updates.no_plan", "сначала подготовьте план обновления")
	errGameRunning     = uierr.New("updates.game_running", "игра запущена — закройте её перед обновлением")
	errBusy            = uierr.New("updates.busy", "операция уже выполняется")
	errNoRollback      = uierr.New("updates.no_rollback", "предыдущая версия недоступна")
	errNoIdentity      = uierr.New("updates.no_identity", "проверка недоступна для этой установки")
	errNoDownloads     = uierr.New("updates.no_downloads", "менеджер загрузок недоступен")
	errNoInstaller     = uierr.New("updates.no_installer", "установщик недоступен")
	errNoLibrary       = uierr.New("updates.no_library", "библиотека недоступна")
	errUpdateFailed    = uierr.New("updates.update_failed", "не удалось применить обновление")
)

// degradedStatus is the update:degraded event payload. It stays unexported
// with no accessor method: adding either would change the wails bindings
// generated for Service, which is not allowed here.
type degradedStatus struct {
	Degraded bool   `json:"degraded"`
	Message  string `json:"message"`
}

type librarySource interface {
	GetInstalledGames() []library.Game
	GetRunningGames() []string
	ApplyInstalledUpdate(u library.InstalledUpdate) (library.Game, error)
	BindDistribution(id, sourceID, releaseID, distributionID string, releaseUploadedAt *time.Time) (library.Game, error)
	LocateSaves(ctx context.Context, id string) (library.SavesResult, error)
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
	checkMu        sync.Mutex
	rollbackActive map[string]bool
	mu             sync.Mutex
	settings       *settings.Service
	library        librarySource
	releases       releaseSource
	downloads      downloadSource
	installs       installer
	store          *store

	updates       map[string]*Update
	verifications map[string]*VerifyState
	rollbacks     map[string]*Rollback
	journals      map[string]*SwapJournal
	history       []UpdateHistory
	status        degradedStatus

	jobs    map[string]*job
	waiters map[string]chan install.Installation

	usageRecorder   func(usagestats.Event)
	historyRecorder func(history.Record) error

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
) (*Service, error) {
	dir, err := settings.ConfigDir()
	if err != nil {
		return nil, fmt.Errorf("resolve config dir: %w", err)
	}
	s, err := newServiceAt(dir, settingsService)
	if err != nil {
		return nil, err
	}
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
	return s, nil
}

func newServiceAt(dir string, settingsService *settings.Service) (*Service, error) {
	if dir == "" {
		return nil, errors.New("updates path unavailable")
	}
	return &Service{
		settings:      settingsService,
		store:         newStore(dir),
		updates:       map[string]*Update{},
		verifications: map[string]*VerifyState{},
		rollbacks:     map[string]*Rollback{},
		journals:      map[string]*SwapJournal{},
		jobs:          map[string]*job{},
		waiters:       map[string]chan install.Installation{},
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

// markDegradedLocked records a persist failure and notifies the frontend.
// The caller must hold s.mu.
func (s *Service) markDegradedLocked(err error) {
	s.status = degradedStatus{Degraded: true, Message: err.Error()}
	emit(eventDegraded, s.status)
}

// clearDegradedLocked resets a previously recorded persist failure once a
// save succeeds again. It only emits when the status actually changes, so a
// healthy service does not fire update:degraded on every successful save.
// The caller must hold s.mu.
func (s *Service) clearDegradedLocked() {
	if !s.status.Degraded {
		return
	}
	s.status = degradedStatus{}
	emit(eventDegraded, s.status)
}

func (s *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.mu.Lock()
	startupCtx, cancel := context.WithCancel(ctx)
	s.ctx, s.cancel = startupCtx, cancel
	storedUpdates, err := s.store.loadUpdates()
	if err != nil {
		s.cancel = nil
		s.mu.Unlock()
		cancel()
		return err
	}
	storedVerifications, err := s.store.loadVerifications()
	if err != nil {
		s.cancel = nil
		s.mu.Unlock()
		cancel()
		return err
	}
	storedRollbacks, err := s.store.loadRollbacks()
	if err != nil {
		s.cancel = nil
		s.mu.Unlock()
		cancel()
		return err
	}
	storedHistory, err := s.store.loadHistory()
	if err != nil {
		s.cancel = nil
		s.mu.Unlock()
		cancel()
		return err
	}
	storedJournals, err := s.store.loadJournals()
	if err != nil {
		s.cancel = nil
		s.mu.Unlock()
		cancel()
		return err
	}
	for _, u := range storedUpdates {
		item := u
		if item.State == StateUpdating || item.State == StateDownloading {
			item.State = StateFailed
			item.Error = interruptedUpdateText
			item.Step = ""
			item.Progress = 0
		}
		item.Planning = false
		item.Plan = nil
		// Availability and plans are derived from mutable source data. Never
		// expose a persisted offer before it has passed the current provenance
		// checks in this process.
		item.Availability = UpdateAvailability{Kind: KindNone, GameID: item.GameID}
		if item.State == StateAvailable || item.State == StateReady {
			item.State = StateIdle
		}
		s.updates[item.GameID] = &item
	}
	for _, v := range storedVerifications {
		state := v
		state.Running = false
		state.Repairing = false
		s.verifications[state.GameID] = &state
	}
	for _, r := range storedRollbacks {
		entry := r
		s.rollbacks[entry.GameID] = &entry
	}
	for _, j := range storedJournals {
		entry := j
		s.journals[entry.GameID] = &entry
	}
	s.history = storedHistory
	interrupted := make([]string, 0, len(s.updates))
	for _, u := range s.updates {
		if u.State == StateFailed {
			interrupted = append(interrupted, u.GameID)
		}
	}
	s.mu.Unlock()

	s.recoverJournals()

	for _, gameID := range interrupted {
		if game, ok := s.installedGame(gameID); ok {
			staging, err := stagingDir(game.InstallDir, gameID)
			if err != nil {
				slog.Error("resolve staging dir", "game", gameID, "err", err)
				continue
			}
			removeTree(staging)
		}
	}

	s.wg.Add(1)
	go s.housekeeping(startupCtx)

	if s.config().UpdateCheckAutomatically {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.checkAll(startupCtx)
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
	err := s.persistLocked()
	s.mu.Unlock()
	if err != nil {
		return fmt.Errorf("persist updates on shutdown: %w", err)
	}
	return nil
}

func (s *Service) persistLocked() error {
	list := make([]Update, 0, len(s.updates))
	for _, u := range s.updates {
		list = append(list, *u)
	}
	sortUpdates(list)
	if err := s.store.saveUpdates(list); err != nil {
		return fmt.Errorf("save updates: %w", err)
	}
	return nil
}

func (s *Service) persistVerifyLocked() error {
	list := make([]VerifyState, 0, len(s.verifications))
	for _, v := range s.verifications {
		list = append(list, *v)
	}
	if err := s.store.saveVerifications(list); err != nil {
		return fmt.Errorf("save verifications: %w", err)
	}
	return nil
}

func (s *Service) persistRollbacksLocked() error {
	list := make([]Rollback, 0, len(s.rollbacks))
	for _, r := range s.rollbacks {
		list = append(list, *r)
	}
	if err := s.store.saveRollbacks(list); err != nil {
		return fmt.Errorf("save rollbacks: %w", err)
	}
	return nil
}

func (s *Service) persistJournalsLocked() error {
	list := make([]SwapJournal, 0, len(s.journals))
	for _, j := range s.journals {
		list = append(list, *j)
	}
	return s.store.saveJournals(list)
}

// setJournal persists a swap journal before the caller's first destructive
// filesystem step. A persist failure must not leave a journal in memory that
// disk recovery cannot see, so it is rolled back on error (invariant I.4).
func (s *Service) setJournal(j SwapJournal) error {
	if g, ok := s.installedGame(j.GameID); ok {
		j.Original = &library.InstalledUpdate{ID: g.ID, Executable: g.Executable, InstallDir: g.InstallDir, Version: g.Version, VersionSource: g.VersionSource, ReleaseID: g.ReleaseID, SourceID: g.SourceID, DistributionID: g.DistributionID, ReleaseUploadedAt: g.ReleaseUploadedAt}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if j.Kind == JournalSwap && s.rollbacks[j.GameID] != nil {
		copyEntry := *s.rollbacks[j.GameID]
		j.PreviousRollback = &copyEntry
	}
	previous, had := s.journals[j.GameID]
	entry := j
	s.journals[j.GameID] = &entry
	if err := s.persistJournalsLocked(); err != nil {
		if had {
			s.journals[j.GameID] = previous
		} else {
			delete(s.journals, j.GameID)
		}
		return err
	}
	return nil
}

func (s *Service) clearJournal(gameID string) error {
	s.mu.Lock()
	previous, ok := s.journals[gameID]
	if !ok {
		s.mu.Unlock()
		return nil
	}
	delete(s.journals, gameID)
	if err := s.persistJournalsLocked(); err != nil {
		s.journals[gameID] = previous
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()
	if previous.RetainedPrevious != "" {
		removeTree(previous.RetainedPrevious)
	}
	return nil
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

// mutate applies apply to the tracked update for gameID and persists the
// result. A persist failure rolls the in-memory copy back to what apply
// started from, marks the service degraded (update:degraded) and logs the
// error: mutate has no caller that could act differently on the error than
// log it, so the handling lives here once instead of at every call site.
func (s *Service) mutate(gameID string, apply func(*Update)) (Update, bool) {
	s.mu.Lock()
	u, ok := s.updates[gameID]
	if !ok {
		s.mu.Unlock()
		return Update{}, false
	}
	before := *u
	apply(u)
	if err := s.persistLocked(); err != nil {
		*u = before
		s.markDegradedLocked(err)
		s.mu.Unlock()
		slog.Error("persist update", "game", gameID, "error", err)
		return before, true
	}
	s.clearDegradedLocked()
	snap := *u
	s.mu.Unlock()
	emit(eventUpdated, snap)
	return snap, true
}

// updateFieldsBestEffort is mutate for recovery code that runs once at
// startup for many games in a row: a single unwritable state file must not
// stop every other game's journal recovery, so a persist failure here rolls
// the change back, marks the service degraded and logs, rather than
// aborting ServiceStartup.
func (s *Service) updateFieldsBestEffort(gameID string, apply func(*Update)) {
	s.mu.Lock()
	u, ok := s.updates[gameID]
	if !ok {
		s.mu.Unlock()
		return
	}
	before := *u
	apply(u)
	if err := s.persistLocked(); err != nil {
		*u = before
		s.markDegradedLocked(err)
		s.mu.Unlock()
		slog.Error("persist update", "game", gameID, "error", err)
		return
	}
	s.clearDegradedLocked()
	s.mu.Unlock()
}

// forgetRollbackBestEffort removes gameID's kept-previous-version record and
// persists it, rolling back and marking the service degraded on failure. See
// updateFieldsBestEffort for why this stays a logged best effort rather than
// a propagated error.
func (s *Service) forgetRollbackBestEffort(gameID string) {
	s.mu.Lock()
	entry, ok := s.rollbacks[gameID]
	if !ok {
		s.mu.Unlock()
		return
	}
	delete(s.rollbacks, gameID)
	if err := s.persistRollbacksLocked(); err != nil {
		s.rollbacks[gameID] = entry
		s.markDegradedLocked(err)
		s.mu.Unlock()
		slog.Error("persist rollbacks", "game", gameID, "error", err)
		return
	}
	s.clearDegradedLocked()
	s.mu.Unlock()
}

// planProgress reports progress of a long planning step without persisting it:
// the numbers are transient and a disk write every tick would be pointless.
func (s *Service) planProgress(gameID string, apply func(*Update)) {
	s.mu.Lock()
	u, ok := s.updates[gameID]
	if !ok {
		s.mu.Unlock()
		return
	}
	apply(u)
	snap := *u
	s.mu.Unlock()
	emit(eventUpdated, snap)
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

//wails:ignore
func (s *Service) Busy(gameID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.jobs[gameID] != nil || s.rollbackActive[gameID]
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
		GameID:            g.ID,
		CanonicalGameID:   g.CanonicalGameID,
		Title:             g.Title,
		InstallDir:        g.InstallDir,
		Executable:        g.Executable,
		ReleaseID:         g.ReleaseID,
		SourceID:          g.SourceID,
		DistributionID:    g.DistributionID,
		ReleaseUploadedAt: g.ReleaseUploadedAt,
		Version:           g.Version,
		VersionSource:     versionSourceOf(g.VersionSource),
		SizeBytes:         g.SizeBytes,
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

// CheckUpdates recomputes availability for every installed game. When the
// service has not completed ServiceStartup yet there is no context to run
// the check under, so it is skipped rather than run with a substitute
// context (invariant 20): the caller still gets whatever was already known.
func (s *Service) CheckUpdates() []Update {
	s.mu.Lock()
	ctx := s.ctx
	s.mu.Unlock()
	if ctx != nil {
		s.checkAll(ctx)
	}
	return s.GetUpdates()
}

func (s *Service) CheckGame(gameID string) (Update, error) {
	game, ok := s.installedGame(gameID)
	if !ok {
		return Update{}, errNotTracked
	}
	if err := s.check(game); err != nil {
		return Update{}, err
	}
	u, ok := s.snapshot(gameID)
	if !ok {
		return Update{}, errNotTracked
	}
	return u, nil
}

func (s *Service) checkAll(ctx context.Context) {
	s.checkMu.Lock()
	defer s.checkMu.Unlock()
	if ctx.Err() != nil {
		return
	}
	if s.library == nil {
		return
	}
	games := s.library.GetInstalledGames()
	known := make(map[string]bool, len(games))
	for _, game := range games {
		if ctx.Err() != nil {
			return
		}
		known[game.ID] = true
		if err := s.check(game); err != nil {
			slog.Error("check update", "game", game.ID, "error", err)
		}
	}
	s.prune(known)
}

// prune drops tracked updates and verifications for games no longer
// installed. A persist failure rolls the deleted entries back into memory
// instead of leaving them removed only in RAM (invariant I.4), marks the
// service degraded and logs: prune runs from checkAll in the background, so
// there is no caller left to hand the error to.
func (s *Service) prune(known map[string]bool) {
	s.mu.Lock()
	removedUpdates := map[string]*Update{}
	removedVerifications := map[string]*VerifyState{}
	for id := range s.updates {
		if known[id] {
			continue
		}
		removedUpdates[id] = s.updates[id]
		if v, ok := s.verifications[id]; ok {
			removedVerifications[id] = v
		}
	}
	if len(removedUpdates) == 0 {
		s.mu.Unlock()
		return
	}
	for id := range removedUpdates {
		delete(s.updates, id)
		delete(s.verifications, id)
	}
	restore := func() {
		for id, u := range removedUpdates {
			s.updates[id] = u
		}
		for id, v := range removedVerifications {
			s.verifications[id] = v
		}
	}
	if err := s.persistLocked(); err != nil {
		restore()
		s.markDegradedLocked(err)
		s.mu.Unlock()
		slog.Error("persist pruned updates", "error", err)
		return
	}
	if err := s.persistVerifyLocked(); err != nil {
		restore()
		if persistErr := s.persistLocked(); persistErr != nil {
			slog.Error("restore updates on disk after failed verification prune", "error", persistErr)
		}
		s.markDegradedLocked(err)
		s.mu.Unlock()
		slog.Error("persist pruned verifications", "error", err)
		return
	}
	s.clearDegradedLocked()
	s.mu.Unlock()
}

// check resolves update availability for game and persists it. A persist
// failure rolls the in-memory entry back to what it was before this call
// (removing it entirely if check itself created it), marks the service
// degraded and returns the error so CheckGame can surface it to the UI.
func (s *Service) check(game library.Game) error {
	if s.releases == nil {
		return nil
	}
	list := s.releases.ReleasesFor(game.CanonicalGameID, game.Title)
	game = s.bindLegacyDistribution(game, list)
	installed := installedOf(game)
	availability := ResolveUpdate(installed, list, PatchesFrom(list))
	availability.GameID = game.ID

	s.mu.Lock()
	current, existed := s.updates[game.ID]
	var before Update
	if existed {
		before = *current
	} else {
		current = &Update{GameID: game.ID}
		s.updates[game.ID] = current
	}
	busy := current.State == StateDownloading || current.State == StateUpdating
	previous := current.Availability
	changed := availabilityChanged(previous, availability)
	current.Title = game.Title
	current.CheckedAt = time.Now()
	current.CanRollback = s.rollbacks[game.ID] != nil
	if !busy {
		current.Availability = availability
		if changed {
			current.Plan = nil
			current.DownloadID = ""
			current.Error = ""
		}
		switch {
		case availability.Available && (current.State != StateReady || changed):
			current.State = StateAvailable
		case !availability.Available:
			current.State = StateIdle
			current.Plan = nil
		}
	}
	if err := s.persistLocked(); err != nil {
		if existed {
			*current = before
		} else {
			delete(s.updates, game.ID)
		}
		s.markDegradedLocked(err)
		s.mu.Unlock()
		return fmt.Errorf("persist update check for %s: %w", game.ID, err)
	}
	s.clearDegradedLocked()
	snap := *current
	s.mu.Unlock()

	emit(eventUpdated, snap)
	if !busy && availability.Available && changed {
		slog.Info("update available", "game", game.ID, "kind", availability.Kind,
			"from", availability.InstalledVersion, "to", availability.TargetVersion,
			"strategy", availability.Strategy, "confidence", availability.Confidence)
		emit(eventAvailable, snap)
	}
	return nil
}

func availabilityChanged(a, b UpdateAvailability) bool {
	return a.Available != b.Available || a.Kind != b.Kind ||
		a.InstalledReleaseID != b.InstalledReleaseID || a.InstalledVersion != b.InstalledVersion ||
		a.TargetReleaseID != b.TargetReleaseID || a.TargetVersion != b.TargetVersion ||
		a.SourceID != b.SourceID || a.DistributionID != b.DistributionID ||
		!samePlanTime(a.InstalledReleaseUploadedAt, b.InstalledReleaseUploadedAt) ||
		!samePlanTime(a.TargetReleaseUploadedAt, b.TargetReleaseUploadedAt)
}

func (s *Service) bindLegacyDistribution(game library.Game, releases []sources.Release) library.Game {
	if game.SourceID == "" || game.ReleaseID == "" || s.library == nil ||
		(game.DistributionID != "" && game.ReleaseUploadedAt != nil) {
		return game
	}
	var match *sources.Release
	for i := range releases {
		r := &releases[i]
		if r.ID != game.ReleaseID || r.SourceID != game.SourceID || r.DistributionID == "" ||
			(game.DistributionID != "" && r.DistributionID != game.DistributionID) {
			continue
		}
		if match != nil {
			return game
		}
		match = r
	}
	if match == nil {
		return game
	}
	var releaseUploadedAt *time.Time
	if game.ReleaseUploadedAt == nil && game.Version == releaseVersion(*match) {
		releaseUploadedAt = match.UploadedAt
	}
	if game.DistributionID != "" && releaseUploadedAt == nil {
		return game
	}
	bound, err := s.library.BindDistribution(game.ID, game.SourceID, game.ReleaseID, match.DistributionID, releaseUploadedAt)
	if err != nil {
		slog.Warn("bind legacy installation to distribution", "game", game.ID, "error", err)
		return game
	}
	return bound
}

// HandleSourcesRefreshed re-resolves availability once new releases arrive.
// The recheck runs in its own goroutine because it walks every installed
// game, but that goroutine is counted in s.wg and refuses to start once the
// service is closing or has no context yet, so ServiceShutdown never races a
// check still writing state after it returns (invariant 19).
//
//wails:ignore
func (s *Service) HandleSourcesRefreshed() {
	if !s.config().UpdateCheckAutomatically {
		return
	}
	s.mu.Lock()
	if s.closing || s.ctx == nil {
		s.mu.Unlock()
		return
	}
	ctx := s.ctx
	s.wg.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.wg.Done()
		s.checkAll(ctx)
	}()
}

// HandleInstallFinished routes installer results back to a waiting update job.
// An install nobody waits for is a fresh installation: it gets a manifest right
// away, while the files are still exactly what the installer produced.
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
		return
	}
	if item.Status != install.StatusCompleted || item.GameID == "" {
		return
	}
	if err := s.BuildManifest(item.GameID); err != nil {
		slog.Warn("manifest after install", "game", item.GameID, "install", item.ID, "error", err)
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
	if !ok || !entry.AwaitLaunch || s.rollbackActive[gameID] || s.journals[gameID] != nil {
		s.mu.Unlock()
		return
	}
	path := entry.Path
	delete(s.rollbacks, gameID)
	if err := s.persistRollbacksLocked(); err != nil {
		s.rollbacks[gameID] = entry
		s.markDegradedLocked(err)
		s.mu.Unlock()
		slog.Error("persist rollbacks after session end", "game", gameID, "error", err)
		return
	}
	var beforeUpdate *Update
	if u, tracked := s.updates[gameID]; tracked {
		before := *u
		beforeUpdate = &before
		u.CanRollback = false
	}
	if err := s.persistLocked(); err != nil {
		// rollbacks.json already committed the removal above; restoring the
		// in-memory entry here would only put memory out of sync with disk
		// again, so only the update flag is rolled back (invariant I.4).
		if beforeUpdate != nil {
			*s.updates[gameID] = *beforeUpdate
		}
		s.markDegradedLocked(err)
		s.mu.Unlock()
		slog.Error("persist updates after session end", "game", gameID, "error", err)
		return
	}
	s.clearDegradedLocked()
	s.mu.Unlock()

	slog.Info("previous version removed after a successful launch", "game", gameID, "path", path)
	removeTree(path)
}

func (s *Service) housekeeping(ctx context.Context) {
	defer s.wg.Done()
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

// sweepPrevious removes kept-previous-version directories past their
// KeepUntil deadline. A persist failure rolls the removed rollback entries
// (and the update flags they cleared) back into memory and skips deleting
// their directories, so a state file the disk refused to write never causes
// a backup to be deleted while memory still thinks it exists.
func (s *Service) sweepPrevious() {
	now := time.Now()
	var stale []string
	s.mu.Lock()
	removedRollbacks := map[string]*Rollback{}
	changedUpdates := map[string]Update{}
	for id, entry := range s.rollbacks {
		if s.rollbackActive[id] || s.journals[id] != nil {
			continue
		}
		if entry.KeepUntil == nil || now.Before(*entry.KeepUntil) {
			continue
		}
		stale = append(stale, entry.Path)
		removedRollbacks[id] = entry
		delete(s.rollbacks, id)
		if u, ok := s.updates[id]; ok {
			changedUpdates[id] = *u
			u.CanRollback = false
		}
	}
	if len(stale) == 0 {
		s.mu.Unlock()
		return
	}
	restore := func() {
		for id, entry := range removedRollbacks {
			s.rollbacks[id] = entry
		}
		for id, before := range changedUpdates {
			if u, ok := s.updates[id]; ok {
				*u = before
			}
		}
	}
	if err := s.persistRollbacksLocked(); err != nil {
		restore()
		s.markDegradedLocked(err)
		s.mu.Unlock()
		slog.Error("persist rollbacks during sweep", "error", err)
		return
	}
	if err := s.persistLocked(); err != nil {
		restore()
		if persistErr := s.persistRollbacksLocked(); persistErr != nil {
			slog.Error("restore rollbacks on disk after failed sweep persist", "error", persistErr)
		}
		s.markDegradedLocked(err)
		s.mu.Unlock()
		slog.Error("persist updates during sweep", "error", err)
		return
	}
	s.clearDegradedLocked()
	s.mu.Unlock()

	for _, path := range stale {
		slog.Info("previous version expired", "path", path)
		removeTree(path)
	}
}

// beginJob claims the job slot for gameID. It refuses instead of starting
// when the service has not completed ServiceStartup yet (s.ctx is nil):
// invariant 20 forbids substituting context.Background() here, so a job that
// cannot be tied to the service's lifecycle simply does not start.
func (s *Service) beginJob(gameID string) (context.Context, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing || s.ctx == nil || s.jobs[gameID] != nil || s.rollbackActive[gameID] {
		return nil, false
	}
	ctx, cancel := context.WithCancel(s.ctx)
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

// appendHistory records entry in the update-history journal. A persist
// failure rolls the journal back to its previous contents, marks the
// service degraded and logs: appendHistory runs from the background update
// goroutine, which has nothing more useful to do with the error than log it.
func (s *Service) appendHistory(entry UpdateHistory) {
	s.mu.Lock()
	previous := append([]UpdateHistory(nil), s.history...)
	s.history = append(s.history, entry)
	if len(s.history) > maxHistory {
		s.history = s.history[len(s.history)-maxHistory:]
	}
	list := append([]UpdateHistory(nil), s.history...)
	if err := s.store.saveHistory(list); err != nil {
		s.history = previous
		s.markDegradedLocked(err)
		s.mu.Unlock()
		slog.Error("persist update history", "error", err)
		return
	}
	s.clearDegradedLocked()
	s.mu.Unlock()
}

func (s *Service) finishHistory(id, status, message string) {
	now := time.Now()
	s.mu.Lock()
	previous := append([]UpdateHistory(nil), s.history...)
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
	if err := s.store.saveHistory(list); err != nil {
		s.history = previous
		s.markDegradedLocked(err)
		s.mu.Unlock()
		slog.Error("persist update history", "error", err)
		return
	}
	s.clearDegradedLocked()
	s.mu.Unlock()
}

func (s *Service) recheck(gameID string) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if _, err := s.CheckGame(gameID); err != nil {
			slog.Warn("recheck update", "game", gameID, "err", err)
		}
	}()
}

func stagingDir(installDir, gameID string) (string, error) {
	if installDir == "" {
		return "", errEmptyInstallDir
	}
	if gameID == "" {
		return "", errors.New("идентификатор игры не задан")
	}
	return filepath.Join(filepath.Dir(installDir), stagingDirName, gameID), nil
}

func previousDir(installDir string) (string, error) {
	if installDir == "" {
		return "", errEmptyInstallDir
	}
	return installDir + previousSuffix, nil
}

func newID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("u%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
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
