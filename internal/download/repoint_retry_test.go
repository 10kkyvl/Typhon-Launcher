package download

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// A removal of the old root that stops half way (an antivirus holds a file, a
// handle is open) leaves a remainder there and the complete tree at the new
// root. Running the move again has to finish it, not fail for good because the
// new root holds more than the remainder does.
func TestMoveTreeFinishesWhenTheOldRootHoldsOnlyWhatTheNewOneAlreadyHas(t *testing.T) {
	full := map[string]string{"GameA/file.bin": "data", "GameB/other.bin": "more", "GameB/deep/x.bin": "x"}
	cases := []struct {
		name       string
		remainder  map[string]string
		emptyDirs  []string
		wantFailed bool
	}{
		{name: "a file that is left", remainder: map[string]string{"GameB/other.bin": "more"}},
		{name: "several files that are left", remainder: map[string]string{"GameA/file.bin": "data", "GameB/deep/x.bin": "x"}},
		{name: "only the directories are left", emptyDirs: []string{"GameA", "GameB/deep"}},
		{name: "a left file whose content differs", remainder: map[string]string{"GameB/other.bin": "MORE"}, wantFailed: true},
		{name: "a left file of another size", remainder: map[string]string{"GameB/other.bin": "m"}, wantFailed: true},
		{name: "a left file the new root does not have", remainder: map[string]string{"GameB/other.bin": "more", "GameC/new.bin": "n"}, wantFailed: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			old, next := filepath.Join(root, "old"), filepath.Join(root, "new")
			writeTree(t, old, c.remainder)
			for _, dir := range c.emptyDirs {
				if err := os.MkdirAll(filepath.Join(old, filepath.FromSlash(dir)), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			writeTree(t, next, full)

			err := moveTreeIfPresent(t.Context(), old, next)

			assertTree(t, next, full)
			if c.wantFailed {
				if !errors.Is(err, errRepointVerifyFailed) {
					t.Fatalf("error = %v, want errRepointVerifyFailed", err)
				}
				if _, statErr := os.Stat(old); statErr != nil {
					t.Fatalf("the old root was touched although it holds something the new one lacks: %v", statErr)
				}
				assertTree(t, old, c.remainder)
				return
			}
			if err != nil {
				t.Fatalf("moveTreeIfPresent: %v", err)
			}
			if _, statErr := os.Stat(old); !errors.Is(statErr, fs.ErrNotExist) {
				t.Fatalf("old root still present: %v", statErr)
			}
		})
	}
}

func TestMoveTreeNeverReplacesAnythingButAnEmptyDirectoryAtTheTarget(t *testing.T) {
	files := map[string]string{"GameA/file.bin": "data"}
	cases := []struct {
		name   string
		target func(t *testing.T, path string)
		body   string
	}{
		{"a regular file with content", func(t *testing.T, path string) { writeFile(t, path, "precious") }, "precious"},
		{"an empty regular file", func(t *testing.T, path string) { writeFile(t, path, "") }, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			old, next := filepath.Join(root, "old"), filepath.Join(root, "new")
			writeTree(t, old, files)
			c.target(t, next)

			err := moveTreeIfPresent(t.Context(), old, next)

			if !errors.Is(err, errRepointTargetNotDir) {
				t.Fatalf("error = %v, want errRepointTargetNotDir", err)
			}
			st, statErr := os.Stat(next)
			if statErr != nil || !st.Mode().IsRegular() {
				t.Fatalf("the file at the target was replaced: %v", statErr)
			}
			got, readErr := os.ReadFile(next)
			if readErr != nil || string(got) != c.body {
				t.Fatalf("the file at the target = %q, %v", got, readErr)
			}
			assertTree(t, old, files)
		})
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
