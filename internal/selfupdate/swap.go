package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"typhon/internal/storage"
	"typhon/internal/uierr"
)

var ErrSwapHash = uierr.New("selfupdate.swap_hash", "selfupdate: the replacement launcher differs from its expected hash")

// swapLeftoverAge is how old a temp file beside the launcher must be before
// startup treats it as the remains of a swap that died: a younger one may
// belong to a write that is still going on.
const swapLeftoverAge = time.Hour

var currentExecutable = os.Executable

// swapExecutable replaces target with the file at src in one atomic rename.
// The replacement is written beside target, synced and checked against
// wantSHA256 before the rename, so target is either the old launcher or the
// complete new one and never a torn file or a missing one. The caller owns
// src and the checks that decide it is the right file; wantSHA256 only stops
// a src that changed or was damaged on the way.
func swapExecutable(ctx context.Context, target, src, wantSHA256 string) error {
	if target == "" || !filepath.IsAbs(target) {
		return fmt.Errorf("selfupdate: swap target %q is not an absolute path", target)
	}
	if err := validateHash(wantSHA256); err != nil {
		return err
	}
	mode := fs.FileMode(0o755)
	if info, err := os.Stat(target); err == nil {
		mode = info.Mode().Perm()
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("selfupdate: stat swap target: %w", err)
	}

	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("selfupdate: open the replacement: %w", err)
	}
	defer func() {
		if cerr := in.Close(); cerr != nil {
			slog.Warn("close replacement launcher", "path", src, "error", cerr)
		}
	}()

	return storage.WriteAtomicStream(target, mode, func(w io.Writer) error {
		hasher := sha256.New()
		if _, err := io.Copy(io.MultiWriter(w, hasher), &ctxReader{ctx: ctx, r: in}); err != nil {
			return err
		}
		if hex.EncodeToString(hasher.Sum(nil)) != wantSHA256 {
			return ErrSwapHash
		}
		return nil
	})
}

// recoverSwap clears what a swap that died leaves beside the launcher: the
// temp file it was writing. Nothing else can be left, because the swap itself
// is one rename. Failures are logged and retried on the next start.
func recoverSwap(exe string) {
	dir := filepath.Dir(exe)
	entries, err := os.ReadDir(dir)
	if err != nil {
		slog.Warn("selfupdate: scan for swap leftovers", "dir", dir, "error", err)
		return
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".tmp-") || !e.Type().IsRegular() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			slog.Warn("selfupdate: stat swap leftover", "name", e.Name(), "error", err)
			continue
		}
		if time.Since(info.ModTime()) < swapLeftoverAge {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if err := removeIfExists(path); err != nil {
			slog.Warn("selfupdate: remove swap leftover, will retry on the next start", "path", path, "error", err)
		}
	}
}
