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

// crossOverInstalled сообщает, стоит ли на машине настоящий CrossOver.
// attemptDiscovery берёт wine.Detect() напрямую и не подменяет его в
// тестах (в отличие от wineRunner.detect в runner_darwin.go), поэтому без
// реального CrossOver проверка ветки с общим бутылём до неё не доедет: она
// отвалится раньше, на "CrossOver не найден".
func crossOverInstalled(t *testing.T) bool {
	t.Helper()
	_, err := wine.Detect()
	return err == nil
}

// TestAttemptDiscoveryUsesSharedBottle закрывает находку "разведка молча
// отключается навсегда для игры в общем бутыле": Lookup по метке
// typhon-bottle.json общий бутыль не находит никогда — метки в нём нет, —
// и без fallback на SharedBottle attemptDiscovery всегда возвращала бы
// reason «бутыль установки ещё не заведён».
//
// Установщик лежит вне games специально: ToWindows внутри общего бутыля
// откажет на первом шаге discoverWithBottle, и настоящий cxstart так и не
// запустится, а по тексту причины видно, что вызов всё же дошёл до бутыля.
func TestAttemptDiscoveryUsesSharedBottle(t *testing.T) {
	if !crossOverInstalled(t) {
		t.Skip("CrossOver не установлен: attemptDiscovery не подменяет wine.Detect() в тестах")
	}
	bottlesDir := fakeBottlesDir(t)
	games := t.TempDir()
	dest := filepath.Join(games, "Kebab Chefs! Restaurant Simulator")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	makeSharedBottleDir(t, bottlesDir, wine.SharedBottleName(), games)

	installer := filepath.Join(t.TempDir(), "setup.exe")
	if err := os.WriteFile(installer, []byte("MZ"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := attemptDiscovery(context.Background(), discoverySpec{
		Engine: EngineInno, InstallerPath: installer, Destination: dest,
		InfPath: filepath.Join(dest, "discovery.inf"),
		Options: installOptions{SkipExtras: true},
	})
	if err != nil {
		t.Fatalf("attemptDiscovery: %v", err)
	}
	if got.reason == "бутыль установки ещё не заведён" {
		t.Fatal("attemptDiscovery не попробовала общий бутыль перед тем, как сдаться")
	}
	if !strings.Contains(got.reason, "путь установщика") {
		t.Fatalf("reason = %q, want an installer-path failure proving the shared bottle was used", got.reason)
	}
}

// TestAttemptDiscoveryFallsBackToLookupWithoutSharedBottle проверяет, что
// поведение без общего бутыля не изменилось: Lookup всё так же промахивается
// по свежему каталогу без метки, и reason остаётся прежним.
func TestAttemptDiscoveryFallsBackToLookupWithoutSharedBottle(t *testing.T) {
	if !crossOverInstalled(t) {
		t.Skip("CrossOver не установлен: attemptDiscovery не подменяет wine.Detect() в тестах")
	}
	fakeBottlesDir(t)
	dest := t.TempDir()

	got, err := attemptDiscovery(context.Background(), discoverySpec{
		Engine: EngineInno, InstallerPath: filepath.Join(dest, "setup.exe"), Destination: dest,
		InfPath: filepath.Join(dest, "discovery.inf"),
		Options: installOptions{SkipExtras: true},
	})
	if err != nil {
		t.Fatalf("attemptDiscovery: %v", err)
	}
	if got.reason != "бутыль установки ещё не заведён" {
		t.Fatalf("reason = %q, want the old lookup-miss reason when there is no shared bottle either", got.reason)
	}
}

// TestAttemptDiscoverySharedBottleNameFromEnv проверяет, что fallback на
// SharedBottle учитывает переопределённое имя общего бутыля.
func TestAttemptDiscoverySharedBottleNameFromEnv(t *testing.T) {
	if !crossOverInstalled(t) {
		t.Skip("CrossOver не установлен: attemptDiscovery не подменяет wine.Detect() в тестах")
	}
	bottlesDir := fakeBottlesDir(t)
	t.Setenv(wine.SharedBottleEnv, "MyPlayStore")
	games := t.TempDir()
	dest := filepath.Join(games, "Demo")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	makeSharedBottleDir(t, bottlesDir, "MyPlayStore", games)

	installer := filepath.Join(t.TempDir(), "setup.exe")
	if err := os.WriteFile(installer, []byte("MZ"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := attemptDiscovery(context.Background(), discoverySpec{
		Engine: EngineInno, InstallerPath: installer, Destination: dest,
		InfPath: filepath.Join(dest, "discovery.inf"),
		Options: installOptions{SkipExtras: true},
	})
	if err != nil {
		t.Fatalf("attemptDiscovery: %v", err)
	}
	if !strings.Contains(got.reason, "путь установщика") {
		t.Fatalf("reason = %q, want an installer-path failure proving the renamed shared bottle was used", got.reason)
	}
}

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
