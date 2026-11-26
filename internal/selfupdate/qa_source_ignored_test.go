//go:build !(qa && windows)

package selfupdate

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"testing"

	"typhon/internal/account"
)

// Only the qa Windows build reads the qa update source: a release or devmock
// build keeps its own server and its embedded key whatever the shell holds,
// even a value the qa build would refuse.
func TestBuildsWithoutQAIgnoreTheQAUpdateSource(t *testing.T) {
	_, pub := testKeyPair(t)
	t.Setenv("TYPHON_DEVMOCK_MANIFEST_URL", "")
	t.Setenv("TYPHON_DEVMOCK_RELEASE_PUBKEY", "")
	raw, err := base64.StdEncoding.DecodeString(publicKeyBase64)
	if err != nil {
		t.Fatalf("decode embedded key: %v", err)
	}
	embedded := ed25519.PublicKey(raw)

	for name, env := range map[string][2]string{
		"valid pair":   {"http://127.0.0.1:8099", base64.StdEncoding.EncodeToString(pub)},
		"remote url":   {"https://example.com", "not*base64"},
		"url only":     {"http://127.0.0.1:8099", ""},
		"bad key only": {"", "not*base64"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("TYPHON_QA_MANIFEST_URL", env[0])
			t.Setenv("TYPHON_QA_RELEASE_PUBKEY", env[1])
			base, err := manifestBaseURL()
			if err != nil || base != account.BaseURL() {
				t.Fatalf("manifestBaseURL() = %q, %v; want %q", base, err, account.BaseURL())
			}
			key, err := PublicKey()
			if err != nil || !bytes.Equal(key, embedded) {
				t.Fatalf("PublicKey() = %x, %v; want the embedded key", key, err)
			}
		})
	}
}
