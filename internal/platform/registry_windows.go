package platform

import (
	"log/slog"

	"golang.org/x/sys/windows/registry"
)

func openRegistryKey(path string) (registry.Key, error) {
	return registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE)
}

// closeKey: ключ открыт только на чтение, закрытие ничего не фиксирует, но
// потерянный хэндл реестра — это утечка, о которой надо знать из лога
// (тот же приём, что internal/install/uninstall_windows.go:closeKey).
func closeKey(key registry.Key) {
	if err := key.Close(); err != nil {
		slog.Warn("close registry key", "error", err)
	}
}
