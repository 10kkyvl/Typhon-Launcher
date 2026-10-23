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
)

const (
	previousDirName       = "previous"
	previousExeName       = "typhon.exe"
	previousStateName     = "state"
	previousRecordName    = "previous.json"
	previousRecordVersion = 1

	// The selfupdate cache holds installers and a copy of the launcher, none of
	// which another account has any business reading.
	cacheDirMode = 0o700
)

// writeStateFile is a seam for tests: the write that puts a saved state file
// over the live one.
var writeStateFile = storage.WriteAtomic

// stateFile is one state file as it stood before the update: Version is the
// storage envelope version the launcher that wrote it understood.
type stateFile struct {
	Path    string `json:"path"`
	Version int    `json:"version"`
	SHA256  string `json:"sha256"`
	Size    int64  `json:"size"`
}

// previousRecord describes the single launcher version kept for a rollback.
// It is written last, so a snapshot that died half-way has no record and
// therefore offers nothing.
type previousRecord struct {
	Version     string      `json:"version"`
	SHA256      string      `json:"sha256"`
	Size        int64       `json:"size"`
	SavedAt     time.Time   `json:"savedAt"`
	StateBackup []stateFile `json:"stateBackup"`
}

func PreviousDir(configDir string) (string, error) {
	dir, err := CacheDir(configDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, previousDirName), nil
}

func previousRecordPath(configDir string) (string, error) {
	dir, err := CacheDir(configDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, previousRecordName), nil
}

// skippedStateDir reports whether a directory of the config dir holds nothing
// a rollback restores: webview profiles, the updater's own cache, backups of
// the user's saves and in-flight helper processes.
func skippedStateDir(rel, name string) bool {
	lower := strings.ToLower(name)
	if lower == "webview2" || lower == "ebwebview" {
		return true
	}
	if strings.Contains(rel, "/") {
		return false
	}
	switch lower {
	case "selfupdate", "diagnostics", "cloud-saves-tmp", "save-backups", "media":
		return true
	}
	return strings.HasPrefix(lower, "broker-")
}

// skippedStateFile keeps in-flight helper files out of the backup: restoring
// the spec of an install worker that finished long ago would start it again.
func skippedStateFile(rel, name string) bool {
	lower := strings.ToLower(name)
	if !strings.HasSuffix(lower, ".json") || strings.HasPrefix(lower, ".tmp-") {
		return true
	}
	return !strings.Contains(rel, "/") && strings.HasPrefix(lower, "worker-")
}

// snapshotPrevious saves the running launcher and the versioned state next to
// it, so that a rollback can put both back. It builds the copy in a staging
// directory and leaves the snapshot in use untouched: the new copy replaces
// it only when the update has gone through (commitSnapshot), and an update
// that fails must not cost the user the snapshot they already had. The record
// goes last, so a copy that died half-way has none and is never promoted.
func snapshotPrevious(ctx context.Context, configDir, exe, ver string) (err error) {
	if ver == "" || validatePathSegment(ver) != nil {
		return ErrSnapshotVersion
	}
	if exe == "" {
		return errors.New("selfupdate: snapshot: executable path is empty")
	}
	dir, err := stagingDir(configDir)
	if err != nil {
		return err
	}
	recordPath, err := stagingRecordPath(configDir)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			if derr := discardStaging(configDir); derr != nil {
				slog.Warn("selfupdate: remove the unfinished snapshot", "error", derr)
			}
		}
	}()
	if err := discardStaging(configDir); err != nil {
		return fmt.Errorf("selfupdate: drop the earlier unfinished snapshot: %w", err)
	}
	stateDir := filepath.Join(dir, previousStateName)
	if err := os.MkdirAll(stateDir, cacheDirMode); err != nil {
		return fmt.Errorf("selfupdate: create the snapshot directory: %w", err)
	}

	sum, size, err := copyHashed(ctx, exe, filepath.Join(dir, previousExeName))
	if err != nil {
		return fmt.Errorf("selfupdate: save the running launcher: %w", err)
	}
	files, err := backupState(ctx, configDir, stateDir)
	if err != nil {
		return fmt.Errorf("selfupdate: save the launcher state: %w", err)
	}
	rec := previousRecord{Version: ver, SHA256: sum, Size: size, SavedAt: time.Now().UTC(), StateBackup: files}
	if err := storage.Save(recordPath, previousRecordVersion, rec); err != nil {
		return fmt.Errorf("selfupdate: write the snapshot record: %w", err)
	}
	return nil
}

// copyHashed copies src to dst atomically and reports what it wrote.
func copyHashed(ctx context.Context, src, dst string) (sum string, size int64, err error) {
	in, err := os.Open(src)
	if err != nil {
		return "", 0, fmt.Errorf("open %s: %w", filepath.Base(src), err)
	}
	defer func() {
		if cerr := in.Close(); cerr != nil {
			slog.Warn("close copied file", "path", src, "error", cerr)
		}
	}()
	info, err := in.Stat()
	if err != nil {
		return "", 0, fmt.Errorf("stat %s: %w", filepath.Base(src), err)
	}
	if !info.Mode().IsRegular() {
		return "", 0, fmt.Errorf("%s is not a regular file", filepath.Base(src))
	}
	if err := os.MkdirAll(filepath.Dir(dst), cacheDirMode); err != nil {
		return "", 0, err
	}

	hasher := sha256.New()
	err = storage.WriteAtomicStream(dst, info.Mode().Perm(), func(w io.Writer) error {
		n, cerr := io.Copy(io.MultiWriter(w, hasher), &ctxReader{ctx: ctx, r: in})
		size = n
		return cerr
	})
	if err != nil {
		return "", 0, err
	}
	if size != info.Size() {
		return "", 0, fmt.Errorf("%s changed while it was copied: %d bytes read, %d expected", filepath.Base(src), size, info.Size())
	}
	return hex.EncodeToString(hasher.Sum(nil)), size, nil
}

// backupState copies every versioned state file under configDir into
// stateDir. A file without an envelope is not copied: the launcher that wrote
// it reads it back without a version check. A directory or file that cannot be
// read fails the whole backup, because a rollback that silently left one of
// them out would restore a state the old launcher then trips over.
func backupState(ctx context.Context, configDir, stateDir string) ([]stateFile, error) {
	root, err := os.OpenRoot(configDir)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := root.Close(); cerr != nil {
			slog.Warn("close config root", "error", cerr)
		}
	}()

	var files []stateFile
	err = fs.WalkDir(root.FS(), ".", func(rel string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			if skippedStateDir(rel, d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if skippedStateFile(rel, d.Name()) {
			return nil
		}
		if !d.Type().IsRegular() {
			slog.Warn("selfupdate: state file is not a regular file, it is not saved for a rollback", "path", rel)
			return nil
		}
		raw, err := root.ReadFile(filepath.FromSlash(rel))
		if err != nil {
			return fmt.Errorf("read %s: %w", rel, err)
		}
		ver, enveloped, err := storage.EnvelopeVersion(raw)
		if err != nil {
			slog.Warn("selfupdate: state file is not valid JSON, it is not saved for a rollback", "path", rel, "error", err)
			return nil
		}
		if !enveloped {
			return nil
		}
		dst := filepath.Join(stateDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), cacheDirMode); err != nil {
			return err
		}
		if err := storage.WriteAtomic(dst, raw); err != nil {
			return fmt.Errorf("save %s: %w", rel, err)
		}
		sum := sha256.Sum256(raw)
		files = append(files, stateFile{Path: rel, Version: ver, SHA256: hex.EncodeToString(sum[:]), Size: int64(len(raw))})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

// loadPrevious reads and checks the rollback record. A missing record is
// ErrNoPrevious; anything else wrong with it is ErrPreviousBroken, because
// either way the files beside it cannot be trusted.
func loadPrevious(configDir string) (previousRecord, error) {
	path, err := previousRecordPath(configDir)
	if err != nil {
		return previousRecord{}, err
	}
	return loadRecord(path)
}

func loadRecord(path string) (previousRecord, error) {
	var rec previousRecord
	if err := storage.Load(path, previousRecordVersion, nil, &rec); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return previousRecord{}, ErrNoPrevious
		}
		return previousRecord{}, fmt.Errorf("%w: %w", ErrPreviousBroken, err)
	}
	if err := rec.validate(); err != nil {
		return previousRecord{}, fmt.Errorf("%w: %w", ErrPreviousBroken, err)
	}
	return rec, nil
}

func (r previousRecord) validate() error {
	if r.Version == "" || validatePathSegment(r.Version) != nil {
		return fmt.Errorf("version %q", r.Version)
	}
	if err := validateHash(r.SHA256); err != nil {
		return err
	}
	if r.Size <= 0 {
		return fmt.Errorf("size %d", r.Size)
	}
	seen := make(map[string]bool, len(r.StateBackup))
	for _, f := range r.StateBackup {
		native := filepath.FromSlash(f.Path)
		if f.Path == "" || !filepath.IsLocal(native) || filepath.ToSlash(filepath.Clean(native)) != f.Path || !strings.HasSuffix(f.Path, ".json") {
			return fmt.Errorf("state file path %q", f.Path)
		}
		if seen[f.Path] {
			return fmt.Errorf("state file %q listed twice", f.Path)
		}
		seen[f.Path] = true
		if f.Version <= 0 || f.Size < 0 {
			return fmt.Errorf("state file %q: version %d, size %d", f.Path, f.Version, f.Size)
		}
		if err := validateHash(f.SHA256); err != nil {
			return fmt.Errorf("state file %q: %w", f.Path, err)
		}
	}
	return nil
}

// verifyPreviousExe checks the saved launcher against its record and returns
// its path. Hashing the whole file is what separates "the file is there" from
// "the file is what was saved".
func verifyPreviousExe(ctx context.Context, configDir string, rec previousRecord) (string, error) {
	dir, err := PreviousDir(configDir)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, previousExeName)
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrPreviousBroken, err)
	}
	if !info.Mode().IsRegular() || info.Size() != rec.Size {
		return "", fmt.Errorf("%w: %s has %d bytes, the record says %d", ErrPreviousHash, previousExeName, info.Size(), rec.Size)
	}
	sum, err := fileDigest(ctx, path)
	if err != nil {
		return "", err
	}
	if sum != rec.SHA256 {
		return "", ErrPreviousHash
	}
	return path, nil
}

// restoreItem is one state file chosen for restoring: rel is its path under
// the config dir, data the backup (already checked against its hash) and old
// what the live file holds now, kept so the restore can be undone.
type restoreItem struct {
	rel  string
	dst  string
	data []byte
	old  []byte
}

// planStateRestore picks the backed-up files whose envelope version is no
// longer the one the live file carries. A live file at the saved version holds
// everything the older launcher understands plus whatever was added since, so
// it stays. A file that is gone stays gone: the update or the user removed it.
// Nothing is written here: every backup is read and hashed first, so a damaged
// one stops the rollback before it changes anything.
func planStateRestore(ctx context.Context, configDir string, rec previousRecord) ([]restoreItem, error) {
	dir, err := PreviousDir(configDir)
	if err != nil {
		return nil, err
	}
	var items []restoreItem
	for _, f := range rec.StateBackup {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		native := filepath.FromSlash(f.Path)
		live := filepath.Join(configDir, native)
		liveRaw, err := os.ReadFile(live)
		if errors.Is(err, fs.ErrNotExist) {
			slog.Info("selfupdate: state file is gone, it is not restored", "path", f.Path)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f.Path, err)
		}
		liveVersion, _, verr := storage.EnvelopeVersion(liveRaw)
		if verr == nil && liveVersion == f.Version {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, previousStateName, native))
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %w", ErrPreviousBroken, f.Path, err)
		}
		sum := sha256.Sum256(data)
		if int64(len(data)) != f.Size || hex.EncodeToString(sum[:]) != f.SHA256 {
			return nil, fmt.Errorf("%w: %s", ErrPreviousStateHash, f.Path)
		}
		items = append(items, restoreItem{rel: f.Path, dst: live, data: data, old: liveRaw})
	}
	return items, nil
}

// writeRestored writes the planned files one by one. Each write is atomic on
// its own; what a rollback does when it stops half-way is the caller's job
// (undoReplaced), this only reports the first failure.
func writeRestored(ctx context.Context, items []restoreItem) error {
	for _, it := range items {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := writeStateFile(it.dst, it.data); err != nil {
			return fmt.Errorf("restore %s: %w", it.rel, err)
		}
	}
	return nil
}

// dropPrevious removes the snapshot once it has been used. The record goes
// first so that a removal that dies half-way leaves nothing on offer.
func dropPrevious(configDir string) error {
	dir, err := PreviousDir(configDir)
	if err != nil {
		return err
	}
	recordPath, err := previousRecordPath(configDir)
	if err != nil {
		return err
	}
	if err := removeIfExists(recordPath); err != nil {
		return err
	}
	return os.RemoveAll(dir)
}
