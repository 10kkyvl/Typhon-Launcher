package selfupdate

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func readyArtifact(t *testing.T, content []byte) Artifact {
	t.Helper()
	return Artifact{
		OS: runtime.GOOS, Arch: runtime.GOARCH, Kind: KindInstaller, Name: "setup.exe",
		URL: "https://example.com/setup.exe", Size: int64(len(content)), SHA256: sha256Hex(t, content),
	}
}

// state.json sits in a directory the user's own account can write, so the path
// it names is input: startup must not delete, or accept as an installer, a file
// the updater did not put in its cache.
func TestServiceStartupNeverTouchesAReadyPathOutsideTheCache(t *testing.T) {
	foreign := []byte("not the updater's file")
	tests := []struct {
		name string
		// path returns the ready path to record and the file that must survive.
		path func(t *testing.T, dir string) (ready, survivor string)
		art  Artifact
	}{
		{
			name: "foreign file whose hash does not match",
			path: func(t *testing.T, _ string) (string, string) {
				p := filepath.Join(t.TempDir(), "precious.txt")
				writeTestFile(t, p, foreign)
				return p, p
			},
			art: readyArtifact(t, []byte("expected-bytes")),
		},
		{
			name: "foreign file whose hash matches the record",
			path: func(t *testing.T, _ string) (string, string) {
				p := filepath.Join(t.TempDir(), "precious.txt")
				writeTestFile(t, p, foreign)
				return p, p
			},
			art: readyArtifact(t, foreign),
		},
		{
			name: "file beside the cache directory",
			path: func(t *testing.T, dir string) (string, string) {
				p := filepath.Join(dir, "settings.json")
				writeTestFile(t, p, foreign)
				return p, p
			},
			art: readyArtifact(t, []byte("expected-bytes")),
		},
		{
			name: "dot-dot out of the cache",
			path: func(t *testing.T, dir string) (string, string) {
				survivor := filepath.Join(dir, "settings.json")
				writeTestFile(t, survivor, foreign)
				sep := string(os.PathSeparator)
				return filepath.Join(dir, "selfupdate") + sep + "1.2.3" + sep + ".." + sep + ".." + sep + "settings.json", survivor
			},
			art: readyArtifact(t, []byte("expected-bytes")),
		},
		{
			name: "the cache directory itself",
			path: func(t *testing.T, dir string) (string, string) {
				cacheDir, err := CacheDir(dir)
				if err != nil {
					t.Fatalf("CacheDir: %v", err)
				}
				marker := filepath.Join(cacheDir, "1.2.3", "keep.txt")
				writeTestFile(t, marker, foreign)
				return cacheDir, marker
			},
			art: readyArtifact(t, []byte("expected-bytes")),
		},
		{
			name: "relative path",
			path: func(t *testing.T, _ string) (string, string) {
				return filepath.Join("selfupdate", "1.2.3", "setup.exe"), ""
			},
			art: readyArtifact(t, []byte("expected-bytes")),
		},
		{
			name: "file straight in the cache instead of a version directory",
			path: func(t *testing.T, dir string) (string, string) {
				cacheDir, err := CacheDir(dir)
				if err != nil {
					t.Fatalf("CacheDir: %v", err)
				}
				p := filepath.Join(cacheDir, notesName)
				writeTestFile(t, p, foreign)
				return p, p
			},
			art: readyArtifact(t, []byte("expected-bytes")),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			ready, survivor := tt.path(t, dir)
			art := tt.art
			store := mustStore(t, dir)
			if err := store.Save(stored{AvailableVersion: "1.2.3", Artifact: &art, ReadyPath: ready}); err != nil {
				t.Fatalf("seed store: %v", err)
			}

			s := &Service{dir: dir, notes: mustNotesStore(t, dir), store: store, client: mustQuietClient(t), currentVersion: "1.0.0"}
			if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
				t.Fatalf("ServiceStartup() error = %v, want the foreign record dropped and the launcher started", err)
			}
			if err := s.ServiceShutdown(); err != nil {
				t.Fatalf("ServiceShutdown: %v", err)
			}

			if survivor != "" {
				if _, err := os.Stat(survivor); err != nil {
					t.Fatalf("%s: %v; startup deleted a file outside the update cache", survivor, err)
				}
			}
			if got := s.GetStatus(); got.State == StateReady {
				t.Fatalf("status = %+v: a path outside the cache was accepted as the ready installer", got)
			}
			s.mu.Lock()
			path, kept := s.readyPath, s.readyArtifact
			s.mu.Unlock()
			if path != "" || kept != nil {
				t.Fatalf("service remembers the foreign path: %q, %+v", path, kept)
			}
			v, err := store.Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if v.Artifact != nil || v.ReadyPath != "" {
				t.Fatalf("saved state still carries the foreign ready path: %+v", v)
			}
			if v.AvailableVersion != "1.2.3" {
				t.Fatalf("AvailableVersion = %q, want 1.2.3 kept: only the ready record is foreign", v.AvailableVersion)
			}
		})
	}
}

func TestDismissUpdateNeverDeletesAReadyPathOutsideTheCache(t *testing.T) {
	dir := t.TempDir()
	foreign := filepath.Join(t.TempDir(), "precious.txt")
	writeTestFile(t, foreign, []byte("not the updater's file"))

	s := &Service{dir: dir, notes: mustNotesStore(t, dir), store: mustStore(t, dir), currentVersion: "1.0.0", readyPath: foreign}
	s.status = Status{State: StateReady, CurrentVersion: "1.0.0", AvailableVersion: "1.2.3"}

	if err := s.DismissUpdate(); err != nil {
		t.Fatalf("DismissUpdate() error = %v, want the record dropped", err)
	}
	got, err := os.ReadFile(foreign)
	if err != nil || !bytes.Equal(got, []byte("not the updater's file")) {
		t.Fatalf("file outside the cache = %q, %v; dismissing deleted it", got, err)
	}
	if status := s.GetStatus(); status.State != StateAvailable {
		t.Fatalf("status = %+v, want the update back to available", status)
	}
	s.mu.Lock()
	path := s.readyPath
	s.mu.Unlock()
	if path != "" {
		t.Fatalf("readyPath = %q after the dismiss", path)
	}
}
