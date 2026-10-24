package savebackup

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"typhon/internal/hashdir"
	"typhon/internal/library"
	"typhon/internal/settings"
	"typhon/internal/uierr"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	rootDirName   = "save-backups"
	legacyDirName = "saves"
)

// playerBackups gates everything the player can reach: the window, the Wails
// methods and the copy after a session. The pre-update snapshot goes through
// SnapshotPath and keeps working either way, as it did before this service.
const playerBackups = true

const (
	launchTimeout   = 20 * time.Second
	largeSavesBytes = 500 << 20
)

// Played is the play time after one moment; Known is false when the play log
// cannot answer for it.
type Played struct {
	Seconds int64
	Known   bool
}

// PlaytimeFunc answers for every moment in since at once, in the same order:
// the play log is read once per List, not once per snapshot. An error means
// nothing is known.
type PlaytimeFunc func(gameID string, since []time.Time) ([]Played, error)

type games interface {
	Find(id string) (library.Game, error)
	LocateSaves(ctx context.Context, id string) (library.SavesResult, error)
	GetRunningGames() []string
}

type Service struct {
	root       string
	legacyRoot string
	config     func() settings.Settings
	games      games
	emit       func(Event)
	now        func() time.Time
	enabled    bool

	launchTimeout time.Duration
	largeAt       int64
	playtime      PlaytimeFunc

	mu      sync.Mutex
	ctx     context.Context
	cancel  context.CancelFunc
	closing bool
	locks   map[string]chan struct{}
	warned  map[string]bool
	build   func(ctx context.Context, root string, onProgress func(hashdir.Progress)) (hashdir.Manifest, error)
	wg      sync.WaitGroup
}

func NewService(settingsService *settings.Service, lib *library.Service) (*Service, error) {
	if settingsService == nil {
		return nil, errors.New("savebackup: settings service is required")
	}
	if lib == nil {
		return nil, errors.New("savebackup: library service is required")
	}
	dir, err := settings.ConfigDir()
	if err != nil {
		return nil, fmt.Errorf("resolve config dir: %w", err)
	}
	s, err := newServiceAt(dir, settingsService.GetSettings, lib)
	if err != nil {
		return nil, err
	}
	s.enabled = playerBackups
	return s, nil
}

func newServiceAt(dir string, config func() settings.Settings, g games) (*Service, error) {
	if dir == "" || !filepath.IsAbs(dir) {
		return nil, fmt.Errorf("savebackup: config dir %q is not an absolute path", dir)
	}
	if config == nil {
		return nil, errors.New("savebackup: settings source is required")
	}
	if g == nil {
		return nil, errors.New("savebackup: library source is required")
	}
	return &Service{
		root:       filepath.Join(dir, rootDirName),
		legacyRoot: filepath.Join(dir, legacyDirName),
		config:     config,
		games:      g,
		emit:       emitEvent,
		now:        time.Now,
		locks:      map[string]chan struct{}{},
		warned:     map[string]bool{},
		build:      hashdir.Build,
		enabled:    true,

		launchTimeout: launchTimeout,
		largeAt:       largeSavesBytes,
	}, nil
}

//wails:ignore
func (s *Service) SetPlaytime(fn PlaytimeFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.playtime = fn
}

func (s *Service) Enabled() bool {
	return s.enabled
}

func emitEvent(ev Event) {
	if app := application.Get(); app != nil {
		app.Event.Emit(EventName, ev)
	}
}

func (s *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.mu.Lock()
	if s.ctx != nil {
		s.mu.Unlock()
		return errors.New("savebackup: already started")
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.ctx, s.cancel = runCtx, cancel
	s.mu.Unlock()

	ids, err := s.recoveryTargets()
	if err != nil {
		s.mu.Lock()
		s.ctx, s.cancel = nil, nil
		s.mu.Unlock()
		cancel()
		return err
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.recoverGames(runCtx, ids)
	}()
	return nil
}

func (s *Service) ServiceShutdown() error {
	s.mu.Lock()
	s.closing = true
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	s.wg.Wait()
	return nil
}

// begin ties one operation to both the caller context and the service one:
// shutdown cancels it and waits for it, so no copy outlives the service.
func (s *Service) begin(parent context.Context) (context.Context, func(), error) {
	if err := parent.Err(); err != nil {
		return nil, nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx == nil || s.closing {
		return nil, nil, errNotStarted
	}
	s.wg.Add(1)
	ctx, cancel := context.WithCancel(parent)
	stop := context.AfterFunc(s.ctx, cancel)
	return ctx, func() {
		stop()
		cancel()
		s.wg.Done()
	}, nil
}

func (s *Service) beginRooted() (context.Context, func(), error) {
	s.mu.Lock()
	root := s.ctx
	s.mu.Unlock()
	if root == nil {
		return nil, nil, errNotStarted
	}
	return s.begin(root)
}

func (s *Service) gameSem(id string) chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.locks[id]
	if !ok {
		l = make(chan struct{}, 1)
		s.locks[id] = l
	}
	return l
}

func (s *Service) lockGame(id string) func() {
	l := s.gameSem(id)
	l <- struct{}{}
	return func() { <-l }
}

// lockGameCtx is lockGame that gives up when ctx ends, for the launch path
// that must not wait behind a long copy.
func (s *Service) lockGameCtx(ctx context.Context, id string) (func(), error) {
	l := s.gameSem(id)
	select {
	case l <- struct{}{}:
		return func() { <-l }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *Service) gameDir(id string) string {
	return filepath.Join(s.root, id)
}

func (s *Service) List(ctx context.Context, gameID string) ([]Snapshot, error) {
	if !s.enabled {
		return nil, errDisabled
	}
	ctx, end, err := s.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer end()
	if !validName(gameID) {
		return nil, errInvalidID
	}
	list, err := s.list(ctx, gameID)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	played := s.playtime
	s.mu.Unlock()
	if played == nil {
		return list, nil
	}
	var idx []int
	var since []time.Time
	for i := range list {
		if !list[i].Broken {
			idx = append(idx, i)
			since = append(since, list[i].CreatedAt)
		}
	}
	if len(idx) == 0 {
		return list, nil
	}
	answers, err := played(gameID, since)
	if err == nil && len(answers) != len(since) {
		err = fmt.Errorf("play log answered %d of %d moments", len(answers), len(since))
	}
	if err != nil {
		slog.Warn("playtime after snapshots unknown", "game", gameID, "error", err)
		return list, nil
	}
	for n, i := range idx {
		if answers[n].Known {
			list[i].PlayedAfterSeconds, list[i].PlayedAfterKnown = answers[n].Seconds, true
		}
	}
	return list, nil
}

func (s *Service) Create(ctx context.Context, gameID string) (Snapshot, error) {
	if !s.enabled {
		return Snapshot{}, errDisabled
	}
	ctx, end, err := s.begin(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	defer end()
	if !validName(gameID) {
		return Snapshot{}, errInvalidID
	}
	unlock := s.lockGame(gameID)
	defer unlock()
	if err := s.recoverGame(ctx, gameID); err != nil {
		return Snapshot{}, err
	}
	src, err := s.locate(ctx, gameID)
	if err != nil {
		return Snapshot{}, err
	}
	snap, created, err := s.capture(ctx, gameID, src, KindManual, captureOpts{rotate: true})
	if created {
		s.emit(createdEvent(snap, err))
	}
	return snap, err
}

// SnapshotPath is the entry point for services that already know which folder
// to copy: it takes the same locked, verified, rotated copy as Create. When
// trimming old copies fails the snapshot still exists and is returned together
// with the error.
//
//wails:ignore
func (s *Service) SnapshotPath(ctx context.Context, gameID, sourcePath string, kind Kind) (Snapshot, error) {
	ctx, end, err := s.begin(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	defer end()
	if !validName(gameID) {
		return Snapshot{}, errInvalidID
	}
	if !kind.valid() {
		return Snapshot{}, errInvalidKind
	}
	if sourcePath == "" {
		return Snapshot{}, errNoSource
	}
	unlock := s.lockGame(gameID)
	defer unlock()
	if err := s.recoverGame(ctx, gameID); err != nil {
		return Snapshot{}, err
	}
	snap, created, err := s.capture(ctx, gameID, sourcePath, kind, captureOpts{dedup: dedupFor(kind), rotate: true})
	if created {
		s.emit(createdEvent(snap, err))
	}
	return snap, err
}

func (s *Service) Delete(gameID, snapshotID string) error {
	if !s.enabled {
		return errDisabled
	}
	_, end, err := s.beginRooted()
	if err != nil {
		return err
	}
	defer end()
	if !validName(gameID) || !validSnapshotID(snapshotID) {
		return errInvalidID
	}
	unlock := s.lockGame(gameID)
	defer unlock()

	dir := filepath.Join(s.gameDir(gameID), snapshotID)
	info, err := os.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return errSnapshotNotFound
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errSnapshotNotFound
	}
	snap, ok := s.readSnapshot(gameID, snapshotID)
	if !ok {
		return errSnapshotNotFound
	}
	if err := removeSnapshotDir(dir); err != nil {
		return fmt.Errorf("удаление копии %s: %w", snapshotID, err)
	}
	s.emit(Event{GameID: gameID, Kind: snap.Kind, Snapshot: &snap, Status: StatusDeleted})
	return nil
}

// SessionStarted and SessionStopped make the service a library session
// watcher. The library calls them under its own lock, so nothing here may call
// back into it synchronously: the copy runs in a goroutine owned by the
// service.
//
//wails:ignore
func (s *Service) SessionStarted(library.Game) {}

//wails:ignore
func (s *Service) SessionStopped(gameID string) {
	if !s.enabled || !s.config().SaveBackupAfterSession {
		return
	}
	s.mu.Lock()
	if s.ctx == nil || s.closing {
		s.mu.Unlock()
		slog.Warn("session backup skipped: service is not running", "game", gameID)
		return
	}
	ctx := s.ctx
	s.wg.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.wg.Done()
		s.autoBackup(ctx, gameID)
	}()
}

// SessionStarting takes the copy before the game process starts. It is a
// library.LaunchHook: the game launches whatever this returns, except for a
// library.LaunchBlockedError, which means an interrupted rollback is still
// unresolved and launching could cost the player's saves. The copy runs in a
// goroutine owned by the service and the hook waits for it only until the
// deadline: a step that ignores ctx (a dead network share, a cloud
// placeholder) cannot hold the game back, and the goroutine cleans up after
// itself when it finally returns.
//
//wails:ignore
func (s *Service) SessionStarting(ctx context.Context, game library.Game) error {
	if !s.enabled || !s.config().SaveBackupBeforeLaunch {
		return nil
	}
	ctx, end, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer end()
	capCtx, cancel := context.WithTimeout(ctx, s.launchTimeout)
	defer cancel()

	done := make(chan launchResult, 1)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		snap, created, err := s.launchCapture(capCtx, game.ID)
		done <- launchResult{snap, created, err}
	}()
	select {
	case r := <-done:
		return s.finishLaunch(ctx, capCtx, game.ID, r)
	case <-capCtx.Done():
		select {
		case r := <-done:
			return s.finishLaunch(ctx, capCtx, game.ID, r)
		default:
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return s.launchTimedOut(game.ID)
	}
}

type launchResult struct {
	snap    Snapshot
	created bool
	err     error
}

func (s *Service) launchTimedOut(gameID string) error {
	if _, err := os.Lstat(s.journalPath(gameID)); !errors.Is(err, fs.ErrNotExist) {
		s.emit(Event{GameID: gameID, Kind: KindLaunch, Status: StatusFailed, Code: uierr.Code(errRecoveryPending), Error: errRecoveryPending.Error()})
		return &library.LaunchBlockedError{Err: errRecoveryPending}
	}
	s.emit(Event{GameID: gameID, Kind: KindLaunch, Status: StatusFailed, Code: uierr.Code(errLaunchTimeout), Error: errLaunchTimeout.Error()})
	return errLaunchTimeout
}

func (s *Service) finishLaunch(ctx, capCtx context.Context, gameID string, r launchResult) error {
	failed := func(err error) error {
		s.emit(Event{GameID: gameID, Kind: KindLaunch, Status: StatusFailed, Code: uierr.Code(err), Error: err.Error()})
		return err
	}
	var blocked *library.LaunchBlockedError
	switch {
	case r.err == nil && r.created:
		s.emit(createdEvent(r.snap, nil))
		return nil
	case r.created:
		s.emit(createdEvent(r.snap, r.err))
		return r.err
	case ctx.Err() != nil:
		return ctx.Err()
	case errors.As(r.err, &blocked):
		return failed(r.err)
	case errors.Is(r.err, context.DeadlineExceeded) && errors.Is(capCtx.Err(), context.DeadlineExceeded):
		return s.launchTimedOut(gameID)
	case library.IsGameNotFound(r.err), errors.Is(r.err, errUnchanged):
		return nil
	case errors.Is(r.err, errSavesNotFound) && !errors.Is(r.err, errSavesUnreadable) && !errors.Is(r.err, errSavesHintsFailed):
		return nil
	case errors.Is(r.err, errSavesNotFound), errors.Is(r.err, errSavesAmbiguous), errors.Is(r.err, errSavesCacheLike):
		code := uierr.Code(r.err)
		s.emit(Event{GameID: gameID, Kind: KindLaunch, Status: StatusSkipped, Code: code, Error: r.err.Error()})
		s.warnOnce(gameID, code, r.err)
		return nil
	default:
		return failed(r.err)
	}
}

func (s *Service) warnOnce(gameID, code string, err error) {
	key := gameID + "|" + code
	s.mu.Lock()
	seen := s.warned[key]
	s.warned[key] = true
	s.mu.Unlock()
	if !seen {
		slog.Warn("pre-launch save copy skipped", "game", gameID, "code", code, "error", err)
	}
}

func (s *Service) launchCapture(ctx context.Context, gameID string) (Snapshot, bool, error) {
	if !validName(gameID) {
		return Snapshot{}, false, errInvalidID
	}
	unlock, err := s.lockGameCtx(ctx, gameID)
	if err != nil {
		return Snapshot{}, false, err
	}
	defer unlock()
	if err := s.recoverJournal(gameID); err != nil {
		return Snapshot{}, false, &library.LaunchBlockedError{Err: fmt.Errorf("%w: %w", errRecoveryPending, err)}
	}
	if err := s.recoverGame(ctx, gameID); err != nil {
		return Snapshot{}, false, err
	}
	src, err := s.locate(ctx, gameID)
	if err != nil {
		return Snapshot{}, false, err
	}
	snap, created, err := s.capture(ctx, gameID, src, KindLaunch, captureOpts{dedup: dedupLatest, rotate: true, cheap: true})
	if err == nil && !created {
		return snap, false, errUnchanged
	}
	return snap, created, err
}

func (s *Service) autoBackup(ctx context.Context, gameID string) {
	ev := Event{GameID: gameID, Kind: KindSession}
	snap, created, err := s.autoCapture(ctx, gameID)
	switch {
	case ctx.Err() != nil:
		slog.Info("session backup interrupted", "game", gameID, "error", ctx.Err())
		return
	case library.IsGameNotFound(err):
		slog.Info("session backup skipped: game left the library", "game", gameID)
		return
	case created:
		ev = createdEvent(snap, err)
	case errors.Is(err, errUnchanged):
		ev.Status, ev.Code, ev.Error, ev.Snapshot = StatusSkipped, uierr.Code(err), err.Error(), &snap
	case errors.Is(err, errSavesNotFound), errors.Is(err, errSavesAmbiguous), errors.Is(err, errSavesCacheLike):
		ev.Status, ev.Code, ev.Error = StatusSkipped, uierr.Code(err), err.Error()
	default:
		ev.Status, ev.Code, ev.Error = StatusFailed, uierr.Code(err), err.Error()
		slog.Error("session backup failed", "game", gameID, "error", err)
	}
	s.emit(ev)
}

func (s *Service) autoCapture(ctx context.Context, gameID string) (Snapshot, bool, error) {
	if !validName(gameID) {
		return Snapshot{}, false, errInvalidID
	}
	unlock := s.lockGame(gameID)
	defer unlock()
	if err := s.recoverGame(ctx, gameID); err != nil {
		return Snapshot{}, false, err
	}
	src, err := s.locate(ctx, gameID)
	if err != nil {
		return Snapshot{}, false, err
	}
	snap, created, err := s.capture(ctx, gameID, src, KindSession, captureOpts{dedup: dedupLatest, rotate: true})
	if err == nil && !created {
		return snap, false, errUnchanged
	}
	return snap, created, err
}

func createdEvent(snap Snapshot, rotationErr error) Event {
	ev := Event{GameID: snap.GameID, Kind: snap.Kind, Snapshot: &snap, Status: StatusCreated}
	if rotationErr != nil {
		ev.Code = uierr.Code(rotationErr)
		ev.Error = rotationErr.Error()
	}
	return ev
}

func (s *Service) locate(ctx context.Context, gameID string) (string, error) {
	res, err := s.games.LocateSaves(ctx, gameID)
	if err != nil {
		return "", err
	}
	switch {
	case res.Path != "":
		return res.Path, nil
	case len(res.Candidates) > 0:
		return "", fmt.Errorf("%w: вариантов %d", errSavesAmbiguous, len(res.Candidates))
	case len(res.Rejected) > 0:
		return "", fmt.Errorf("%w: %s; укажите папку сохранений вручную", errSavesCacheLike, res.Rejected[0].Path)
	case res.Unreadable > 0:
		return "", fmt.Errorf("%w: %w: %d", errSavesNotFound, errSavesUnreadable, res.Unreadable)
	}
	if res.HintsError != "" {
		return "", fmt.Errorf("%w: %w: %s", errSavesNotFound, errSavesHintsFailed, res.HintsError)
	}
	return "", errSavesNotFound
}

func (s *Service) isRunning(gameID string) bool {
	return slices.Contains(s.games.GetRunningGames(), gameID)
}
