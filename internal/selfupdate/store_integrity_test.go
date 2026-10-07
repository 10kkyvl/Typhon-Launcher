package selfupdate

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func stateFilePath(t *testing.T, configDir string) string {
	t.Helper()
	cacheDir, err := CacheDir(configDir)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}
	return filepath.Join(cacheDir, "state.json")
}

func TestStoreUnreadableStateIsNeverTreatedAsEmpty(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
	}{
		{"empty file", nil},
		{"whitespace only", []byte(" \n\t ")},
		{"truncated envelope", []byte(`{"version":1,"data":{"availableVersion":"1.2`)},
		{"binary garbage", []byte{0x00, 0xff, 0xfe, 0x01}},
		{"json null", []byte(`null`)},
		{"empty object without an envelope", []byte(`{}`)},
		{"json array", []byte(`[1,2,3]`)},
		{"record written without an envelope", []byte(`{"availableVersion":"1.2.3"}`)},
		{"format from a newer launcher", []byte(`{"version":2,"data":{"availableVersion":"9.9.9"}}`)},
		{"data of the wrong type", []byte(`{"version":1,"data":"1.2.3"}`)},
		{"field of the wrong type", []byte(`{"version":1,"data":{"artifact":"setup.exe"}}`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := stateFilePath(t, dir)
			writeTestFile(t, path, tt.raw)

			s, err := NewStore(dir)
			if err != nil {
				t.Fatalf("NewStore: %v", err)
			}
			v, err := s.Load()
			if err == nil {
				t.Fatalf("Load() error = nil with %+v, an unreadable state must not pass for an empty one", v)
			}
			if errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("Load() error = %v, must not be confused with a missing file", err)
			}
			if v != (stored{}) {
				t.Fatalf("Load() = %+v next to an error", v)
			}

			if err := s.Save(stored{AvailableVersion: "9.9.9"}); !errors.Is(err, ErrReadOnly) {
				t.Fatalf("Save() error = %v, want ErrReadOnly: the next write would overwrite whatever the file held", err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read state: %v", err)
			}
			if !bytes.Equal(got, tt.raw) {
				t.Fatalf("state file changed from %q to %q", tt.raw, got)
			}
			if _, err := s.Load(); err == nil {
				t.Fatal("second Load() error = nil, the store must stay unhealed until restart")
			}
		})
	}
}

func TestStoreStateFileThatIsADirectory(t *testing.T) {
	dir := t.TempDir()
	path := stateFilePath(t, dir)
	if err := os.MkdirAll(filepath.Join(path, "inside"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if _, err := s.Load(); err == nil || errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Load() error = %v, want a read error that is not a missing file", err)
	}
	if err := s.Save(stored{AvailableVersion: "1.2.3"}); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("Save() error = %v, want ErrReadOnly", err)
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		t.Fatalf("state path = %v, %v; the directory must be left alone", info, err)
	}
}

func TestStoreMissingStateStartsEmptyAndStaysWritable(t *testing.T) {
	dir := t.TempDir()
	path := stateFilePath(t, dir)

	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	v, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil for a missing file", err)
	}
	if v != (stored{}) {
		t.Fatalf("Load() = %+v, want empty", v)
	}
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Load() created the state file: %v", err)
	}

	if err := s.Save(stored{AvailableVersion: "1.2.3", Notes: "n"}); err != nil {
		t.Fatalf("Save() after a missing-file Load error = %v, want nil", err)
	}
	again, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	got, err := again.Load()
	if err != nil {
		t.Fatalf("Load() after Save error = %v", err)
	}
	if got.AvailableVersion != "1.2.3" || got.Notes != "n" {
		t.Fatalf("Load() = %+v, want what was saved", got)
	}
}

func TestStoreSaveLeavesNoTemporaryFiles(t *testing.T) {
	dir := t.TempDir()
	path := stateFilePath(t, dir)
	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	for _, version := range []string{"1.0.1", "1.0.2", "1.0.3"} {
		if err := s.Save(stored{AvailableVersion: version}); err != nil {
			t.Fatalf("Save(%s): %v", version, err)
		}
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("read cache dir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "state.json" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("cache dir holds %v, want only state.json", names)
	}
}
