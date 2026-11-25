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
	if err := unix.RenamexNp(j.OldPath, p.Target, unix.RENAME_SWAP); err != nil {
		return nil, errors.Join(fmt.Errorf("selfupdate: swap the bundles: %w", err), os.RemoveAll(j.OldPath))
	}
	return func() error { return undoBundleSwap(j) }, nil
}

// undoBundleSwap exchanges the bundles back and drops the new one. It is safe
// to repeat: with nothing parked there is nothing to undo.
func undoBundleSwap(j handoffJournal) error {
	if _, err := os.Lstat(j.OldPath); errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("selfupdate: look for the parked bundle: %w", err)
	}
	if err := unix.RenamexNp(j.OldPath, j.Target, unix.RENAME_SWAP); err != nil {
		return fmt.Errorf("selfupdate: swap the bundles back: %w", err)
	}
	return os.RemoveAll(j.OldPath)
}

// handoffOutput leaves the successor without the old launcher's output: a
// released launcher writes its own log file.
func handoffOutput(*exec.Cmd) {}
