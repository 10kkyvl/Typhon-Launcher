//go:build windows

package selfupdate

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

const (
	installDirValue = "InstallDir"
	uninstallerName = "uninstall.exe"
)

// Swapped in tests so a run never rewrites the real installation record.
var installDirKey = `Software\Typhon`

func recordInstallDir() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("selfupdate: resolve executable: %w", err)
	}
	return recordInstallDirFor(filepath.Dir(exe))
}

// recordInstallDirFor mirrors InstallDirRegKey in
// build/windows/nsis/project.nsi. The installer reads this to land on the
// installation that already exists; installations made before the key existed
// only learn their own directory from the launcher running out of them.
func recordInstallDirFor(dir string) error {
	if err := validateInstallDir(dir); err != nil {
		return err
	}
	// Only a directory the installer built may steer a later installer run.
	// Without this a development build, or a copy someone unpacked elsewhere,
	// would point the next install at itself.
	if _, err := os.Stat(filepath.Join(dir, uninstallerName)); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("selfupdate: stat uninstaller: %w", err)
	}

	key, _, err := registry.CreateKey(registry.CURRENT_USER, installDirKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("selfupdate: open %s: %w", installDirKey, err)
	}
	defer func() {
		if cerr := key.Close(); cerr != nil {
			slog.Warn("close install dir key", "key", installDirKey, "error", cerr)
		}
	}()

	if current, _, rerr := key.GetStringValue(installDirValue); rerr == nil && current == dir {
		return nil
	}
	if err := key.SetStringValue(installDirValue, dir); err != nil {
		return fmt.Errorf("selfupdate: write %s: %w", installDirValue, err)
	}
	return nil
}
