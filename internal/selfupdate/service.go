package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"time"

	"typhon/internal/app"
	"typhon/internal/settings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	eventStatus       = "launcher:update_status"
	eventProgress     = "launcher:update_progress"
	eventReleaseNotes = "launcher:release_notes"
)

var checkInterval = 6 * time.Hour

var errNoUpdateChecked = errors.New("selfupdate: check for an update before downloading")

// startWorker is a seam for tests: the real one spawns a detached process.
var startWorker = startUpdateWorker

// onCheckJoined lets a test observe a second caller attaching to the check in
// flight, the moment that decides whether it shares one request or starts its own.
var onCheckJoined = func() {}

type Service struct {
	mu     sync.Mutex
	dir    string
	client *Client
	store  *Store
	notes  *notesStore
	status Status
	// busy covers download, apply and dismiss. A manifest check is tracked
	// separately in check so a user action can cancel it instead of bouncing off.
	busy           bool
	check          *checkCall
	downloadCancel context.CancelFunc
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup

	currentVersion  string
	pendingArtifact *Artifact
	pendingVersion  string
	readyPath       string
	readyArtifact   *Artifact
	outcome         Outcome
	notesState      notesState
}

func NewService() (*Service, error) {
	dir, err := settings.ConfigDir()
	if err != nil {
		return nil, fmt.Errorf("selfupdate: resolve config dir: %w", err)
	}
	base, err := manifestBaseURL()
	if err != nil {
		return nil, fmt.Errorf("selfupdate: resolve manifest url: %w", err)
	}
	client, err := NewClient(base)
	if err != nil {
		return nil, fmt.Errorf("selfupdate: create client: %w", err)
	}
	store, err := NewStore(dir)
	if err != nil {
		return nil, fmt.Errorf("selfupdate: create store: %w", err)
	}
	notes, err := newNotesStore(dir)
	if err != nil {
		return nil, fmt.Errorf("selfupdate: create release notes store: %w", err)
	}
	return &Service{
		dir:            dir,
		client:         client,
		store:          store,
		notes:          notes,
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

	// A release-notes file this launcher cannot read costs the user a
	// changelog, not the ability to update: the store switches to read-only
	// and every later save reports why.
	notes, notesErr := s.notes.Load()
	if notesErr != nil {
		slog.Warn("selfupdate: load release notes", "error", notesErr)
	}
	s.mu.Lock()
	s.notesState = notes
	s.mu.Unlock()

	// Best effort on purpose: this only tells a future installer run where the
	// launcher lives, and a registry it cannot write is no reason to refuse to
	// start. Updates still carry the directory on the installer command line.
	if err := recordInstallDir(); err != nil {
		slog.Warn("record install dir", "error", err)
	}

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
	if s.status.State == StateReady || s.status.State == StateAvailable {
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
	download := s.downloadCancel
	s.mu.Unlock()
	if download != nil {
		download()
	}
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
			if d.Name() == "state.json" || d.Name() == outcomeName || d.Name() == notesName {
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
	if _, err := s.runCheck(s.ctx, false); err != nil {
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

// checkCall is one manifest check in flight. Every caller that arrives
// while it runs joins it instead of starting a second request, and a user
// action that needs the service cancels it rather than waiting it out.
type checkCall struct {
	done   chan struct{}
	cancel context.CancelFunc
	loud   bool
	status Status
	err    error
}

func clearError(status Status) Status {
	status.Error = ""
	status.ErrorCode = ""
	return status
}

func (s *Service) CheckForUpdate(ctx context.Context) (Status, error) {
	return s.runCheck(ctx, true)
}

// runCheck serialises manifest checks. The periodic check is quiet: nobody
// asked for it, so a blocked network must not paint the banner red and hide
// an update that is already downloaded. A loud caller joining a quiet check
// turns it loud, since that caller is now waiting for the answer.
func (s *Service) runCheck(ctx context.Context, loud bool) (Status, error) {
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		return Status{}, ErrBusy
	}
	if call := s.check; call != nil {
		if loud {
			call.loud = true
		}
		s.mu.Unlock()
		onCheckJoined()
		select {
		case <-call.done:
			return call.status, call.err
		case <-ctx.Done():
			return s.GetStatus(), ctx.Err()
		}
	}
	checkCtx, cancel := context.WithCancel(ctx)
	call := &checkCall{done: make(chan struct{}), cancel: cancel, loud: loud}
	s.check = call
	s.mu.Unlock()

	status, err := s.doCheck(checkCtx, call)
	cancel()

	s.mu.Lock()
	call.status, call.err = status, err
	s.check = nil
	s.mu.Unlock()
	close(call.done)
	return status, err
}

// acquire claims the service for a download, apply or dismiss. A manifest
// check still in flight is cancelled and waited out: it only reads, and the
// user's click matters more than a request that may be hanging on a blocked
// network for the next half minute.
func (s *Service) acquire() error {
	for {
		s.mu.Lock()
		if s.busy {
			s.mu.Unlock()
			return ErrBusy
		}
		call := s.check
		if call == nil {
			s.busy = true
			s.mu.Unlock()
			return nil
		}
		s.mu.Unlock()
		call.cancel()
		<-call.done
	}
}

func (s *Service) release() {
	s.mu.Lock()
	s.busy = false
	s.mu.Unlock()
}

func (s *Service) doCheck(ctx context.Context, call *checkCall) (Status, error) {
	// A cancelled check was preempted by the user or by shutdown: nobody is
	// waiting for a failure, and a quiet failure belongs in the log only.
	fail := func(code string, err error) (Status, error) {
		if ctx.Err() != nil {
			return s.GetStatus(), err
		}
		s.mu.Lock()
		loud := call.loud
		s.mu.Unlock()
		if !loud {
			return s.GetStatus(), err
		}
		return s.setError(code, err), err
	}

	m, err := s.client.FetchManifest(ctx)
	if err != nil {
		return fail("manifest", err)
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
		return fail(status.ErrorCode, buildErr)
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

	if err := s.storeReleaseNotes(m.Releases); err != nil {
		return status, err
	}
	return status, nil
}

func (s *Service) DownloadUpdate(ctx context.Context) (Status, error) {
	if err := s.acquire(); err != nil {
		return Status{}, err
	}
	defer s.release()

	s.mu.Lock()
	if s.status.State == StateReady {
		// The installer is already on disk: whatever went wrong before this
		// click is over, and an error left in the status would keep the UI on
		// the failure banner with no way to install.
		status := clearError(s.status)
		changed := status != s.status
		s.status = status
		s.mu.Unlock()
		if changed {
			emit(eventStatus, status)
		}
		return status, nil
	}
	if s.pendingArtifact == nil {
		status := errorStatus(s.status, "not-checked", errNoUpdateChecked)
		s.status = status
		s.mu.Unlock()
		emit(eventStatus, status)
		return status, errNoUpdateChecked
	}
	art := *s.pendingArtifact
	version := s.pendingVersion
	base := clearError(s.status)
	base.State = StateAvailable
	base.TotalBytes = 0
	base.DownloadedBytes = 0
	downloading := base
	downloading.State = StateDownloading
	downloading.TotalBytes = art.Size
	s.status = downloading
	dlCtx, cancel := context.WithCancel(ctx)
	s.downloadCancel = cancel
	s.mu.Unlock()
	defer func() {
		cancel()
		s.mu.Lock()
		s.downloadCancel = nil
		s.mu.Unlock()
	}()
	emit(eventStatus, downloading)

	failed := func(err error) (Status, error) {
		// Cancelled by the user, by shutdown or by the caller going away:
		// not a failure anyone needs to see.
		if dlCtx.Err() != nil {
			s.commitStatus(base)
			return base, err
		}
		status := errorStatus(base, "download", err)
		s.commitStatus(status)
		return status, err
	}

	destDir, err := VersionDir(s.dir, version)
	if err == nil {
		err = os.MkdirAll(destDir, 0o755)
	}
	if err != nil {
		return failed(err)
	}

	onProgress := func(downloaded int64) {
		emit(eventProgress, Progress{Version: version, TotalBytes: art.Size, DownloadedBytes: downloaded})
	}

	path, err := s.client.Download(dlCtx, art, destDir, onProgress)
	if err != nil {
		return failed(err)
	}

	newStored := stored{
		AvailableVersion: base.AvailableVersion,
		Notes:            base.Notes,
		PublishedAt:      base.PublishedAt,
		CheckedAt:        base.CheckedAt,
		Artifact:         &art,
		ReadyPath:        path,
	}
	if err := s.store.Save(newStored); err != nil {
		return failed(err)
	}

	ready := base
	ready.State = StateReady
	ready.TotalBytes = art.Size
	ready.DownloadedBytes = art.Size
	s.mu.Lock()
	s.status = ready
	s.readyPath = path
	s.readyArtifact = &art
	s.mu.Unlock()
	emit(eventStatus, ready)
	return ready, nil
}

func (s *Service) CancelDownload() error {
	s.mu.Lock()
	cancel := s.downloadCancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

func (s *Service) ApplyUpdate() error {
	if err := s.acquire(); err != nil {
		return err
	}
	s.mu.Lock()
	if s.status.State != StateReady {
		s.busy = false
		s.mu.Unlock()
		return ErrNotReady
	}
	readyPath := s.readyPath
	version := s.status.AvailableVersion
	s.status = clearError(s.status)
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

	// The installer has to land on the installation this launcher runs from.
	// Left to itself a silent NSIS run uses the directory compiled into it, so
	// anyone who installed elsewhere would get a second copy while the running
	// one stayed on the old build.
	installDir := filepath.Dir(exe)
	if err := validateInstallDir(installDir); err != nil {
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
		InstallDir:    installDir,
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
	if err := s.acquire(); err != nil {
		return err
	}
	defer s.release()

	s.mu.Lock()
	readyPath := s.readyPath
	if readyPath == "" {
		s.mu.Unlock()
		return ErrNotReady
	}
	current := s.status
	s.mu.Unlock()

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
	s.status = clearError(s.status)
	s.status.TotalBytes = 0
	s.status.DownloadedBytes = 0
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

// storeReleaseNotes runs after the status is committed on purpose: a
// changelog that cannot be written must not turn a working update check into
// a failed one, but the caller still hears about it.
func (s *Service) storeReleaseNotes(incoming []ReleaseNote) error {
	s.mu.Lock()
	current := s.notesState
	currentVersion := s.currentVersion
	s.mu.Unlock()

	merged, err := mergeReleaseNotes(current.Releases, incoming)
	if err != nil {
		return fmt.Errorf("selfupdate: merge release notes: %w", err)
	}
	next := notesState{Releases: merged, LastSeenVersion: current.LastSeenVersion}
	// The first successful check is where "everything up to this build has
	// been seen" becomes true. Without it a fresh install has nothing to
	// compare against and the first update would show no changelog at all.
	if next.LastSeenVersion == "" {
		next.LastSeenVersion = currentVersion
	}
	if reflect.DeepEqual(current, next) {
		return nil
	}
	if err := s.notes.Save(next); err != nil {
		return err
	}

	s.mu.Lock()
	s.notesState = next
	s.mu.Unlock()
	s.emitReleaseNotes()
	return nil
}

func (s *Service) releaseNotesView() (ReleaseNotes, error) {
	s.mu.Lock()
	notes := s.notesState
	currentVersion := s.currentVersion
	s.mu.Unlock()

	unseen, err := unseenReleaseNotes(notes.Releases, notes.LastSeenVersion, currentVersion)
	if err != nil {
		return ReleaseNotes{}, fmt.Errorf("selfupdate: select unseen release notes: %w", err)
	}
	return ReleaseNotes{
		CurrentVersion: currentVersion,
		Unseen:         unseen,
		History:        notes.Releases,
	}, nil
}

func (s *Service) emitReleaseNotes() {
	view, err := s.releaseNotesView()
	if err != nil {
		slog.Warn("selfupdate: release notes event", "error", err)
		return
	}
	emit(eventReleaseNotes, view)
}

func (s *Service) GetReleaseNotes() (ReleaseNotes, error) {
	return s.releaseNotesView()
}

func (s *Service) AcknowledgeReleaseNotes() error {
	s.mu.Lock()
	current := s.notesState
	next := current
	next.LastSeenVersion = s.currentVersion
	s.mu.Unlock()

	if next.LastSeenVersion == current.LastSeenVersion {
		return nil
	}
	if err := s.notes.Save(next); err != nil {
		return err
	}

	s.mu.Lock()
	s.notesState = next
	s.mu.Unlock()
	s.emitReleaseNotes()
	return nil
}
