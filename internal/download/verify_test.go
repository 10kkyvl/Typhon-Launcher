package download

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeVerifyFile(t *testing.T, dir, name string, size int64) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	return path
}

func TestVerifyFilesOnDisk(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "game.bin")
		files := []FileState{{Path: "game.bin", Size: 10, Selected: true}}
		err := verifyFilesOnDisk(context.Background(), files, []string{path})
		if !errors.Is(err, errFileMissing) {
			t.Fatalf("err = %v, want errFileMissing", err)
		}
	})

	t.Run("no path recorded for file", func(t *testing.T) {
		files := []FileState{{Path: "game.bin", Size: 10, Selected: true}}
		err := verifyFilesOnDisk(context.Background(), files, nil)
		if !errors.Is(err, errFileMissing) {
			t.Fatalf("err = %v, want errFileMissing", err)
		}
	})

	t.Run("only part file remains", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "game.bin")
		writeVerifyFile(t, dir, "game.bin"+PartFileSuffix, 5)
		files := []FileState{{Path: "game.bin", Size: 10, Selected: true}}
		err := verifyFilesOnDisk(context.Background(), files, []string{path})
		if !errors.Is(err, errFileIncomplete) {
			t.Fatalf("err = %v, want errFileIncomplete", err)
		}
	})

	t.Run("file shorter than expected", func(t *testing.T) {
		dir := t.TempDir()
		path := writeVerifyFile(t, dir, "game.bin", 5)
		files := []FileState{{Path: "game.bin", Size: 10, Selected: true}}
		err := verifyFilesOnDisk(context.Background(), files, []string{path})
		if !errors.Is(err, errFileTruncated) {
			t.Fatalf("err = %v, want errFileTruncated", err)
		}
	})

	t.Run("full file with unrelated stray part file", func(t *testing.T) {
		dir := t.TempDir()
		path := writeVerifyFile(t, dir, "game.bin", 10)
		writeVerifyFile(t, dir, "other.bin"+PartFileSuffix, 3)
		files := []FileState{{Path: "game.bin", Size: 10, Selected: true}}
		if err := verifyFilesOnDisk(context.Background(), files, []string{path}); err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
	})

	t.Run("unselected file is skipped even when absent", func(t *testing.T) {
		files := []FileState{{Path: "extra.bin", Size: 10, Selected: false}}
		if err := verifyFilesOnDisk(context.Background(), files, nil); err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
	})

	t.Run("cancelled context", func(t *testing.T) {
		dir := t.TempDir()
		path := writeVerifyFile(t, dir, "game.bin", 10)
		files := []FileState{{Path: "game.bin", Size: 10, Selected: true}}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := verifyFilesOnDisk(ctx, files, []string{path})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	})
}
