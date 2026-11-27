//go:build qa && windows

package selfupdate

import "os"

// A qa launcher fed by tools/devrelease -windows updates as a portable copy
// and refuses the source outside portable mode (qa_source.go).
func updatesPortable(portable bool) (bool, error) {
	return qaUpdatesPortable(portable, os.LookupEnv)
}
