package selfupdate

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func newRobustService(t *testing.T) (*Service, string) {
	t.Helper()
	dir := t.TempDir()
	s := &Service{dir: dir, notes: mustNotesStore(t, dir), store: mustStore(t, dir), client: mustQuietClient(t), currentVersion: "1.0.0"}
	s.status = Status{State: StateAvailable, CurrentVersion: "1.0.0", AvailableVersion: "1.2.3"}
	return s, dir
}

// A download that failed once (no VPN) and then succeeded must not keep the
// old error in the status: the frontend renders any error as "failed" and
// hides the install button.
func TestDownloadUpdateRetrySuccessClearsError(t *testing.T) {
	content := []byte("this is a fake installer payload")
	_, art := newArtifactServer(t, content)
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(dead.Close)

	s, _ := newRobustService(t)
	bad := art
	bad.URL = dead.URL + "/missing.exe"
	s.pendingArtifact = &bad
	s.pendingVersion = "1.2.3"
	if _, err := s.DownloadUpdate(context.Background()); err == nil {
		t.Fatal("first DownloadUpdate() error = nil, want a 404 failure")
	}
	if got := s.GetStatus(); got.ErrorCode != "download" || got.State != StateAvailable {
		t.Fatalf("after failure status = %+v, want available with a download error", got)
	}

	s.pendingArtifact = &art
	status, err := s.DownloadUpdate(context.Background())
	if err != nil {
		t.Fatalf("second DownloadUpdate() error = %v", err)
	}
	if status.State != StateReady || status.Error != "" || status.ErrorCode != "" {
		t.Fatalf("status after a successful retry = %+v, want ready without an error", status)
	}
}

func TestDownloadUpdateWhenReadyDropsStaleError(t *testing.T) {
	s, _ := newRobustService(t)
	s.status = Status{State: StateReady, CurrentVersion: "1.0.0", AvailableVersion: "1.2.3", Error: "old", ErrorCode: "download"}
	s.readyPath = "/x/setup.exe"

	status, err := s.DownloadUpdate(context.Background())
	if err != nil {
		t.Fatalf("DownloadUpdate() error = %v", err)
	}
	if status.State != StateReady || status.Error != "" || status.ErrorCode != "" {
		t.Fatalf("status = %+v, want ready without an error", status)
	}
	if got := s.GetStatus(); got.Error != "" {
		t.Fatalf("committed status still carries %q", got.Error)
	}
}

func TestDismissUpdateClearsError(t *testing.T) {
	content := []byte("payload")
	_, art := newArtifactServer(t, content)
	s, _ := newRobustService(t)
	s.pendingArtifact = &art
	s.pendingVersion = "1.2.3"
	if _, err := s.DownloadUpdate(context.Background()); err != nil {
		t.Fatalf("DownloadUpdate() error = %v", err)
	}
	s.mu.Lock()
	s.status.Error, s.status.ErrorCode = "stale", "manifest"
	s.mu.Unlock()

	if err := s.DismissUpdate(); err != nil {
		t.Fatalf("DismissUpdate() error = %v", err)
	}
	if got := s.GetStatus(); got.State != StateAvailable || got.Error != "" || got.ErrorCode != "" {
		t.Fatalf("status = %+v, want available without an error", got)
	}
}

// A reloaded UI (or one that never saw the click) can only learn that a
// download is running from the status itself.
func TestDownloadUpdateReportsDownloadingStateAndRevertsOnFailure(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	s, _ := newRobustService(t)
	art := testArtifact([]byte("payload"), "setup.exe")
	art.URL = srv.URL
	s.pendingArtifact = &art
	s.pendingVersion = "1.2.3"

	done := make(chan error, 1)
	go func() {
		_, err := s.DownloadUpdate(context.Background())
		done <- err
	}()
	<-started
	if got := s.GetStatus(); got.State != StateDownloading {
		t.Fatalf("State during download = %v, want %v", got.State, StateDownloading)
	}
	close(release)
	if err := <-done; err == nil {
		t.Fatal("DownloadUpdate() error = nil, want the 404 surfaced")
	}
	if got := s.GetStatus(); got.State != StateAvailable || got.ErrorCode != "download" {
		t.Fatalf("status after failure = %+v, want available with a download error", got)
	}
}

// The background check runs without anyone asking: its failures belong in
// the log, not in a red banner that hides "install" for an update that is
// already on disk.
func TestQuietCheckFailureLeavesStatusAlone(t *testing.T) {
	s, _ := newRobustService(t)
	s.status = Status{State: StateReady, CurrentVersion: "1.0.0", AvailableVersion: "1.2.3"}
	s.ctx = context.Background()

	s.checkQuiet()

	if got := s.GetStatus(); got.State != StateReady || got.Error != "" || got.ErrorCode != "" {
		t.Fatalf("status after a failed quiet check = %+v, want ready without an error", got)
	}
}

func blockingManifestServer(t *testing.T) (*httptest.Server, chan struct{}, chan struct{}, *atomic.Int64) {
	t.Helper()
	started := make(chan struct{}, 8)
	release := make(chan struct{})
	var requests atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		started <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
		srv.Close()
	})
	return srv, started, release, &requests
}

// The user does not care that a background check is hanging on a blocked
// network: "download" must cancel it and go ahead instead of bouncing off
// with "another operation is in progress".
func TestDownloadUpdatePreemptsInFlightCheck(t *testing.T) {
	srv, started, release, _ := blockingManifestServer(t)
	client, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	content := []byte("payload")
	_, art := newArtifactServer(t, content)

	s, _ := newRobustService(t)
	s.client = client
	s.pendingArtifact = &art
	s.pendingVersion = "1.2.3"

	checkErr := make(chan error, 1)
	go func() {
		_, err := s.CheckForUpdate(context.Background())
		checkErr <- err
	}()
	<-started

	status, err := s.DownloadUpdate(context.Background())
	if err != nil {
		t.Fatalf("DownloadUpdate() error = %v, want it to preempt the check", err)
	}
	if status.State != StateReady {
		t.Fatalf("State = %v, want ready", status.State)
	}
	if err := <-checkErr; !errors.Is(err, context.Canceled) {
		t.Fatalf("CheckForUpdate() error = %v, want context.Canceled", err)
	}
	if got := s.GetStatus(); got.State != StateReady || got.Error != "" {
		t.Fatalf("status = %+v, want ready without an error from the cancelled check", got)
	}
	close(release)
}

// A manual check that lands while the background one is still in flight
// joins it: one request, one answer for both, no ErrBusy.
func TestConcurrentChecksShareOneRequest(t *testing.T) {
	srv, started, release, requests := blockingManifestServer(t)
	client, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	s, _ := newRobustService(t)
	s.client = client

	joined := make(chan struct{}, 1)
	prev := onCheckJoined
	onCheckJoined = func() { joined <- struct{}{} }
	t.Cleanup(func() { onCheckJoined = prev })

	first := make(chan error, 1)
	go func() {
		_, err := s.CheckForUpdate(context.Background())
		first <- err
	}()
	<-started

	second := make(chan error, 1)
	go func() {
		_, err := s.CheckForUpdate(context.Background())
		second <- err
	}()
	select {
	case <-joined:
	case <-time.After(5 * time.Second):
		t.Fatal("second check never joined the one in flight")
	}
	close(release)

	for _, ch := range []chan error{first, second} {
		err := <-ch
		if errors.Is(err, ErrBusy) {
			t.Fatal("a concurrent check got ErrBusy instead of joining the one in flight")
		}
		if err == nil {
			t.Fatal("check error = nil, want the 500 surfaced")
		}
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("server saw %d manifest requests, want 1", got)
	}
	if got := s.GetStatus(); got.ErrorCode != "manifest" {
		t.Fatalf("a manual check joined the quiet one but its failure was not surfaced: %+v", got)
	}
}

func TestCancelDownloadStopsInFlightDownload(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() { close(release); srv.Close() })

	s, _ := newRobustService(t)
	art := testArtifact([]byte("payload"), "setup.exe")
	art.URL = srv.URL
	s.pendingArtifact = &art
	s.pendingVersion = "1.2.3"

	done := make(chan error, 1)
	go func() {
		_, err := s.DownloadUpdate(context.Background())
		done <- err
	}()
	<-started

	if err := s.CancelDownload(); err != nil {
		t.Fatalf("CancelDownload() error = %v", err)
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("DownloadUpdate() error = %v, want context.Canceled", err)
	}
	got := s.GetStatus()
	if got.State != StateAvailable || got.Error != "" || got.ErrorCode != "" {
		t.Fatalf("status after cancel = %+v, want available without an error", got)
	}
	s.mu.Lock()
	busy := s.busy
	s.mu.Unlock()
	if busy {
		t.Fatal("service still busy after a cancelled download")
	}
}

func TestCancelDownloadWithoutDownloadIsNoop(t *testing.T) {
	s, _ := newRobustService(t)
	if err := s.CancelDownload(); err != nil {
		t.Fatalf("CancelDownload() error = %v, want nil", err)
	}
}

type dialFailTransport struct{ attempts atomic.Int64 }

func (d *dialFailTransport) RoundTrip(*http.Request) (*http.Response, error) {
	d.attempts.Add(1)
	return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connect: connection refused")}
}

// Retrying only helps when the link dropped mid-transfer. A host that cannot
// be reached at all (no VPN, DNS poisoned, port closed) answers the same way
// three times and just makes the user wait.
func TestDownloadDoesNotRetryWhenDialFails(t *testing.T) {
	shortBackoff(t)
	c, err := NewClient("https://updates.example.test")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	tr := &dialFailTransport{}
	c.downloadClient.Transport = tr

	art := testArtifact([]byte("payload"), "setup.exe")
	art.URL = "https://updates.example.test/setup.exe"
	_, err = c.Download(context.Background(), art, t.TempDir(), nil)
	if err == nil {
		t.Fatal("Download() error = nil")
	}
	if got := tr.attempts.Load(); got != 1 {
		t.Fatalf("dial failure was attempted %d times, want 1", got)
	}
}
