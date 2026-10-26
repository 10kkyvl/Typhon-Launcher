package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"typhon/internal/settings"
	"typhon/internal/storage"
	"typhon/internal/uierr"
)

const (
	workerDirName  = "worker"
	outcomeName    = "last-update.json"
	specName       = "update-spec.json"
	hashBufSize    = 256 * 1024
	outcomeMaxAge  = 24 * time.Hour
	outcomeVersion = 1
)

const (
	specKindUpdate   = "update"
	specKindRollback = "rollback"
	// specKindExe is an update whose new typhon.exe is already built and sits in
	// the cache: the worker swaps the file instead of running an installer.
	specKindExe = "exe"

	// outcomeRolledBack marks the record of a rollback; an update leaves Kind empty.
	outcomeRolledBack = "rolled_back"
)

// updateSpec.Kind is empty in a spec written before rollbacks existed, which
// is an update. Version is the version the worker moves the launcher to;
// FromVersion is the one it moves away from.
type updateSpec struct {
	Kind          string `json:"kind,omitempty"`
	Language      string `json:"language,omitempty"`
	InstallerPath string `json:"installerPath"`
	InstallDir    string `json:"installDir"`
	ParentPID     int    `json:"parentPid"`
	RelaunchPath  string `json:"relaunchPath"`
	Version       string `json:"version"`
	FromVersion   string `json:"fromVersion,omitempty"`
	Portable      bool   `json:"portable,omitempty"`
	// Downgrade marks the way off a beta to an older stable build. The beta is
	// not saved as the rollback snapshot: a stable launcher cannot read the
	// record of a beta.
	Downgrade bool `json:"downgrade,omitempty"`
}

// Outcome is what the worker leaves behind for the relaunched launcher: the
// install runs after the UI is gone, so without it a failed update is
// indistinguishable from an update that never started. Kind, From and To are
// set only for a rollback, where Version repeats To.
type Outcome struct {
	Kind    string `json:"kind,omitempty"`
	Version string `json:"version"`
	From    string `json:"from,omitempty"`
	To      string `json:"to,omitempty"`
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
	// SnapshotError is set when the update went ahead without saving the
	// launcher it replaced (code selfupdate.snapshot_failed): it succeeded,
	// and a rollback to the previous version is not on offer.
	SnapshotError string    `json:"snapshotError,omitempty"`
	FinishedAt    time.Time `json:"finishedAt"`
}

var (
	// The launcher flushes telemetry and waits for its services on the way
	// out; on a blocked network every one of those hits its own timeout in
	// turn, and an antivirus scanning the new binary adds more. Waiting
	// longer costs a progress window, giving up costs the update.
	parentExitTimeout  = 90 * time.Second
	parentPollInterval = 250 * time.Millisecond
	applyTimeout       = 5 * time.Minute
)

var errParentStillRunning = uierr.New("selfupdate.parent_still_running", "selfupdate: launcher did not exit before the timeout")

func WorkerDir(configDir string) (string, error) {
	dir, err := CacheDir(configDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, workerDirName), nil
}

// workerEnvironment is nil, which inherits the launcher environment, outside
// portable mode. In portable mode the worker is a copy in the cache with no
// marker beside it, so it is told where the data lives.
func workerEnvironment() ([]string, error) {
	dir, ok, err := settings.PortableDir()
	if err != nil {
		return nil, fmt.Errorf("selfupdate: portable data directory: %w", err)
	}
	if !ok {
		return nil, nil
	}
	return append(os.Environ(), settings.PortableEnv+"="+dir), nil
}

func OutcomePath(configDir string) (string, error) {
	dir, err := CacheDir(configDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, outcomeName), nil
}

func SpecPath(configDir string) (string, error) {
	dir, err := CacheDir(configDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, specName), nil
}

func writeUpdateSpec(path string, spec updateSpec) error {
	if err := os.MkdirAll(filepath.Dir(path), cacheDirMode); err != nil {
		return fmt.Errorf("create selfupdate spec dir: %w", err)
	}
	data, err := json.Marshal(spec)
	if err != nil {
		return fmt.Errorf("encode selfupdate spec: %w", err)
	}
	if err := storage.WriteAtomic(path, data); err != nil {
		return fmt.Errorf("write selfupdate spec: %w", err)
	}
	return nil
}

func readUpdateSpec(path string) (updateSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return updateSpec{}, fmt.Errorf("read selfupdate spec: %w", err)
	}
	var spec updateSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return updateSpec{}, fmt.Errorf("decode selfupdate spec: %w", err)
	}
	switch spec.Kind {
	case "", specKindUpdate, specKindRollback, specKindExe:
	default:
		return updateSpec{}, fmt.Errorf("selfupdate spec: unknown kind %q", spec.Kind)
	}
	return spec, nil
}

func writeOutcome(path string, o Outcome) error {
	if err := os.MkdirAll(filepath.Dir(path), cacheDirMode); err != nil {
		return fmt.Errorf("create selfupdate outcome dir: %w", err)
	}
	if err := storage.Save(path, outcomeVersion, o); err != nil {
		return fmt.Errorf("write selfupdate outcome: %w", err)
	}
	return nil
}

func readOutcome(path string) (Outcome, error) {
	var o Outcome
	if err := storage.Load(path, outcomeVersion, nil, &o); err != nil {
		return Outcome{}, err
	}
	return o, nil
}

// copyExecutable duplicates the launcher binary so the worker never runs from
// the file the installer has to overwrite: Windows keeps a running image
// locked, NSIS then skips the file and still exits 0, and the launcher
// relaunches into the version it started from.
func copyExecutable(src, dst string) (err error) {
	if err := os.MkdirAll(filepath.Dir(dst), cacheDirMode); err != nil {
		return fmt.Errorf("create worker dir: %w", err)
	}
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open launcher binary: %w", err)
	}
	defer func() {
		if cerr := in.Close(); cerr != nil {
			slog.Warn("close launcher binary", "path", src, "error", cerr)
		}
	}()
	info, err := in.Stat()
	if err != nil {
		return fmt.Errorf("stat launcher binary: %w", err)
	}

	// The copy lands under a temp name and is renamed only after fsync: a
	// crash mid-copy must never leave a truncated file at the path the
	// worker is later launched from.
	tmp := dst + ".tmp"
	if err := os.Remove(tmp); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove stale worker copy: %w", err)
	}
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return fmt.Errorf("create worker binary: %w", err)
	}
	closed := false
	defer func() {
		if !closed {
			if cerr := out.Close(); cerr != nil {
				slog.Warn("close worker binary", "path", tmp, "error", cerr)
			}
		}
		if err != nil {
			if rerr := os.Remove(tmp); rerr != nil && !errors.Is(rerr, fs.ErrNotExist) {
				slog.Warn("remove partial worker binary", "path", tmp, "error", rerr)
			}
		}
	}()
	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy worker binary: %w", err)
	}
	if err := out.Sync(); err != nil {
		return fmt.Errorf("sync worker binary: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close worker binary: %w", err)
	}
	closed = true
	if err := os.Rename(tmp, dst); err != nil {
		return fmt.Errorf("rename worker binary: %w", err)
	}
	return nil
}

// fileDigest reports the sha-256 of path; a missing file yields an empty
// digest so the caller can tell "was not there" from "was not replaced".
func fileDigest(ctx context.Context, path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("open %s: %w", filepath.Base(path), err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			slog.Warn("close hashed file", "path", path, "error", cerr)
		}
	}()

	hasher := sha256.New()
	buf := make([]byte, hashBufSize)
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, rerr := f.Read(buf)
		if n > 0 {
			if _, herr := hasher.Write(buf[:n]); herr != nil {
				return "", fmt.Errorf("hash %s: %w", filepath.Base(path), herr)
			}
		}
		if errors.Is(rerr, io.EOF) {
			break
		}
		if rerr != nil {
			return "", fmt.Errorf("read %s: %w", filepath.Base(path), rerr)
		}
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
