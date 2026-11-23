//go:build devmock && !windows

package selfupdate

import (
	"os"
	"strings"
)

// A devmock test app runs in portable mode for its own data folder and
// single-instance id, while tools/devrelease publishes a plain launcher
// binary that Apply renames over the running one. With that source in use
// the update takes the plain path; the worker and the relaunch inherit the
// environment, so the portable folder still holds the data afterwards.
func updatesPortable(portable bool) bool {
	if strings.TrimSpace(os.Getenv(manifestURLEnv)) != "" {
		return false
	}
	return portable
}
