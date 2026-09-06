//go:build darwin && !devmock

package install

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverySkippedWhenNotNeeded(t *testing.T) {
	got, err := attemptDiscovery(context.Background(), discoverySpec{})
	if err != nil {
		t.Fatalf("attemptDiscovery: %v", err)
	}
	if got.reason != "" || len(got.components) != 0 || got.elevate {
		t.Fatalf("outcome = %+v, want empty", got)
	}
}

// Разведка на macOS не может потребовать повышения прав: UAC здесь нет,
// поэтому путь через повышенный воркер не должен запускаться никогда.
func TestDiscoveryNeverElevates(t *testing.T) {
	dir := t.TempDir()
	inf := filepath.Join(dir, "discovery.inf")
	if err := os.WriteFile(inf, append([]byte{0xEF, 0xBB, 0xBF}, []byte("[Setup]\nComponents=main,extra\n")...), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := attemptDiscovery(context.Background(), discoverySpec{
		Engine: EngineInno, InstallerPath: filepath.Join(dir, "setup.exe"),
		Destination: dir, WorkingDir: dir, InfPath: inf,
	})
	if err != nil {
		t.Fatalf("attemptDiscovery: %v", err)
	}
	if got.elevate {
		t.Fatal("elevate = true, want false: on macOS there is no UAC")
	}
}
