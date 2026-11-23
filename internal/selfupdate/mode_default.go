//go:build !devmock

package selfupdate

// updatesPortable decides whether this launcher updates as a portable copy
// (a zip with typhon.exe swapped beside it) or as an installed one, given
// whether it runs in portable mode.
func updatesPortable(portable bool) bool {
	return portable
}
