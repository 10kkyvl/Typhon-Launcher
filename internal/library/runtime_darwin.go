//go:build darwin && !devmock

package library

import (
	"errors"
	"path/filepath"

	"typhon/internal/wine"
)

// prepareRuntime заводит бутыль под игру, если его ещё нет. Нужен потому, что
// бутыль появляется на пути установщика, а игра может попасть в библиотеку и
// мимо него: портативной сборкой, распакованным архивом, переносом или
// сканированием диска. Без этого такая игра ставится, но не запускается.
//
// Буква диска нацеливается на каталог, в котором лежит установка: он же и есть
// папка игр при обычной раскладке, а при необычной — всё равно корректный
// корень, потому что каталог установки заведомо лежит внутри него.
func prepareRuntime(installDir, executable string) error {
	if installDir == "" || !isWindowsExecutable(executable) {
		return nil
	}
	rt, err := wine.Detect()
	if errors.Is(err, wine.ErrNotInstalled) {
		// Без CrossOver готовить нечего: запуск откажет сам, с понятной
		// ошибкой про отсутствующий рантайм.
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
