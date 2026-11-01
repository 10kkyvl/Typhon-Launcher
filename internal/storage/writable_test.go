package storage

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeFileMode(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func isWritable(t *testing.T, path string) bool {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()&0o200 != 0
}

func TestMakeWritable(t *testing.T) {
	t.Run("read-only files at any depth, keeping mtime", func(t *testing.T) {
		root := t.TempDir()
		top, deep := filepath.Join(root, "game.exe"), filepath.Join(root, "data", "nested", "x.bin")
		writeFileMode(t, top, 0o444)
		writeFileMode(t, deep, 0o444)
		mtime := time.Unix(1_700_000_000, 0)
		if err := os.Chtimes(deep, mtime, mtime); err != nil {
			t.Fatal(err)
		}

		if err := MakeWritable(context.Background(), root); err != nil {
			t.Fatalf("MakeWritable: %v", err)
		}

		if !isWritable(t, top) || !isWritable(t, deep) {
			t.Fatal("file stayed read-only")
		}
		info, err := os.Stat(deep)
		if err != nil {
			t.Fatal(err)
		}
		if !info.ModTime().Equal(mtime) {
			t.Fatalf("mtime = %v, want %v", info.ModTime(), mtime)
		}
	})

	t.Run("a second run finishes nothing and fails nothing", func(t *testing.T) {
		root := t.TempDir()
		file := filepath.Join(root, "game.exe")
		writeFileMode(t, file, 0o444)

		for run := 1; run <= 2; run++ {
			if err := MakeWritable(context.Background(), root); err != nil {
				t.Fatalf("run %d: %v", run, err)
			}
		}

		if !isWritable(t, file) {
			t.Fatal("file stayed read-only")
		}
	})

	t.Run("cancelled ctx leaves files alone", func(t *testing.T) {
		root := t.TempDir()
		file := filepath.Join(root, "game.exe")
		writeFileMode(t, file, 0o444)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := MakeWritable(ctx, root)

		if !errors.Is(err, context.Canceled) {
			t.Fatalf("MakeWritable error = %v, want context.Canceled", err)
		}
		if isWritable(t, file) {
			t.Fatal("cancelled walk still changed a file")
		}
	})

	t.Run("missing root", func(t *testing.T) {
		err := MakeWritable(context.Background(), filepath.Join(t.TempDir(), "gone"))
		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("MakeWritable error = %v, want fs.ErrNotExist", err)
		}
	})

	t.Run("empty path is refused", func(t *testing.T) {
		if err := MakeWritable(context.Background(), ""); err == nil {
			t.Fatal("MakeWritable accepted an empty path")
		}
	})

	t.Run("symlink target outside the root stays read-only", func(t *testing.T) {
		outside := filepath.Join(t.TempDir(), "outside.txt")
		writeFileMode(t, outside, 0o444)
		root := t.TempDir()
		if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
			t.Skipf("symlink not supported on this system: %v", err)
		}

		if err := MakeWritable(context.Background(), root); err != nil {
			t.Fatalf("MakeWritable: %v", err)
		}

		if isWritable(t, outside) {
			t.Fatal("walk followed a symlink out of the root")
		}
	})
}
