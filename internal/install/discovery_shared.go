//go:build windows || (darwin && !devmock)

package install

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
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

// discoveryFileLimit — потолок размера /SAVEINF (инвариант VI.34: разбор
// любого внешнего файла ограничен по размеру). Файл пишет установщик
// стороннего разработчика, и разумная секция Components даже с тысячами
// компонентов укладывается в считанные килобайты — многократный запас на
// случай необычно длинного списка, а не лимит "впритык".
const discoveryFileLimit = 4 << 20

var errDiscoveryFileTooLarge = errors.New("install: файл разведки превышает лимит размера")

// readLimited читает файл целиком, но отказывается вернуть данные, если их
// оказалось больше limit: io.LimitReader(limit+1) ограничивает память вне
// зависимости от заявленного os.Stat-размера (который мог устареть между
// проверкой и чтением), а сравнение полученной длины с limit отличает
// «ровно limit байт» от «больше limit» — тихое усечение до потолка тут
// невозможно в принципе, лишний байт всегда либо есть, либо его нет.
func readLimited(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			slog.Warn("close discovery file", "path", path, "error", cerr)
		}
	}()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%w: %s: >%d байт", errDiscoveryFileTooLarge, path, limit)
	}
	return data, nil
}

// readDiscoveredComponents читает файл, который установщик написал по
// /SAVEINF. Здесь только чтение и разбор, поэтому код общий для всех
// платформ, где разведка вообще возможна.
func readDiscoveredComponents(infPath string, opts installOptions) ([]string, string, error) {
	data, err := readLimited(infPath, discoveryFileLimit)
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
