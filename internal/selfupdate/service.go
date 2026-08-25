package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"typhon/internal/account"
	"typhon/internal/app"
	"typhon/internal/settings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	eventStatus   = "launcher:update_status"
	eventProgress = "launcher:update_progress"
)

var checkInterval = 6 * time.Hour

var errNoUpdateChecked = errors.New("selfupdate: check for an update before downloading")

// startWorker is a seam for tests: the real one spawns a detached process.
var startWorker = startUpdateWorker

type Service struct {
	mu     sync.Mutex
	dir    string
	client *Client
	store  *Store
	status Status
	busy   bool
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	currentVersion  string
	pendingArtifact *Artifact
	pendingVersion  string
	readyPath       string
	readyArtifact   *Artifact
	outcome         Outcome
}

func NewService() (*Service, error) {
	dir, err := settings.ConfigDir()
	if err != nil {
		return nil, fmt.Errorf("selfupdate: resolve config dir: %w", err)
	}
	client, err := NewClient(account.BaseURL())
	if err != nil {
		return nil, fmt.Errorf("selfupdate: create client: %w", err)
	}
	store, err := NewStore(dir)
	if err != nil {
		return nil, fmt.Errorf("selfupdate: create store: %w", err)
	}
	return &Service{
		dir:            dir,
		client:         client,
		store:          store,
		currentVersion: app.Version,
	}, nil
}

func emit(name string, data any) {
	if a := application.Get(); a != nil {
		a.Event.Emit(name, data)
	}
}

func deriveState(v stored) State {
	if v.Artifact != nil && v.ReadyPath != "" {
		return StateReady
	}
	if v.AvailableVersion != "" {
		return StateAvailable
	}
	return StateIdle
}

func statusFromStored(v stored, currentVersion string) Status {
	return Status{
		State:            deriveState(v),
		CurrentVersion:   currentVersion,
		AvailableVersion: v.AvailableVersion,
		Notes:            v.Notes,
		PublishedAt:      v.PublishedAt,
		CheckedAt:        v.CheckedAt,
	}
}

func errorStatus(current Status, code string, cause error) Status {
	next := current
	next.Error = cause.Error()
	next.ErrorCode = code
	return next
}

func (s *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	startupCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.ctx, s.cancel = startupCtx, cancel
	v, err := s.store.Load()
	if err != nil {
		s.cancel = nil
		s.mu.Unlock()
		cancel()
		return fmt.Errorf("selfupdate: load state: %w", err)
	}
	s.mu.Unlock()

	if err := s.takeOutcome(); err != nil {
		return err
	}
	if err := s.dropStaleWorker(); err != nil {
		return err
	}

	// currentVersion moves only when an update is actually installed, so a
	// record kept from before the restart still claims the version this build
	// already is. Left alone it shows up as "update available" for the running
	// version and keeps its installer alive in the cache forever.
	stale, err := s.staleForCurrent(v)
	if err != nil {
		return err
	}
	if stale {
		v = stored{CheckedAt: v.CheckedAt}
		if serr := s.store.Save(v); serr != nil {
			return fmt.Errorf("selfupdate: persist cleared state: %w", serr)
		}
	}

	s.mu.Lock()
	s.status = statusFromStored(v, s.currentVersion)
	if s.status.State == StateReady {
		s.readyPath = v.ReadyPath
		s.readyArtifact = v.Artifact
	}
	s.mu.Unlock()

	if v.ReadyPath != "" && v.Artifact != nil {
		if verr := VerifyFile(startupCtx, v.ReadyPath, *v.Artifact); verr != nil {
			if rerr := os.Remove(v.ReadyPath); rerr != nil && !errors.Is(rerr, fs.ErrNotExist) {
				return fmt.Errorf("selfupdate: remove invalid ready artifact: %w", rerr)
			}
			v.Artifact = nil
			v.ReadyPath = ""
			if serr := s.store.Save(v); serr != nil {
				return fmt.Errorf("selfupdate: persist cleared ready state: %w", serr)
			}
			s.mu.Lock()
			s.status = statusFromStored(v, s.currentVersion)
			s.readyPath = ""
			s.readyArtifact = nil
			s.mu.Unlock()
		}
	}

	s.mu.Lock()
	keepVersion := ""
	if s.status.State == StateReady {
		keepVersion = s.status.AvailableVersion
	}
	s.mu.Unlock()

	if err := s.cleanupCache(startupCtx, keepVersion); err != nil {
		return fmt.Errorf("selfupdate: clean cache: %w", err)
	}

	s.wg.Add(1)
	go s.periodicCheck()
	return nil
}

func (s *Service) ServiceShutdown() error {
	s.mu.Lock()
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	s.wg.Wait()
	return nil
}

// takeOutcome picks up what the update worker left behind. The install runs
// with no UI on screen, so this record is the only way a failed update reaches
// the user instead of looking like a launcher that restarted for no reason.
func (s *Service) takeOutcome() error {
	path, err := OutcomePath(s.dir)
	if err != nil {
		return err
	}
	o, readErr := readOutcome(path)
	switch {
	case errors.Is(readErr, fs.ErrNotExist):
		return nil
	case readErr != nil:
		o = Outcome{OK: false, Error: readErr.Error(), FinishedAt: time.Now()}
	case time.Since(o.FinishedAt) > outcomeMaxAge:
		o = Outcome{}
	}

	s.mu.Lock()
	s.outcome = o
	s.mu.Unlock()

	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("selfupdate: remove update outcome: %w", err)
	}
	return nil
}

// dropStaleWorker removes the copy of the launcher the worker ran from. The
// copy is the process that relaunched us and may still be exiting, and Windows
// keeps a running image locked, so a failure here is expected and retried on
// the next start rather than blocking it.
func (s *Service) dropStaleWorker() error {
	dir, err := WorkerDir(s.dir)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		slog.Warn("remove selfupdate worker copy", "dir", dir, "error", err)
	}
	return nil
}

func (s *Service) staleForCurrent(v stored) (bool, error) {
	if v.AvailableVersion == "" {
		return false, nil
	}
	newer, err := IsNewer(v.AvailableVersion, s.currentVersion)
	if err != nil {
		return false, fmt.Errorf("selfupdate: compare stored version %q: %w", v.AvailableVersion, err)
	}
	return !newer, nil
}

// stageWorker names the copy after this process so a leftover copy from an
// earlier attempt, which Windows may still hold open, cannot block a new one.
func (s *Service) stageWorker(exe string) (string, error) {
	dir, err := WorkerDir(s.dir)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, fmt.Sprintf("typhon-update-%d.exe", os.Getpid()))
	if err := copyExecutable(exe, path); err != nil {
		return "", err
	}
	return path, nil
}

func (s *Service) GetOutcome() Outcome {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.outcome
}

// Removal goes through an os.Root scoped to the cache directory: absolute
// paths handed to os.Remove race with anything swapping a directory entry
// between the walk's stat and the removal (gosec G122).
func (s *Service) cleanupCache(ctx context.Context, keepVersion string) error {
	cacheDir, err := CacheDir(s.dir)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(cacheDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	defer func() {
		if cerr := root.Close(); cerr != nil {
			slog.Warn("close selfupdate cache root", "error", cerr)
		}
	}()

	return fs.WalkDir(root.FS(), ".", func(relPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if relPath == "." {
			return nil
		}
		depth := strings.Count(relPath, "/") + 1
		switch {
		case depth == 1 && d.IsDir():
			// dropStaleWorker owns the worker directory: it holds a running
			// image right after a restart, and a failed removal there must not
			// abort the cache sweep.
			if d.Name() == workerDirName {
				return fs.SkipDir
			}
			if keepVersion != "" && d.Name() == keepVersion {
				return nil
			}
			if err := root.RemoveAll(relPath); err != nil {
				return err
			}
			return fs.SkipDir
		case depth == 1:
			if d.Name() == "state.json" || d.Name() == outcomeName {
				return nil
			}
			return root.Remove(relPath)
		case depth == 2 && !d.IsDir() && strings.HasPrefix(d.Name(), "."):
			return root.Remove(relPath)
		}
		return nil
	})
}

func (s *Service) periodicCheck() {
	defer s.wg.Done()
	s.checkQuiet()
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.checkQuiet()
		}
	}
}

func (s *Service) checkQuiet() {
	if _, err := s.CheckForUpdate(s.ctx); err != nil {
		slog.Debug("periodic selfupdate check", "error", err)
	}
}

func (s *Service) GetStatus() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

func (s *Service) commitStatus(status Status) {
	s.mu.Lock()
	s.status = status
	s.mu.Unlock()
	emit(eventStatus, status)
}

func (s *Service) setError(code string, cause error) Status {
	status := errorStatus(s.GetStatus(), code, cause)
	s.commitStatus(status)
	return status
}

// currentVersion stays put until the update is actually applied, so a
// periodic check keeps reporting the ready version as "newer": the ready
// artifact and path must survive it, or state.json loses them and the
// downloaded installer gets deleted by the next cleanupCache.
func (s *Service) buildCheckStatus(current Status, readyPath string, readyArtifact *Artifact, m Manifest, art Artifact, artErr error, newer bool, newerErr error) (Status, stored, error) {
	switch {
	case artErr != nil:
		return errorStatus(current, "artifact", artErr), stored{}, artErr
	case newerErr != nil:
		return errorStatus(current, "version", newerErr), stored{}, newerErr
	}

	if newer && current.State == StateReady && current.AvailableVersion == m.Version &&
		readyArtifact != nil && *readyArtifact == art {
		st := stored{
			AvailableVersion: m.Version,
			Notes:            m.Notes,
			PublishedAt:      m.PublishedAt,
			CheckedAt:        time.Now(),
			Artifact:         readyArtifact,
			ReadyPath:        readyPath,
		}
		status := Status{
			State:            StateReady,
			CurrentVersion:   s.currentVersion,
			AvailableVersion: st.AvailableVersion,
			Notes:            st.Notes,
			PublishedAt:      st.PublishedAt,
			CheckedAt:        st.CheckedAt,
			TotalBytes:       art.Size,
			DownloadedBytes:  art.Size,
		}
		return status, st, nil
	}

	// A version that is not newer is not an update: recording it makes the
	// next start derive StateAvailable for the build already running.
	st := stored{CheckedAt: time.Now()}
	state := StateIdle
	if newer {
		st.AvailableVersion = m.Version
		st.Notes = m.Notes
		st.PublishedAt = m.PublishedAt
		state = StateAvailable
	}
	status := Status{
		State:            state,
		CurrentVersion:   s.currentVersion,
		AvailableVersion: st.AvailableVersion,
		Notes:            st.Notes,
		PublishedAt:      st.PublishedAt,
		CheckedAt:        st.CheckedAt,
	}
	return status, st, nil
}

func (s *Service) CheckForUpdate(ctx context.Context) (Status, error) {
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		return Status{}, ErrBusy
	}
	s.busy = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.busy = false
		s.mu.Unlock()
	}()

	m, err := s.client.FetchManifest(ctx)
	if err != nil {
		return s.setError("manifest", err), err
	}

	art, artErr := m.ArtifactFor(runtime.GOOS, runtime.GOARCH)
	var newer bool
	var newerErr error
	if artErr == nil {
		newer, newerErr = IsNewer(m.Version, s.currentVersion)
	}

	s.mu.Lock()
	current := s.status
	readyPath := s.readyPath
	readyArtifact := s.readyArtifact
	s.mu.Unlock()

	status, newStored, buildErr := s.buildCheckStatus(current, readyPath, readyArtifact, m, art, artErr, newer, newerErr)
	if buildErr != nil {
		s.commitStatus(status)
		return status, buildErr
	}

	if err := s.store.Save(newStored); err != nil {
		return Status{}, err
	}

	s.mu.Lock()
	s.status = status
	if newer {
		s.pendingArtifact = &art
		s.pendingVersion = m.Version
	} else {
		s.pendingArtifact = nil
		s.pendingVersion = ""
	}
	if newStored.Artifact == nil && (s.readyPath != "" || s.readyArtifact != nil) {
		// This check did not confirm the previously ready artifact (different
		// version, different bytes, or no update at all anymore): the freshly
		// saved stored record no longer carries it, so the in-memory pointers
		// must not keep pointing at it either.
		s.readyPath = ""
		s.readyArtifact = nil
	}
	s.mu.Unlock()
	emit(eventStatus, status)
	return status, nil
}

func (s *Service) DownloadUpdate(ctx context.Context) (Status, error) {
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		return Status{}, ErrBusy
	}
	if s.status.State == StateReady {
		status := s.status
		s.mu.Unlock()
		return status, nil
	}
	if s.pendingArtifact == nil {
		status := errorStatus(s.status, "not-checked", errNoUpdateChecked)
		s.status = status
		s.mu.Unlock()
		emit(eventStatus, status)
		return status, errNoUpdateChecked
	}
	s.busy = true
	art := *s.pendingArtifact
	version := s.pendingVersion
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.busy = false
		s.mu.Unlock()
	}()

	destDir, err := VersionDir(s.dir, version)
	if err == nil {
		err = os.MkdirAll(destDir, 0o755)
	}
	if err != nil {
		return s.setError("download", err), err
	}

	onProgress := func(downloaded int64) {
		emit(eventProgress, Progress{Version: version, TotalBytes: art.Size, DownloadedBytes: downloaded})
	}

	path, err := s.client.Download(ctx, art, destDir, onProgress)
	if err != nil {
		return s.setError("download", err), err
	}

	current := s.GetStatus()
	newStored := stored{
		AvailableVersion: current.AvailableVersion,
		Notes:            current.Notes,
		PublishedAt:      current.PublishedAt,
		CheckedAt:        current.CheckedAt,
		Artifact:         &art,
		ReadyPath:        path,
	}
	if err := s.store.Save(newStored); err != nil {
		return Status{}, err
	}

	s.mu.Lock()
	s.status.State = StateReady
	s.status.TotalBytes = art.Size
	s.status.DownloadedBytes = art.Size
	s.readyPath = path
	s.readyArtifact = &art
	status := s.status
	s.mu.Unlock()
	emit(eventStatus, status)
	return status, nil
}

func (s *Service) ApplyUpdate() error {
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		return ErrBusy
	}
	if s.status.State != StateReady {
		s.mu.Unlock()
		return ErrNotReady
	}
	readyPath := s.readyPath
	version := s.status.AvailableVersion
	s.busy = true
	s.status.State = StateApplying
	applying := s.status
	s.mu.Unlock()
	emit(eventStatus, applying)

	rollback := func(cause error) error {
		s.mu.Lock()
		s.busy = false
		s.status.State = StateReady
		status := s.status
		s.mu.Unlock()
		emit(eventStatus, status)
		return cause
	}

	exe, err := os.Executable()
	if err != nil {
		return rollback(err)
	}

	// The worker must not run from the binary the installer replaces: Windows
	// keeps a running image locked, NSIS then skips the file, still exits 0,
	// and the launcher comes back on the version it started from.
	workerPath, err := s.stageWorker(exe)
	if err != nil {
		return rollback(err)
	}

	spec := updateSpec{
		InstallerPath: readyPath,
		ParentPID:     os.Getpid(),
		RelaunchPath:  exe,
		Version:       version,
	}

	specPath, err := SpecPath(s.dir)
	if err != nil {
		return rollback(err)
	}
	if err := writeUpdateSpec(specPath, spec); err != nil {
		return rollback(err)
	}

	if err := startWorker(workerPath, specPath); err != nil {
		return rollback(err)
	}

	if a := application.Get(); a != nil {
		a.Quit()
	}
	return nil
}

func (s *Service) DismissUpdate() error {
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		return ErrBusy
	}
	readyPath := s.readyPath
	if readyPath == "" {
		s.mu.Unlock()
		return ErrNotReady
	}
	s.busy = true
	current := s.status
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.busy = false
		s.mu.Unlock()
	}()

	newStored := stored{
		AvailableVersion: current.AvailableVersion,
		Notes:            current.Notes,
		PublishedAt:      current.PublishedAt,
		CheckedAt:        current.CheckedAt,
	}
	if err := s.store.Save(newStored); err != nil {
		return err
	}

	s.mu.Lock()
	if current.AvailableVersion != "" {
		s.status.State = StateAvailable
	} else {
		s.status.State = StateIdle
	}
	s.readyPath = ""
	s.readyArtifact = nil
	status := s.status
	s.mu.Unlock()
	emit(eventStatus, status)

	if err := os.Remove(readyPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
