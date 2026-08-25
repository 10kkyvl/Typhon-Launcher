package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/platform"
)

func TestFindExecutablesReportsFailures(t *testing.T) {
	root := t.TempDir()
	mkFile(t, filepath.Join(root, "Game.exe"), 1<<20)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	cases := []struct {
		name string
		ctx  context.Context
		root string
		want error
	}{
		{name: "missing root", ctx: context.Background(), root: filepath.Join(root, "gone"), want: os.ErrNotExist},
		{name: "file on the path", ctx: context.Background(), root: filepath.Join(root, "Game.exe", "sub"), want: nil},
		{name: "cancelled", ctx: cancelled, root: root, want: context.Canceled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := FindExecutables(tc.ctx, tc.root, "Game")
			if err == nil {
				t.Fatalf("FindExecutables = %+v, want error", got)
			}
			if got != nil {
				t.Fatalf("candidates = %+v, want none alongside the error", got)
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestInspectHonoursCancelledContext(t *testing.T) {
	root := t.TempDir()
	mkFile(t, filepath.Join(root, "data", "assets.pak"), 1<<20)
	mkFile(t, filepath.Join(root, "Game.exe"), 1<<20)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if plan, err := Inspect(ctx, root); !errors.Is(err, context.Canceled) {
		t.Fatalf("Inspect = %+v, err = %v, want context.Canceled", plan, err)
	}
}

func TestScanDirReportsFailures(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file.txt")
	mkFile(t, file, 8)

	out := map[string]dirState{}
	if err := scanDir(out, file, 1); err == nil {
		t.Fatal("scanDir over a non-directory must fail instead of recording an empty state")
	}
	if len(out) != 0 {
		t.Fatalf("states = %+v, want none", out)
	}
}

func TestScanDirKeepsModTime(t *testing.T) {
	root := t.TempDir()
	mkFile(t, filepath.Join(root, "sub", "file.txt"), 8)

	snap, err := takeSnapshot([]string{root})
	if err != nil {
		t.Fatalf("takeSnapshot: %v", err)
	}
	state, ok := snap.dirs[root]
	if !ok {
		t.Fatalf("dirs = %+v, want %s", snap.dirs, root)
	}
	if state.modTime.IsZero() {
		t.Fatal("modTime must never stay zero for a directory that was read")
	}
}

func TestCheckSpaceFailsWhenFreeSpaceIsUnknown(t *testing.T) {
	failing := errors.New("volume unavailable")
	cases := []struct {
		name        string
		destination string
		need        int64
		free        func(string) (platform.StorageInfo, error)
		wantErr     bool
	}{
		{
			name:        "free space query fails",
			destination: t.TempDir(),
			need:        1 << 20,
			free:        func(string) (platform.StorageInfo, error) { return platform.StorageInfo{}, failing },
			wantErr:     true,
		},
		{
			name:        "empty destination",
			destination: "",
			need:        1 << 20,
			free:        func(string) (platform.StorageInfo, error) { return platform.StorageInfo{FreeBytes: 1 << 30}, nil },
			wantErr:     true,
		},
		{
			name:        "not enough space",
			destination: t.TempDir(),
			need:        1 << 30,
			free:        func(string) (platform.StorageInfo, error) { return platform.StorageInfo{FreeBytes: 1 << 20}, nil },
			wantErr:     true,
		},
		{
			name:        "enough space",
			destination: t.TempDir(),
			need:        1 << 20,
			free:        func(string) (platform.StorageInfo, error) { return platform.StorageInfo{FreeBytes: 1 << 30}, nil },
			wantErr:     false,
		},
		{
			name:        "nothing to check",
			destination: "",
			need:        0,
			free:        func(string) (platform.StorageInfo, error) { return platform.StorageInfo{}, failing },
			wantErr:     false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &Service{freeSpace: tc.free}
			err := s.checkSpace(tc.destination, tc.need)
			if tc.wantErr != (err != nil) {
				t.Fatalf("checkSpace = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}
