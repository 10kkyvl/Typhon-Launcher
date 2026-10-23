package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"time"

	"typhon/internal/settings"
)

// The per-OS primitives behind the worker are reachable through these seams
// so the orchestration can be exercised without a real parent process, a
// real installer or a real relaunch.
var (
	parentAlive      = workerProcessAlive
	applyInstaller   = Apply
	applyPortable    = ApplyPortable
	relaunchLauncher = relaunch
	saveCurrent      = snapshotPrevious
	rollBack         = performRollback
)

// newWorkerContexts derives two independent timeout budgets from a single
// base context: waiting for the parent launcher to exit and running the
// installer must not share one clock. Sharing it meant that whenever
// waitForParentExit ate most of parentExitTimeout (antivirus holding a
// handle, a slow webview teardown), Apply/exec.CommandContext inherited
// whatever was left and could kill the installer mid-write over the
// launcher's own files, with no recovery path on the next start.
func newWorkerContexts(base context.Context) (waitCtx, applyCtx context.Context, cancel func()) {
	waitCtx, waitCancel := context.WithTimeout(base, parentExitTimeout)
	applyCtx, applyCancel := context.WithTimeout(base, applyTimeout)
	return waitCtx, applyCtx, func() {
		waitCancel()
		applyCancel()
	}
}

// stageReporter is what the user watches while the launcher is gone. Tests
// swap in silentReporter so a unit run neither pops a window nor blocks on
// one being closed.
type stageReporter interface {
	setStage(title, detail string)
	fail(title, detail string)
	wait()
	close()
}

type silentReporter struct{}

func (silentReporter) setStage(string, string) {}
func (silentReporter) fail(string, string)     {}
func (silentReporter) wait()                   {}
func (silentReporter) close()                  {}

func runWorker(specPath string, newReporter func(title, detail string) stageReporter) error {
	spec, err := readUpdateSpec(specPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := os.Remove(specPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("remove selfupdate spec", "path", specPath, "error", err)
		}
	}()

	dir, err := settings.ConfigDir()
	if err != nil {
		return fmt.Errorf("selfupdate worker: resolve config dir: %w", err)
	}
	outcomePath, err := OutcomePath(dir)
	if err != nil {
		return err
	}

	//nolint:forbidigo // RunWorker is a separate process's entry point, the worker's equivalent of main: there is no caller ctx (invariant 20 allows Background only in main). This base feeds both waitCtx (parent-exit wait) and applyCtx (installer run) below, each with its own independent timeout.
	waitCtx, applyCtx, cancel := newWorkerContexts(context.Background())
	defer cancel()

	text := workerText(spec.Language)
	rollingBack := spec.Kind == specKindRollback
	title := text.title(spec.Version)
	if rollingBack {
		title = text.rollbackTitle(spec.Version)
	}
	ui := newReporter(title, text.waiting)
	var applyErr error
	if rollingBack {
		applyErr = runRollback(waitCtx, applyCtx, dir, spec, ui)
	} else {
		applyErr = runUpdate(waitCtx, applyCtx, dir, spec, ui)
	}

	outcome := Outcome{Version: spec.Version, OK: applyErr == nil, FinishedAt: time.Now()}
	if rollingBack {
		outcome.Kind, outcome.From, outcome.To = outcomeRolledBack, spec.FromVersion, spec.Version
	}
	if applyErr != nil {
		outcome.Error = applyErr.Error()
		slog.Error("selfupdate worker: apply failed", "kind", spec.Kind, "version", spec.Version, "error", applyErr)
	}
	if err := writeOutcome(outcomePath, outcome); err != nil {
		applyErr = errors.Join(applyErr, err)
		slog.Error("selfupdate worker: record outcome", "error", err)
	}

	// A launcher that never quit is still on screen: relaunching would leave
	// the user with two of them.
	if errors.Is(applyErr, errParentStillRunning) {
		ui.fail(text.failed, text.parentRunning)
		ui.wait()
		return applyErr
	}

	switch {
	case applyErr != nil && rollingBack:
		ui.setStage(text.rollbackFailed, text.rollbackKept)
	case applyErr != nil:
		ui.setStage(text.failed, text.restoring)
	case rollingBack:
		ui.setStage(title, text.rollbackRestarting)
	default:
		ui.setStage(title, text.restarting)
	}

	if err := relaunchLauncher(spec.RelaunchPath); err != nil {
		slog.Error("selfupdate worker: relaunch failed", "path", spec.RelaunchPath, "error", err)
		ui.fail(text.launchFailed, text.openManually)
		ui.wait()
		return errors.Join(applyErr, err)
	}
	ui.close()
	return applyErr
}

func runUpdate(waitCtx, applyCtx context.Context, configDir string, spec updateSpec, ui stageReporter) error {
	if err := waitForParentExit(waitCtx, spec.ParentPID); err != nil {
		return err
	}

	text := workerText(spec.Language)
	ui.setStage(text.title(spec.Version), text.installing)

	// Taken after the parent is gone, so no state is being written, and before
	// the installer touches anything. A failed copy stops the update: an
	// update that quietly cannot be rolled back is worse than one retried.
	if rollbackSupported() == nil {
		if err := saveCurrent(applyCtx, configDir, spec.RelaunchPath, spec.FromVersion); err != nil {
			return err
		}
	}

	before, err := fileDigest(applyCtx, spec.RelaunchPath)
	if err != nil {
		return err
	}
	if spec.Portable {
		err = applyPortable(applyCtx, spec.InstallerPath, spec.RelaunchPath)
	} else {
		err = applyInstaller(applyCtx, spec.InstallerPath, spec.InstallDir, spec.RelaunchPath)
	}
	if err != nil {
		return err
	}
	after, err := fileDigest(applyCtx, spec.RelaunchPath)
	if err != nil {
		return err
	}

	return ensureReplaced(before, after, spec.RelaunchPath)
}

func runRollback(waitCtx, applyCtx context.Context, configDir string, spec updateSpec, ui stageReporter) error {
	if err := waitForParentExit(waitCtx, spec.ParentPID); err != nil {
		return err
	}
	text := workerText(spec.Language)
	ui.setStage(text.rollbackTitle(spec.Version), text.rollingBack)
	return rollBack(applyCtx, configDir, spec.RelaunchPath, spec.FromVersion, spec.Version, spec.Portable)
}

// ensureReplaced is what tells a real install from a no-op. The NSIS installer
// exits 0 even when it could not open the launcher binary for writing, which is
// exactly what happened while the worker still ran from that binary: the file
// staying byte-identical is the only signal that the install did nothing.
func ensureReplaced(before, after, path string) error {
	if after == "" {
		return fmt.Errorf("%w: %s", ErrNotReplaced, path)
	}
	if before != "" && after == before {
		return fmt.Errorf("%w: %s", ErrNotReplaced, path)
	}
	return nil
}

func waitForParentExit(ctx context.Context, pid int) error {
	ticker := time.NewTicker(parentPollInterval)
	defer ticker.Stop()
	for {
		alive, err := parentAlive(pid)
		if err != nil {
			return err
		}
		if !alive {
			return nil
		}
		select {
		case <-ctx.Done():
			return errParentStillRunning
		case <-ticker.C:
		}
	}
}
