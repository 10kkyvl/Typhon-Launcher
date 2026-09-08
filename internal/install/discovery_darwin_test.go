//go:build darwin && !devmock

package install

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"typhon/internal/wine"
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

// TestDiscoverWithBottleReusesGivenBottle закрывает находку "каталог бутылей
// сканируется дважды": wineRunner.run уже получает бутыль через Ensure, а
// attemptDiscovery раньше всё равно заново делала wine.Detect()+Lookup —
// второй полный os.ReadDir по каталогу бутылей на каждую установку.
// discoverWithBottle берёт бутыль параметром и не трогает Detect/Lookup
// вовсе. Доказательство: Destination теста — обычный t.TempDir(), которого
// в реальном каталоге бутылей CrossOver нет и быть не может, поэтому старая
// attemptDiscovery на нём всегда отвечала «бутыль установки ещё не
// заведён» — а discoverWithBottle обязана довести разведку до конца, раз ей
// бутыль уже дали.
func TestDiscoverWithBottleReusesGivenBottle(t *testing.T) {
	dir := t.TempDir()
	installer := filepath.Join(dir, "setup.exe")
	if err := os.WriteFile(installer, []byte("MZ"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	inf := filepath.Join(dir, "discovery.inf")
	bottle := wine.Bottle{Key: dir, Name: "Typhon-Test", Path: filepath.Join(dir, ".bottle"), Drive: "t", Games: dir}

	var recorded []wine.Cmd
	run := func(_ context.Context, _ wine.Bottle, c wine.Cmd) (int, error) {
		recorded = append(recorded, c)
		if strings.Contains(strings.Join(c.Args, " "), "/SAVEINF=") {
			if err := os.WriteFile(inf, []byte("[Setup]\nComponents=main,vcredist\n"), 0o644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
		}
		return 0, nil
	}

	outcome, err := discoverWithBottle(context.Background(), discoverySpec{
		Engine: EngineInno, InstallerPath: installer, Destination: dir, InfPath: inf,
		Options: installOptions{SkipExtras: true},
	}, bottle, run)
	if err != nil {
		t.Fatalf("discoverWithBottle: %v", err)
	}
	if outcome.reason != "" {
		t.Fatalf("reason = %q, want discovery to use the bottle it was given instead of re-scanning CrossOver's bottle directory", outcome.reason)
	}
	if len(recorded) != 1 {
		t.Fatalf("run calls = %d, want exactly 1", len(recorded))
	}
	wantPath, err := bottle.ToWindows(installer)
	if err != nil {
		t.Fatalf("ToWindows: %v", err)
	}
	if recorded[0].Path != wantPath {
		t.Fatalf("Path = %q, want %q", recorded[0].Path, wantPath)
	}
	if len(outcome.components) != 1 || outcome.components[0] != "main" {
		t.Fatalf("components = %v, want [main]", outcome.components)
	}
}
