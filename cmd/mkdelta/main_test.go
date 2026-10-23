package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/selfupdate/delta"
)

func testBytes(seed uint64, n int) []byte {
	out := make([]byte, 0, n+sha256.Size)
	var block [16]byte
	binary.LittleEndian.PutUint64(block[:8], seed)
	for counter := uint64(0); len(out) < n; counter++ {
		binary.LittleEndian.PutUint64(block[8:], counter)
		sum := sha256.Sum256(block[:])
		out = append(out, sum[:]...)
	}
	return out[:n]
}

func writeFile(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func TestRunBuildsAPatchThatRebuildsTheNewExecutable(t *testing.T) {
	dir := t.TempDir()
	oldExe := testBytes(1, 128<<10)
	newExe := append(append([]byte(nil), oldExe[:64<<10]...), append(testBytes(2, 1<<10), oldExe[64<<10:]...)...)
	oldPath := writeFile(t, dir, "old.exe", oldExe)
	newPath := writeFile(t, dir, "new.exe", newExe)
	out := filepath.Join(dir, "out", "patch.delta")
	if err := os.Mkdir(filepath.Dir(out), 0o755); err != nil {
		t.Fatal(err)
	}

	rep, err := run(t.Context(), []string{"-old", oldPath, "-new", newPath, "-out", out}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}

	patch, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if rep.PatchSize != int64(len(patch)) || rep.OldSize != int64(len(oldExe)) || rep.NewSize != int64(len(newExe)) {
		t.Fatalf("report = %+v, want the sizes of what was read and written", rep)
	}
	var rebuilt bytes.Buffer
	if _, err := delta.Apply(t.Context(), &rebuilt, oldExe, bytes.NewReader(patch), int64(len(newExe))); err != nil {
		t.Fatalf("the written patch does not apply: %v", err)
	}
	if !bytes.Equal(rebuilt.Bytes(), newExe) {
		t.Fatal("the written patch rebuilds something else than the new executable")
	}
}

func TestRunRefuses(t *testing.T) {
	dir := t.TempDir()
	same := testBytes(10, 32<<10)
	oldPath := writeFile(t, dir, "old.exe", same)
	sameCopy := writeFile(t, dir, "same.exe", same)
	unrelated := writeFile(t, dir, "unrelated.exe", testBytes(11, 32<<10))
	empty := writeFile(t, dir, "empty.exe", nil)
	out := filepath.Join(dir, "patch.delta")

	tests := []struct {
		name    string
		args    []string
		wantErr error
		anyErr  bool
	}{
		{"no old", []string{"-new", unrelated, "-out", out}, errMissingOld, false},
		{"no new", []string{"-old", oldPath, "-out", out}, errMissingNew, false},
		{"no out", []string{"-old", oldPath, "-new", unrelated}, errMissingOut, false},
		{"the same build twice", []string{"-old", oldPath, "-new", sameCopy, "-out", out}, errSameBuild, false},
		{"a patch that is not smaller", []string{"-old", oldPath, "-new", unrelated, "-out", out}, errNotSmaller, false},
		{"old does not exist", []string{"-old", filepath.Join(dir, "absent.exe"), "-new", unrelated, "-out", out}, nil, true},
		{"new does not exist", []string{"-old", oldPath, "-new", filepath.Join(dir, "absent.exe"), "-out", out}, nil, true},
		{"empty new", []string{"-old", oldPath, "-new", empty, "-out", out}, delta.ErrEmptyTarget, false},
		{"empty old", []string{"-old", empty, "-new", unrelated, "-out", out}, delta.ErrEmptyBase, false},
		{"output in a directory that does not exist", []string{"-old", oldPath, "-new", append1(t, dir, same), "-out", filepath.Join(dir, "no", "such", "patch.delta")}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := run(t.Context(), tt.args, &bytes.Buffer{})
			if err == nil {
				t.Fatal("run() error = nil")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("run() error = %v, want %v", err, tt.wantErr)
			}
			if _, statErr := os.Stat(out); statErr == nil {
				t.Fatal("a refused patch was written")
			}
		})
	}
}

// append1 is a build that differs from same by one trailing byte, so a patch
// exists and is tiny.
func append1(t *testing.T, dir string, same []byte) string {
	t.Helper()
	return writeFile(t, dir, "plusone.exe", append(append([]byte(nil), same...), 1))
}

func TestRunStopsWhenCancelled(t *testing.T) {
	dir := t.TempDir()
	oldExe := testBytes(20, 64<<10)
	oldPath := writeFile(t, dir, "old.exe", oldExe)
	newPath := writeFile(t, dir, "new.exe", append(append([]byte(nil), oldExe...), testBytes(21, 4<<10)...))
	out := filepath.Join(dir, "patch.delta")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := run(ctx, []string{"-old", oldPath, "-new", newPath, "-out", out}, &bytes.Buffer{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("run() error = %v, want context.Canceled", err)
	}
	if _, err := os.Stat(out); err == nil {
		t.Fatal("a cancelled run wrote a patch")
	}
}
