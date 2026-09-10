//go:build darwin

package platform

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"typhon/internal/exepicker/asset"
	"typhon/internal/uierr"
	"typhon/internal/wine"
)

func SelectGameExecutable(title, installDir, current string) (string, error) {
	//nolint:gosec // G703: local user-selected game/helper path; no network path input or privileged filesystem access.
	info, err := os.Stat(installDir)
	if err != nil {
		return "", uierr.Wrap("library.no_install_dir", fmt.Errorf("каталог игры недоступен: %w", err))
	}
	if !info.IsDir() {
		return "", uierr.New("library.no_install_dir", fmt.Sprintf("каталог игры не является папкой: %s", installDir))
	}
	rt, err := wine.Detect()
	if err != nil {
		return "", uierr.Wrap("library.crossover_unavailable", fmt.Errorf("CrossOver недоступен: %w", err))
	}
	manager := wine.NewManager(rt)
	bottle, err := manager.SharedBottle(installDir)
	if err != nil {
		if !wine.SharedBottleUnavailable(err) {
			return "", uierr.Wrap("library.bottle_unavailable", err)
		}
		var ok bool
		bottle, ok = manager.Lookup(installDir)
		if !ok {
			return "", uierr.New("library.bottle_unavailable", "бутыль CrossOver для игры не найден")
		}
	}
	initial := installDir
	if current != "" {
		//nolint:gosec // G703: local user-selected game/helper path; no network path input or privileged filesystem access.
		if currentInfo, statErr := os.Stat(current); statErr == nil && !currentInfo.IsDir() {
			initial = current
		}
	}
	winInitial, err := bottle.ToWindows(initial)
	if err != nil {
		return "", uierr.Wrap("library.bottle_unavailable", fmt.Errorf("начальная папка CrossOver: %w", err))
	}
	helper, cleanup, err := asset.Extract(filepath.Join(bottle.Path, "drive_c"))
	if err != nil {
		return "", uierr.Wrap("library.executable_picker_failed", err)
	}
	defer cleanup()
	winHelper, err := bottle.ToWindows(helper)
	if err != nil {
		return "", uierr.Wrap("library.executable_picker_failed", err)
	}
	result := filepath.Join(filepath.Dir(helper), "selection.txt")
	winResult, err := bottle.ToWindows(result)
	if err != nil {
		return "", uierr.Wrap("library.executable_picker_failed", err)
	}
	//nolint:forbidigo // standalone UI/helper operation owns its lifetime; cancellation is handled by its dialog or cancel marker.
	code, err := manager.Run(context.Background(), bottle, wine.Cmd{Path: winHelper, Args: []string{winResult, winInitial, title}})
	if err != nil {
		return "", uierr.Wrap("library.executable_picker_failed", err)
	}
	if code != 0 {
		return "", uierr.New("library.executable_picker_failed", fmt.Sprintf("диалог CrossOver завершился с кодом %d", code))
	}
	data, err := os.ReadFile(result)
	if os.IsNotExist(err) {
		return "", nil // user cancelled
	}
	if err != nil {
		return "", uierr.Wrap("library.executable_picker_failed", err)
	}
	winSelected := strings.TrimSpace(string(data))
	selected, err := bottle.ToNative(winSelected)
	if err != nil {
		return "", uierr.Wrap("library.executable_outside_install", fmt.Errorf("выбранный путь CrossOver: %w", err))
	}
	//nolint:gosec // G703: local user-selected game/helper path; no network path input or privileged filesystem access.
	selectedInfo, err := os.Stat(selected)
	if err != nil || selectedInfo.IsDir() || !strings.EqualFold(filepath.Ext(selected), ".exe") {
		return "", uierr.New("library.no_executable", fmt.Sprintf("выбранный исполняемый файл недоступен: %s", selected))
	}
	return selected, nil
}
