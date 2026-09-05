package install

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

type logSink struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (s *logSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *logSink) text() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func captureLogs(t *testing.T) *logSink {
	t.Helper()
	sink := &logSink{}
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(sink, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return sink
}

// TestSweepPartialItemDistinguishesMissingFromStatError closes finding 4:
// sweepPartialItem treated every os.Stat error on the .partial path as "there
// is nothing to sweep", silently swallowing permission and I/O errors the
// same way it swallows a plain ErrNotExist. Only ErrNotExist may stay quiet;
// any other stat failure must be logged since the caller has nowhere to
// return it (this runs from the startup sweep too).
func TestSweepPartialItemDistinguishesMissingFromStatError(t *testing.T) {
	t.Run("missing partial is a silent no-op", func(t *testing.T) {
		sink := captureLogs(t)
		s := mustServiceAt(t, t.TempDir())
		dest := filepath.Join(t.TempDir(), "Game")
		item := Installation{ID: "a", Type: TypePortable, Destination: dest}

		s.sweepPartialItem(context.Background(), item)

		if strings.Contains(sink.text(), "stat partial install") {
			t.Fatalf("logged a warning for a simply-missing partial: %q", sink.text())
		}
	})

	t.Run("stat error is logged instead of treated as missing", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("POSIX-права каталога не действуют на windows")
		}
		if os.Geteuid() == 0 {
			t.Skip("запущено от root: chmod не мешает доступу")
		}

		root := t.TempDir()
		locked := filepath.Join(root, "locked")
		if err := os.Mkdir(locked, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		dest := filepath.Join(locked, "Game")
		mkFile(t, dest+partialSuffix, 4)
		if err := os.Chmod(locked, 0o000); err != nil { //nolint:gosec // G302: тест инварианта, требует непроходимый каталог
			t.Fatalf("chmod: %v", err)
		}
		t.Cleanup(func() {
			if err := os.Chmod(locked, 0o700); err != nil { //nolint:gosec // G302: возврат прав, выставленных выше по той же причине
				t.Errorf("restore chmod: %v", err)
			}
		})

		sink := captureLogs(t)
		s := mustServiceAt(t, t.TempDir())
		item := Installation{ID: "a", Type: TypePortable, Destination: dest}

		s.sweepPartialItem(context.Background(), item)

		if !strings.Contains(sink.text(), "stat partial install") {
			t.Fatalf("stat error was not logged: %q", sink.text())
		}
	})
}
