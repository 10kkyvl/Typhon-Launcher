//go:build devmock && !windows

package selfupdate

func updatesPortable(portable bool) bool {
	return portable
}
