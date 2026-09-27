package selfupdate

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestCopyExecutableLeavesNoPartialCopy(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "typhon.exe")
	writeTestFile(t, src, []byte("launcher bytes"))
	dst := filepath.Join(dir, "worker", "typhon-update.exe")
	writeTestFile(t, dst+".tmp", []byte("half of an earlier copy"))

	if err := copyExecutable(src, dst); err != nil {
		t.Fatalf("copyExecutable: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read copy: %v", err)
	}
	if string(got) != "launcher bytes" {
		t.Fatalf("copy content = %q, want the source bytes", got)
	}
	if _, err := os.Stat(dst + ".tmp"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("temp copy survived the rename: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(dst))
	if err != nil {
		t.Fatalf("read worker dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("worker dir holds %d entries, want only the finished copy", len(entries))
	}
}

func TestCopyExecutableUnwritableDestination(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "typhon.exe")
	writeTestFile(t, src, []byte("launcher bytes"))
	blocked := filepath.Join(dir, "worker")
	writeTestFile(t, blocked, []byte("a file where the directory should be"))

	dst := filepath.Join(blocked, "typhon-update.exe")
	if err := copyExecutable(src, dst); err == nil {
		t.Fatal("copyExecutable() error = nil, want an error when the worker dir cannot be created")
	}
	if _, err := os.Stat(dst + ".tmp"); err == nil {
		t.Fatal("temp copy left behind after a failure")
	}
}
