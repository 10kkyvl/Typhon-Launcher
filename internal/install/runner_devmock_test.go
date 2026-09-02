//go:build devmock && !windows

package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestMockRunnerInstallWithDestination(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "Foo Game")
	logPath := filepath.Join(t.TempDir(), "installer.log")
	r := newRunner(func() string { return "" })
	spec := runSpec{
		Path:          filepath.Join(t.TempDir(), "download", "FooGame-setup.exe"),
		InstallerPath: filepath.Join(t.TempDir(), "download", "FooGame-setup.exe"),
		Destination:   dest,
		LogPath:       logPath,
	}
	code, err := r.run(context.Background(), spec)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	exe := filepath.Join(dest, "FooGame.exe")
	if info, statErr := os.Stat(exe); statErr != nil {
		t.Fatalf("stat exe: %v", statErr)
	} else if info.Size() != devmockExeSize {
		t.Fatalf("exe size = %d, want %d", info.Size(), devmockExeSize)
	}
	if _, statErr := os.Stat(filepath.Join(dest, devmockMarkerName)); statErr != nil {
		t.Fatalf("stat marker: %v", statErr)
	}
	if _, statErr := os.Stat(logPath); statErr != nil {
		t.Fatalf("stat log: %v", statErr)
	}
}

func TestMockRunnerInstallWithoutDestinationUsesGamesPath(t *testing.T) {
	gamesRoot := t.TempDir()
	r := newRunner(func() string { return gamesRoot })
	spec := runSpec{
		Path:          filepath.Join(t.TempDir(), "FooGame-setup.exe"),
		InstallerPath: filepath.Join(t.TempDir(), "FooGame-setup.exe"),
	}
	code, err := r.run(context.Background(), spec)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	dest := filepath.Join(gamesRoot, "FooGame")
	if _, statErr := os.Stat(filepath.Join(dest, "FooGame.exe")); statErr != nil {
		t.Fatalf("stat exe: %v", statErr)
	}
}

func TestMockRunnerInstallEmptyGamesPathErrors(t *testing.T) {
	r := newRunner(func() string { return "" })
	spec := runSpec{
		Path:          filepath.Join(t.TempDir(), "FooGame-setup.exe"),
		InstallerPath: filepath.Join(t.TempDir(), "FooGame-setup.exe"),
	}
	if _, err := r.run(context.Background(), spec); err == nil {
		t.Fatal("expected error for empty games path")
	}
}

func TestMockRunnerInstallEmptyNameErrors(t *testing.T) {
	r := newRunner(func() string { return t.TempDir() })
	spec := runSpec{
		Path:          filepath.Join(t.TempDir(), "setup.exe"),
		InstallerPath: filepath.Join(t.TempDir(), "setup.exe"),
	}
	if _, err := r.run(context.Background(), spec); err == nil {
		t.Fatal("expected error for empty derived name")
	}
}

func TestMockRunnerUninstallRemovesMarkedDir(t *testing.T) {
	dir := t.TempDir()
	installDir := filepath.Join(dir, "Foo Game")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installDir, devmockMarkerName), []byte("installer"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := newRunner(func() string { return "" })
	spec := runSpec{Path: filepath.Join(installDir, "unins000.exe")}
	code, err := r.run(context.Background(), spec)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if _, statErr := os.Stat(installDir); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("install dir still present: %v", statErr)
	}
}

func TestMockRunnerUninstallRefusesUnmarkedDir(t *testing.T) {
	dir := t.TempDir()
	installDir := filepath.Join(dir, "Foo Game")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatal(err)
	}
	r := newRunner(func() string { return "" })
	spec := runSpec{Path: filepath.Join(installDir, "unins000.exe")}
	if _, err := r.run(context.Background(), spec); err == nil {
		t.Fatal("expected error removing unmarked directory")
	}
	if _, statErr := os.Stat(installDir); statErr != nil {
		t.Fatalf("install dir removed: %v", statErr)
	}
}

func TestMockRunnerProductCodeUninstallSucceeds(t *testing.T) {
	r := newRunner(func() string { return "" })
	spec := runSpec{
		Path: filepath.Join(t.TempDir(), "system32", "msiexec.exe"),
		Args: []string{"/x", "{PRODUCT-CODE}", "/qn", "/norestart"},
	}
	code, err := r.run(context.Background(), spec)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
}

func TestMockRunnerCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := newRunner(func() string { return "" })
	spec := runSpec{Path: filepath.Join(t.TempDir(), "FooGame-setup.exe")}
	if _, err := r.run(ctx, spec); !errors.Is(err, context.Canceled) {
		t.Fatalf("run error = %v, want context.Canceled", err)
	}
}
