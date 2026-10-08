package selfupdate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyExecutableFailureLeavesNothingBehind(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, dir string) (src, dst string)
	}{
		{
			name: "source cannot be read to the end",
			setup: func(t *testing.T, dir string) (string, string) {
				src := filepath.Join(dir, "launcher")
				if err := os.MkdirAll(src, 0o755); err != nil {
					t.Fatalf("MkdirAll: %v", err)
				}
				return src, filepath.Join(dir, "worker", "typhon-update.exe")
			},
		},
		{
			name: "destination is a directory that cannot be replaced",
			setup: func(t *testing.T, dir string) (string, string) {
				src := filepath.Join(dir, "typhon.exe")
				writeTestFile(t, src, []byte("launcher bytes"))
				dst := filepath.Join(dir, "worker", "typhon-update.exe")
				writeTestFile(t, filepath.Join(dst, "occupant"), []byte("x"))
				return src, dst
			},
		},
		{
			name: "source is missing",
			setup: func(_ *testing.T, dir string) (string, string) {
				return filepath.Join(dir, "gone.exe"), filepath.Join(dir, "worker", "typhon-update.exe")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			src, dst := tt.setup(t, dir)

			if err := copyExecutable(src, dst); err == nil {
				t.Fatal("copyExecutable() error = nil, want the failure reported")
			}
			if _, err := os.Stat(dst + ".tmp"); err == nil {
				t.Fatal("half-written temp copy left in the worker directory")
			}
			if info, err := os.Stat(dst); err == nil && !info.IsDir() {
				t.Fatalf("a file appeared at %s although the copy failed: the worker would be launched from it", dst)
			}
		})
	}
}

func TestCopyExecutableReplacesAnEarlierCopy(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "typhon.exe")
	writeTestFile(t, src, []byte("new launcher bytes"))
	dst := filepath.Join(dir, "worker", "typhon-update.exe")
	writeTestFile(t, dst, []byte("copy from an earlier attempt"))

	if err := copyExecutable(src, dst); err != nil {
		t.Fatalf("copyExecutable() error = %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil || string(got) != "new launcher bytes" {
		t.Fatalf("copy = %q, %v; want the current launcher bytes", got, err)
	}
}
