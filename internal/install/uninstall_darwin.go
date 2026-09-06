//go:build darwin && !devmock

package install

import (
	"log/slog"

	"typhon/internal/wine"
)

// readUninstallEntries собирает записи со всех наших бутылей. Ключ склеен с
// именем бутыля: одна и та же программа, поставленная в двух играх, даёт две
// разные записи, и путать их нельзя. InstallLocation переводится в native —
// вызывающий сравнивает его с каталогом установки, который знает только в
// нативном виде.
func readUninstallEntries() (map[string]uninstallEntry, error) {
	out := map[string]uninstallEntry{}
	rt, err := wine.Detect()
	if err != nil {
		return out, nil
	}
	bottles, err := wine.NewManager(rt).List()
	if err != nil {
		return out, err
	}
	for _, bottle := range bottles {
		entries, err := bottle.UninstallEntries()
		if err != nil {
			slog.Warn("read bottle registry", "bottle", bottle.Name, "error", err)
			continue
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
