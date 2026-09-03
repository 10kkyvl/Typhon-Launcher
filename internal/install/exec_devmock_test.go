//go:build devmock && !windows

package install

import (
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/devmock"
)

func TestSystemExecutableCreatesFileUnderStateDir(t *testing.T) {
	t.Setenv(devmock.StateDirEnv, t.TempDir())

	path, err := systemExecutable("msiexec.exe")
	if err != nil {
		t.Fatalf("systemExecutable: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if info.IsDir() {
		t.Fatalf("%s is a directory", path)
	}

	stateDir, err := devmock.StateDir()
	if err != nil {
		t.Fatalf("devmock.StateDir: %v", err)
	}
	want := filepath.Join(stateDir, "system32", "msiexec.exe")
	if path != want {
		t.Fatalf("systemExecutable path = %s, want %s", path, want)
	}
}

func TestSystemExecutableReusesExistingFile(t *testing.T) {
	t.Setenv(devmock.StateDirEnv, t.TempDir())

	first, err := systemExecutable("msiexec.exe")
	if err != nil {
		t.Fatalf("systemExecutable: %v", err)
	}
	if err := os.WriteFile(first, []byte("marker"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := systemExecutable("msiexec.exe")
	if err != nil {
		t.Fatalf("systemExecutable: %v", err)
	}
	if second != first {
		t.Fatalf("systemExecutable path changed: %s != %s", second, first)
	}
	data, err := os.ReadFile(second)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "marker" {
		t.Fatalf("existing file overwritten, content = %q", data)
	}
}
