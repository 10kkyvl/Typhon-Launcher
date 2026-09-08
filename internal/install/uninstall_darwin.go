//go:build darwin && !devmock

package install

import (
	"errors"
	"fmt"

	"typhon/internal/wine"
)

// readUninstallEntries собирает записи со всех наших бутылей. Ключ склеен с
// именем бутыля: одна и та же программа, поставленная в двух играх, даёт две
// разные записи, и путать их нельзя. InstallLocation переводится в native —
// вызывающий сравнивает его с каталогом установки, который знает только в
// нативном виде.
func readUninstallEntries() (map[string]uninstallEntry, error) {
	rt, err := wine.Detect()
	if errors.Is(err, wine.ErrNotInstalled) {
		// Без CrossOver бутылей нет, а значит нет и записей. Это не сбой:
		// вызывающий сравнивает снимки «до» и «после», и пустой корректен.
		return map[string]uninstallEntry{}, nil
	}
	if err != nil {
		return nil, err
	}
	bottles, err := wine.NewManager(rt).List()
	if err != nil {
		return nil, err
	}
	return collectUninstallEntries(bottles)
}

// collectUninstallEntries делает основную работу readUninstallEntries по уже
// перечисленным бутылям: вынесено отдельно, чтобы тест мог подставить бутыли
// напрямую, без реального CrossOver и без записи в его каталог бутылей.
//
// Ошибка чтения реестра одной бутыли раньше глоталась через continue, и
// readUninstallEntries всё равно возвращала nil-ошибку с тем, что успело
// накопиться. Вызывающий (setRemoval) сравнивает такой снимок «до» с другим,
// снятым позже, и если чтение чужой бутыли упало транзиентно ровно на одном
// из двух вызовов, разница снимков выглядит как «эта запись появилась при
// нашей установке» — и pickUninstall может отдать деинсталлятор чужой
// программы. Неполный снимок обязан быть виден как ошибка, а не как пустой
// (но «успешный») результат: тут ошибка одной бутыли останавливает всё
// чтение, и вызывающие уже умеют её обрабатывать — runInstaller проваливает
// установку, а setRemoval помечает UninstallUnknown вместо того, чтобы
// довериться неполным данным.
func collectUninstallEntries(bottles []wine.Bottle) (map[string]uninstallEntry, error) {
	out := map[string]uninstallEntry{}
	for _, bottle := range bottles {
		entries, err := bottle.UninstallEntries()
		if err != nil {
			return nil, fmt.Errorf("прочитать реестр бутыля %s: %w", bottle.Name, err)
		}
		for _, entry := range entries {
			location := entry.InstallLocation
			if native, convErr := bottle.ToNative(location); convErr == nil {
				location = native
			}
			out[bottle.Name+"|"+entry.Key] = uninstallEntry{
				Key:             entry.Key,
				DisplayName:     entry.DisplayName,
				Command:         entry.Command,
				QuietCommand:    entry.QuietCommand,
				InstallLocation: location,
				ProductCode:     entry.ProductCode,
				SystemComponent: entry.SystemComponent,
			}
		}
	}
	return out, nil
}
