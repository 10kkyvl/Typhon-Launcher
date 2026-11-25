package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"typhon/internal/windowhandoff"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	// errorCodeHandoffFailed is the status error code of a handoff that was
	// rolled back; the launcher keeps running on its previous version.
	errorCodeHandoffFailed = "handoff_failed"

	// errorCodeHandoffUndoFailed is a hand-off that failed and could not put
	// the previous launcher back. No update can start until the next launch
	// has settled it, so the screen offers neither retry nor restart.
	errorCodeHandoffUndoFailed = "handoff_undo_failed"

	// handoffKillWait is how long the old process waits for a killed
	// successor to release its image before it undoes the swap.
	handoffKillWait = 5 * time.Second
)

var (
	// handoffReadyTimeout is N in the design: the successor has this long to
	// render its window after it starts.
	handoffReadyTimeout = 20 * time.Second
	handoffPollInterval = 50 * time.Millisecond

	// Seams for tests: the successor process, the foreground right, the
	// window geometry and the exit.
	startHandoff    = startHandoffProcess
	allowForeground = windowhandoff.AllowForeground
	yieldForeground = windowhandoff.YieldForeground
	captureGeometry = windowhandoff.Capture
	quitApplication = func() {
		if a := application.Get(); a != nil {
			a.Quit()
		}
	}
	handoffSaveCurrent = snapshotPrevious
	methodFor          = handoffMethodFor
	targetFor          = handoffTargetFor
)

// handoffProc is the successor as the old process sees it.
type handoffProc interface {
	Pid() int
	Kill() error
	// Done is closed when the process has exited.
	Done() <-chan struct{}
}

type execProc struct {
	cmd  *exec.Cmd
	done chan struct{}
}

func (p *execProc) Pid() int              { return p.cmd.Process.Pid }
func (p *execProc) Kill() error           { return p.cmd.Process.Kill() }
func (p *execProc) Done() <-chan struct{} { return p.done }

func startHandoffProcess(exe string, args []string) (handoffProc, error) {
	env, err := workerEnvironment()
	if err != nil {
		return nil, err
	}
	//nolint:gosec // G204: exe is the launcher's own path just replaced by the verified build, args are generated here (invariant 33)
	cmd := exec.Command(exe, args...)
	cmd.Env = env
	cmd.SysProcAttr = handoffProcAttr()
	handoffOutput(cmd)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start the successor launcher: %w", err)
	}
	p := &execProc{cmd: cmd, done: make(chan struct{})}
	go func() {
		defer close(p.done)
		if err := cmd.Wait(); err != nil {
			slog.Debug("successor launcher exited", "error", err)
		}
	}()
	return p, nil
}

// HandoffInProgress reports that this launcher is handing itself over; second
// launches that reach it meanwhile are left for the successor.
//
//wails:ignore
func (s *Service) HandoffInProgress() bool { return s.handoffRunning.Load() }

// SetHandoffWindow gives the service the main window: the old process reads
// its geometry, the successor reveals it.
//
//wails:ignore
func (s *Service) SetHandoffWindow(w *application.WebviewWindow) {
	s.mu.Lock()
	s.handoffWindow = w
	s.mu.Unlock()
}

// SetHandoffBackground tells the service where to read the page colour of the
// current theme (#rrggbb) to pass to the successor.
//
//wails:ignore
func (s *Service) SetHandoffBackground(f func() string) {
	s.mu.Lock()
	s.handoffBackground = f
	s.mu.Unlock()
}

func (s *Service) serviceContext() context.Context {
	s.mu.Lock()
	ctx := s.ctx
	s.mu.Unlock()
	if ctx != nil {
		return ctx
	}
	//nolint:forbidigo // a Service used before ServiceStartup (tests) has no ctx; the handoff must not run on a nil one
	return context.Background()
}

// BeginHandoff updates without a visible restart: it swaps the launcher on
// disk, starts the new build beside this one and, once the new window is on
// screen, quits. phaseStart is when the update screen began animating (unix
// ms) and is carried to the successor so the animation continues. Unlike
// ApplyUpdate it takes no language: there is no worker window to translate,
// and the successor reads the language from the settings like any start.
//
// An error means nothing was changed and ApplyUpdate, the worker path that
// restarts the launcher, is still available; ErrHandoffUnsupported (code
// selfupdate.handoff_unsupported) says this build, platform or installation
// cannot hand over at all. After a nil return the outcome arrives as status
// events: the process exits on success, and on failure the swap is undone and
// the state is ready again with error code handoff_failed.
func (s *Service) BeginHandoff(phaseStart int64) error {
	if err := s.acquire(); err != nil {
		return err
	}
	plan, err := s.prepareHandoff()
	if err != nil {
		s.release()
		return err
	}

	s.mu.Lock()
	s.status = clearError(s.status)
	s.status.State = StateApplying
	applying := s.status
	s.mu.Unlock()
	emit(eventStatus, applying)
	s.handoffRunning.Store(true)

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.runHandoff(s.serviceContext(), plan, phaseStart)
	}()
	return nil
}

// prepareHandoff decides how the swap will go and verifies the download. It
// changes nothing on disk.
func (s *Service) prepareHandoff() (handoffPlan, error) {
	s.mu.Lock()
	state := s.status.State
	readyPath, art, version := s.readyPath, s.readyArtifact, s.status.AvailableVersion
	s.mu.Unlock()
	if state != StateReady || art == nil {
		return handoffPlan{}, ErrNotReady
	}
	if err := s.refuseUnsettledHandoff(); err != nil {
		return handoffPlan{}, err
	}
	isExe := art.Kind == KindExe
	if !isExe && s.portable != (art.Kind == KindPortable) {
		return handoffPlan{}, ErrNotReady
	}
	method := methodFor(*art, s.portable)
	if method == "" {
		return handoffPlan{}, ErrHandoffUnsupported
	}
	if _, _, err := artifactRel(s.dir, readyPath); err != nil {
		return handoffPlan{}, err
	}
	exe, err := currentExecutable()
	if err != nil {
		return handoffPlan{}, fmt.Errorf("selfupdate: resolve the running launcher: %w", err)
	}
	target, err := targetFor(exe, method)
	if err != nil {
		return handoffPlan{}, err
	}
	installDir := filepath.Dir(exe)
	if method == methodInstaller {
		// NSIS reads the directory off its command line; a path it would read
		// differently is the worker's business.
		if err := validateInstallDir(installDir); err != nil {
			return handoffPlan{}, err
		}
	}
	// The hash is checked again here, right before anything changes: a file
	// altered in the cache since it was downloaded must not reach the swap.
	if err := VerifyFile(s.serviceContext(), readyPath, *art); err != nil {
		return handoffPlan{}, err
	}
	nonce, err := newNonce()
	if err != nil {
		return handoffPlan{}, err
	}
	return handoffPlan{
		Method: method, Exe: exe, Target: target, ArtifactPath: readyPath, Art: *art,
		Portable: s.portable, InstallDir: installDir, Nonce: nonce,
		Version: version, FromVersion: s.currentVersion,
	}, nil
}

// runHandoff is the old process's side from journal to exit. Every failure
// ends in failHandoff, which undoes whatever was done.
func (s *Service) runHandoff(ctx context.Context, plan handoffPlan, phaseStart int64) {
	var proc handoffProc
	var undo func() error
	staged := false
	j := plan.journalFor(os.Getpid())

	fail := func(cause error) {
		s.failHandoff(plan, j, proc, undo, staged, cause)
	}

	readyPath, err := HandoffReadyPath(s.dir, plan.Nonce)
	if err != nil {
		fail(err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(readyPath), cacheDirMode); err != nil {
		fail(fmt.Errorf("selfupdate: create the hand-off marker dir: %w", err))
		return
	}
	if err := clearReadyMarkers(s.dir); err != nil {
		slog.Warn("selfupdate: remove old hand-off markers", "error", err)
	}

	// The same snapshot a worker update takes, so the user can still go back
	// to this version from the new one.
	if rollbackSupported() == nil {
		if serr := handoffSaveCurrent(ctx, s.dir, plan.Exe, plan.FromVersion); serr != nil {
			slog.Warn("selfupdate: the running launcher could not be saved, updating without a rollback", "error", serr)
			if rerr := recordSnapshotFailure(s.dir, plan.FromVersion, fmt.Errorf("%w: %w", ErrSnapshotFailed, serr)); rerr != nil {
				slog.Warn("selfupdate: record the snapshot failure", "error", rerr)
			}
		} else {
			staged = true
		}
	}

	// The journal comes first: from here on the launcher on disk may differ
	// from the one running, and a start after a crash reads this to settle it.
	if err := saveJournal(s.dir, j); err != nil {
		fail(err)
		return
	}
	undo, err = performSwap(ctx, plan, j)
	if err != nil {
		// undo is nil when the swap put everything back, or the undo it
		// still owes.
		fail(err)
		return
	}
	j.Stage = stageSwapped
	if err := saveJournal(s.dir, j); err != nil {
		fail(err)
		return
	}

	spec := HandoffSpec{
		Nonce: plan.Nonce, ParentPID: os.Getpid(), PhaseStart: phaseStart, ReadyPath: readyPath,
		ConfigDir: s.dir, Version: plan.Version, FromVersion: plan.FromVersion, Background: s.themeBackground(),
	}
	s.mu.Lock()
	window := s.handoffWindow
	s.mu.Unlock()
	if window != nil {
		spec.Geometry = captureGeometry(window)
	}
	arg, err := spec.Encode()
	if err != nil {
		fail(err)
		return
	}
	proc, err = startHandoff(plan.Exe, []string{handoffFlag, arg})
	if err != nil {
		fail(err)
		return
	}
	if err := allowForeground(proc.Pid()); err != nil {
		slog.Warn("selfupdate: let the successor take the foreground", "error", err)
	}

	if err := waitHandoffReady(ctx, readyPath, proc); err != nil {
		fail(err)
		return
	}

	// The successor is on screen. A snapshot that cannot be promoted now is
	// finished by the next start (RecoverInterrupted).
	if staged {
		if cerr := commitCurrent(s.dir); cerr != nil {
			slog.Warn("selfupdate: promote the snapshot, it is finished on the next start", "error", cerr)
		}
	}
	j.Stage = stageReady
	if err := saveJournal(s.dir, j); err != nil {
		// The successor is up and the swap is complete; the successor settles
		// the journal itself, so this only costs the "ready" stage.
		slog.Warn("selfupdate: record that the successor is ready", "error", err)
	}
	if spec.Geometry.Focused {
		// The successor activates itself once this process has exited; from
		// macOS 14 that succeeds only when the active app yielded to it.
		if err := yieldForeground(proc.Pid()); err != nil {
			slog.Warn("selfupdate: let the successor take the activation", "error", err)
		}
	}
	// Quit from its own goroutine: the shutdown waits for this one.
	go quitApplication()
}

func (s *Service) themeBackground() string {
	s.mu.Lock()
	f := s.handoffBackground
	s.mu.Unlock()
	if f != nil {
		if c := f(); ValidBackground(c) {
			return c
		}
	}
	return defaultHandoffBackground
}

var errHandoffTimeout = errors.New("selfupdate: the new launcher did not show its window in time")

// waitHandoffReady polls for the marker the successor writes after it has
// rendered, and gives up when the successor dies or the time is up.
func waitHandoffReady(ctx context.Context, readyPath string, proc handoffProc) error {
	ticker := time.NewTicker(handoffPollInterval)
	defer ticker.Stop()
	deadline := time.NewTimer(handoffReadyTimeout)
	defer deadline.Stop()
	for {
		if _, err := os.Stat(readyPath); err == nil {
			return nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("selfupdate: look for the hand-off marker: %w", err)
		}
		select {
		case <-proc.Done():
			// It may have written the marker as its last act.
			if _, err := os.Stat(readyPath); err == nil {
				return nil
			}
			return errors.New("selfupdate: the new launcher exited before it showed its window")
		case <-deadline.C:
			return errHandoffTimeout
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// failHandoff takes the swap back and leaves the launcher as it was: the
// successor is stopped first (Windows will not give its image back while it
// runs), then the files are put back and the record removed. The user stays
// on the old version with the update still ready.
func (s *Service) failHandoff(plan handoffPlan, j handoffJournal, proc handoffProc, undo func() error, staged bool, cause error) {
	slog.Error("selfupdate: the hand-off failed, rolling back", "version", plan.Version, "error", cause)
	errs := []error{cause}

	if proc != nil {
		select {
		case <-proc.Done():
		default:
			if kerr := proc.Kill(); kerr != nil {
				select {
				case <-proc.Done():
				default:
					errs = append(errs, fmt.Errorf("selfupdate: stop the new launcher: %w", kerr))
				}
			}
			select {
			case <-proc.Done():
			case <-time.After(handoffKillWait):
				errs = append(errs, errors.New("selfupdate: the new launcher did not stop"))
			}
		}
	}

	undone := true
	if undo != nil {
		if uerr := undo(); uerr != nil {
			errs = append(errs, uerr)
			undone = false
		}
	}
	code := errorCodeHandoffFailed
	if undone {
		if readyPath, rerr := HandoffReadyPath(s.dir, plan.Nonce); rerr == nil {
			if rerr := removeIfExists(readyPath); rerr != nil {
				slog.Warn("selfupdate: remove the hand-off marker", "error", rerr)
			}
		}
		if cerr := clearJournal(s.dir); cerr != nil {
			errs = append(errs, cerr)
		}
		sweepOld(handoffOldDir(plan.Exe))
		if staged {
			if derr := discardStaging(s.dir); derr != nil {
				slog.Warn("selfupdate: remove the unused snapshot, it is dropped on the next start", "error", derr)
			}
		}
	} else {
		// The old launcher may be parked with nothing, or the new build, at
		// its name. Nothing is swept, the snapshot of the old build stays
		// (the next start promotes or drops it by the version it runs) and
		// the journal says the undo is owed, so no update starts on top.
		code = errorCodeHandoffUndoFailed
		j.Stage = stageUndoOwed
		if serr := saveJournal(s.dir, j); serr != nil {
			errs = append(errs, serr)
		}
		slog.Error("selfupdate: the previous launcher could not be put back, it stays parked", "parked", j.OldPath, "target", j.Target)
	}

	s.handoffRunning.Store(false)
	s.mu.Lock()
	s.busy = false
	if !undone {
		s.owedUndo = undo
	}
	status := errorStatus(s.status, code, errors.Join(errs...))
	status.State = StateReady
	s.status = status
	s.mu.Unlock()
	emit(eventStatus, status)
}

// refuseUnsettledHandoff keeps any update from starting while a hand-over
// journal is open. The version a snapshot is labelled with is the running
// one, and nothing here reads a version out of a binary: the label is true
// only while the launcher's name holds the build that runs, and an open
// journal is the one state in which it may not. An undo this process owes is
// tried first; once paid, the old build is back at the name.
func (s *Service) refuseUnsettledHandoff() error {
	if err := s.payOwedUndo(); err != nil {
		slog.Warn("selfupdate: the previous launcher is still parked", "error", err)
	}
	j, err := loadJournal(s.dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return fmt.Errorf("%w: %w", ErrHandoffPending, err)
	case j.Stage == stageDone:
		return nil
	default:
		return ErrHandoffPending
	}
}

// payOwedUndo retries, in the process that failed it, the undo a hand-off
// still owes. On success the journal goes and updates may start again.
func (s *Service) payOwedUndo() error {
	s.mu.Lock()
	undo := s.owedUndo
	s.owedUndo = nil
	s.mu.Unlock()
	if undo == nil {
		return nil
	}
	if err := undo(); err != nil {
		s.mu.Lock()
		if s.owedUndo == nil {
			s.owedUndo = undo
		}
		s.mu.Unlock()
		return err
	}
	slog.Info("selfupdate: the previous launcher is back in place")
	if derr := discardStaging(s.dir); derr != nil {
		slog.Warn("selfupdate: remove the unused snapshot, it is dropped on the next start", "error", derr)
	}
	return clearJournal(s.dir)
}
