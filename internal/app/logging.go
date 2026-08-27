package app

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"typhon/internal/settings"
)

const (
	maxLogSize    int64 = 10 << 20
	maxLogBackups       = 5
)

// В windowsgui-сборке нет консоли, запись в os.Stderr возвращает ошибку, а
// anacrolix/log паникует на любой ошибке обработчика, роняя приложение.
type quietWriter struct {
	w io.Writer
}

func (q quietWriter) Write(p []byte) (int, error) {
	_, _ = q.w.Write(p) //nolint:errcheck // windowsgui build: anacrolix/log panics if the handler returns an error, see comment above
	return len(p), nil
}

// InitLogging configures the process-wide slog default handler: text output to
// os.Stderr and, when the log directory is writable, to a size-rotated
// typhon.log under settings.ConfigDir(). main.go must call it as:
//
//	if err := app.InitLogging(); err != nil {
//	    // report err (stderr logging is already active at this point)
//	}
//
// A non-nil error means the file sink could not be opened (locked file, no
// permissions, unresolved config dir); logging to stderr is wired up before
// the file is attempted, so the launcher keeps starting either way — the
// caller decides how to surface the error.
func InitLogging() error {
	writers := []io.Writer{quietWriter{os.Stderr}}
	rotator, err := openLogWriter()
	if err == nil {
		writers = append(writers, quietWriter{rotator})
	}
	handler := slog.NewTextHandler(io.MultiWriter(writers...), &slog.HandlerOptions{Level: slog.LevelInfo})
	slog.SetDefault(slog.New(handler))
	return err
}

// Logger returns a logger with a stable "component" field attached. Safe to
// call before InitLogging: falls back to slog.Default(), which is always a
// valid logger even before SetDefault has been called.
func Logger(component string) *slog.Logger {
	return slog.Default().With("component", component)
}

func openLogWriter() (*rotatingWriter, error) {
	dir, err := settings.ConfigDir()
	if err != nil {
		return nil, fmt.Errorf("config dir: %w", err)
	}
	if dir == "" {
		return nil, errors.New("config dir: empty")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create %s: %w", dir, err)
	}
	return newRotatingWriter(filepath.Join(dir, logFileName), maxLogSize, maxLogBackups)
}

// rotatingWriter is an io.WriteCloser over typhon.log that rotates to
// typhon.log.1 .. typhon.log.<backups> once the current file would exceed
// maxSize, tracking bytes written in memory so a write never needs to stat
// the file. Safe for concurrent use: slog handlers are invoked concurrently
// by callers across goroutines.
type rotatingWriter struct {
	mu      sync.Mutex
	path    string
	file    *os.File
	written int64
	maxSize int64
	backups int
}

func newRotatingWriter(path string, maxSize int64, backups int) (*rotatingWriter, error) {
	if path == "" {
		return nil, errors.New("rotating writer: empty path")
	}
	if backups < 1 {
		return nil, fmt.Errorf("rotating writer: backups must be >= 1, got %d", backups)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	info, err := f.Stat()
	if err != nil {
		closeErr := f.Close()
		return nil, errors.Join(fmt.Errorf("stat %s: %w", path, err), closeErr)
	}
	return &rotatingWriter{
		path:    path,
		file:    f,
		written: info.Size(),
		maxSize: maxSize,
		backups: backups,
	}, nil
}

func (r *rotatingWriter) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var rotateErr error
	if r.written > 0 && r.written+int64(len(p)) > r.maxSize {
		rotateErr = r.rotate()
	}
	n, err := r.file.Write(p)
	r.written += int64(n)
	if err != nil {
		return n, errors.Join(rotateErr, fmt.Errorf("write %s: %w", r.path, err))
	}
	if rotateErr != nil {
		return n, fmt.Errorf("rotate %s: %w", r.path, rotateErr)
	}
	return n, nil
}

func (r *rotatingWriter) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.file.Close(); err != nil {
		return fmt.Errorf("close %s: %w", r.path, err)
	}
	return nil
}

func (r *rotatingWriter) backupPath(n int) string {
	return fmt.Sprintf("%s.%d", r.path, n)
}

// rotate closes the active file, shuffles typhon.log[.N] backups down by one
// slot and reopens r.path. When the shuffle fails partway (a stale ".old"
// backup could not be removed, a hop in the chain could not be renamed), it
// still reopens whatever now sits at r.path so the writer keeps accepting
// log lines instead of failing every future write — the shuffle error is
// always returned to the caller, it is never swallowed.
func (r *rotatingWriter) rotate() error {
	if err := r.file.Close(); err != nil {
		return fmt.Errorf("close %s: %w", r.path, err)
	}
	shuffleErr := r.shuffleBackups()
	f, openErr := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if openErr != nil {
		return errors.Join(shuffleErr, fmt.Errorf("open %s: %w", r.path, openErr))
	}
	info, statErr := f.Stat()
	if statErr != nil {
		return errors.Join(shuffleErr, fmt.Errorf("stat %s: %w", r.path, statErr), f.Close())
	}
	r.file = f
	r.written = info.Size()
	return shuffleErr
}

func (r *rotatingWriter) shuffleBackups() error {
	oldest := r.backupPath(r.backups)
	if _, err := os.Stat(oldest); err == nil {
		if err := os.Remove(oldest); err != nil {
			return fmt.Errorf("remove %s: %w", oldest, err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("stat %s: %w", oldest, err)
	}
	for n := r.backups - 1; n >= 1; n-- {
		from := r.backupPath(n)
		to := r.backupPath(n + 1)
		if _, err := os.Stat(from); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return fmt.Errorf("stat %s: %w", from, err)
		}
		if err := os.Rename(from, to); err != nil {
			return fmt.Errorf("rename %s to %s: %w", from, to, err)
		}
	}
	if err := os.Rename(r.path, r.backupPath(1)); err != nil {
		return fmt.Errorf("rename %s to %s: %w", r.path, r.backupPath(1), err)
	}
	return nil
}
