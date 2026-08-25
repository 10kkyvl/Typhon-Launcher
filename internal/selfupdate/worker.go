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

	"typhon/internal/storage"
)

const (
	workerDirName  = "worker"
	outcomeName    = "last-update.json"
	specName       = "update-spec.json"
	hashBufSize    = 256 * 1024
	outcomeMaxAge  = 24 * time.Hour
	outcomeVersion = 1
)

type updateSpec struct {
	InstallerPath string `json:"installerPath"`
	ParentPID     int    `json:"parentPid"`
	RelaunchPath  string `json:"relaunchPath"`
	Version       string `json:"version"`
}

// Outcome is what the worker leaves behind for the relaunched launcher: the
// install runs after the UI is gone, so without it a failed update is
// indistinguishable from an update that never started.
type Outcome struct {
	Version    string    `json:"version"`
	OK         bool      `json:"ok"`
	Error      string    `json:"error,omitempty"`
	FinishedAt time.Time `json:"finishedAt"`
}

var (
	parentExitTimeout  = 30 * time.Second
	parentPollInterval = 250 * time.Millisecond
	applyTimeout       = 5 * time.Minute
)

var errParentStillRunning = errors.New("selfupdate: launcher did not exit before the timeout")

func WorkerDir(configDir string) (string, error) {
	dir, err := CacheDir(configDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, workerDirName), nil
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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
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
	return spec, nil
}

func writeOutcome(path string, o Outcome) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
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
func copyExecutable(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
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
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return fmt.Errorf("create worker binary: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		if cerr := out.Close(); cerr != nil {
			slog.Warn("close worker binary", "path", dst, "error", cerr)
		}
		return fmt.Errorf("copy worker binary: %w", err)
	}
	if err := out.Sync(); err != nil {
		if cerr := out.Close(); cerr != nil {
			slog.Warn("close worker binary", "path", dst, "error", cerr)
		}
		return fmt.Errorf("sync worker binary: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close worker binary: %w", err)
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
