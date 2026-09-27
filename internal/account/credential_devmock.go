//go:build devmock && !windows

package account

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"typhon/internal/settings"
	"typhon/internal/storage"
)

func newSystemCredentialStore() (CredentialStore, error) {
	dir, err := settings.ConfigDir()
	if err != nil {
		return nil, fmt.Errorf("resolve config dir: %w", err)
	}
	return newFileCredentialStore(filepath.Join(dir, "devmock-credential.json")), nil
}

type fileCredentialStore struct {
	path string
}

func newFileCredentialStore(path string) CredentialStore {
	return fileCredentialStore{path: path}
}

func (s fileCredentialStore) Load() (Credential, error) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Credential{}, ErrNoCredential
		}
		return Credential{}, fmt.Errorf("read credential %s: %w", s.path, err)
	}
	var cred Credential
	if err := json.Unmarshal(raw, &cred); err != nil {
		return Credential{}, fmt.Errorf("parse credential %s: %w", s.path, err)
	}
	if cred.Token == "" {
		return Credential{}, ErrNoCredential
	}
	return cred, nil
}

func (s fileCredentialStore) Save(cred Credential) error {
	if cred.Token == "" {
		return errors.New("refusing to store an empty token")
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create credential dir: %w", err)
	}
	encoded, err := json.Marshal(cred)
	if err != nil {
		return fmt.Errorf("encode credential: %w", err)
	}
	if err := storage.WriteAtomic(s.path, encoded); err != nil {
		return fmt.Errorf("write credential %s: %w", s.path, err)
	}
	return nil
}

func (s fileCredentialStore) Delete() error {
	if err := os.Remove(s.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("delete credential %s: %w", s.path, err)
	}
	return nil
}
