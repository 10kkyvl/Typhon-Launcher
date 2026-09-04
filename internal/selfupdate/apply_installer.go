//go:build windows || devmock

package selfupdate

import (
	"os"
	"path/filepath"
	"strings"

	"typhon/internal/uierr"
)

var (
	errInstallerPathNotAbsolute = uierr.New("selfupdate.installer_path_not_absolute", "selfupdate: installer path is not absolute")
	errInstallerPathNotClean    = uierr.New("selfupdate.installer_path_not_clean", "selfupdate: installer path is not clean")
	errInstallerOutsideCache    = uierr.New("selfupdate.installer_outside_cache", "selfupdate: installer path is outside the selfupdate cache")
	errInstallerNotRegularFile  = uierr.New("selfupdate.installer_not_regular_file", "selfupdate: installer path is not a regular file")
)

func validateInstallerPath(configDir, installerPath string) error {
	if !filepath.IsAbs(installerPath) {
		return errInstallerPathNotAbsolute
	}
	if installerPath != filepath.Clean(installerPath) {
		return errInstallerPathNotClean
	}
	cacheDir, err := CacheDir(configDir)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(cacheDir, installerPath)
	if err != nil {
		return errInstallerOutsideCache
	}
	if rel == "." || strings.HasPrefix(rel, "..") {
		return errInstallerOutsideCache
	}
	info, err := os.Lstat(installerPath)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errInstallerNotRegularFile
	}
	return nil
}
