//go:build qa && windows

package selfupdate

import (
	"bytes"
	"encoding/base64"
	"errors"
	"testing"
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
