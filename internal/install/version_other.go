//go:build !windows && (!darwin || devmock)

package install

func ExeVersion(string) (VersionInfo, bool) {
	return VersionInfo{}, false
}
