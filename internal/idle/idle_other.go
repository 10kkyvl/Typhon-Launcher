//go:build !windows && !darwin

package idle

import "time"

// Since has no implementation on this platform: it reports that the idle time
// is unknown rather than an invented zero.
func Since() (time.Duration, bool) {
	return 0, false
}
