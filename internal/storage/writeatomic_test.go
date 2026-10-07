package storage

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func TestWriteAtomicReplacesContentWhole(t *testing.T) {
	large := bytes.Repeat([]byte("0123456789abcdef"), 1<<18)
	cases := []struct {
		name string
		seed []byte
		have bool
		data []byte
	}{
		{"shorter replaces longer", []byte("a much longer previous document"), true, []byte("short")},
		{"longer replaces shorter", []byte("short"), true, []byte("a much longer replacement document")},
		{"empty replaces content", []byte("previous"), true, []byte{}},
		{"content replaces empty file", []byte{}, true, []byte("payload")},
		{"creates a missing file", nil, false, []byte("payload")},
		{"binary with nul and high bytes", []byte("text"), true, []byte{0, 1, 0xff, 0, 0x80}},
		{"large document", []byte("small"), true, large},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "state.json")
			if tc.have {
				if err := os.WriteFile(path, tc.seed, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := WriteAtomic(path, tc.data); err != nil {
				t.Fatalf("write: %v", err)
			}
			got, err := os.ReadFile(filepath.Clean(path))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, tc.data) {
				t.Fatalf("stored %d bytes, want %d", len(got), len(tc.data))
			}
			if names := dirNames(t, dir); len(names) != 1 || names[0] != "state.json" {
				t.Fatalf("directory holds %v, want only the target", names)
			}
		})
	}
}

func TestWriteAtomicStagesNextToTarget(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such-tmp")
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(name, missing)
	}
	if probe, err := os.CreateTemp("", "probe-*"); err == nil {
		name := probe.Name()
		if err := probe.Close(); err != nil {
			t.Error(err)
		}
		if err := os.Remove(name); err != nil {
			t.Error(err)
		}
		t.Fatal("system temp dir is still usable, the test proves nothing")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := WriteAtomic(path, []byte("payload")); err != nil {
		t.Fatalf("a temp file outside the target directory was used: %v", err)
	}
	if names := dirNames(t, dir); len(names) != 1 {
		t.Fatalf("directory holds %v, want only the target", names)
	}
}

func TestWriteAtomicOverDirectoryFailsAndKeepsIt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(path, "child.txt")
	if err := os.WriteFile(child, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := WriteAtomic(path, []byte("payload")); err == nil {
		t.Fatal("writing over a directory must fail")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatal("the directory was replaced")
	}
	got, err := os.ReadFile(filepath.Clean(child))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "keep" {
		t.Fatalf("child = %q", got)
	}
	for _, name := range dirNames(t, dir) {
		if strings.HasPrefix(name, ".tmp-") {
			t.Fatalf("temp file left behind: %s", name)
		}
	}
}

func TestWriteAtomicNewFileIsOwnerWritable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := WriteAtomic(path, []byte("payload")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	perm := info.Mode().Perm()
	if perm&0o200 == 0 {
		t.Fatalf("new file is not writable by its owner: %v", perm)
	}
	if runtime.GOOS != "windows" && perm != 0o600 {
		t.Fatalf("new file mode = %v, want 0600", perm)
	}
}

func TestWriteAtomicConcurrentWritersLeaveOnePayload(t *testing.T) {
	const (
		writers = 8
		rounds  = 15
		size    = 256 << 10
	)
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := WriteAtomic(path, []byte("seed")); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, writers*rounds)
	for w := range writers {
		payload := bytes.Repeat([]byte{byte('A' + w)}, size)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range rounds {
				if err := WriteAtomic(path, payload); err != nil {
					errs <- err
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("write: %v", err)
	}

	got, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != size {
		t.Fatalf("stored %d bytes, want %d", len(got), size)
	}
	if want := bytes.Repeat(got[:1], size); !bytes.Equal(got, want) {
		t.Fatal("stored file mixes bytes of different writers")
	}
	if names := dirNames(t, dir); len(names) != 1 {
		t.Fatalf("directory holds %v, want only the target", names)
	}
}
