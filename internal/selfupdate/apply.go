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
	errInstallerPathUnsafe      = errors.New("selfupdate: installer path cannot be quoted on a command line")

	errInstallDirEmpty       = errors.New("selfupdate: install dir is empty")
	errInstallDirNotAbsolute = errors.New("selfupdate: install dir is not absolute")
	errInstallDirNotClean    = errors.New("selfupdate: install dir is not clean")
	errInstallDirUnsafe      = errors.New("selfupdate: install dir cannot be passed on a command line")
	errInstallDirNotDir      = errors.New("selfupdate: install dir is not a directory")
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

// validateInstallDir guards the directory handed to the installer as /D=. NSIS
// reads everything after /D= literally to the end of the line, so a trailing
// separator, a quote or a control character silently lands the install
// somewhere else instead of failing.
func validateInstallDir(dir string) error {
	if dir == "" {
		return errInstallDirEmpty
	}
	if !filepath.IsAbs(dir) {
		return errInstallDirNotAbsolute
	}
	if dir != filepath.Clean(dir) {
		return errInstallDirNotClean
	}
	if hasUnsafePathChars(dir) {
		return errInstallDirUnsafe
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errInstallDirNotDir
	}
	return nil
}

func hasUnsafePathChars(s string) bool {
	for _, r := range s {
		if r == '"' || r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}
