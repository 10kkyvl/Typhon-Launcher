//go:build !windows && !devmock && !darwin

package account

func newSystemCredentialStore() (CredentialStore, error) {
	return nil, ErrNoCredentialStore
}

func newNamedCredentialStore(string) (CredentialStore, error) {
	return nil, ErrNoCredentialStore
}
