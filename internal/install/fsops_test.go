package install

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCopyDirCopiesTree(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	mkText(t, filepath.Join(src, "a.txt"), "alpha")
	mkText(t, filepath.Join(src, "sub", "b.txt"), "beta")
	mkFile(t, filepath.Join(src, "sub", "deep", "c.bin"), 4096)
	if err := os.MkdirAll(filepath.Join(src, "empty"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	dst := filepath.Join(tmp, "dst")

	var last Progress
	if err := CopyDir(context.Background(), src, dst, func(p Progress) { last = p }); err != nil {
		t.Fatalf("CopyDir: %v", err)
	}

	if data, err := os.ReadFile(filepath.Join(dst, "a.txt")); err != nil || string(data) != "alpha" {
		t.Fatalf("a.txt = %q, err = %v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(dst, "sub", "b.txt")); err != nil || string(data) != "beta" {
		t.Fatalf("b.txt = %q, err = %v", data, err)
	}
	if info, err := os.Stat(filepath.Join(dst, "sub", "deep", "c.bin")); err != nil || info.Size() != 4096 {
		t.Fatalf("c.bin stat err = %v", err)
	}
	if info, err := os.Stat(filepath.Join(dst, "empty")); err != nil || !info.IsDir() {
		t.Fatalf("empty dir not copied: %v", err)
	}
	if DirSize(src) != DirSize(dst) {
		t.Fatalf("sizes differ: %d vs %d", DirSize(src), DirSize(dst))
	}
	if last.BytesTotal == 0 || last.BytesDone != last.BytesTotal {
		t.Fatalf("final progress = %+v", last)
	}
	if err := verifyCopy(src, dst); err != nil {
		t.Fatalf("verifyCopy: %v", err)
	}
}

func TestCopyDirRefusesNonEmptyDest(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	mkText(t, filepath.Join(src, "a.txt"), "alpha")
	dst := filepath.Join(tmp, "dst")
	mkText(t, filepath.Join(dst, "keep.txt"), "keep")

	if err := CopyDir(context.Background(), src, dst, nil); !errors.Is(err, errDestExists) {
		t.Fatalf("err = %v, want errDestExists", err)
	}
	if data, err := os.ReadFile(filepath.Join(dst, "keep.txt")); err != nil || string(data) != "keep" {
		t.Fatalf("existing file damaged: %q %v", data, err)
	}
}

func TestCopyDirPreCancelled(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	mkText(t, filepath.Join(src, "a.txt"), "alpha")
	dst := filepath.Join(tmp, "dst")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := CopyDir(ctx, src, dst, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if exists(filepath.Join(dst, "a.txt")) {
		t.Fatal("file copied despite cancellation")
	}
}

func TestCopyDirCancelMidway(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	for _, name := range []string{"1.bin", "2.bin", "3.bin", "4.bin", "5.bin"} {
		mkFile(t, filepath.Join(src, name), 300*1024)
	}
	dst := filepath.Join(tmp, "dst")

	ctx, cancel := context.WithCancel(context.Background())
	err := CopyDir(ctx, src, dst, func(Progress) { cancel() })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if info, statErr := os.Stat(filepath.Join(dst, "5.bin")); statErr == nil && info.Size() == 300*1024 {
		t.Fatal("copy did not stop early")
	}
}

func TestCopyDirRejectsFileSource(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "a.txt")
	mkText(t, src, "alpha")
	if err := CopyDir(context.Background(), src, filepath.Join(tmp, "dst"), nil); !errors.Is(err, errNotDir) {
		t.Fatalf("err = %v, want errNotDir", err)
	}
}

func TestMoveDirSameVolumeRename(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	mkText(t, filepath.Join(src, "a.txt"), "alpha")
	mkFile(t, filepath.Join(src, "sub", "b.bin"), 2048)
	want := DirSize(src)
	dst := filepath.Join(tmp, "dst")

	var last Progress
	if err := MoveDir(context.Background(), src, dst, func(p Progress) { last = p }); err != nil {
		t.Fatalf("MoveDir: %v", err)
	}
	if exists(src) {
		t.Fatal("source still present after move")
	}
	if data, err := os.ReadFile(filepath.Join(dst, "a.txt")); err != nil || string(data) != "alpha" {
		t.Fatalf("a.txt = %q, err = %v", data, err)
	}
	if DirSize(dst) != want {
		t.Fatalf("size = %d, want %d", DirSize(dst), want)
	}
	if last.BytesDone != want || last.BytesTotal != want {
		t.Fatalf("final progress = %+v, want %d", last, want)
	}
}

func TestMoveDirRefusesNonEmptyDest(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	mkText(t, filepath.Join(src, "a.txt"), "alpha")
	dst := filepath.Join(tmp, "dst")
	mkText(t, filepath.Join(dst, "keep.txt"), "keep")

	if err := MoveDir(context.Background(), src, dst, nil); !errors.Is(err, errDestExists) {
		t.Fatalf("err = %v, want errDestExists", err)
	}
	if !exists(filepath.Join(src, "a.txt")) {
		t.Fatal("source removed on refused move")
	}
}

func TestMoveDirCopyFallback(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	mkText(t, filepath.Join(src, "a.txt"), "alpha")
	mkFile(t, filepath.Join(src, "sub", "b.bin"), 1024)
	dst := filepath.Join(tmp, "dst")

	if err := CopyDir(context.Background(), src, dst, nil); err != nil {
		t.Fatalf("CopyDir: %v", err)
	}
	if err := verifyCopy(src, dst); err != nil {
		t.Fatalf("verifyCopy: %v", err)
	}
	if err := os.RemoveAll(src); err != nil {
		t.Fatalf("remove src: %v", err)
	}
	if !exists(filepath.Join(dst, "sub", "b.bin")) {
		t.Fatal("fallback move lost files")
	}
}

func TestVerifyCopyDetectsMismatch(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	dst := filepath.Join(tmp, "dst")
	mkText(t, filepath.Join(src, "a.txt"), "alpha")
	mkText(t, filepath.Join(dst, "a.txt"), "al")
	if err := verifyCopy(src, dst); !errors.Is(err, errCopyVerify) {
		t.Fatalf("err = %v, want errCopyVerify", err)
	}

	missing := filepath.Join(tmp, "missing")
	if err := os.MkdirAll(missing, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := verifyCopy(src, missing); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestDirSize(t *testing.T) {
	root := t.TempDir()
	mkFile(t, filepath.Join(root, "a.bin"), 1000)
	mkFile(t, filepath.Join(root, "sub", "b.bin"), 2000)
	if got := DirSize(root); got != 3000 {
		t.Fatalf("DirSize = %d, want 3000", got)
	}
	if got := DirSize(filepath.Join(root, "nope")); got != 0 {
		t.Fatalf("DirSize(missing) = %d, want 0", got)
	}
}

func TestSameVolume(t *testing.T) {
	tmp := t.TempDir()
	a := filepath.Join(tmp, "a")
	b := filepath.Join(tmp, "b")
	got := SameVolume(a, b)
	if runtime.GOOS == "windows" {
		if !got {
			t.Fatalf("SameVolume(%s, %s) = false, want true", a, b)
		}
		if !SameVolume(`c:\one`, `C:\two`) {
			t.Fatal("volume comparison must be case-insensitive")
		}
		if SameVolume(`C:\one`, `D:\two`) {
			t.Fatal("different volumes must not match")
		}
		return
	}
	if got {
		t.Fatal("SameVolume must be false on non-windows")
	}
}

func TestEntryModeCaps(t *testing.T) {
	if got := entryMode(0o777); got != 0o755 {
		t.Fatalf("entryMode(0777) = %o, want 755", got)
	}
	if got := entryMode(0o666); got != 0o644 {
		t.Fatalf("entryMode(0666) = %o, want 644", got)
	}
}

func TestCopyStreamPropagatesWriteError(t *testing.T) {
	rep := newReporter(nil, 0)
	err := copyStream(context.Background(), failWriter{}, bytes.NewReader([]byte("data")), rep, make([]byte, 4))
	if err == nil {
		t.Fatal("expected write error")
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("boom") }
