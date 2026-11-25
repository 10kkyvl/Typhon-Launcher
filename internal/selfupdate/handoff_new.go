package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"typhon/internal/handoffgate"
	"typhon/internal/storage"
	"typhon/internal/windowhandoff"
)

// eventHandoffDone tells the update screen that the old launcher is gone and
// this one owns the installation.
const eventHandoffDone = "launcher:handoff_done"

// Seams for tests: the reveal and the step to an ordinary application.
var (
	revealWindow = windowhandoff.Reveal
	settleWindow = windowhandoff.Settle
)

// handoffSettleTimeout bounds the wait for held-back services to start.
const handoffSettleTimeout = 30 * time.Second

// eventHandoffWaiting tells the update screen that the old launcher is still
// shutting down handoffSlowAfter after this window was revealed.
const eventHandoffWaiting = "launcher:handoff_waiting"

// handoffSlowAfter is how long after the reveal an old launcher still
// shutting down is shown as a wait. A variable for the tests only.
var handoffSlowAfter = 10 * time.Second

var errNoHandoffWindow = errors.New("selfupdate: no window to reveal")

// SetHandoff marks this launcher as the successor in a seamless update.
//
//wails:ignore
func (s *Service) SetHandoff(spec HandoffSpec) {
	s.mu.Lock()
	s.handoffSpec = &spec
	s.mu.Unlock()
}

// HandoffInfo is polled by the update screen. In a normal launcher, and in a
// successor once it has taken over, Active is false.
func (s *Service) HandoffInfo() HandoffInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	spec := s.handoffSpec
	if spec == nil {
		return HandoffInfo{}
	}
	bg := spec.Background
	if !ValidBackground(bg) {
		bg = defaultHandoffBackground
	}
	return HandoffInfo{Active: !s.handoffFinished, PhaseStart: spec.PhaseStart, Version: spec.Version, Background: bg, Waiting: s.handoffWaiting}
}

// HandoffRendered is called once by the update screen after it has really
// painted (mounted and two animation frames). It shows the prepared window in
// the old one's place and only then tells the old process it may close, so a
// window that failed to appear never costs the user the old launcher.
func (s *Service) HandoffRendered() error {
	s.renderOnce.Do(func() { s.renderErr = s.reveal() })
	return s.renderErr
}

func (s *Service) reveal() error {
	s.mu.Lock()
	spec, window := s.handoffSpec, s.handoffWindow
	s.mu.Unlock()
	if spec == nil {
		return errors.New("selfupdate: this launcher was not started by a hand-off")
	}
	if window == nil {
		return errNoHandoffWindow
	}
	if err := revealWindow(window, spec.Geometry.Focused); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(spec.ReadyPath), cacheDirMode); err != nil {
		return fmt.Errorf("selfupdate: create the hand-off marker dir: %w", err)
	}
	if err := storage.WriteAtomic(spec.ReadyPath, []byte(spec.Nonce)); err != nil {
		return fmt.Errorf("selfupdate: tell the old launcher the window is up: %w", err)
	}
	s.mu.Lock()
	s.handoffRevealed = true
	s.handoffRevealedAt = time.Now()
	s.mu.Unlock()
	s.settleIfDue()
	return nil
}

// settleIfDue makes the successor an ordinary application, once, when both its
// window is on screen and the old launcher is gone, whichever comes last.
// Before the reveal there is nothing to activate, and the launch-time
// activation policy is still being applied by the window toolkit.
func (s *Service) settleIfDue() {
	s.mu.Lock()
	due := s.handoffRevealed && s.handoffOldGone && !s.handoffSettled
	if due {
		s.handoffSettled = true
	}
	spec, window := s.handoffSpec, s.handoffWindow
	s.mu.Unlock()
	if !due || spec == nil || window == nil {
		return
	}
	if err := settleWindow(window, spec.Geometry.Focused); err != nil {
		slog.Warn("selfupdate: make the launcher an ordinary application after the hand-off", "error", err)
	}
}

// CompleteHandoff runs in the successor once its window is up or coming. It
// waits for the old process to exit, takes over the single-instance role
// through takeover, lets the held-back services start, shows its window if
// the page has not, becomes an ordinary application (settleIfDue), settles
// the journal and removes the parked old launcher.
//
//wails:ignore
func (s *Service) CompleteHandoff(ctx context.Context, takeover func() error) error {
	s.mu.Lock()
	spec := s.handoffSpec
	s.mu.Unlock()
	if spec == nil {
		return errors.New("selfupdate: this launcher was not started by a hand-off")
	}
	if err := s.waitForOldLauncher(ctx, spec.ParentPID); err != nil {
		return err
	}
	if err := takeover(); err != nil {
		return fmt.Errorf("selfupdate: take over the single instance: %w", err)
	}
	handoffgate.Open()
	s.mu.Lock()
	s.handoffOldGone = true
	s.handoffWaiting = false
	s.mu.Unlock()
	// Normally the window is up already: the old launcher left only after its
	// ready marker. One that died before it leaves this window hidden, and a
	// page that never reports its first frame would keep it so; nothing can
	// show through any more, so it is revealed now, once, as HandoffRendered
	// would, and settles there. A late HandoffRendered is then a no-op.
	if err := s.HandoffRendered(); err != nil {
		slog.Warn("selfupdate: show the window after the old launcher left", "error", err)
	}
	s.settleIfDue()

	s.finishHandoff(ctx)
	// The held-back services start in their own goroutines; the update screen
	// is told the launcher is ready only once they have.
	settleCtx, settleCancel := context.WithTimeout(ctx, handoffSettleTimeout)
	if err := handoffgate.Settled(settleCtx); err != nil {
		slog.Warn("selfupdate: services still starting after the hand-off", "error", err)
	}
	settleCancel()
	s.mu.Lock()
	s.handoffFinished = true
	s.mu.Unlock()
	emit(eventHandoffDone, s.HandoffInfo())
	return nil
}

// waitForOldLauncher waits for the old process to exit, for as long as this
// launcher runs. The old one has hidden its window and is stopping its
// services, which flush state and may sit out network timeouts; ending it
// would cut those saves short and stop the installers in its job, and giving
// up here would quit the only window the user sees. Past parentExitTimeout
// the wait is logged; past handoffSlowAfter after the reveal the update
// screen is told (markSlow).
func (s *Service) waitForOldLauncher(ctx context.Context, pid int) error {
	ticker := time.NewTicker(parentPollInterval)
	defer ticker.Stop()
	logAt := time.Now().Add(parentExitTimeout)
	logged := false
	for {
		alive, err := parentAlive(pid)
		if err != nil {
			return err
		}
		if !alive {
			return nil
		}
		now := time.Now()
		if !logged && !now.Before(logAt) {
			slog.Warn("selfupdate: the old launcher is still shutting down, waiting on", "pid", pid, "after", parentExitTimeout)
			logged = true
		}
		s.markSlow(now)
		select {
		case <-ctx.Done():
			return errParentStillRunning
		case <-ticker.C:
		}
	}
}

// markSlow records, once, that the old launcher is still shutting down
// handoffSlowAfter after this window was revealed, and tells the update
// screen. Before the reveal the old window is the one on screen.
func (s *Service) markSlow(now time.Time) {
	s.mu.Lock()
	due := s.handoffRevealed && !s.handoffWaiting && !s.handoffOldGone && now.Sub(s.handoffRevealedAt) >= handoffSlowAfter
	if due {
		s.handoffWaiting = true
	}
	s.mu.Unlock()
	if due {
		slog.Info("selfupdate: the old launcher is slow to exit, the update screen says so", "after", handoffSlowAfter)
		emit(eventHandoffWaiting, nil)
	}
}

// finishHandoff settles the journal after the old process is gone. Failures
// are logged: the next start sweeps again (RecoverHandoff).
func (s *Service) finishHandoff(ctx context.Context) {
	s.settleAfterHandoff(ctx)

	oldDir := ""
	j, err := loadJournal(s.dir)
	switch {
	case err == nil:
		j.Stage = stageDone
		if serr := saveJournal(s.dir, j); serr != nil {
			slog.Warn("selfupdate: record that the hand-off is done", "error", serr)
		}
		oldDir = filepath.Dir(j.OldPath)
	case errors.Is(err, fs.ErrNotExist):
	default:
		slog.Warn("selfupdate: read the hand-off journal", "error", err)
	}
	if oldDir == "" {
		if exe, eerr := currentExecutable(); eerr == nil {
			oldDir = handoffOldDir(exe)
		}
	}
	sweepOld(oldDir)
	if err := clearReadyMarkers(s.dir); err != nil {
		slog.Warn("selfupdate: remove hand-off markers", "error", err)
	}
	if err := clearJournal(s.dir); err != nil {
		slog.Warn("selfupdate: remove the hand-off journal", "error", err)
	}
}

// successorPending reports a successor whose old launcher has not exited yet.
func (s *Service) successorPending() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.handoffSpec != nil && !s.handoffOldGone
}

// settleAfterHandoff does what a successor's startup held back: it settles an
// interrupted snapshot, drops the record of the update that was just applied
// and sweeps the cache.
func (s *Service) settleAfterHandoff(ctx context.Context) {
	s.recoverRollback()
	v, err := s.store.Load()
	if err != nil {
		slog.Warn("selfupdate: reload state after the hand-off", "error", err)
		return
	}
	if stale, serr := s.staleForCurrent(v); serr != nil {
		slog.Warn("selfupdate: compare the stored version after the hand-off", "error", serr)
		return
	} else if stale {
		v = stored{CheckedAt: v.CheckedAt, SkippedVersion: v.SkippedVersion}
		if err := s.store.Save(v); err != nil {
			slog.Warn("selfupdate: persist cleared state after the hand-off", "error", err)
			return
		}
	}
	if err := s.cleanupCache(ctx, v.AvailableVersion); err != nil {
		slog.Warn("selfupdate: clean cache after the hand-off", "error", err)
	}
}
