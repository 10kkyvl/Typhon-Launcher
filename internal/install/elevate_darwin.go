//go:build darwin && !devmock

package install

import (
	"errors"
	"fmt"
)

// На macOS повышения прав нет и не нужно: UAC отсутствует, установщик
// запускается прямо в бутыле. Сюда не заходят — wineRunner никогда не
// возвращает outcome.elevate, — но контракт платформы обязан быть полным.
var errElevationUnsupported = errors.New("повышение прав на macOS не применяется")

func startElevated(runSpec) (workerHandle, error) { return nil, errElevationUnsupported }

func workerStartError(path string, err error) error {
	return fmt.Errorf("запуск воркера установки %s: %w", path, err)
}
