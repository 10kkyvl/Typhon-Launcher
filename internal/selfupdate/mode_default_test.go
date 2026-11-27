//go:build !devmock && !(qa && windows)

package selfupdate

import (
	"encoding/base64"
	"testing"
)

// A release build has no mock update source: a variable a developer may have
// left in the shell, devmock's or qa's, must not move a portable launcher onto
// the installer path, nor an installed one onto the portable path or a refusal.
func TestReleaseBuildUpdatesInTheModeItRunsIn(t *testing.T) {
	_, pub := testKeyPair(t)
	t.Setenv("TYPHON_DEVMOCK_MANIFEST_URL", "http://127.0.0.1:8099")
	t.Setenv("TYPHON_QA_MANIFEST_URL", "http://127.0.0.1:8099")
	t.Setenv("TYPHON_QA_RELEASE_PUBKEY", base64.StdEncoding.EncodeToString(pub))
	for _, portable := range []bool{true, false} {
		if got, err := updatesPortable(portable); err != nil || got != portable {
			t.Fatalf("updatesPortable(%v) = %v, %v; want %v", portable, got, err, portable)
		}
	}
}
