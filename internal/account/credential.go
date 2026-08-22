package account

import (
	"errors"
	"os"
)

var ErrNoCredential = errors.New("no stored credential")

const storeTarget = "Typhon Launcher"

type Credential struct {
	Token    string
	Username string
}

type CredentialStore interface {
	Load() (Credential, error)
	Save(cred Credential) error
	Delete() error
}

type envCredentialStore struct {
	inner CredentialStore
}

func NewCredentialStore() (CredentialStore, error) {
	inner, err := newSystemCredentialStore()
	if err != nil {
		return nil, err
	}
	return envCredentialStore{inner: inner}, nil
}

func (s envCredentialStore) Load() (Credential, error) {
	if token := os.Getenv("TYPHON_API_TOKEN"); token != "" {
		return Credential{Token: token}, nil
	}
	return s.inner.Load()
}

func (s envCredentialStore) Save(cred Credential) error {
	return s.inner.Save(cred)
}

func (s envCredentialStore) Delete() error {
	return s.inner.Delete()
}
