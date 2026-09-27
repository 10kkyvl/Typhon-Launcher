package account

import (
	"errors"
	"os"
)

var ErrNoCredential = errors.New("no stored credential")

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

//nolint:staticcheck // SA4023: на платформах без системного хранилища (credential_other.go) конструктор всегда возвращает ошибку, поэтому проверка err «всегда истинна»; под windows и devmock та же ветка берётся по-настоящему, и убирать её нельзя
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
