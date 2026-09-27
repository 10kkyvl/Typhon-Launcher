//go:build darwin && !devmock

package install

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/wine"
)

// Живая проверка установки в настоящем бутыле CrossOver. Включается двумя
// переменными: TYPHON_WINE_LIVE=1 и TYPHON_WINE_LIVE_INSTALLER с путём до
// windows-установщика. Установщик задаётся снаружи намеренно: скачивать
// что-либо из теста нельзя, а подходящий свободный есть у любого
// разработчика (проверялось на innosetup-6.7.3.exe).
func TestLiveInstallThroughBottle(t *testing.T) {
	if os.Getenv("TYPHON_WINE_LIVE") == "" {
		t.Skip("живые проверки выключены: установите TYPHON_WINE_LIVE=1")
	}
	source := os.Getenv("TYPHON_WINE_LIVE_INSTALLER")
	if source == "" {
		t.Skip("не задан TYPHON_WINE_LIVE_INSTALLER")
	}
	if _, err := wine.Detect(); err != nil {
		t.Skipf("CrossOver не найден: %v", err)
	}

	games := t.TempDir()
	dest := filepath.Join(games, "LiveInstall")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// Установщик обязан лежать внутри папки игр: снаружи бутыль его не видит.
	//nolint:gosec // G703: путь задаёт сам разработчик переменной окружения ради этой проверки
	payload, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", source, err)
	}
	installer := filepath.Join(dest, "setup.exe")
	//nolint:gosec // G703: путь целиком из t.TempDir(), внешнего ввода в нём нет
	if err := os.WriteFile(installer, payload, 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	t.Cleanup(func() {
		rt, detectErr := wine.Detect()
		if detectErr != nil {
			return
		}
		if err := wine.NewManager(rt).Remove(dest); err != nil {
			t.Errorf("cleanup remove bottle: %v", err)
		}
	})

	logPath := filepath.Join(t.TempDir(), "install.log")
	runner := newRunner(func() string { return games })
	code, err := runner.run(context.Background(), runSpec{
		Path:          installer,
		InstallerPath: installer,
		Destination:   dest,
		Dir:           dest,
		LogPath:       logPath,
		Args: []string{
			"/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART",
			`/DIR=` + `T:\LiveInstall\app`,
		},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if code != 0 {
		t.Fatalf("код возврата установщика = %d, want 0", code)
	}

	entries, err := os.ReadDir(filepath.Join(dest, "app"))
	if err != nil {
		t.Fatalf("установщик ничего не положил в папку установки: %v", err)
	}
	t.Logf("установлено файлов: %d", len(entries))
	if len(entries) == 0 {
		t.Fatal("папка установки пуста")
	}
}
