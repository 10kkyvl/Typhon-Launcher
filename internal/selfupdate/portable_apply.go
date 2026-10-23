package selfupdate

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"strings"

	"typhon/internal/storage"
)

const (
	portableExeName = "typhon.exe"
	newSuffix       = ".new"
	previousSuffix  = ".previous"
)

// Test seams: the second rename of a swap and the running executable.
var (
	portableRename     = os.Rename
	portableExecutable = os.Executable
)

// applyPortableZip replaces target with typhon.exe from the archive. The old
// binary is parked as target.previous rather than deleted: a crash between the
// two renames is undone by recoverPortableSwap on the next start.
func applyPortableZip(ctx context.Context, zipPath, target string) error {
	if target == "" {
		return errors.New("selfupdate: portable target path is empty")
	}
	current, err := os.Stat(target)
	if err != nil {
		return fmt.Errorf("selfupdate: portable target: %w", err)
	}

	zr, err := zip.OpenReader(zipPath)
	// ErrInsecurePath comes with a usable reader; the entry names are judged below with a
	// reason the user can be told.
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

	staged := target + newSuffix
	previous := target + previousSuffix
	if err := removeIfExists(staged); err != nil {
		return fmt.Errorf("selfupdate: remove stale %s: %w", staged, err)
	}
	if err := removeIfExists(previous); err != nil {
		return fmt.Errorf("selfupdate: remove stale %s: %w", previous, err)
	}
	if err := extractPortableExecutable(ctx, entry, staged, current.Mode().Perm()); err != nil {
		return err
	}

	if err := portableRename(target, previous); err != nil {
		removeQuietly(staged)
		return fmt.Errorf("selfupdate: park the current launcher: %w", err)
	}
	if err := portableRename(staged, target); err != nil {
		restoreErr := portableRename(previous, target)
		removeQuietly(staged)
		return errors.Join(fmt.Errorf("selfupdate: put the new launcher in place: %w", err), restoreErr)
	}
	return nil
}

func portableExecutableEntry(zr *zip.ReadCloser) (*zip.File, error) {
	var found *zip.File
	for _, f := range zr.File {
		if err := validateZipEntryName(f.Name); err != nil {
			return nil, err
		}
		if strings.TrimSuffix(f.Name, "/") != portableExeName {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("%w: %s appears twice", ErrPortableZipExecutable, portableExeName)
		}
		found = f
	}
	if found == nil {
		return nil, ErrPortableZipNoExecutable
	}
	if !found.Mode().IsRegular() || found.UncompressedSize64 == 0 || found.UncompressedSize64 > MaxArtifactSize {
		return nil, fmt.Errorf("%w: %s", ErrPortableZipExecutable, portableExeName)
	}
	return found, nil
}

func validateZipEntryName(name string) error {
	if name == "" || strings.ContainsAny(name, "\x00:") || strings.HasPrefix(name, "/") || strings.HasPrefix(name, `\`) {
		return fmt.Errorf("%w: %q", ErrPortableZipUnsafeEntry, name)
	}
	for _, seg := range strings.FieldsFunc(name, func(r rune) bool { return r == '/' || r == '\\' }) {
		if seg == ".." || seg == "." {
			return fmt.Errorf("%w: %q", ErrPortableZipUnsafeEntry, name)
		}
	}
	return nil
}

func extractPortableExecutable(ctx context.Context, entry *zip.File, dst string, perm os.FileMode) (err error) {
	rc, err := entry.Open()
	if err != nil {
		return fmt.Errorf("selfupdate: open %s in the portable archive: %w", entry.Name, err)
	}
	defer func() {
		if cerr := rc.Close(); cerr != nil {
			slog.Warn("close portable archive entry", "name", entry.Name, "error", cerr)
		}
	}()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return fmt.Errorf("selfupdate: create %s: %w", dst, err)
	}
	closed := false
	defer func() {
		if !closed {
			if cerr := out.Close(); cerr != nil {
				slog.Warn("close staged launcher", "path", dst, "error", cerr)
			}
		}
		if err != nil {
			removeQuietly(dst)
		}
	}()

	written, err := io.Copy(out, io.LimitReader(&ctxReader{ctx: ctx, r: rc}, MaxArtifactSize+1))
	if err != nil {
		return fmt.Errorf("selfupdate: extract %s: %w", entry.Name, err)
	}
	declared := entry.UncompressedSize64
	if written < 0 || written > MaxArtifactSize || declared > MaxArtifactSize || written != int64(declared) {
		return fmt.Errorf("%w: %s unpacked to %d bytes, the archive declares %d", ErrPortableZipExecutable, entry.Name, written, entry.UncompressedSize64)
	}
	if err := out.Sync(); err != nil {
		return fmt.Errorf("selfupdate: sync %s: %w", dst, err)
	}
	if err := out.Close(); err != nil {
		closed = true
		return fmt.Errorf("selfupdate: close %s: %w", dst, err)
	}
	closed = true
	return nil
}

type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c *ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// recoverPortableSwap finishes or undoes an exe replacement the process did not
// survive. exe is the running launcher's path.
func recoverPortableSwap(exe string) error {
	staged := exe + newSuffix
	previous := exe + previousSuffix

	_, exeErr := os.Stat(exe)
	_, prevErr := os.Stat(previous)
	switch {
	case exeErr != nil && !errors.Is(exeErr, fs.ErrNotExist):
		return fmt.Errorf("selfupdate: stat %s: %w", exe, exeErr)
	case prevErr != nil && !errors.Is(prevErr, fs.ErrNotExist):
		return fmt.Errorf("selfupdate: stat %s: %w", previous, prevErr)
	}

	if errors.Is(exeErr, fs.ErrNotExist) && prevErr == nil {
		if err := portableRename(previous, exe); err != nil {
			return fmt.Errorf("selfupdate: restore the previous launcher: %w", err)
		}
	}
	for _, leftover := range []string{staged, previous} {
		if err := removeIfExists(leftover); err != nil {
			slog.Warn("selfupdate: remove portable leftover, will retry on the next start", "path", leftover, "error", err)
		}
	}
	return nil
}

func removeIfExists(path string) error {
	if err := storage.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func removeQuietly(path string) {
	if err := removeIfExists(path); err != nil {
		slog.Warn("selfupdate: remove portable staging file", "path", path, "error", err)
	}
}
