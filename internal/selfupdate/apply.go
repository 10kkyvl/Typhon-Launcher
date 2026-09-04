package selfupdate

import (
	"os"
	"path/filepath"

	"typhon/internal/uierr"
)

var (
	errInstallDirEmpty       = uierr.New("selfupdate.install_dir_empty", "selfupdate: install dir is empty")
	errInstallDirNotAbsolute = uierr.New("selfupdate.install_dir_not_absolute", "selfupdate: install dir is not absolute")
	errInstallDirNotClean    = uierr.New("selfupdate.install_dir_not_clean", "selfupdate: install dir is not clean")
	errInstallDirUnsafe      = uierr.New("selfupdate.install_dir_unsafe", "selfupdate: install dir cannot be passed on a command line")
	errInstallDirNotDir      = uierr.New("selfupdate.install_dir_not_dir", "selfupdate: install dir is not a directory")
)

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
