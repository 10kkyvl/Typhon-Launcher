package download

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	for rel, body := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func assertTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, want := range files {
		got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
		if string(got) != want {
			t.Fatalf("%s = %q, want %q", rel, got, want)
		}
	}
}

func TestCopyTree(t *testing.T) {
	big := string(bytes.Repeat([]byte("0123456789abcdef"), 1<<17))
	files := map[string]string{
		"game.exe":           "exe",
		"data/level1.pak":    big,
		"data/deep/a/b.txt":  "deep",
		"data/deep/empty.db": "",
	}

	t.Run("copies files nested directories and empty directories", func(t *testing.T) {
		src := filepath.Join(t.TempDir(), "src")
		writeTree(t, src, files)
		if err := os.MkdirAll(filepath.Join(src, "logs", "empty"), 0o755); err != nil {
			t.Fatal(err)
		}
		dst := filepath.Join(t.TempDir(), "out", "dst")

		if err := copyTree(t.Context(), src, dst); err != nil {
			t.Fatalf("copyTree: %v", err)
		}

		assertTree(t, dst, files)
		if st, err := os.Stat(filepath.Join(dst, "logs", "empty")); err != nil || !st.IsDir() {
			t.Fatalf("empty directory not copied: %v", err)
		}
		assertTree(t, src, files)
	})

	t.Run("a missing source is an error and not an empty copy", func(t *testing.T) {
		err := copyTree(t.Context(), filepath.Join(t.TempDir(), "gone"), filepath.Join(t.TempDir(), "dst"))
		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("error = %v, want the walk error to reach the caller", err)
		}
	})

	t.Run("a cancelled context stops the copy and keeps the source", func(t *testing.T) {
		src := filepath.Join(t.TempDir(), "src")
		writeTree(t, src, files)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		err := copyTree(ctx, src, filepath.Join(t.TempDir(), "dst"))
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v", err)
		}
		assertTree(t, src, files)
	})

	t.Run("a destination below a regular file fails", func(t *testing.T) {
		src := filepath.Join(t.TempDir(), "src")
		writeTree(t, src, files)
		blocker := filepath.Join(t.TempDir(), "blocker")
		if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := copyTree(t.Context(), src, filepath.Join(blocker, "dst")); err == nil {
			t.Fatal("the copy below a file succeeded")
		}
		assertTree(t, src, files)
	})
}

func TestMoveTreeIfPresent(t *testing.T) {
	files := map[string]string{"GameA/file.bin": "data", "GameB/other.bin": "more"}

	t.Run("a source that is a regular file is refused", func(t *testing.T) {
		root := t.TempDir()
		old := filepath.Join(root, "old")
		if err := os.WriteFile(old, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := moveTreeIfPresent(t.Context(), old, filepath.Join(root, "new")); !errors.Is(err, errRepointNotDir) {
			t.Fatalf("error = %v, want errRepointNotDir", err)
		}
		if _, err := os.Stat(old); err != nil {
			t.Fatalf("the source was touched: %v", err)
		}
	})

	t.Run("a cancelled context moves nothing", func(t *testing.T) {
		root := t.TempDir()
		old, next := filepath.Join(root, "old"), filepath.Join(root, "new")
		writeTree(t, old, files)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if err := moveTreeIfPresent(ctx, old, next); !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v", err)
		}
		assertTree(t, old, files)
		if _, err := os.Stat(next); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("destination appeared: %v", err)
		}
	})

	t.Run("a destination that already holds the same tree finishes the interrupted move", func(t *testing.T) {
		root := t.TempDir()
		old, next := filepath.Join(root, "old"), filepath.Join(root, "new")
		writeTree(t, old, files)
		writeTree(t, next, files)
		if err := moveTreeIfPresent(t.Context(), old, next); err != nil {
			t.Fatalf("moveTreeIfPresent: %v", err)
		}
		if _, err := os.Stat(old); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("old root still present: %v", err)
		}
		assertTree(t, next, files)
	})

	t.Run("a destination that differs from the source is never trusted", func(t *testing.T) {
		cases := map[string]map[string]string{
			"altered content": {"GameA/file.bin": "DATA", "GameB/other.bin": "more"},
			"extra file":      {"GameA/file.bin": "data", "GameB/other.bin": "more", "stray.txt": "x"},
			"missing file":    {"GameA/file.bin": "data"},
		}
		for name, other := range cases {
			t.Run(name, func(t *testing.T) {
				root := t.TempDir()
				old, next := filepath.Join(root, "old"), filepath.Join(root, "new")
				writeTree(t, old, files)
				writeTree(t, next, other)
				err := moveTreeIfPresent(t.Context(), old, next)
				if !errors.Is(err, errRepointVerifyFailed) {
					t.Fatalf("error = %v, want errRepointVerifyFailed", err)
				}
				assertTree(t, old, files)
				assertTree(t, next, other)
			})
		}
	})

	t.Run("an empty destination is replaced by a rename without a staging copy", func(t *testing.T) {
		root := t.TempDir()
		old, next := filepath.Join(root, "old"), filepath.Join(root, "new", "deeper")
		writeTree(t, old, files)
		if err := moveTreeIfPresent(t.Context(), old, next); err != nil {
			t.Fatal(err)
		}
		assertTree(t, next, files)
		if _, err := os.Stat(old); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("old root still present: %v", err)
		}
		if _, err := os.Stat(next + ".repoint-staging"); err == nil {
			t.Fatal("a staging copy was made for a plain rename")
		}
	})
}

func TestRepointRefusesAnythingNotSettled(t *testing.T) {
	for _, status := range allStatuses {
		settled := status == StatusCompleted || status == StatusFailed || status == StatusPaused
		t.Run(string(status), func(t *testing.T) {
			root := t.TempDir()
			old, next := filepath.Join(root, "old"), filepath.Join(root, "new")
			writeTree(t, old, map[string]string{"GameA/file.bin": "data"})
			m := mustManagerAt(t, t.TempDir())
			m.addRepointItem("a", filepath.Join(old, "GameA"), status, nil)

			err := m.Repoint(t.Context(), old, next)
			m.wg.Wait()

			if settled {
				if err != nil {
					t.Fatalf("Repoint: %v", err)
				}
				assertTree(t, next, map[string]string{"GameA/file.bin": "data"})
				return
			}
			if !errors.Is(err, errDownloadsActive) {
				t.Fatalf("error = %v, want errDownloadsActive", err)
			}
			assertTree(t, old, map[string]string{"GameA/file.bin": "data"})
			if _, err := os.Stat(next); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("destination appeared: %v", err)
			}
		})
	}

	t.Run("a settled download with a job in flight", func(t *testing.T) {
		root := t.TempDir()
		old, next := filepath.Join(root, "old"), filepath.Join(root, "new")
		writeTree(t, old, map[string]string{"GameA/file.bin": "data"})
		m := mustManagerAt(t, t.TempDir())
		m.addRepointItem("a", filepath.Join(old, "GameA"), StatusCompleted, nil)
		m.mu.Lock()
		m.jobs["a"] = &jobState{cancel: func() {}, done: make(chan struct{})}
		m.mu.Unlock()

		if err := m.Repoint(t.Context(), old, next); !errors.Is(err, errDownloadsActive) {
			t.Fatalf("error = %v, want errDownloadsActive", err)
		}
		assertTree(t, old, map[string]string{"GameA/file.bin": "data"})
	})
}

func TestRepointKeepsOldDestinationsWhenTheRecordsCannotBeSavedAndRetries(t *testing.T) {
	root := t.TempDir()
	old, next := filepath.Join(root, "old"), filepath.Join(root, "new")
	writeTree(t, old, map[string]string{"GameA/file.bin": "data"})
	m := mustManagerAt(t, t.TempDir())
	eng := &fakeTorrent{size: 100}
	m.addRepointItem("a", filepath.Join(old, "GameA"), StatusPaused, eng)
	blockDownloadsFile(t, m)

	err := m.Repoint(t.Context(), old, next)

	if err == nil {
		t.Fatal("the persist failure was swallowed")
	}
	if got := mustGet(t, m, "a").Destination; got != filepath.Join(old, "GameA") {
		t.Fatalf("destination = %q: records and disk must not be told different things", got)
	}
	if st := m.degradedStatus(); !st.Degraded {
		t.Fatalf("degraded = %+v", st)
	}

	if err := os.Remove(filepath.Join(m.store.dir, "downloads.json")); err != nil {
		t.Fatal(err)
	}
	if err := m.Repoint(t.Context(), old, next); err != nil {
		t.Fatalf("retry after the disk recovered: %v", err)
	}
	m.wg.Wait()
	want := filepath.Join(next, "GameA")
	if got := mustGet(t, m, "a").Destination; got != want {
		t.Fatalf("destination after the retry = %q, want %q", got, want)
	}
	if got := persistedRecords(t, m)[0].Destination; got != want {
		t.Fatalf("persisted destination = %q, want %q", got, want)
	}
	assertTree(t, next, map[string]string{"GameA/file.bin": "data"})
}
