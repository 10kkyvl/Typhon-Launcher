package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tempNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			names = append(names, e.Name())
		}
	}
	return names
}

func TestSwapExecutable(t *testing.T) {
	newBytes := []byte("the replacement launcher")
	goodSum := sha256Hex(t, newBytes)
	cases := []struct {
		name      string
		hasTarget bool
		src       func(t *testing.T, dir string) string
		target    func(dir string) string
		sum       string
		ctx       func() (context.Context, context.CancelFunc)
		wantErr   error
		anyErr    bool
	}{
		{name: "replaces the launcher", hasTarget: true, sum: goodSum},
		{name: "creates a missing launcher", hasTarget: false, sum: goodSum},
		{name: "wrong hash keeps the launcher", hasTarget: true, sum: sha256Hex(t, []byte("something else")), wantErr: ErrSwapHash},
		{name: "malformed hash is refused before anything is read", hasTarget: true, sum: "nope", wantErr: ErrInvalidHash},
		{name: "missing replacement keeps the launcher", hasTarget: true, sum: goodSum,
			src: func(t *testing.T, dir string) string { return filepath.Join(dir, "absent.exe") }, anyErr: true},
		{name: "relative target is refused", hasTarget: true, sum: goodSum,
			target: func(string) string { return "typhon.exe" }, anyErr: true},
		{name: "empty target is refused", hasTarget: true, sum: goodSum,
			target: func(string) string { return "" }, anyErr: true},
		{name: "cancelled context keeps the launcher", hasTarget: true, sum: goodSum,
			ctx: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx, cancel
			}, wantErr: context.Canceled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "typhon.exe")
			if tc.hasTarget {
				writeTestFile(t, target, []byte("the running launcher"))
			}
			src := filepath.Join(t.TempDir(), "replacement.exe")
			writeTestFile(t, src, newBytes)
			if tc.src != nil {
				src = tc.src(t, dir)
			}
			path := target
			if tc.target != nil {
				path = tc.target(dir)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.ctx != nil {
				var c context.CancelFunc
				ctx, c = tc.ctx()
				defer c()
			}

			err := swapExecutable(ctx, path, src, tc.sum)

			failing := tc.wantErr != nil || tc.anyErr
			if !failing {
				if err != nil {
					t.Fatalf("swapExecutable: %v", err)
				}
				if got := readTestFile(t, target); got != string(newBytes) {
					t.Fatalf("launcher = %q", got)
				}
			} else {
				if err == nil {
					t.Fatal("swapExecutable() error = nil")
				}
				if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", err, tc.wantErr)
				}
				if tc.hasTarget && path == target {
					if got := readTestFile(t, target); got != "the running launcher" {
						t.Fatalf("a failed swap changed the launcher to %q", got)
					}
				} else if !tc.hasTarget {
					if _, statErr := os.Stat(target); statErr == nil {
						t.Fatal("a failed swap created the launcher")
					}
				}
			}
			if left := tempNames(t, dir); len(left) != 0 {
				t.Fatalf("temp files left beside the launcher: %v", left)
			}
		})
	}
}

// The swap is one rename, so a worker that dies between writing the
// replacement and renaming it leaves the old launcher in place and a temp file
// beside it. These are the states a start has to cope with.
func TestRecoverSwapRemovesOnlyOldTempFiles(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "typhon.exe")
	writeTestFile(t, exe, []byte("the launcher"))
	writeTestFile(t, filepath.Join(dir, "uninstall.exe"), []byte("uninstaller"))
	stale := filepath.Join(dir, ".tmp-111")
	young := filepath.Join(dir, ".tmp-222")
	other := filepath.Join(dir, "notes.tmp-1")
	writeTestFile(t, stale, []byte("half of a replacement"))
	writeTestFile(t, young, []byte("a write in flight"))
	writeTestFile(t, other, []byte("not ours"))
	ageFile(t, stale, 2*swapLeftoverAge)
	ageFile(t, young, swapLeftoverAge/4)
	if err := os.Mkdir(filepath.Join(dir, ".tmp-dir"), 0o700); err != nil {
		t.Fatal(err)
	}
	ageFile(t, filepath.Join(dir, ".tmp-dir"), 2*swapLeftoverAge)

	recoverSwap(exe)

	if _, err := os.Stat(stale); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale temp file survived: %v", err)
	}
	for _, keep := range []string{exe, young, other, filepath.Join(dir, "uninstall.exe"), filepath.Join(dir, ".tmp-dir")} {
		if _, err := os.Stat(keep); err != nil {
			t.Fatalf("recoverSwap removed %s: %v", keep, err)
		}
	}
	if got := readTestFile(t, exe); got != "the launcher" {
		t.Fatalf("launcher = %q", got)
	}
}

func TestRecoverSwapOnAMissingDirectoryDoesNotPanic(t *testing.T) {
	recoverSwap(filepath.Join(t.TempDir(), "no-such-dir", "typhon.exe"))
}
