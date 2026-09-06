//go:build windows || (darwin && !devmock)

package install

import (
	"fmt"
	"os"
)

// discovery сводит runSpec к discoverySpec (worker.go), чтобы разведкой
// компонентов Inno пользовались все пути запуска сразу: неэлевированный на
// Windows, повышенный воркер и запуск в бутыле на macOS (инвариант 28 — один
// источник правды на понятие).
func (s runSpec) discovery() discoverySpec {
	return discoverySpec{
		Engine: s.Engine, InstallerPath: s.InstallerPath, Destination: s.Destination,
		WorkingDir: s.Dir, InfPath: s.InfPath, Options: s.Options,
	}
}

// readDiscoveredComponents читает файл, который установщик написал по
// /SAVEINF. Здесь только чтение и разбор, поэтому код общий для всех
// платформ, где разведка вообще возможна.
func readDiscoveredComponents(infPath string, opts installOptions) ([]string, string, error) {
	data, err := os.ReadFile(infPath)
	if err != nil {
		return nil, fmt.Sprintf("чтение файла разведки: %v", err), nil
	}
	list, ok := infComponents(data)
	if !ok {
		return nil, "секция Components не найдена в файле разведки", nil
	}
	filtered, changed := filterComponents(list, opts)
	if !changed {
		return nil, "", nil
	}
	return filtered, "", nil
}
