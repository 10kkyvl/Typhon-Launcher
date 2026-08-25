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
			if keepVersion != "" && d.Name() == keepVersion {
				return nil
			}
			if err := root.RemoveAll(relPath); err != nil {
				return err
			}
			return fs.SkipDir
		case depth == 1:
			if d.Name() == "state.json" {
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

	st := stored{
		AvailableVersion: m.Version,
		Notes:            m.Notes,
		PublishedAt:      m.PublishedAt,
		CheckedAt:        time.Now(),
	}
	state := StateIdle
	if newer {
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
	s.busy = true
	s.status.State = StateApplying
	s.mu.Unlock()

	rollback := func() {
		s.mu.Lock()
		s.busy = false
		s.status.State = StateReady
		s.mu.Unlock()
	}

	exe, err := os.Executable()
	if err != nil {
		rollback()
		return err
	}

	spec := updateSpec{InstallerPath: readyPath, ParentPID: os.Getpid(), RelaunchPath: exe}

	cacheDir, err := CacheDir(s.dir)
	if err != nil {
		rollback()
		return err
	}
	specPath := filepath.Join(cacheDir, "update-spec.json")
	if err := writeUpdateSpec(specPath, spec); err != nil {
		rollback()
		return err
	}

	if err := startUpdateWorker(exe, specPath); err != nil {
		rollback()
		return err
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
