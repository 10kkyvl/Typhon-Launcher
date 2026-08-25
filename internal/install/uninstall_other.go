//go:build !windows

package install

// Реестра вне Windows нет, а внешние установщики туда и не запускаются:
// runInstaller на этих платформах отказывает раньше.
func readUninstallEntries() (map[string]uninstallEntry, error) {
	return map[string]uninstallEntry{}, nil
}
