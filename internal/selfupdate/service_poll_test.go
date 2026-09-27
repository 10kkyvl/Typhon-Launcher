package selfupdate

import (
	"context"
	"crypto/ed25519"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// pollServer is a manifest endpoint whose answer the test swaps at will:
// nothing (a 500) until serve is called, the given signed manifest after.
type pollServer struct {
	srv      *httptest.Server
	mu       sync.Mutex
	body     []byte
	requests atomic.Int64
}

func newPollServer(t *testing.T) *pollServer {
	t.Helper()
	ps := &pollServer{}
	ps.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ps.requests.Add(1)
		ps.mu.Lock()
		body := ps.body
		ps.mu.Unlock()
		if body == nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if _, err := w.Write(body); err != nil {
			t.Errorf("write manifest: %v", err)
		}
	}))
	t.Cleanup(ps.srv.Close)
	return ps
}

func (ps *pollServer) serve(body []byte) {
	ps.mu.Lock()
	ps.body = body
	ps.mu.Unlock()
}

// releaseManifest describes a release the running platform can install, so
// the check goes all the way to "available" instead of stopping at a
// missing artifact.
func releaseManifest(version string) Manifest {
	kind := KindInstaller
	name := "typhon-setup.exe"
	if runtime.GOOS == "darwin" {
		kind = KindBundle
		name = "typhon-darwin-bundle.zip"
	}
	return Manifest{
		Version:     version,
		PublishedAt: time.Now(),
		Artifacts: []Artifact{{
			OS: runtime.GOOS, Arch: runtime.GOARCH, Kind: kind, Name: name,
			URL: "https://cdn.example.com/" + name, Size: 1024, SHA256: strings.Repeat("0123456789abcdef", 4),
		}},
	}
}

// observeQuietChecks reports every finished background check on the
// returned channel. The send never blocks: a test that is slow to read
// must not stall the service's schedule and distort what it measures.
func observeQuietChecks(t *testing.T) <-chan struct{} {
	t.Helper()
	checks := make(chan struct{}, 16)
	prev := onQuietCheckDone
	onQuietCheckDone = func() {
		select {
		case checks <- struct{}{}:
		default:
		}
	}
	t.Cleanup(func() { onQuietCheckDone = prev })
	return checks
}

// awaitStatus reads finished checks until one leaves the service in a
// status cond accepts.
func awaitStatus(t *testing.T, s *Service, checks <-chan struct{}, what string, cond func(Status) bool) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case <-checks:
			if cond(s.GetStatus()) {
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s, status = %+v", what, s.GetStatus())
		}
	}
}

func startPolling(t *testing.T, ps *pollServer, pub ed25519.PublicKey) *Service {
	t.Helper()
	client, err := newClientWithKey(ps.srv.URL, pub)
	if err != nil {
		t.Fatalf("newClientWithKey: %v", err)
	}
	dir := t.TempDir()
	s := &Service{dir: dir, notes: mustNotesStore(t, dir), store: mustStore(t, dir), client: client, currentVersion: "1.0.0"}
	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	t.Cleanup(func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Errorf("ServiceShutdown: %v", err)
		}
	})
	return s
}

// The launcher autostarts before the network is up more often than not:
// the check at startup fails for a boring reason, and waiting a full
// interval for the next attempt leaves the user without the update for as
// long as they stay in.
func TestPeriodicCheckRetriesAfterFailedStart(t *testing.T) {
	prevInterval, prevRetry := checkInterval, checkRetryBase
	checkInterval, checkRetryBase = time.Hour, 20*time.Millisecond
	t.Cleanup(func() { checkInterval, checkRetryBase = prevInterval, prevRetry })

	priv, pub := testKeyPair(t)
	ps := newPollServer(t)
	checks := observeQuietChecks(t)
	s := startPolling(t, ps, pub)

	awaitStatus(t, s, checks, "the startup check to fail", func(Status) bool { return ps.requests.Load() >= 1 })
	if got := s.GetStatus(); got.State != StateIdle || got.Error != "" {
		t.Fatalf("status after a failed quiet check = %+v, want idle without an error", got)
	}

	ps.serve(signManifest(t, priv, releaseManifest("1.2.3")))
	awaitStatus(t, s, checks, "a retried check to find 1.2.3", func(got Status) bool {
		return got.State == StateAvailable && got.AvailableVersion == "1.2.3"
	})
}

// A release that goes out while the launcher is open must reach it without
// a restart: the periodic check is what turns into the toast and the bell.
func TestPeriodicCheckNoticesReleaseWhileRunning(t *testing.T) {
	prevInterval := checkInterval
	checkInterval = 20 * time.Millisecond
	t.Cleanup(func() { checkInterval = prevInterval })

	priv, pub := testKeyPair(t)
	ps := newPollServer(t)
	ps.serve(signManifest(t, priv, releaseManifest("1.0.0")))
	checks := observeQuietChecks(t)
	s := startPolling(t, ps, pub)

	awaitStatus(t, s, checks, "the startup check to complete", func(got Status) bool { return !got.CheckedAt.IsZero() })
	if got := s.GetStatus(); got.State != StateIdle {
		t.Fatalf("status with the running version published = %+v, want idle", got)
	}

	ps.serve(signManifest(t, priv, releaseManifest("1.2.3")))
	awaitStatus(t, s, checks, "the next poll to find 1.2.3", func(got Status) bool {
		return got.State == StateAvailable && got.AvailableVersion == "1.2.3"
	})
	if got := ps.requests.Load(); got < 2 {
		t.Fatalf("server saw %d manifest requests, want the poll to keep asking", got)
	}
}

func TestNextCheckDelay(t *testing.T) {
	prevInterval, prevRetry := checkInterval, checkRetryBase
	t.Cleanup(func() { checkInterval, checkRetryBase = prevInterval, prevRetry })

	checkInterval, checkRetryBase = 15*time.Minute, 30*time.Second
	cases := []struct {
		failures int
		want     time.Duration
	}{
		{0, 15 * time.Minute},
		{1, 30 * time.Second},
		{2, time.Minute},
		{3, 2 * time.Minute},
		{4, 4 * time.Minute},
		{5, 8 * time.Minute},
		{6, 15 * time.Minute},
		{40, 15 * time.Minute},
	}
	for _, tc := range cases {
		if got := nextCheckDelay(tc.failures); got != tc.want {
			t.Errorf("nextCheckDelay(%d) = %v, want %v", tc.failures, got, tc.want)
		}
	}

	// A test-sized interval below the first retry must still win: the
	// backoff exists to try sooner than the interval, never later.
	checkInterval = 20 * time.Millisecond
	if got := nextCheckDelay(1); got != 20*time.Millisecond {
		t.Errorf("nextCheckDelay(1) with a 20ms interval = %v, want 20ms", got)
	}
}
