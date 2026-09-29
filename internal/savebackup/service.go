package savebackup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"typhon/internal/library"
	"typhon/internal/settings"
	"typhon/internal/uierr"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	rootDirName   = "save-backups"
	legacyDirName = "saves"
)

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

	mu      sync.Mutex
	ctx     context.Context
	cancel  context.CancelFunc
	closing bool
	locks   map[string]*sync.Mutex
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
	return newServiceAt(dir, settingsService.GetSettings, lib)
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
		locks:      map[string]*sync.Mutex{},
	}, nil
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

	if err := s.recoverAll(runCtx); err != nil {
		s.mu.Lock()
		s.ctx, s.cancel = nil, nil
		s.mu.Unlock()
		cancel()
		return err
	}
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

func (s *Service) lockGame(id string) func() {
	s.mu.Lock()
	l, ok := s.locks[id]
	if !ok {
		l = &sync.Mutex{}
		s.locks[id] = l
	}
	s.mu.Unlock()
	l.Lock()
	return l.Unlock
}

func (s *Service) gameDir(id string) string {
	return filepath.Join(s.root, id)
}

func (s *Service) List(ctx context.Context, gameID string) ([]Snapshot, error) {
	ctx, end, err := s.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer end()
	if !validName(gameID) {
		return nil, errInvalidID
	}
	return s.list(ctx, gameID)
}

func (s *Service) Create(ctx context.Context, gameID string) (Snapshot, error) {
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

	snap, ok := s.readSnapshot(gameID, snapshotID)
	if !ok {
		return errSnapshotNotFound
	}
	if err := removeSnapshotDir(filepath.Join(s.gameDir(gameID), snapshotID)); err != nil {
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
	if !s.config().SaveBackupAfterSession {
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

func (s *Service) autoBackup(ctx context.Context, gameID string) {
	ev := Event{GameID: gameID, Kind: KindSession}
	snap, created, err := s.autoCapture(ctx, gameID)
	switch {
	case ctx.Err() != nil:
		slog.Info("session backup interrupted", "game", gameID, "error", ctx.Err())
		return
	case created:
		ev = createdEvent(snap, err)
	case errors.Is(err, errUnchanged):
		ev.Status, ev.Code, ev.Error, ev.Snapshot = StatusSkipped, uierr.Code(err), err.Error(), &snap
	case errors.Is(err, errSavesNotFound), errors.Is(err, errSavesAmbiguous):
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
	case res.Unreadable > 0:
		return "", fmt.Errorf("%w: не удалось прочитать каталогов %d", errSavesNotFound, res.Unreadable)
	}
	return "", errSavesNotFound
}

func (s *Service) isRunning(gameID string) bool {
	return slices.Contains(s.games.GetRunningGames(), gameID)
}
