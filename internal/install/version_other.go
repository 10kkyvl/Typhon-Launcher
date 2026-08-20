//go:build !windows

package install

func ExeVersion(string) (VersionInfo, bool) {
	return VersionInfo{}, false
}
