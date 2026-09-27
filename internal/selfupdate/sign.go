package selfupdate

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// SignManifest marshals m exactly once, signs those raw bytes and embeds them
// verbatim: re-encoding the manifest after signing would change its bytes and
// invalidate the signature.
func SignManifest(m Manifest, priv ed25519.PrivateKey) ([]byte, error) {
	if len(priv) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("%w: got %d bytes", ErrBadPrivateKey, len(priv))
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("marshal manifest: %w", err)
	}
	sig := ed25519.Sign(priv, raw)
	sm := SignedManifest{
		KeyID:     KeyID,
		Signature: base64.StdEncoding.EncodeToString(sig),
		Manifest:  json.RawMessage(raw),
	}
	out, err := json.Marshal(sm)
	if err != nil {
		return nil, fmt.Errorf("marshal signed manifest: %w", err)
	}
	if len(out) > MaxManifestSize {
		return nil, fmt.Errorf("%w: %d > %d bytes", ErrManifestTooLarge, len(out), MaxManifestSize)
	}

	pub, ok := priv.Public().(ed25519.PublicKey)
	if !ok {
		return nil, ErrBadPrivateKey
	}
	if _, err := VerifyManifest(out, pub); err != nil {
		return nil, fmt.Errorf("self-verification failed: %w", err)
	}
	return out, nil
}
