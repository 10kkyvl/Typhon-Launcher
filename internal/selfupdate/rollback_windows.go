//go:build windows

package selfupdate

import (
	"errors"
	"fmt"
	"log/slog"

	"golang.org/x/sys/windows/registry"
)

const displayVersionValue = "DisplayVersion"

// Swapped in tests so a run never rewrites the real uninstall entry. The name
// is INFO_COMPANYNAME + INFO_PRODUCTNAME from build/windows/nsis/wails_tools.nsh.
var uninstallKeyPath = `Software\Microsoft\Windows\CurrentVersion\Uninstall\TyphonTyphon`

func platformRollbackSupported() error { return nil }

// setDisplayVersion points the per-user uninstall entry at ver, the version
// the launcher is about to become, and returns the call that puts the old
// value back. A machine without that entry is not an error: a per-machine
// install keeps its entry under HKLM, which an unelevated launcher cannot
// write, and a portable folder has none.
func setDisplayVersion(ver string) (func() error, error) {
	noop := func() error { return nil }
	key, err := registry.OpenKey(registry.CURRENT_USER, uninstallKeyPath, registry.QUERY_VALUE|registry.SET_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return noop, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", uninstallKeyPath, err)
	}
	defer closeKey(key)

	old, _, err := key.GetStringValue(displayVersionValue)
	hadValue := err == nil
	if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return nil, fmt.Errorf("read %s: %w", displayVersionValue, err)
	}
	if err := key.SetStringValue(displayVersionValue, ver); err != nil {
		return nil, fmt.Errorf("write %s: %w", displayVersionValue, err)
	}
	return func() error { return restoreDisplayVersion(old, hadValue) }, nil
}

func restoreDisplayVersion(old string, hadValue bool) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, uninstallKeyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open %s: %w", uninstallKeyPath, err)
	}
	defer closeKey(key)
	if hadValue {
		return key.SetStringValue(displayVersionValue, old)
	}
	return key.DeleteValue(displayVersionValue)
}

func closeKey(key registry.Key) {
	if err := key.Close(); err != nil {
		slog.Warn("close registry key", "key", uninstallKeyPath, "error", err)
	}
}
