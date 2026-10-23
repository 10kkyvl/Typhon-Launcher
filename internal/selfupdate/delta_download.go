package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"typhon/internal/selfupdate/delta"
	"typhon/internal/storage"
)

// Why a patch is not used. Each is an ordinary outcome that sends the
// launcher to the full file, and the log line carries it once.
var (
	errNoDelta            = errors.New("the manifest carries no patches for this build")
	errDeltaNoSource      = errors.New("the manifest has no patch from the running version")
	errDeltaNotSmaller    = errors.New("the patch is not smaller than the full download")
	errDeltaDirReadOnly   = errors.New("the launcher folder does not accept new files")
	errDeltaSourceChanged = errors.New("the running executable is not the build the patch starts from")
	errDeltaNoBase        = errors.New("the running executable cannot be read")
)

// deltaPlan is a patch that fits the running build, together with the bytes of
// the executable it applies to. The bytes are read once: the hash that picked
// the patch and the base the patch is applied to are the same read.
type deltaPlan struct {
	delta Delta
	base  []byte
}

// planDelta decides whether a patch can replace the full download. Everything
// that can be known without touching the disk comes first; the executable is
// read last, once.
func planDelta(ctx context.Context, art Artifact, currentVersion string) (deltaPlan, error) {
	if len(art.Deltas) == 0 || art.ExeSHA256 == "" {
		return deltaPlan{}, errNoDelta
	}
	var chosen *Delta
	for i := range art.Deltas {
		if sameVersion(art.Deltas[i].From, currentVersion) {
			chosen = &art.Deltas[i]
			break
		}
	}
	if chosen == nil {
		return deltaPlan{}, fmt.Errorf("%w: running %s", errDeltaNoSource, currentVersion)
	}
	if chosen.Size >= art.Size {
		return deltaPlan{}, fmt.Errorf("%w: %d against %d bytes", errDeltaNotSmaller, chosen.Size, art.Size)
	}
	exe, err := currentExecutable()
	if err != nil {
		return deltaPlan{}, fmt.Errorf("%w: %w", errDeltaNoBase, err)
	}
	if err := canCreateIn(filepath.Dir(exe)); err != nil {
		return deltaPlan{}, fmt.Errorf("%w: %w", errDeltaDirReadOnly, err)
	}
	base, err := readBounded(ctx, exe)
	if err != nil {
		return deltaPlan{}, fmt.Errorf("%w: %w", errDeltaNoBase, err)
	}
	sum := sha256.Sum256(base)
	if hex.EncodeToString(sum[:]) != chosen.FromSHA256 {
		return deltaPlan{}, errDeltaSourceChanged
	}
	return deltaPlan{delta: *chosen, base: base}, nil
}

// canCreateIn is the swap's own requirement: the new executable is written
// beside the old one and renamed over it, so the folder has to take a new file.
func canCreateIn(dir string) error {
	f, err := os.CreateTemp(dir, swapTempPrefix+"probe-*")
	if err != nil {
		return err
	}
	name := f.Name()
	closeErr := f.Close()
	removeErr := os.Remove(name)
	return errors.Join(closeErr, removeErr)
}

func readBounded(ctx context.Context, path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(&ctxReader{ctx: ctx, r: f}, MaxArtifactSize+1))
	if cerr := f.Close(); cerr != nil {
		err = errors.Join(err, cerr)
	}
	if err != nil {
		return nil, err
	}
	if len(data) > MaxArtifactSize {
		return nil, fmt.Errorf("%s is larger than %d bytes", filepath.Base(path), MaxArtifactSize)
	}
	return data, nil
}

// downloadViaDelta fetches the patch, rebuilds typhon.exe from it in the
// version's cache directory and checks the result against the hash and size the
// signed manifest gives for the new executable. The returned artifact describes
// that file and is what the update record keeps as ready.
func (c *Client) downloadViaDelta(ctx context.Context, plan deltaPlan, art Artifact, destDir string, onProgress func(int64)) (string, Artifact, error) {
	patchPath, err := c.Download(ctx, plan.delta.asArtifact(art.OS, art.Arch), destDir, onProgress)
	if err != nil {
		return "", Artifact{}, err
	}
	defer removePartial(patchPath)

	exePath := filepath.Join(destDir, portableExeName)
	if err := buildExecutable(ctx, plan.base, patchPath, exePath, art); err != nil {
		return "", Artifact{}, err
	}
	return exePath, Artifact{
		OS:     art.OS,
		Arch:   art.Arch,
		Kind:   KindExe,
		Name:   portableExeName,
		Size:   art.ExeSize,
		SHA256: art.ExeSHA256,
	}, nil
}

func buildExecutable(ctx context.Context, base []byte, patchPath, exePath string, art Artifact) error {
	patch, err := os.Open(patchPath)
	if err != nil {
		return fmt.Errorf("open the patch: %w", err)
	}
	defer func() {
		if cerr := patch.Close(); cerr != nil {
			slog.Warn("close patch file", "path", patchPath, "error", cerr)
		}
	}()

	err = storage.WriteAtomicStream(exePath, 0o700, func(w io.Writer) error {
		hasher := sha256.New()
		n, err := delta.Apply(ctx, io.MultiWriter(w, hasher), base, patch, art.ExeSize)
		if err != nil {
			return err
		}
		if n != art.ExeSize {
			return fmt.Errorf("%w: the patch rebuilt %d bytes, the manifest says %d", ErrSizeMismatch, n, art.ExeSize)
		}
		if hex.EncodeToString(hasher.Sum(nil)) != art.ExeSHA256 {
			return ErrHashMismatch
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("rebuild the executable from the patch: %w", err)
	}
	return nil
}

// applyReadyExe is the worker's half for an update that arrived as a rebuilt
// executable: the same checks Apply makes for an installer, then one atomic
// swap. The uninstall entry follows the swap's outcome, as it does in a
// rollback; a portable folder has no entry to move.
func applyReadyExe(ctx context.Context, configDir, exePath, target, ver string, portable bool) error {
	store, err := NewStore(configDir)
	if err != nil {
		return err
	}
	st, err := store.Load()
	if err != nil {
		return err
	}
	if st.Artifact == nil || st.Artifact.Kind != KindExe || st.ReadyPath == "" || st.ReadyPath != exePath {
		return ErrNotReady
	}
	if _, _, err := artifactRel(configDir, exePath); err != nil {
		return err
	}
	info, err := os.Lstat(exePath)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %s", errNotCached, filepath.Base(exePath))
	}
	if err := VerifyFile(ctx, exePath, *st.Artifact); err != nil {
		return err
	}

	revert := func() error { return nil }
	if !portable {
		revert, err = updateDisplayVersion(ver)
		if err != nil {
			return fmt.Errorf("selfupdate: update the installed version in the registry: %w", err)
		}
	}
	if err := swapExecutable(ctx, target, exePath, st.Artifact.SHA256); err != nil {
		if rerr := revert(); rerr != nil {
			return errors.Join(err, fmt.Errorf("selfupdate: put the registry version back: %w", rerr))
		}
		return err
	}
	return nil
}

// readyMatches says whether the file recorded as ready is still the right one
// for the artifact a fresh manifest offers. A rebuilt executable stands for the
// installer or the zip whose ExeSHA256 it carries.
func readyMatches(ready *Artifact, art Artifact) bool {
	if ready == nil {
		return false
	}
	if ready.Kind == KindExe {
		return art.ExeSHA256 != "" && ready.SHA256 == art.ExeSHA256 && ready.Size == art.ExeSize &&
			ready.OS == art.OS && ready.Arch == art.Arch
	}
	return sameFile(*ready, art)
}

// sameFile compares what identifies the downloaded file. The patches an
// artifact lists are advice for the download, not part of the file.
func sameFile(a, b Artifact) bool {
	return a.OS == b.OS && a.Arch == b.Arch && a.Kind == b.Kind && a.Name == b.Name && a.URL == b.URL &&
		a.Size == b.Size && a.SHA256 == b.SHA256 &&
		a.PortableName == b.PortableName && a.PortableURL == b.PortableURL &&
		a.PortableSize == b.PortableSize && a.PortableSHA256 == b.PortableSHA256 &&
		a.ExeSHA256 == b.ExeSHA256 && a.ExeSize == b.ExeSize
}
