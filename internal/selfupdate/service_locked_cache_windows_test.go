//go:build windows

package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// holdOpen keeps path open the way a virus scanner or an indexer does: Go
// opens files with FILE_SHARE_READ|FILE_SHARE_WRITE and no FILE_SHARE_DELETE,
// so nobody can delete the file until the handle is closed.
func holdOpen(t *testing.T, path string) (release func()) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("hold %s open: %v", path, err)
	}
	release = func() {
		if err := f.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			t.Errorf("release %s: %v", path, err)
		}
	}
	t.Cleanup(release)
	return release
}

func startService(t *testing.T, dir string) *Service {
	t.Helper()
	s := &Service{dir: dir, notes: mustNotesStore(t, dir), store: mustStore(t, dir), client: mustQuietClient(t), currentVersion: "1.0.0"}
	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup() error = %v: a cache entry another process holds open must not keep the launcher from starting", err)
	}
	t.Cleanup(func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Errorf("ServiceShutdown: %v", err)
		}
	})
	return s
}

func TestServiceStartupSurvivesLockedStaleCacheEntry(t *testing.T) {
	dir := t.TempDir()
	cacheDir, err := CacheDir(dir)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}
	locked := filepath.Join(cacheDir, "1.0.0", "old-setup.exe")
	writeTestFile(t, locked, []byte("stale"))
	orphan := filepath.Join(cacheDir, "orphan.txt")
	writeTestFile(t, orphan, []byte("x"))
	release := holdOpen(t, locked)

	store := mustStore(t, dir)
	if err := store.Save(stored{AvailableVersion: "2.0.0", CheckedAt: time.Now()}); err != nil {
		t.Fatalf("seed store: %v", err)
	}

	s := startService(t, dir)
	if got := s.GetStatus(); got.State != StateAvailable || got.AvailableVersion != "2.0.0" {
		t.Fatalf("status = %+v, want 2.0.0 still available", got)
	}
	if _, err := os.Stat(locked); err != nil {
		t.Fatalf("locked entry: %v; the handle did not hold, the test proves nothing", err)
	}
	if _, err := os.Stat(orphan); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("orphan next to the locked entry survived (%v): one entry that cannot go must not stop the sweep", err)
	}
	if v, err := store.Load(); err != nil || v.AvailableVersion != "2.0.0" {
		t.Fatalf("saved state = %+v, %v; the sweep must not touch it", v, err)
	}

	if err := s.ServiceShutdown(); err != nil {
		t.Fatalf("ServiceShutdown: %v", err)
	}
	release()
	startService(t, dir)
	if _, err := os.Stat(filepath.Dir(locked)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale dir survived the next start (%v): the sweep must retry what it could not remove", err)
	}
}

func TestServiceStartupSurvivesLockedInvalidReadyArtifact(t *testing.T) {
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
	holdOpen(t, readyPath)

	s := startService(t, dir)
	if got := s.GetStatus().State; got == StateReady {
		t.Fatal("status is ready: an artifact that failed verification and could not be deleted is still offered for install")
	}
	s.mu.Lock()
	path, kept := s.readyPath, s.readyArtifact
	s.mu.Unlock()
	if path != "" || kept != nil {
		t.Fatalf("service still remembers the invalid artifact: %q, %+v", path, kept)
	}
	v, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if v.Artifact != nil || v.ReadyPath != "" {
		t.Fatalf("saved state still carries the invalid artifact: %+v", v)
	}
	if _, err := os.Stat(readyPath); err != nil {
		t.Fatalf("invalid artifact: %v; the handle did not hold, the test proves nothing", err)
	}
}

func TestServiceStartupSurvivesLockedOutcomeFile(t *testing.T) {
	dir := t.TempDir()
	outcomePath, err := OutcomePath(dir)
	if err != nil {
		t.Fatalf("OutcomePath: %v", err)
	}
	want := Outcome{Version: "1.2.3", Error: "installer left the launcher unchanged", FinishedAt: time.Now()}
	if err := writeOutcome(outcomePath, want); err != nil {
		t.Fatalf("writeOutcome: %v", err)
	}
	holdOpen(t, outcomePath)

	s := startService(t, dir)
	got := s.GetOutcome()
	if got.Version != want.Version || got.OK != want.OK || got.Error != want.Error {
		t.Fatalf("GetOutcome() = %+v, want %+v: the user still has to hear how the update ended", got, want)
	}
	if _, err := os.Stat(outcomePath); err != nil {
		t.Fatalf("outcome file: %v; the handle did not hold, the test proves nothing", err)
	}
}
