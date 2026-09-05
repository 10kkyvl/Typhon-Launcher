package platform

import (
	"log/slog"
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

// TestCloseKeyLogsFailureInsteadOfSwallowingIt closes finding 6
// (storage_windows.go): cpuName/windowsProductName used to `defer key.Close()`
// directly, dropping any error from an already-broken registry handle. The
// shared closeKey helper (mirrors internal/install/uninstall_windows.go)
// must log a close failure instead of discarding it.
func TestCloseKeyLogsFailureInsteadOfSwallowingIt(t *testing.T) {
	key, err := openRegistryKey(`SOFTWARE\Microsoft\Windows NT\CurrentVersion`)
	if err != nil {
		t.Fatalf("open key: %v", err)
	}
	if err := key.Close(); err != nil {
		t.Fatalf("close key: %v", err)
	}

	sink := captureLogs(t)
	// The handle is already closed above: closing it again must fail at the
	// syscall level, and that failure must reach the log.
	closeKey(key)

	if !strings.Contains(sink.text(), "close registry key") {
		t.Fatalf("close registry key failure was not logged: %q", sink.text())
	}
}
