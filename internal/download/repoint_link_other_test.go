//go:build !windows

package download

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestCopyTreeRecreatesSymlinksAsLinks(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src")
	writeTree(t, src, map[string]string{"real.txt": "payload"})
	links := map[string]string{"alias.txt": "real.txt", "dangling.txt": "nowhere.txt"}
	for link, target := range links {
		if err := os.Symlink(target, filepath.Join(src, link)); err != nil {
			t.Fatal(err)
		}
	}
	dst := filepath.Join(t.TempDir(), "dst")

	if err := copyTree(t.Context(), src, dst); err != nil {
		t.Fatalf("copyTree: %v", err)
	}

	for link, want := range links {
		got, err := os.Readlink(filepath.Join(dst, link))
		if err != nil {
			t.Fatalf("%s was not copied as a link: %v", link, err)
		}
		if got != want {
			t.Fatalf("%s points at %q, want %q", link, got, want)
		}
	}
	assertTree(t, dst, map[string]string{"real.txt": "payload"})
	if info, err := os.Lstat(filepath.Join(dst, "real.txt")); err != nil || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("the regular file became a link: %v", err)
	}
}

func TestCopyTreeRefusesAFileThatIsNeitherRegularNorALink(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src")
	writeTree(t, src, map[string]string{"real.txt": "payload"})
	if err := syscall.Mkfifo(filepath.Join(src, "pipe"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := copyTree(t.Context(), src, filepath.Join(t.TempDir(), "dst"))

	if !errors.Is(err, errRepointNonRegular) {
		t.Fatalf("error = %v, want errRepointNonRegular: dropping the file would lose it with the source", err)
	}
	assertTree(t, src, map[string]string{"real.txt": "payload"})
}
