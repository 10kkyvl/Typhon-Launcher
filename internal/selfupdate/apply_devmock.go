//go:build devmock && !windows

package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"typhon/internal/settings"
)

var (
	errTargetNotAbsolute      = errors.New("selfupdate: launcher path is not absolute")
	errTargetNotClean         = errors.New("selfupdate: launcher path is not clean")
	errTargetOutsideInstall   = errors.New("selfupdate: launcher path is outside the install dir")
	errTargetNotRegularFile   = errors.New("selfupdate: launcher path is not a regular file")
	errTargetDigestUnexpected = errors.New("selfupdate: copied launcher differs from the verified artifact")
)

// Apply replaces the launcher binary in place: POSIX lets a running
// executable be renamed over, the old inode keeps running until it exits,
// and the relaunch picks up the new file.
func Apply(ctx context.Context, installerPath, installDir, target string) error {
	dir, err := settings.ConfigDir()
	if err != nil {
		return err
	}
	if err := validateInstallerPath(dir, installerPath); err != nil {
		return err
	}
	if err := validateInstallDir(installDir); err != nil {
		return err
	}
	if err := validateTarget(installDir, target); err != nil {
		return err
	}

	store, err := NewStore(dir)
	if err != nil {
		return err
	}
	st, err := store.Load()
	if err != nil {
		return err
	}
	if st.Artifact == nil || st.ReadyPath == "" || st.ReadyPath != installerPath {
		return ErrNotReady
	}

	if err := VerifyFile(ctx, installerPath, *st.Artifact); err != nil {
		return err
	}

	return replaceBinary(ctx, installerPath, installDir, target, *st.Artifact)
}

func validateTarget(installDir, target string) error {
	if !filepath.IsAbs(target) {
		return errTargetNotAbsolute
	}
	if target != filepath.Clean(target) {
		return errTargetNotClean
	}
	if filepath.Dir(target) != installDir {
		return errTargetOutsideInstall
	}
	info, err := os.Lstat(target)
	if err != nil {
		return fmt.Errorf("stat launcher binary: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errTargetNotRegularFile
	}
	return nil
}

func replaceBinary(ctx context.Context, installerPath, installDir, target string, art Artifact) (err error) {
	tmp := filepath.Join(installDir, fmt.Sprintf(".typhon-update-%d", os.Getpid()))
	in, err := os.Open(installerPath)
	if err != nil {
		return fmt.Errorf("open artifact: %w", err)
	}
	defer func() {
		if cerr := in.Close(); cerr != nil {
			slog.Warn("close artifact", "path", installerPath, "error", cerr)
		}
	}()

	//nolint:gosec // G302: это подменяемый бинарь лаунчера, без бита x его не перезапустить (инвариант 8: режим задаёт источник, а не 0600)
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
	if err != nil {
		return fmt.Errorf("create staged launcher: %w", err)
	}
	staged := true
	defer func() {
		if !staged {
			return
		}
		if rerr := os.Remove(tmp); rerr != nil && !errors.Is(rerr, os.ErrNotExist) {
			slog.Warn("remove staged launcher", "path", tmp, "error", rerr)
		}
	}()

	written, sum, err := copyWithHash(ctx, out, in, art.Size, nil)
	if err != nil {
		closeQuietly(out, tmp)
		return err
	}
	if written != art.Size || sum != art.SHA256 {
		closeQuietly(out, tmp)
		return errTargetDigestUnexpected
	}
	if err := out.Chmod(0o755); err != nil {
		closeQuietly(out, tmp)
		return fmt.Errorf("chmod staged launcher: %w", err)
	}
	if err := out.Sync(); err != nil {
		closeQuietly(out, tmp)
		return fmt.Errorf("sync staged launcher: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close staged launcher: %w", err)
	}
	if err := os.Rename(tmp, target); err != nil {
		return fmt.Errorf("replace launcher binary: %w", err)
	}
	staged = false
	return syncDir(installDir)
}

func closeQuietly(f *os.File, path string) {
	if cerr := f.Close(); cerr != nil {
		slog.Warn("close staged launcher", "path", path, "error", cerr)
	}
}

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open install dir: %w", err)
	}
	syncErr := d.Sync()
	if cerr := d.Close(); cerr != nil && syncErr == nil {
		syncErr = cerr
	}
	if syncErr != nil {
		return fmt.Errorf("sync install dir: %w", syncErr)
	}
	return nil
}
