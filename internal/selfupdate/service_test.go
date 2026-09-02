package selfupdate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func sha256Hex(t *testing.T, data []byte) string {
	t.Helper()
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func writeTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustNotesStore(t *testing.T, dir string) *notesStore {
	t.Helper()
	s, err := newNotesStore(dir)
	if err != nil {
		t.Fatalf("newNotesStore: %v", err)
	}
	return s
}

func mustStore(t *testing.T, dir string) *Store {
	t.Helper()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return s
}

// mustQuietClient points a *Client at a local server that always answers
// fast, so that a Service's ServiceStartup-triggered immediate background
// check never reaches the real network.
func mustQuietClient(t *testing.T) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

func TestNewServiceSucceeds(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	s, err := NewService()
	if err != nil {
		t.Fatalf("NewService() error = %v, want nil", err)
	}
	if s.currentVersion == "" {
		t.Fatal("currentVersion is empty")
	}
}

func TestServiceStartupCorruptStateFailsAndBlocksFurtherPersist(t *testing.T) {
	dir := t.TempDir()
	cacheDir, err := CacheDir(dir)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}
	statePath := filepath.Join(cacheDir, "state.json")
	writeTestFile(t, statePath, []byte("{not valid json"))

	s := &Service{dir: dir, notes: mustNotesStore(t, dir), store: mustStore(t, dir), client: mustQuietClient(t), currentVersion: "1.0.0"}
	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err == nil {
		t.Fatal("ServiceStartup on corrupt state.json returned nil error")
	}
	if s.cancel != nil {
		t.Fatal("cancel was not rolled back after a failed load")
	}

	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read state before Save: %v", err)
	}

	// CheckForUpdate and DownloadUpdate persist through this exact *Store, so
	// proving it now refuses to write proves neither of them can either.
	if err := s.store.Save(stored{AvailableVersion: "9.9.9"}); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("store.Save() error = %v, want ErrReadOnly", err)
	}

	after, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read state after Save: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("state.json was modified: before=%q after=%q", before, after)
	}
}

func TestServiceStartupInvalidReadyArtifactIsCleared(t *testing.T) {
	dir := t.TempDir()
	cacheDir, err := CacheDir(dir)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}
	readyPath := filepath.Join(cacheDir, "1.2.3", "setup.exe")
	writeTestFile(t, readyPath, []byte("corrupted-bytes"))

	art := Artifact{
		OS: runtime.GOOS, Arch: runtime.GOARCH, Kind: KindInstaller, Name: "setup.exe",
		URL: "https://example.com/setup.exe", Size: 999, SHA256: sha256Hex(t, []byte("expected-bytes")),
	}
	store := mustStore(t, dir)
	if err := store.Save(stored{AvailableVersion: "1.2.3", Artifact: &art, ReadyPath: readyPath}); err != nil {
		t.Fatalf("seed store: %v", err)
	}

	s := &Service{dir: dir, notes: mustNotesStore(t, dir), store: store, client: mustQuietClient(t), currentVersion: "1.0.0"}
	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	if err := s.ServiceShutdown(); err != nil {
		t.Fatalf("ServiceShutdown: %v", err)
	}

	if got := s.GetStatus().State; got == StateReady {
		t.Fatalf("State = %v, want anything but StateReady once the artifact fails verification", got)
	}
	if _, statErr := os.Stat(readyPath); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("readyPath still exists after failing verification: %v", statErr)
	}

	v, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if v.Artifact != nil || v.ReadyPath != "" {
		t.Fatalf("stored state still carries the invalid artifact: %+v", v)
	}
}

func TestServiceStartupCleansStaleCacheEntries(t *testing.T) {
	dir := t.TempDir()
	cacheDir, err := CacheDir(dir)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	staleDir := filepath.Join(cacheDir, "1.0.0")
	writeTestFile(t, filepath.Join(staleDir, "old-setup.exe"), []byte("stale"))

	content := []byte("hello world")
	keepFinal := filepath.Join(cacheDir, "2.0.0", "setup.exe")
	writeTestFile(t, keepFinal, content)
	keepTemp := filepath.Join(cacheDir, "2.0.0", ".selfupdate-abandoned")
	writeTestFile(t, keepTemp, []byte("partial-download"))

	orphanFile := filepath.Join(cacheDir, "orphan.txt")
	writeTestFile(t, orphanFile, []byte("x"))

	art := Artifact{
		OS: runtime.GOOS, Arch: runtime.GOARCH, Kind: KindInstaller, Name: "setup.exe",
		URL: "https://example.com/setup.exe", Size: int64(len(content)), SHA256: sha256Hex(t, content),
	}
	store := mustStore(t, dir)
	if err := store.Save(stored{AvailableVersion: "2.0.0", Artifact: &art, ReadyPath: keepFinal}); err != nil {
		t.Fatalf("seed store: %v", err)
	}

	s := &Service{dir: dir, notes: mustNotesStore(t, dir), store: store, client: mustQuietClient(t), currentVersion: "1.0.0"}
	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	if err := s.ServiceShutdown(); err != nil {
		t.Fatalf("ServiceShutdown: %v", err)
	}

	if _, statErr := os.Stat(staleDir); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("stale version dir survived cleanup: %v", statErr)
	}
	if _, statErr := os.Stat(keepFinal); statErr != nil {
		t.Fatalf("kept artifact was removed: %v", statErr)
	}
	if _, statErr := os.Stat(keepTemp); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("leftover temp file inside the kept version dir survived cleanup: %v", statErr)
	}
	if _, statErr := os.Stat(orphanFile); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("orphaned top-level file survived cleanup: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(cacheDir, "state.json")); statErr != nil {
		t.Fatalf("state.json was removed by cleanup: %v", statErr)
	}
}

func TestBuildCheckStatusBranches(t *testing.T) {
	s := &Service{currentVersion: "1.0.0"}
	m := validManifest()
	art, err := m.ArtifactFor("windows", "amd64")
	if err != nil {
		t.Fatalf("ArtifactFor: %v", err)
	}
	current := Status{State: StateAvailable, CurrentVersion: "1.0.0", AvailableVersion: "0.9.0", Notes: "previous notes"}

	t.Run("update available", func(t *testing.T) {
		status, st, err := s.buildCheckStatus(current, "", nil, m, art, nil, true, nil)
		if err != nil {
			t.Fatalf("buildCheckStatus() error = %v, want nil", err)
		}
		if status.State != StateAvailable {
			t.Fatalf("State = %v, want StateAvailable", status.State)
		}
		if status.AvailableVersion != m.Version {
			t.Fatalf("AvailableVersion = %q, want %q", status.AvailableVersion, m.Version)
		}
		if status.Error != "" || status.ErrorCode != "" {
			t.Fatalf("status carries an error on the success path: %+v", status)
		}
		if st.AvailableVersion != m.Version {
			t.Fatalf("stored.AvailableVersion = %q, want %q", st.AvailableVersion, m.Version)
		}
	})

	t.Run("current version", func(t *testing.T) {
		status, _, err := s.buildCheckStatus(current, "", nil, m, art, nil, false, nil)
		if err != nil {
			t.Fatalf("buildCheckStatus() error = %v, want nil", err)
		}
		if status.State != StateIdle {
			t.Fatalf("State = %v, want StateIdle when not newer", status.State)
		}
	})

	t.Run("no artifact for platform", func(t *testing.T) {
		status, st, err := s.buildCheckStatus(current, "", nil, m, Artifact{}, ErrNoArtifact, false, nil)
		if !errors.Is(err, ErrNoArtifact) {
			t.Fatalf("buildCheckStatus() error = %v, want ErrNoArtifact", err)
		}
		if status.ErrorCode != "artifact" {
			t.Fatalf("ErrorCode = %q, want %q", status.ErrorCode, "artifact")
		}
		if status.AvailableVersion != current.AvailableVersion {
			t.Fatalf("AvailableVersion = %q, want the previous value %q to survive an artifact error", status.AvailableVersion, current.AvailableVersion)
		}
		if st != (stored{}) {
			t.Fatalf("stored = %+v, want zero value on the error path", st)
		}
	})

	t.Run("incomparable version", func(t *testing.T) {
		status, _, err := s.buildCheckStatus(current, "", nil, m, art, nil, false, ErrInvalidVersion)
		if !errors.Is(err, ErrInvalidVersion) {
			t.Fatalf("buildCheckStatus() error = %v, want ErrInvalidVersion", err)
		}
		if status.ErrorCode != "version" {
			t.Fatalf("ErrorCode = %q, want %q", status.ErrorCode, "version")
		}
		if status.AvailableVersion != current.AvailableVersion {
			t.Fatalf("AvailableVersion = %q, want the previous value %q to survive a version error", status.AvailableVersion, current.AvailableVersion)
		}
	})
}

// TestBuildCheckStatusCarriesOverMatchingReadyArtifact closes КРИТ-1: without
// the carry-over branch, a periodic background check that lands while a
// verified update is already sitting in the cache (StateReady) would report
// the exact same manifest version as "newer" (currentVersion never changes
// until Apply runs), build an empty stored{} in place of the real one, and
// drop the service back to StateAvailable — after which ApplyUpdate fails
// with ErrNotReady and the next ServiceStartup's cleanupCache deletes the
// already-downloaded installer outright.
func TestBuildCheckStatusCarriesOverMatchingReadyArtifact(t *testing.T) {
	s := &Service{currentVersion: "1.0.0"}
	m := validManifest()
	art, err := m.ArtifactFor("windows", "amd64")
	if err != nil {
		t.Fatalf("ArtifactFor: %v", err)
	}
	readyPath := `C:\cache\selfupdate\1.2.3\typhon-setup.exe`
	readyArtifact := art
	current := Status{
		State: StateReady, CurrentVersion: "1.0.0", AvailableVersion: m.Version,
		Notes: m.Notes, TotalBytes: art.Size, DownloadedBytes: art.Size,
	}

	status, st, err := s.buildCheckStatus(current, readyPath, &readyArtifact, m, art, nil, true, nil)
	if err != nil {
		t.Fatalf("buildCheckStatus() error = %v, want nil", err)
	}
	if status.State != StateReady {
		t.Fatalf("State = %v, want StateReady (the ready update must survive a re-check of the same version)", status.State)
	}
	if status.TotalBytes != art.Size || status.DownloadedBytes != art.Size {
		t.Fatalf("TotalBytes/DownloadedBytes = %d/%d, want %d/%d (fully downloaded, not zeroed)", status.TotalBytes, status.DownloadedBytes, art.Size, art.Size)
	}
	if st.Artifact == nil || *st.Artifact != art {
		t.Fatalf("stored.Artifact = %+v, want the carried-over artifact %+v", st.Artifact, art)
	}
	if st.ReadyPath != readyPath {
		t.Fatalf("stored.ReadyPath = %q, want %q carried over, not dropped", st.ReadyPath, readyPath)
	}
}

func TestBuildCheckStatusNoCarryOverWhenArtifactDiffers(t *testing.T) {
	s := &Service{currentVersion: "1.0.0"}
	m := validManifest()
	art, err := m.ArtifactFor("windows", "amd64")
	if err != nil {
		t.Fatalf("ArtifactFor: %v", err)
	}
	readyPath := `C:\cache\selfupdate\1.2.3\typhon-setup.exe`
	stalerArtifact := art
	stalerArtifact.SHA256 = strings.Repeat("f", 64)
	current := Status{State: StateReady, CurrentVersion: "1.0.0", AvailableVersion: m.Version}

	status, st, err := s.buildCheckStatus(current, readyPath, &stalerArtifact, m, art, nil, true, nil)
	if err != nil {
		t.Fatalf("buildCheckStatus() error = %v, want nil", err)
	}
	if status.State != StateAvailable {
		t.Fatalf("State = %v, want StateAvailable when the manifest's artifact no longer matches what was downloaded", status.State)
	}
	if st.Artifact != nil || st.ReadyPath != "" {
		t.Fatalf("stored = %+v, want no carried-over artifact/path for a hash mismatch", st)
	}
}

func TestBuildCheckStatusNoCarryOverWhenVersionDiffers(t *testing.T) {
	s := &Service{currentVersion: "1.0.0"}
	m := validManifest()
	m.Version = "1.3.0"
	art, err := m.ArtifactFor("windows", "amd64")
	if err != nil {
		t.Fatalf("ArtifactFor: %v", err)
	}
	readyArtifact := art
	readyArtifact.Name = "typhon-setup-1.2.3.exe"
	current := Status{State: StateReady, CurrentVersion: "1.0.0", AvailableVersion: "1.2.3"}

	status, st, err := s.buildCheckStatus(current, `C:\cache\1.2.3\typhon-setup-1.2.3.exe`, &readyArtifact, m, art, nil, true, nil)
	if err != nil {
		t.Fatalf("buildCheckStatus() error = %v, want nil", err)
	}
	if status.State != StateAvailable {
		t.Fatalf("State = %v, want StateAvailable when a newer version was published than the one sitting ready", status.State)
	}
	if st.Artifact != nil || st.ReadyPath != "" {
		t.Fatalf("stored = %+v, want no carried-over artifact/path for a different version", st)
	}
}

// TestCheckForUpdateNetworkFailureDoesNotClearReadyState is the CheckForUpdate-
// level companion to the buildCheckStatus carry-over tests above: a failed
// manifest fetch returns before ever consulting readyPath/readyArtifact, so a
// verified, ready-to-apply update must survive a transient network error.
// (A full CheckForUpdate success-path test through the real network cannot
// be written here — see НЕ СДЕЛАНО in the report: FetchManifest verifies a
// real ed25519 signature against the key embedded in model.go, and this
// package does not have the matching private key.)
func TestCheckForUpdateNetworkFailureDoesNotClearReadyState(t *testing.T) {
	content := []byte("already-downloaded installer payload")
	_, art := newArtifactServer(t, content)

	dir := t.TempDir()
	store := mustStore(t, dir)
	s := &Service{dir: dir, notes: mustNotesStore(t, dir), store: store, client: mustQuietClient(t), currentVersion: "1.0.0"}
	s.pendingArtifact = &art
	s.pendingVersion = "1.2.3"

	if status, err := s.DownloadUpdate(context.Background()); err != nil || status.State != StateReady {
		t.Fatalf("precondition DownloadUpdate() = (%+v, %v), want StateReady, nil", status, err)
	}
	readyPathBefore, readyArtifactBefore := s.readyPath, s.readyArtifact

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	failingClient, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	s.client = failingClient

	if _, err := s.CheckForUpdate(context.Background()); err == nil {
		t.Fatal("CheckForUpdate() error = nil, want an error for a 500 response")
	}

	if s.GetStatus().State != StateReady {
		t.Fatalf("State = %v, want StateReady preserved across a failed check", s.GetStatus().State)
	}
	if s.readyPath != readyPathBefore || s.readyArtifact != readyArtifactBefore {
		t.Fatalf("readyPath/readyArtifact changed after a failed check: got (%q, %p), want (%q, %p)",
			s.readyPath, s.readyArtifact, readyPathBefore, readyArtifactBefore)
	}
}

func TestCheckForUpdateNetworkFailureDoesNotTouchStore(t *testing.T) {
	dir := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	s := &Service{dir: dir, notes: mustNotesStore(t, dir), store: mustStore(t, dir), client: client, currentVersion: "1.0.0"}

	status, err := s.CheckForUpdate(context.Background())
	if err == nil {
		t.Fatal("CheckForUpdate() error = nil, want an error for a 500 response")
	}
	if status.ErrorCode != "manifest" {
		t.Fatalf("ErrorCode = %q, want %q", status.ErrorCode, "manifest")
	}
	if status.Error == "" {
		t.Fatal("Error is empty")
	}

	cacheDir, err := CacheDir(dir)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(cacheDir, "state.json")); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("state.json was created on a failed check: %v", statErr)
	}
}

func TestCheckForUpdateCancelledContext(t *testing.T) {
	dir := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	s := &Service{dir: dir, notes: mustNotesStore(t, dir), store: mustStore(t, dir), client: client, currentVersion: "1.0.0"}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	status, err := s.CheckForUpdate(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CheckForUpdate() error = %v, want context.Canceled", err)
	}
	if status.Error != "" || status.ErrorCode != "" {
		t.Fatalf("a cancelled check is nobody's failure, yet status = %+v", status)
	}

	cacheDir, err := CacheDir(dir)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(cacheDir, "state.json")); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("state.json was created for a cancelled check: %v", statErr)
	}
}

func TestDownloadUpdateNotCheckedYet(t *testing.T) {
	dir := t.TempDir()
	s := &Service{dir: dir, notes: mustNotesStore(t, dir), store: mustStore(t, dir), client: mustQuietClient(t), currentVersion: "1.0.0"}

	status, err := s.DownloadUpdate(context.Background())
	if !errors.Is(err, errNoUpdateChecked) {
		t.Fatalf("DownloadUpdate() error = %v, want errNoUpdateChecked", err)
	}
	if status.ErrorCode != "not-checked" {
		t.Fatalf("ErrorCode = %q, want %q", status.ErrorCode, "not-checked")
	}
}

func TestDownloadUpdateReturnsCurrentStatusWhenAlreadyReady(t *testing.T) {
	dir := t.TempDir()
	s := &Service{dir: dir, notes: mustNotesStore(t, dir), store: mustStore(t, dir), client: mustQuietClient(t), currentVersion: "1.0.0"}
	s.status = Status{State: StateReady, AvailableVersion: "1.2.3"}
	art := Artifact{Name: "setup.exe"}
	s.pendingArtifact = &art

	status, err := s.DownloadUpdate(context.Background())
	if err != nil {
		t.Fatalf("DownloadUpdate() error = %v, want nil when already ready", err)
	}
	if status.State != StateReady {
		t.Fatalf("State = %v, want StateReady", status.State)
	}
	if s.busy {
		t.Fatal("busy was set even though DownloadUpdate short-circuited without touching the network")
	}
}

func newArtifactServer(t *testing.T, content []byte) (*httptest.Server, Artifact) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write(content); err != nil {
			t.Errorf("write artifact response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	art := Artifact{
		OS: runtime.GOOS, Arch: runtime.GOARCH, Kind: KindInstaller, Name: "setup.exe",
		URL: srv.URL + "/setup.exe", Size: int64(len(content)), SHA256: sha256Hex(t, content),
	}
	return srv, art
}

func TestDownloadUpdateSuccessPersistsReadyState(t *testing.T) {
	content := []byte("this is a fake installer payload")
	_, art := newArtifactServer(t, content)

	dir := t.TempDir()
	store := mustStore(t, dir)
	s := &Service{dir: dir, notes: mustNotesStore(t, dir), store: store, client: mustQuietClient(t), currentVersion: "1.0.0"}
	s.pendingArtifact = &art
	s.pendingVersion = "1.2.3"

	status, err := s.DownloadUpdate(context.Background())
	if err != nil {
		t.Fatalf("DownloadUpdate() error = %v, want nil", err)
	}
	if status.State != StateReady {
		t.Fatalf("State = %v, want StateReady", status.State)
	}
	if status.TotalBytes != art.Size || status.DownloadedBytes != art.Size {
		t.Fatalf("TotalBytes/DownloadedBytes = %d/%d, want %d/%d", status.TotalBytes, status.DownloadedBytes, art.Size, art.Size)
	}

	finalPath, err := ArtifactPath(dir, "1.2.3", "setup.exe")
	if err != nil {
		t.Fatalf("ArtifactPath: %v", err)
	}
	got, err := os.ReadFile(finalPath)
	if err != nil {
		t.Fatalf("read downloaded artifact: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("downloaded content mismatch")
	}

	v, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if v.ReadyPath != finalPath || v.Artifact == nil {
		t.Fatalf("stored state = %+v, want ReadyPath=%q with an artifact", v, finalPath)
	}
}

func TestDownloadUpdatePersistFailureLeavesStoreUntouched(t *testing.T) {
	dir := t.TempDir()
	cacheDir, err := CacheDir(dir)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}
	statePath := filepath.Join(cacheDir, "state.json")
	writeTestFile(t, statePath, []byte("{not valid json"))

	store := mustStore(t, dir)
	if _, err := store.Load(); err == nil {
		t.Fatal("store.Load() on corrupt json returned nil error")
	}
	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read state before: %v", err)
	}

	content := []byte("payload-for-readonly-store-test")
	_, art := newArtifactServer(t, content)
	s := &Service{dir: dir, notes: mustNotesStore(t, dir), store: store, client: mustQuietClient(t), currentVersion: "1.0.0"}
	s.pendingArtifact = &art
	s.pendingVersion = "9.9.9"

	status, err := s.DownloadUpdate(context.Background())
	if !errors.Is(err, ErrReadOnly) {
		t.Fatalf("DownloadUpdate() error = %v, want ErrReadOnly", err)
	}
	if status.State != StateAvailable || status.ErrorCode != "download" {
		t.Fatalf("status = %+v, want available with a download error, not a status stuck on downloading", status)
	}

	after, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read state after: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("state.json was modified despite the read-only store: before=%q after=%q", before, after)
	}
}

func TestDownloadUpdateCancelledContext(t *testing.T) {
	content := []byte("does not matter, ctx is already cancelled")
	_, art := newArtifactServer(t, content)

	dir := t.TempDir()
	s := &Service{dir: dir, notes: mustNotesStore(t, dir), store: mustStore(t, dir), client: mustQuietClient(t), currentVersion: "1.0.0"}
	s.pendingArtifact = &art
	s.pendingVersion = "1.2.3"

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	status, err := s.DownloadUpdate(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("DownloadUpdate() error = %v, want context.Canceled", err)
	}
	if status.Error != "" || status.ErrorCode != "" || status.State == StateDownloading {
		t.Fatalf("a cancelled download is nobody's failure, yet status = %+v", status)
	}

	cacheDir, err := CacheDir(dir)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(cacheDir, "state.json")); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("state.json was created for a cancelled download: %v", statErr)
	}
}

func TestApplyUpdateBusy(t *testing.T) {
	s := &Service{busy: true}
	if err := s.ApplyUpdate(); !errors.Is(err, ErrBusy) {
		t.Fatalf("ApplyUpdate() error = %v, want ErrBusy", err)
	}
}

func TestApplyUpdateNotReady(t *testing.T) {
	s := &Service{status: Status{State: StateIdle}}
	if err := s.ApplyUpdate(); !errors.Is(err, ErrNotReady) {
		t.Fatalf("ApplyUpdate() error = %v, want ErrNotReady", err)
	}
}

// TestApplyUpdateRollsBackOnCacheDirFailure exercises the rollback path
// without spawning the real --selfupdate-worker process: an empty dir makes
// CacheDir fail deterministically after os.Executable() and before
// startUpdateWorker is ever called, so no subprocess is started by this test.
func TestApplyUpdateRollsBackOnCacheDirFailure(t *testing.T) {
	s := &Service{dir: "", status: Status{State: StateReady}, readyPath: "somewhere"}

	err := s.ApplyUpdate()
	if !errors.Is(err, ErrEmptyConfigDir) {
		t.Fatalf("ApplyUpdate() error = %v, want ErrEmptyConfigDir", err)
	}
	if s.busy {
		t.Fatal("busy was not rolled back")
	}
	if s.status.State != StateReady {
		t.Fatalf("State = %v, want StateReady restored after rollback", s.status.State)
	}
}

func TestDismissUpdateBusy(t *testing.T) {
	s := &Service{busy: true}
	if err := s.DismissUpdate(); !errors.Is(err, ErrBusy) {
		t.Fatalf("DismissUpdate() error = %v, want ErrBusy", err)
	}
}

func TestDismissUpdateNotReady(t *testing.T) {
	s := &Service{}
	if err := s.DismissUpdate(); !errors.Is(err, ErrNotReady) {
		t.Fatalf("DismissUpdate() error = %v, want ErrNotReady", err)
	}
}

func TestDismissUpdateSuccess(t *testing.T) {
	dir := t.TempDir()
	readyPath := filepath.Join(dir, "selfupdate", "1.2.3", "setup.exe")
	writeTestFile(t, readyPath, []byte("ready-artifact"))

	store := mustStore(t, dir)
	art := Artifact{Name: "setup.exe", Size: 14}
	if err := store.Save(stored{AvailableVersion: "1.2.3", Artifact: &art, ReadyPath: readyPath}); err != nil {
		t.Fatalf("seed store: %v", err)
	}

	s := &Service{dir: dir, notes: mustNotesStore(t, dir), store: store, readyPath: readyPath,
		status: Status{State: StateReady, AvailableVersion: "1.2.3"}}

	if err := s.DismissUpdate(); err != nil {
		t.Fatalf("DismissUpdate() error = %v, want nil", err)
	}
	if s.readyPath != "" {
		t.Fatalf("readyPath = %q, want empty", s.readyPath)
	}
	if s.status.State != StateAvailable {
		t.Fatalf("State = %v, want StateAvailable (an update is still known available)", s.status.State)
	}
	if _, err := os.Stat(readyPath); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("ready artifact still exists after dismiss: %v", err)
	}

	v, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if v.Artifact != nil || v.ReadyPath != "" {
		t.Fatalf("stored state still carries the dismissed artifact: %+v", v)
	}
}

func TestDismissUpdateWhenNoAvailableVersionGoesIdle(t *testing.T) {
	dir := t.TempDir()
	readyPath := filepath.Join(dir, "selfupdate", "1.2.3", "setup.exe")
	writeTestFile(t, readyPath, []byte("ready-artifact"))
	store := mustStore(t, dir)

	s := &Service{dir: dir, notes: mustNotesStore(t, dir), store: store, readyPath: readyPath, status: Status{State: StateReady}}
	if err := s.DismissUpdate(); err != nil {
		t.Fatalf("DismissUpdate() error = %v, want nil", err)
	}
	if s.status.State != StateIdle {
		t.Fatalf("State = %v, want StateIdle when no version remains available", s.status.State)
	}
}

func TestDismissUpdateReadyFileAlreadyMissing(t *testing.T) {
	dir := t.TempDir()
	readyPath := filepath.Join(dir, "selfupdate", "1.2.3", "setup.exe")
	store := mustStore(t, dir)

	s := &Service{dir: dir, notes: mustNotesStore(t, dir), store: store, readyPath: readyPath, status: Status{State: StateReady, AvailableVersion: "1.2.3"}}
	if err := s.DismissUpdate(); err != nil {
		t.Fatalf("DismissUpdate() error = %v, want nil even when the file is already gone", err)
	}
}

func TestDismissUpdateReadOnlyStoreLeavesStateUntouched(t *testing.T) {
	dir := t.TempDir()
	cacheDir, err := CacheDir(dir)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}
	statePath := filepath.Join(cacheDir, "state.json")
	writeTestFile(t, statePath, []byte("{not valid json"))

	store := mustStore(t, dir)
	if _, err := store.Load(); err == nil {
		t.Fatal("store.Load() on corrupt json returned nil error")
	}

	readyPath := filepath.Join(cacheDir, "1.2.3", "setup.exe")
	writeTestFile(t, readyPath, []byte("still-there"))

	s := &Service{dir: dir, notes: mustNotesStore(t, dir), store: store, readyPath: readyPath, status: Status{State: StateReady, AvailableVersion: "1.2.3"}}
	if err := s.DismissUpdate(); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("DismissUpdate() error = %v, want ErrReadOnly", err)
	}
	if s.readyPath != readyPath {
		t.Fatalf("readyPath = %q, want unchanged %q", s.readyPath, readyPath)
	}
	if s.status.State != StateReady {
		t.Fatalf("State = %v, want unchanged StateReady", s.status.State)
	}
	if _, statErr := os.Stat(readyPath); statErr != nil {
		t.Fatalf("ready artifact was removed despite the failed persist: %v", statErr)
	}
}

func TestDeriveState(t *testing.T) {
	cases := []struct {
		name string
		v    stored
		want State
	}{
		{"empty", stored{}, StateIdle},
		{"available", stored{AvailableVersion: "1.2.3"}, StateAvailable},
		{"ready", stored{AvailableVersion: "1.2.3", Artifact: &Artifact{}, ReadyPath: "x"}, StateReady},
		{"artifact without path is not ready", stored{AvailableVersion: "1.2.3", Artifact: &Artifact{}}, StateAvailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := deriveState(tc.v); got != tc.want {
				t.Fatalf("deriveState(%+v) = %v, want %v", tc.v, got, tc.want)
			}
		})
	}
}

func TestErrorStatusPreservesOtherFields(t *testing.T) {
	current := Status{State: StateAvailable, AvailableVersion: "1.2.3", Notes: "n", CheckedAt: time.Now()}
	got := errorStatus(current, "manifest", errors.New("boom"))
	if got.State != current.State || got.AvailableVersion != current.AvailableVersion || got.Notes != current.Notes {
		t.Fatalf("errorStatus() = %+v, want other fields preserved from %+v", got, current)
	}
	if got.Error != "boom" || got.ErrorCode != "manifest" {
		t.Fatalf("errorStatus() = %+v, want Error/ErrorCode set", got)
	}
}
