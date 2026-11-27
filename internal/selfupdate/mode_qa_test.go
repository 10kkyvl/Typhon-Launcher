//go:build qa && windows

package selfupdate

import (
	"encoding/base64"
	"errors"
	"testing"
)

func TestQALauncherTakesItsUpdateModeFromTheEnvironment(t *testing.T) {
	_, pub := testKeyPair(t)
	t.Setenv(qaManifestURLEnv, "")
	t.Setenv(qaReleasePubKeyEnv, "")
	for _, portable := range []bool{true, false} {
		if got, err := updatesPortable(portable); err != nil || got != portable {
			t.Fatalf("updatesPortable(%v) without the test source = %v, %v; want %v", portable, got, err, portable)
		}
	}

	t.Setenv(qaManifestURLEnv, "http://127.0.0.1:8099")
	t.Setenv(qaReleasePubKeyEnv, base64.StdEncoding.EncodeToString(pub))
	if got, err := updatesPortable(true); err != nil || !got {
		t.Fatalf("updatesPortable(true) with the test source = %v, %v; want true", got, err)
	}
	if _, err := updatesPortable(false); !errors.Is(err, errQAUpdateSource) {
		t.Fatalf("updatesPortable(false) with the test source = %v; want errQAUpdateSource", err)
	}
}
