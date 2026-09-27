//go:build darwin && !devmock

package library

import (
	"context"
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
func prepareRuntime(ctx context.Context, req launch) error {
	if req.installDir == "" || !isWindowsExecutable(req.executable) {
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
	// Порядок тот же, что и при запуске (см. wineStarter.bottleFor): если
	// игра поедет в общий бутыль, готовить нечего — он пользовательский,
	// давно прогрет, и заводить рядом с ним пустой собственный бутыль
	// значило бы занять 300 МБ под то, что никогда не запустится.
	if req.shared {
		if _, sharedErr := manager.SharedBottle(req.installDir); sharedErr == nil {
			return nil
		} else if !wine.SharedBottleUnavailable(sharedErr) {
			return sharedErr
		}
	}
	if _, ok := manager.Lookup(req.installDir); ok {
		return nil
	}
	bottle, err := manager.Ensure(req.installDir, filepath.Dir(req.installDir))
	if err != nil {
		return err
	}
	// Прогрев здесь, а не при первом запуске: инициализация свежего префикса
	// занимает минуты, и ждать их в момент нажатия «Играть» нельзя.
	return manager.Boot(ctx, bottle)
}
