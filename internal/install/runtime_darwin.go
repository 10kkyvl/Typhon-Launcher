//go:build darwin && !devmock

package install

import (
	"errors"
	"path/filepath"
	"strings"

	"typhon/internal/wine"
)

// prepareRuntime заводит бутыль под только что установленную игру. Путь
// установщика делает это сам, а портативная сборка и распакованный архив —
// нет: там установка это перенос файлов, и до wine дело не доходит. Заводим
// здесь, а не при первом запуске, чтобы четверть минуты на создание бутыля
// ушла во время установки, где её видно, а не в момент нажатия «Играть».
func prepareRuntime(installDir, executable string) error {
	if installDir == "" || !windowsExecutable(executable) {
		return nil
	}
	rt, err := wine.Detect()
	if errors.Is(err, wine.ErrNotInstalled) {
		// Без CrossOver бутыль не завести, но и установка от этого не
		// перестаёт быть установкой: игра поставлена, запуск откажет сам.
		return nil
	}
	if err != nil {
		return err
	}
	manager := wine.NewManager(rt)
	if _, ok := manager.Lookup(installDir); ok {
		return nil
	}
	_, err = manager.Ensure(installDir, filepath.Dir(installDir))
	return err
}

func windowsExecutable(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".exe", ".bat", ".cmd", ".com", ".msi":
		return true
	default:
		return false
	}
}
