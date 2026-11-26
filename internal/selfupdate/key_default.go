//go:build !devmock && !(qa && windows)

package selfupdate

import "crypto/ed25519"

func overridePublicKey() (ed25519.PublicKey, bool, error) {
	return nil, false, nil
}
