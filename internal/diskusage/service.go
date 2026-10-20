package diskusage

import (
	"context"
	"errors"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"

	"typhon/internal/download"
	"typhon/internal/install"
	"typhon/internal/library"
	"typhon/internal/relocate"
	"typhon/internal/savebackup"
	"typhon/internal/screenshots"
	"typhon/internal/settings"
)

type libraryPort interface {
	GetGames() []library.Game
	IsRunning(id string) bool
	UpdateMeasuredSize(id, installDir string, size int64, unknown bool) (bool, error)
}

type downloadsPort interface {
	List() []download.Download
	Get(id string) (download.Download, error)
	DeleteData(id string) error
	PartFileVerdict(path string) (ok bool, code, reason string)
	PendingCleanups() []download.Cleanup
	CleanupVerdict(path string) (ok bool, code, reason string)
	RemoveCleanup(path string) error
	ReserveCleanup(path string) (func(), error)
}

type installPort interface {
	List() []install.Installation
	PartialVerdict(path string) (ok bool, code, reason string)
	PathInUse(path string) bool
	ReservePartial(path string) (func(), error)
	GameBusy(game library.Game) bool
	CheckDownloadDeletable(d download.Download) error
}

type relocatePort interface {
	Stagings() ([]relocate.Staging, error)
	ReserveStaging(path string) (func(), error)
}

type updatesPort interface {
	PathInUse(path string) bool
}

type screenshotsPort interface {
	Usage(ctx context.Context) ([]screenshots.Usage, error)
	List(gameID string) ([]screenshots.Shot, error)
	Delete(gameID, name string) error
}

type snapshotsPort interface {
	Usage(ctx context.Context) ([]savebackup.Usage, error)
	List(ctx context.Context, gameID string) ([]savebackup.Snapshot, error)
	Delete(gameID, snapshotID string) error
}

type Deps struct {
	Settings    func() settings.Settings
	Library     libraryPort
	Downloads   downloadsPort
	Installs    installPort
	Relocations relocatePort
	Updates     updatesPort
	Screenshots screenshotsPort
	Snapshots   snapshotsPort
}

// entry is what the last scan knew about one leftover: who may veto its
// deletion, which scanned folder it sits in, and how the owner wants it
// removed. reserve, when set, checks and bars the owner from reusing the path
// until released; without it the check is repeated right before the removal.
type entry struct {
	root    string
	check   func() (bool, string, string)
	reserve func() (func(), error)
	remove  func() error
}

type Service struct {
	deps Deps

	mu      sync.Mutex
	ctx     context.Context
	cancel  context.CancelFunc
	closing bool
	wg      sync.WaitGroup
	report  *Report
	entries map[string]entry

	opMu sync.Mutex
}

//wails:ignore
func NewService(deps Deps) (*Service, error) {
	if deps.Settings == nil {
		return nil, errNoSettingsSrc
	}
	if deps.Library == nil || deps.Downloads == nil || deps.Installs == nil ||
		deps.Relocations == nil || deps.Updates == nil || deps.Screenshots == nil || deps.Snapshots == nil {
		return nil, errMissingPort
	}
	return &Service{deps: deps, entries: map[string]entry{}}, nil
}

func (s *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx != nil {
		return errors.New("diskusage: already started")
	}
	s.ctx, s.cancel = context.WithCancel(ctx)
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

// begin ties one operation to the caller context and to the service one:
// shutdown cancels it and waits for it.
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

// Cached returns the report of the last scan, or nil when none ran yet.
func (s *Service) Cached() *Report {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.report == nil {
		return nil
	}
	out := s.report.clone()
	return &out
}
