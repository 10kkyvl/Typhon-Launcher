//go:build devmock && !windows

package selfupdate

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

const releasePublicKeyEnv = "TYPHON_DEVMOCK_RELEASE_PUBKEY"

func overridePublicKey() (ed25519.PublicKey, bool, error) {
	encoded := strings.TrimSpace(os.Getenv(releasePublicKeyEnv))
	if encoded == "" {
		return nil, false, nil
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, false, fmt.Errorf("%w: %s: %w", ErrBadPublicKey, releasePublicKeyEnv, err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, false, fmt.Errorf("%w: %s holds %d bytes, want %d", ErrBadPublicKey, releasePublicKeyEnv, len(raw), ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(raw), true, nil
}
