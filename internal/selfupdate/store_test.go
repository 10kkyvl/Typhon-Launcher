package selfupdate

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestCacheDir(t *testing.T) {
	if _, err := CacheDir(""); !errors.Is(err, ErrEmptyConfigDir) {
		t.Fatalf("CacheDir(\"\") error = %v, want ErrEmptyConfigDir", err)
	}

	dir, err := CacheDir(`C:\config`)
	if err != nil {
		t.Fatalf("CacheDir() error = %v, want nil", err)
	}
	if want := filepath.Join(`C:\config`, "selfupdate"); dir != want {
		t.Fatalf("CacheDir() = %q, want %q", dir, want)
	}
}

func TestVersionDir(t *testing.T) {
	cases := []struct {
		name    string
		config  string
		version string
		wantErr error
	}{
		{"empty config dir", "", "1.2.3", ErrEmptyConfigDir},
		{"empty version", `C:\config`, "", ErrInvalidVersionPath},
		{"dot", `C:\config`, ".", ErrInvalidVersionPath},
		{"parent traversal", `C:\config`, "..", ErrInvalidVersionPath},
		{"embedded traversal", `C:\config`, `..\1.2.3`, ErrInvalidVersionPath},
		{"forward slash", `C:\config`, "1.2/3", ErrInvalidVersionPath},
		{"valid", `C:\config`, "1.2.3", nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, err := VersionDir(tc.config, tc.version)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("VersionDir() error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("VersionDir() unexpected error = %v", err)
			}
			want := filepath.Join(tc.config, "selfupdate", tc.version)
			if dir != want {
				t.Fatalf("VersionDir() = %q, want %q", dir, want)
			}
		})
	}
}

func TestArtifactPath(t *testing.T) {
	cases := []struct {
		name    string
		config  string
		version string
		artName string
		wantErr error
	}{
		{"empty config dir", "", "1.2.3", "setup.exe", ErrEmptyConfigDir},
		{"invalid version", `C:\config`, "..", "setup.exe", ErrInvalidVersionPath},
		{"traversal artifact name", `C:\config`, "1.2.3", `..\evil.exe`, ErrInvalidArtifactName},
		{"empty artifact name", `C:\config`, "1.2.3", "", ErrInvalidArtifactName},
		{"valid", `C:\config`, "1.2.3", "setup.exe", nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path, err := ArtifactPath(tc.config, tc.version, tc.artName)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("ArtifactPath() error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ArtifactPath() unexpected error = %v", err)
			}
			want := filepath.Join(tc.config, "selfupdate", tc.version, tc.artName)
			if path != want {
				t.Fatalf("ArtifactPath() = %q, want %q", path, want)
			}
		})
	}
}

func TestNewStore(t *testing.T) {
	if _, err := NewStore(""); !errors.Is(err, ErrEmptyConfigDir) {
		t.Fatalf("NewStore(\"\") error = %v, want ErrEmptyConfigDir", err)
	}

	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore() error = %v, want nil", err)
	}
	if s == nil {
		t.Fatalf("NewStore() store = nil, want non-nil")
	}
}

func TestStoreLoadMissingFile(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	v, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil for a missing file", err)
	}
	if v != (stored{}) {
		t.Fatalf("Load() = %+v, want zero value", v)
	}
}

func TestStoreSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	want := stored{
		AvailableVersion: "1.2.3",
		Notes:            "fixes things",
		PublishedAt:      time.Now().UTC().Truncate(time.Second),
		CheckedAt:        time.Now().UTC().Truncate(time.Second),
		Artifact: &Artifact{
			OS:     "windows",
			Arch:   "amd64",
			Kind:   KindInstaller,
			Name:   "setup.exe",
			URL:    "https://example.com/setup.exe",
			Size:   1024,
			SHA256: "abc",
		},
		ReadyPath: filepath.Join(dir, "selfupdate", "1.2.3", "setup.exe"),
	}
	if err := s.Save(want); err != nil {
		t.Fatalf("Save() error = %v, want nil", err)
	}

	cacheDir, err := CacheDir(dir)
	if err != nil {
		t.Fatalf("CacheDir() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "state.json")); err != nil {
		t.Fatalf("stat state file: %v", err)
	}

	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if got.AvailableVersion != want.AvailableVersion || got.Notes != want.Notes || got.ReadyPath != want.ReadyPath {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
	if got.Artifact == nil || *got.Artifact != *want.Artifact {
		t.Fatalf("Load() artifact = %+v, want %+v", got.Artifact, want.Artifact)
	}
}

func TestStoreCorruptFileRefusesToSave(t *testing.T) {
	dir := t.TempDir()
	cacheDir, err := CacheDir(dir)
	if err != nil {
		t.Fatalf("CacheDir() error = %v", err)
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("mkdir cache dir: %v", err)
	}
	statePath := filepath.Join(cacheDir, "state.json")
	if err := os.WriteFile(statePath, []byte("{not valid json"), 0o600); err != nil {
		t.Fatalf("write corrupt state: %v", err)
	}

	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	if _, err := s.Load(); err == nil {
		t.Fatalf("Load() error = nil, want an error for corrupt JSON")
	}

	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read state before Save: %v", err)
	}

	err = s.Save(stored{AvailableVersion: "9.9.9"})
	if !errors.Is(err, ErrReadOnly) {
		t.Fatalf("Save() error = %v, want ErrReadOnly", err)
	}

	after, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read state after Save: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("Save() modified the corrupt state file: before=%q after=%q", before, after)
	}
}

func TestStoreConcurrentAccess(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if err := s.Save(stored{AvailableVersion: "1.0.0"}); err != nil {
				t.Errorf("concurrent Save() error = %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := s.Load(); err != nil {
				t.Errorf("concurrent Load() error = %v", err)
			}
		}()
	}
	wg.Wait()
}
