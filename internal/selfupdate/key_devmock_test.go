//go:build devmock && !windows

package selfupdate

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"testing"
)

func embeddedKey(t *testing.T) ed25519.PublicKey {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(publicKeyBase64)
	if err != nil {
		t.Fatalf("decode embedded key: %v", err)
	}
	return ed25519.PublicKey(raw)
}

func TestPublicKeyWithoutOverrideUsesEmbeddedKey(t *testing.T) {
	t.Setenv(releasePublicKeyEnv, "")
	key, err := PublicKey()
	if err != nil {
		t.Fatalf("PublicKey: %v", err)
	}
	if !bytes.Equal(key, embeddedKey(t)) {
		t.Fatal("PublicKey() differs from the embedded key with the env unset")
	}
}

func TestPublicKeyBlankOverrideUsesEmbeddedKey(t *testing.T) {
	t.Setenv(releasePublicKeyEnv, "  \n")
	key, err := PublicKey()
	if err != nil {
		t.Fatalf("PublicKey: %v", err)
	}
	if !bytes.Equal(key, embeddedKey(t)) {
		t.Fatal("PublicKey() differs from the embedded key with a blank env")
	}
}

func TestPublicKeyOverride(t *testing.T) {
	_, pub := testKeyPair(t)
	t.Setenv(releasePublicKeyEnv, base64.StdEncoding.EncodeToString(pub))
	key, err := PublicKey()
	if err != nil {
		t.Fatalf("PublicKey: %v", err)
	}
	if !bytes.Equal(key, pub) {
		t.Fatal("PublicKey() did not return the override")
	}
}

func TestPublicKeyOverrideInvalid(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "not base64", value: "not*base64"},
		{name: "wrong length", value: base64.StdEncoding.EncodeToString([]byte("short"))},
		{name: "too long", value: base64.StdEncoding.EncodeToString(make([]byte, ed25519.PublicKeySize+1))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(releasePublicKeyEnv, tt.value)
			if _, err := PublicKey(); !errors.Is(err, ErrBadPublicKey) {
				t.Fatalf("PublicKey() = %v, want ErrBadPublicKey", err)
			}
		})
	}
}
