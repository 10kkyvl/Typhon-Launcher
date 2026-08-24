//go:build windows

package install

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const uninstallPath = `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`

type uninstallRoot struct {
	hive   registry.Key
	label  string
	access uint32
}

var uninstallRoots = []uninstallRoot{
	{registry.LOCAL_MACHINE, "HKLM", registry.ENUMERATE_SUB_KEYS | registry.QUERY_VALUE | registry.WOW64_64KEY},
	{registry.LOCAL_MACHINE, "HKLM32", registry.ENUMERATE_SUB_KEYS | registry.QUERY_VALUE | registry.WOW64_32KEY},
	{registry.CURRENT_USER, "HKCU", registry.ENUMERATE_SUB_KEYS | registry.QUERY_VALUE},
}

func readUninstallEntries() (map[string]uninstallEntry, error) {
	out := make(map[string]uninstallEntry, 256)
	for _, root := range uninstallRoots {
		if err := readUninstallRoot(root, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func readUninstallRoot(root uninstallRoot, out map[string]uninstallEntry) error {
	key, err := registry.OpenKey(root.hive, uninstallPath, root.access)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf(`open %s\%s: %w`, root.label, uninstallPath, err)
	}
	defer closeKey(key)

	names, err := key.ReadSubKeyNames(-1)
	if err != nil {
		return fmt.Errorf(`enumerate %s\%s: %w`, root.label, uninstallPath, err)
	}
	for _, name := range names {
		entry, ok, err := readUninstallEntry(key, name)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		entry.Key = root.label + `\` + name
		out[entry.Key] = entry
	}
	return nil
}

func readUninstallEntry(parent registry.Key, name string) (uninstallEntry, bool, error) {
	key, err := registry.OpenKey(parent, name, registry.QUERY_VALUE)
	// Запись могли удалить между перечислением и открытием, а часть ключей
	// закрыта ACL от установщиков, ставивших их из-под другого пользователя.
	if errors.Is(err, registry.ErrNotExist) || errors.Is(err, fs.ErrPermission) {
		return uninstallEntry{}, false, nil
	}
	if err != nil {
		return uninstallEntry{}, false, fmt.Errorf("open uninstall key %s: %w", name, err)
	}
	defer closeKey(key)

	entry := uninstallEntry{}
	if entry.DisplayName, err = readString(key, "DisplayName"); err != nil {
		return uninstallEntry{}, false, err
	}
	if entry.Command, err = readString(key, "UninstallString"); err != nil {
		return uninstallEntry{}, false, err
	}
	if entry.QuietCommand, err = readString(key, "QuietUninstallString"); err != nil {
		return uninstallEntry{}, false, err
	}
	if entry.InstallLocation, err = readString(key, "InstallLocation"); err != nil {
		return uninstallEntry{}, false, err
	}
	system, err := readUint(key, "SystemComponent")
	if err != nil {
		return uninstallEntry{}, false, err
	}
	entry.SystemComponent = system == 1
	installer, err := readUint(key, "WindowsInstaller")
	if err != nil {
		return uninstallEntry{}, false, err
	}
	if installer == 1 && strings.HasPrefix(name, "{") && strings.HasSuffix(name, "}") {
		entry.ProductCode = name
	}
	return entry, true, nil
}

func readString(key registry.Key, name string) (string, error) {
	value, _, err := key.GetStringValue(name)
	if errors.Is(err, registry.ErrNotExist) {
		return "", nil
	}
	if errors.Is(err, registry.ErrUnexpectedType) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read value %s: %w", name, err)
	}
	return strings.TrimSpace(value), nil
}

func readUint(key registry.Key, name string) (uint64, error) {
	value, _, err := key.GetIntegerValue(name)
	if errors.Is(err, registry.ErrNotExist) {
		return 0, nil
	}
	if errors.Is(err, registry.ErrUnexpectedType) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read value %s: %w", name, err)
	}
	return value, nil
}

// closeKey: ключ открыт только на чтение, закрытие ничего не фиксирует, но
// потерянный хэндл реестра — это утечка, о которой надо знать из лога.
func closeKey(key registry.Key) {
	if err := key.Close(); err != nil {
		slog.Warn("close registry key", "error", err)
	}
}
