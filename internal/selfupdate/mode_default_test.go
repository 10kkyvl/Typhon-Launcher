//go:build !devmock

package selfupdate

import "testing"

// A release build has no mock update source: the variable a developer may
// have left in the shell must not move a portable launcher onto the
// installer path.
func TestReleaseBuildUpdatesInTheModeItRunsIn(t *testing.T) {
	t.Setenv("TYPHON_DEVMOCK_MANIFEST_URL", "http://127.0.0.1:8099")
	for _, portable := range []bool{true, false} {
		if got := updatesPortable(portable); got != portable {
			t.Fatalf("updatesPortable(%v) = %v, want %v", portable, got, portable)
		}
	}
}
