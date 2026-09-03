//go:build windows || devmock

package selfupdate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var (
	errInstallerPathNotAbsolute = errors.New("selfupdate: installer path is not absolute")
	errInstallerPathNotClean    = errors.New("selfupdate: installer path is not clean")
	errInstallerOutsideCache    = errors.New("selfupdate: installer path is outside the selfupdate cache")
	errInstallerNotRegularFile  = errors.New("selfupdate: installer path is not a regular file")
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
