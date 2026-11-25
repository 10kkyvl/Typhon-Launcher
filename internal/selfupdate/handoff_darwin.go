//go:build darwin && !devmock

package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func handoffMethodFor(art Artifact, _ bool) handoffMethod {
	if art.Kind == KindBundle {
		return methodBundle
	}
	return ""
}

// The swap replaces the whole bundle, not the executable inside it.
func handoffTargetFor(exe string, _ handoffMethod) (string, error) { return bundleOf(exe) }

func handoffOldDir(exe string) string {
	if bundle, err := bundleOf(exe); err == nil {
		return filepath.Dir(bundle)
	}
	return filepath.Dir(exe)
}

func handoffProcAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setsid: true} }

// swapBundleParked unpacks the new bundle beside the current one, moves it to
// the parked name and exchanges the two in one atomic call: the old bundle
// stays on disk, at the parked name, until the successor has taken over. The
// bundle is signed at build time and is not touched here.
func swapBundleParked(ctx context.Context, p handoffPlan, j handoffJournal) (func() error, error) {
	parent := filepath.Dir(p.Target)
	if err := validateInstallDir(parent); err != nil {
		return nil, err
	}
	// The bundle this launcher runs from, identified before anything moves:
	// every undo acts on it by this identity, not on a name.
	running, err := os.Lstat(p.Target)
	if err != nil {
		return nil, fmt.Errorf("selfupdate: look at the running bundle: %w", err)
	}
	staging, err := os.MkdirTemp(parent, ".typhon-update-")
	if err != nil {
		return nil, fmt.Errorf("selfupdate: unpack dir: %w", err)
	}
	defer func() {
		if rerr := os.RemoveAll(staging); rerr != nil {
			slog.Warn("selfupdate: remove the unpack dir", "path", staging, "error", rerr)
		}
	}()
	staged, err := extractBundle(p.ArtifactPath, staging)
	if err != nil {
		return nil, err
	}
	dropQuarantine(ctx, staged)
	if err := handoffRename(staged, j.OldPath); err != nil {
		return nil, fmt.Errorf("selfupdate: move the new bundle beside the old one: %w", err)
	}
	undo := bundleUndo(j, running)
	if err := unix.RenamexNp(j.OldPath, p.Target, unix.RENAME_SWAP); err != nil {
		cause := fmt.Errorf("selfupdate: swap the bundles: %w", err)
		// Nothing was exchanged: the parked name holds the new bundle, and
		// removing it is all the undo there is.
		if rerr := undo(); rerr != nil {
			return undo, errors.Join(cause, rerr)
		}
		return nil, cause
	}
	return undo, nil
}

// bundleUndo returns the undo of a bundle swap. It may be called again after
// it failed (at shutdown, before the next attempt) and decides every time from
// the disk which of the two bundles is the one this launcher ran from; it also
// remembers that it has exchanged them back, and acts only when the disk
// agrees.
func bundleUndo(j handoffJournal, running fs.FileInfo) func() error {
	exchangedBack := false
	return func() error {
		exchanged, err := undoBundleSwap(j, running, exchangedBack)
		exchangedBack = exchangedBack || exchanged
		return err
	}
}

// undoBundleSwap puts the bundle this launcher ran from (running) back at the
// launcher's name and removes the other one from the parked name. With
// nothing parked there is nothing to undo. A parked name that does not hold
// running is exchanged only when the launcher's name does not hold it either
// and the bundles have not been exchanged back before; anything else is a
// state it cannot vouch for, and it removes nothing.
func undoBundleSwap(j handoffJournal, running fs.FileInfo, exchangedBack bool) (exchanged bool, err error) {
	parked, err := os.Lstat(j.OldPath)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("selfupdate: look for the parked bundle: %w", err)
	}
	atTarget, err := os.Lstat(j.Target)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, fmt.Errorf("selfupdate: look at the launcher's bundle: %w", err)
	}
	targetRuns := err == nil && os.SameFile(running, atTarget)
	switch {
	case targetRuns:
	case os.SameFile(running, parked) && !exchangedBack:
		if err := unix.RenamexNp(j.OldPath, j.Target, unix.RENAME_SWAP); err != nil {
			return false, fmt.Errorf("selfupdate: swap the bundles back: %w", err)
		}
		exchanged = true
	default:
		return false, fmt.Errorf("selfupdate: neither %s nor %s is the bundle this launcher ran from; nothing is removed", j.Target, j.OldPath)
	}
	if err := os.RemoveAll(j.OldPath); err != nil {
		return exchanged, fmt.Errorf("selfupdate: remove the new bundle: %w", err)
	}
	return exchanged, nil
}

// handoffOutput leaves the successor without the old launcher's output: a
// released launcher writes its own log file.
func handoffOutput(*exec.Cmd) {}
