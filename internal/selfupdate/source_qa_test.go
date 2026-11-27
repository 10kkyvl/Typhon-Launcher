//go:build qa && windows

package selfupdate

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"typhon/internal/settings"
)

func TestQANewServiceRefusesARemoteOrHalfSource(t *testing.T) {
	_, pub := testKeyPair(t)
	encoded := base64.StdEncoding.EncodeToString(pub)
	for name, env := range map[string][2]string{
		"remote url": {"https://example.com", encoded},
		"url only":   {"http://127.0.0.1:8099", ""},
		"key only":   {"", encoded},
		"bad key":    {"http://127.0.0.1:8099", "not*base64"},
	} {
		t.Run(name, func(t *testing.T) {
			testConfigDir(t)
			t.Setenv(qaManifestURLEnv, env[0])
			t.Setenv(qaReleasePubKeyEnv, env[1])
			if _, err := NewService(); !errors.Is(err, errQAUpdateSource) {
				t.Fatalf("NewService() = %v, want errQAUpdateSource", err)
			}
		})
	}
}

// The test binary runs without portable.txt beside it, so with the source
// set the service must refuse to start on the installed launcher's data.
func TestQANewServiceRefusesTheSourceOutsidePortableMode(t *testing.T) {
	if settings.IsPortable() {
		t.Skip("this test process runs in portable mode")
	}
	_, pub := testKeyPair(t)
	testConfigDir(t)
	t.Setenv(qaManifestURLEnv, "http://127.0.0.1:8099")
	t.Setenv(qaReleasePubKeyEnv, base64.StdEncoding.EncodeToString(pub))
	_, err := NewService()
	if !errors.Is(err, errQAUpdateSource) || !strings.Contains(err.Error(), settings.PortableEnv) {
		t.Fatalf("NewService() = %v, want errQAUpdateSource naming %s", err, settings.PortableEnv)
	}
}

func TestQASourceReplacesTheServerAndTheKey(t *testing.T) {
	_, pub := testKeyPair(t)
	t.Setenv(qaManifestURLEnv, "http://127.0.0.1:8099")
	t.Setenv(qaReleasePubKeyEnv, base64.StdEncoding.EncodeToString(pub))
	base, err := manifestBaseURL()
	if err != nil || base != "http://127.0.0.1:8099" {
		t.Fatalf("manifestBaseURL() = %q, %v", base, err)
	}
	key, err := PublicKey()
	if err != nil || !bytes.Equal(key, pub) {
		t.Fatalf("PublicKey() = %x, %v; want the qa key", key, err)
	}
}
