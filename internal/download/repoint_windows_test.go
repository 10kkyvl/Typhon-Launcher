package download

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestMoveTreeCopiesAndVerifiesWhenAnOpenFileBlocksTheRename(t *testing.T) {
	files := map[string]string{"GameA/file.bin": "data", "GameB/other.bin": "more"}
	root := t.TempDir()
	old, next := filepath.Join(root, "old"), filepath.Join(root, "new")
	writeTree(t, old, files)
	holdOpen(t, filepath.Join(old, "GameA", "file.bin"))

	err := moveTreeIfPresent(t.Context(), old, next)

	if err == nil {
		t.Fatal("the old root was removed under an open file")
	}
	assertTree(t, next, files)
	if _, err := os.Stat(next + ".repoint-staging"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the staging copy was left behind: %v", err)
	}
	if _, err := os.Stat(filepath.Join(old, "GameA", "file.bin")); err != nil {
		t.Fatalf("the file that is still open must stay where it is: %v", err)
	}
}

func TestMoveTreeRetryFinishesAfterTheLockThatStoppedTheRemovalIsGone(t *testing.T) {
	files := map[string]string{"GameA/file.bin": "data", "GameB/other.bin": "more", "GameB/deep/x.bin": "x"}
	root := t.TempDir()
	old, next := filepath.Join(root, "old"), filepath.Join(root, "new")
	writeTree(t, old, files)
	held := holdOpen(t, filepath.Join(old, "GameA", "file.bin"))

	first := moveTreeIfPresent(t.Context(), old, next)

	if !errors.Is(first, errRepointOldRootStuck) {
		t.Fatalf("first attempt = %v, want errRepointOldRootStuck: the data is moved, only the old folder is stuck", first)
	}
	assertTree(t, next, files)
	if _, err := os.Stat(filepath.Join(old, "GameB", "other.bin")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("setup: the removal was expected to get through the files nothing holds: %v", err)
	}
	if err := held.Close(); err != nil {
		t.Fatal(err)
	}

	if err := moveTreeIfPresent(t.Context(), old, next); err != nil {
		t.Fatalf("retry once the file is free: %v", err)
	}

	if _, err := os.Stat(old); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("old root still present: %v", err)
	}
	assertTree(t, next, files)
}

func TestMoveTreeRetryReportsAnOldRootThatStaysLocked(t *testing.T) {
	files := map[string]string{"GameA/file.bin": "data"}
	root := t.TempDir()
	old, next := filepath.Join(root, "old"), filepath.Join(root, "new")
	writeTree(t, old, files)
	writeTree(t, next, files)
	holdOpen(t, filepath.Join(old, "GameA", "file.bin"))

	err := moveTreeIfPresent(t.Context(), old, next)

	if !errors.Is(err, errRepointOldRootStuck) {
		t.Fatalf("error = %v, want errRepointOldRootStuck", err)
	}
	assertTree(t, next, files)
}
