package account

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"typhon/internal/settings"
	"typhon/internal/storage"
)

const (
	portableCredentialFile    = "credentials.json"
	portableCredentialVersion = 1
	accountCredentialKey      = "account"
	namedCredentialKeyPrefix  = "named/"
)

var portableDir = settings.PortableDir

// One mutex for every store: the account token and the proxy password live in
// the same file, so two stores updating it at once would lose one of the writes.
var portableCredentialMu sync.Mutex

type portableCredentialRecord struct {
	Token    string `json:"token"`
	Username string `json:"username,omitempty"`
}

type portableCredentialDoc struct {
	Entries map[string]portableCredentialRecord `json:"entries"`
}

type portableCredentialStore struct {
	path string
	key  string
}

func openCredentialStore(key string, system func() (CredentialStore, error)) (CredentialStore, error) {
	dir, portable, err := portableDir()
	if err != nil {
		return nil, fmt.Errorf("portable mode: %w", err)
	}
	if !portable {
		return system()
	}
	return portableCredentialStore{path: filepath.Join(dir, portableCredentialFile), key: key}, nil
}

func (s portableCredentialStore) read() (portableCredentialDoc, error) {
	var doc portableCredentialDoc
	if err := storage.Load(s.path, portableCredentialVersion, nil, &doc); err != nil {
		return portableCredentialDoc{}, err
	}
	if doc.Entries == nil {
		doc.Entries = map[string]portableCredentialRecord{}
	}
	return doc, nil
}

func (s portableCredentialStore) Load() (Credential, error) {
	portableCredentialMu.Lock()
	defer portableCredentialMu.Unlock()
	doc, err := s.read()
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Credential{}, ErrNoCredential
		}
		return Credential{}, fmt.Errorf("read credentials: %w", err)
	}
	rec, ok := doc.Entries[s.key]
	if !ok || rec.Token == "" {
		return Credential{}, ErrNoCredential
	}
	return Credential(rec), nil
}

func (s portableCredentialStore) Save(cred Credential) error {
	if cred.Token == "" {
		return errors.New("refusing to store an empty token")
	}
	portableCredentialMu.Lock()
	defer portableCredentialMu.Unlock()
	doc, err := s.read()
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("read credentials: %w", err)
		}
		doc = portableCredentialDoc{Entries: map[string]portableCredentialRecord{}}
	}
	doc.Entries[s.key] = portableCredentialRecord(cred)
	return s.write(doc)
}

func (s portableCredentialStore) Delete() error {
	portableCredentialMu.Lock()
	defer portableCredentialMu.Unlock()
	doc, err := s.read()
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read credentials: %w", err)
	}
	if _, ok := doc.Entries[s.key]; !ok {
		return nil
	}
	delete(doc.Entries, s.key)
	return s.write(doc)
}

func (s portableCredentialStore) write(doc portableCredentialDoc) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create credential dir: %w", err)
	}
	if err := storage.Save(s.path, portableCredentialVersion, doc); err != nil {
		return fmt.Errorf("write credentials: %w", err)
	}
	return nil
}
