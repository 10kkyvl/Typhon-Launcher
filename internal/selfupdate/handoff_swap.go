package selfupdate

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"typhon/internal/storage"
)

// handoffRename is the rename the swap is built from; a test seam for a swap
// that fails midway.
var handoffRename = storage.Rename

// handoffPlan is everything the old process decided before touching the disk.
type handoffPlan struct {
	Method       handoffMethod
	Exe          string // the running executable
	Target       string // the file the swap replaces: the executable, or the .app bundle
	ArtifactPath string // the verified download
	Art          Artifact
	Portable     bool
	InstallDir   string
	Nonce        string
	Version      string
	FromVersion  string
}

// journalFor derives the on-disk record, with the parked and staged names the
// swap will use, so recovery knows them even if the process dies mid-swap.
func (p handoffPlan) journalFor(parentPID int) handoffJournal {
	dir := filepath.Dir(p.Target)
	ext := filepath.Ext(p.Target)
	j := handoffJournal{
		Nonce:       p.Nonce,
		Stage:       stagePrepared,
		Method:      p.Method,
		Target:      p.Target,
		OldPath:     filepath.Join(dir, oldPrefix+p.Nonce+ext),
		Version:     p.Version,
		FromVersion: p.FromVersion,
		ParentPID:   parentPID,
		StartedAt:   time.Now(),
	}
	if p.Method == methodExeFile || p.Method == methodExeZip {
		j.StagedPath = filepath.Join(dir, stagedPrefix+p.Nonce+ext)
	}
	return j
}

// performSwap puts the new launcher in place and returns the call that puts
// the old one back. On error it has tried to put everything back itself: a nil
// undo then means the disk is as it was, and a non-nil one means that undo
// failed and is still owed: the launcher's name may hold the new build or
// nothing, and the old one is parked. Calling it again retries.
func performSwap(ctx context.Context, p handoffPlan, j handoffJournal) (undo func() error, err error) {
	switch p.Method {
	case methodExeFile, methodExeZip:
		return swapParkedExe(ctx, p, j)
	case methodInstaller:
		return swapInstaller(ctx, p, j)
	case methodBundle:
		return swapBundleParked(ctx, p, j)
	default:
		return nil, fmt.Errorf("%w: method %q", ErrHandoffUnsupported, p.Method)
	}
}

// swapParkedExe renames the running executable out of the way, which every
// supported OS allows, and renames the new one into its name. The new file is
// fully written and hash-checked under its staged name first, so the window
// in which the launcher's name points at nothing is two renames wide.
func swapParkedExe(ctx context.Context, p handoffPlan, j handoffJournal) (func() error, error) {
	revert := func() error { return nil }
	if err := stageExe(ctx, p, j.StagedPath); err != nil {
		removeQuietly(j.StagedPath)
		return nil, err
	}
	if p.Art.Kind == KindExe && !p.Portable {
		var err error
		revert, err = updateDisplayVersion(p.Version)
		if err != nil {
			removeQuietly(j.StagedPath)
			return nil, fmt.Errorf("selfupdate: update the installed version in the registry: %w", err)
		}
	}
	undo := func() error {
		return errors.Join(undoParkedExe(j), revert())
	}
	if err := handoffRename(j.Target, j.OldPath); err != nil {
		removeQuietly(j.StagedPath)
		return nil, errors.Join(fmt.Errorf("selfupdate: park the running launcher: %w", err), revert())
	}
	if err := handoffRename(j.StagedPath, j.Target); err != nil {
		cause := fmt.Errorf("selfupdate: put the new launcher in place: %w", err)
		if rerr := revert(); rerr != nil {
			// The installed-programs entry shows the new version; the launcher
			// itself is what the undo below is about.
			slog.Warn("selfupdate: put the installed version back in the registry", "error", rerr)
		}
		owed := func() error { return undoParkedExe(j) }
		if uerr := owed(); uerr != nil {
			return owed, errors.Join(cause, uerr)
		}
		return nil, cause
	}
	return undo, nil
}

func stageExe(ctx context.Context, p handoffPlan, staged string) error {
	if p.Method == methodExeZip {
		return stageExeFromZip(ctx, p.ArtifactPath, staged, p.Target)
	}
	return swapExecutable(ctx, staged, p.ArtifactPath, p.Art.SHA256)
}

func stageExeFromZip(ctx context.Context, zipPath, staged, target string) error {
	info, err := os.Stat(target)
	if err != nil {
		return fmt.Errorf("selfupdate: stat the launcher: %w", err)
	}
	zr, err := zip.OpenReader(zipPath)
	if err != nil && (zr == nil || !errors.Is(err, zip.ErrInsecurePath)) {
		return fmt.Errorf("selfupdate: open portable archive: %w", err)
	}
	defer func() {
		if cerr := zr.Close(); cerr != nil {
			slog.Warn("close portable archive", "path", zipPath, "error", cerr)
		}
	}()
	entry, err := portableExecutableEntry(zr)
	if err != nil {
		return err
	}
	return extractPortableExecutable(ctx, entry, staged, info.Mode().Perm())
}

// undoParkedExe is safe to repeat and to run with nothing parked.
func undoParkedExe(j handoffJournal) error {
	var errs []error
	if _, err := os.Lstat(j.OldPath); err == nil {
		if rerr := handoffRename(j.OldPath, j.Target); rerr != nil {
			errs = append(errs, fmt.Errorf("selfupdate: put the previous launcher back: %w", rerr))
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		errs = append(errs, fmt.Errorf("selfupdate: look for the parked launcher: %w", err))
	}
	if j.StagedPath != "" {
		errs = append(errs, removeIfExists(j.StagedPath))
	}
	return errors.Join(errs...)
}

// undoSwap reverts an executable swap a journal describes, from disk alone.
// A bundle swap is not: the journal cannot say which bundle is parked. Its own
// undo knows (bundleUndo), and a start settles it (settleBundleJournal).
func undoSwap(j handoffJournal) error {
	if err := checkOwnedName(j); err != nil {
		return err
	}
	if j.Method == methodBundle {
		return fmt.Errorf("selfupdate: a bundle swap is not undone from its journal alone")
	}
	return undoParkedExe(j)
}

// checkOwnedName refuses to act on a journal whose paths are not the ones a
// swap would have written: the file is under the user's account, so it is
// input.
func checkOwnedName(j handoffJournal) error {
	if j.Target == "" || !filepath.IsAbs(j.Target) || j.OldPath == "" || !filepath.IsAbs(j.OldPath) {
		return fmt.Errorf("selfupdate: the hand-off journal has unusable paths")
	}
	if filepath.Dir(j.OldPath) != filepath.Dir(j.Target) || !strings.HasPrefix(filepath.Base(j.OldPath), oldPrefix) {
		return fmt.Errorf("selfupdate: the hand-off journal names a file that is not a parked launcher: %s", j.OldPath)
	}
	if j.StagedPath != "" && (filepath.Dir(j.StagedPath) != filepath.Dir(j.Target) || !strings.HasPrefix(filepath.Base(j.StagedPath), stagedPrefix)) {
		return fmt.Errorf("selfupdate: the hand-off journal names a file that is not a staged launcher: %s", j.StagedPath)
	}
	return nil
}

// swapInstaller parks the running executable so the silent installer can write
// a fresh one (a running image cannot be overwritten on Windows), runs it to
// completion and checks that it really replaced the launcher.
func swapInstaller(ctx context.Context, p handoffPlan, j handoffJournal) (func() error, error) {
	if err := handoffRename(j.Target, j.OldPath); err != nil {
		return nil, fmt.Errorf("selfupdate: park the running launcher: %w", err)
	}
	undo := func() error { return undoParkedExe(j) }
	fail := func(cause error) (func() error, error) {
		if uerr := undo(); uerr != nil {
			return undo, errors.Join(cause, uerr)
		}
		return nil, cause
	}
	if err := applyInstaller(ctx, p.ArtifactPath, p.InstallDir, p.Target); err != nil {
		return fail(err)
	}
	before, err := fileDigest(ctx, j.OldPath)
	if err != nil {
		return fail(err)
	}
	after, err := fileDigest(ctx, j.Target)
	if err != nil {
		return fail(err)
	}
	if err := ensureReplaced(before, after, j.Target); err != nil {
		return fail(err)
	}
	return undo, nil
}
