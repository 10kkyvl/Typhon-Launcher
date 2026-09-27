package install

import (
	"path/filepath"
	"strings"

	"typhon/internal/download"
)

func autoInstallFor(d download.Download, global bool) bool {
	if d.Origin.AutoInstall != nil {
		return *d.Origin.AutoInstall
	}
	return global
}

// InstallerLikely answers the only question available before the files exist:
// whether the download carries something that will be run rather than
// unpacked. The real decision needs DetectEngine on the downloaded file, so
// this stays a name check and is used only to decide whether asking for
// administrator rights up front makes sense at all.
func InstallerLikely(paths []string) bool {
	for _, path := range paths {
		switch strings.ToLower(filepath.Ext(path)) {
		case ".exe", ".msi":
			return true
		}
	}
	return false
}
