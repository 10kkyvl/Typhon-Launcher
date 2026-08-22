//go:build !windows

package account

import "errors"

func newSystemCredentialStore() (CredentialStore, error) {
	return nil, errors.New("no OS credential store is available on this platform")
}
