package updates

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func writeFileAt(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCarryOverExtrasCopiesUserFiles(t *testing.T) {
	previous := t.TempDir()
	current := t.TempDir()
	writeFileAt(t, filepath.Join(previous, "saves", "slot1.sav"), "save")
	writeFileAt(t, filepath.Join(previous, "game.exe"), "old binary")
	writeFileAt(t, filepath.Join(current, "game.exe"), "new binary")

	report, err := carryOverExtras(context.Background(), previous, current)
	if err != nil {
		t.Fatalf("carryOverExtras: %v", err)
	}
	if report.skipped != 0 {
		t.Fatalf("skipped = %d, want 0", report.skipped)
	}
	if report.carried != 4 {
		t.Fatalf("carried = %d, want 4", report.carried)
	}
	if _, err := os.Stat(filepath.Join(current, "saves", "slot1.sav")); err != nil {
		t.Fatalf("save not carried over: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(current, "game.exe"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new binary" {
		t.Fatalf("binary overwritten: %q", data)
	}
}

func TestCarryOverExtrasReportsFailures(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	cases := []struct {
		name     string
		ctx      context.Context
		previous func(t *testing.T) string
		current  func(t *testing.T) string
	}{
		{
			name: "missing previous",
			ctx:  context.Background(),
			previous: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "gone")
			},
			current: func(t *testing.T) string { return t.TempDir() },
		},
		{
			name:     "empty paths",
			ctx:      context.Background(),
			previous: func(t *testing.T) string { return "" },
			current:  func(t *testing.T) string { return t.TempDir() },
		},
		{
			name: "previous is a file",
			ctx:  context.Background(),
			previous: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "file.txt")
				writeFileAt(t, path, "data")
				return filepath.Join(path, "sub")
			},
			current: func(t *testing.T) string { return t.TempDir() },
		},
		{
			name: "target path blocked by a file",
			ctx:  context.Background(),
			previous: func(t *testing.T) string {
				dir := t.TempDir()
				writeFileAt(t, filepath.Join(dir, "saves", "slot1.sav"), "save")
				return dir
			},
			current: func(t *testing.T) string {
				dir := t.TempDir()
				writeFileAt(t, filepath.Join(dir, "saves"), "not a directory")
				return dir
			},
		},
		{
			name: "cancelled context",
			ctx:  cancelled,
			previous: func(t *testing.T) string {
				dir := t.TempDir()
				writeFileAt(t, filepath.Join(dir, "config.cfg"), "cfg")
				return dir
			},
			current: func(t *testing.T) string { return t.TempDir() },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			previous := tc.previous(t)
			current := tc.current(t)
			if _, err := carryOverExtras(tc.ctx, previous, current); err == nil {
				t.Fatal("carryOverExtras must report the failure instead of silently skipping files")
			}
		})
	}
}

func TestCarryOverExtrasReportsSkippedOverLimit(t *testing.T) {
	previous := t.TempDir()
	current := t.TempDir()
	big := make([]byte, 1024)
	writeFileAt(t, filepath.Join(previous, "huge.sav"), string(big))

	report, err := carryOverLimited(context.Background(), previous, current, 512)
	if err != nil {
		t.Fatalf("carryOverExtras: %v", err)
	}
	if report.carried != 0 {
		t.Fatalf("carried = %d, want 0", report.carried)
	}
	if report.skipped != int64(len(big)) {
		t.Fatalf("skipped = %d, want %d", report.skipped, len(big))
	}
}

func TestCarryOverExtrasKeepsSymlinks(t *testing.T) {
	previous := t.TempDir()
	current := t.TempDir()
	target := filepath.Join(previous, "real.cfg")
	writeFileAt(t, target, "cfg")
	link := filepath.Join(previous, "link.cfg")
	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("создание симлинка на Windows требует прав разработчика или администратора:", err)
		}
		t.Fatal(err)
	}

	if _, err := carryOverExtras(context.Background(), previous, current); err != nil {
		t.Fatalf("carryOverExtras: %v", err)
	}
	info, err := os.Lstat(filepath.Join(current, "link.cfg"))
	if err != nil {
		t.Fatalf("symlink not carried over: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("mode = %s, want a symlink", info.Mode())
	}
}

func TestResolveExecutableReportsScanErrors(t *testing.T) {
	if _, err := resolveExecutable(context.Background(), "", "", "", ""); !errors.Is(err, errEmptyInstallDir) {
		t.Fatalf("err = %v, want %v", err, errEmptyInstallDir)
	}
	missing := filepath.Join(t.TempDir(), "gone")
	if _, err := resolveExecutable(context.Background(), missing, "", "", ""); err == nil {
		t.Fatal("scan of a missing install dir must fail")
	}
}
