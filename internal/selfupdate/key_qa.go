//go:build qa && windows

package selfupdate

import (
	"crypto/ed25519"
	"os"
)

func overridePublicKey() (ed25519.PublicKey, bool, error) {
	src, ok, err := parseQASource(os.LookupEnv)
	if err != nil || !ok {
		return nil, false, err
	}
	return src.key, true, nil
}
