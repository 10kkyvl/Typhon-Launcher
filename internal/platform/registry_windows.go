package platform

import (
	"golang.org/x/sys/windows/registry"
)

func openRegistryKey(path string) (registry.Key, error) {
	return registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE)
}
