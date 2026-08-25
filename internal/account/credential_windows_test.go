package account

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

func testCredentialStore(t *testing.T) CredentialStore {
	t.Helper()
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("random target suffix: %v", err)
	}

	store, err := newWindowsCredentialStore("Typhon Launcher Test " + hex.EncodeToString(buf))
	if err != nil {
		t.Fatalf("new credential store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Delete(); err != nil {
			t.Errorf("cleanup credential: %v", err)
		}
	})
	return store
}

func TestWindowsCredentialStoreRoundTrip(t *testing.T) {
	store := testCredentialStore(t)

	if _, err := store.Load(); !errors.Is(err, ErrNoCredential) {
		t.Fatalf("Load() on an empty store error = %v, want ErrNoCredential", err)
	}

	//nolint:gosec // G101: фикстура теста OS credential storage (инвариант 10), а не встроенный секрет.
	want := Credential{Token: "token-with-Алексей", Username: "playerone"}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got != want {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}

	//nolint:gosec // G101: фикстура теста OS credential storage (инвариант 10), а не встроенный секрет.
	replaced := Credential{Token: "second-token", Username: "playertwo"}
	if err := store.Save(replaced); err != nil {
		t.Fatalf("Save() replacement error = %v", err)
	}
	got, err = store.Load()
	if err != nil {
		t.Fatalf("Load() after replacement error = %v", err)
	}
	if got != replaced {
		t.Fatalf("Load() = %+v, want %+v", got, replaced)
	}

	if err := store.Delete(); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := store.Load(); !errors.Is(err, ErrNoCredential) {
		t.Fatalf("Load() after delete error = %v, want ErrNoCredential", err)
	}
	if err := store.Delete(); err != nil {
		t.Fatalf("Delete() on an empty store error = %v, want nil", err)
	}
}

func TestWindowsCredentialStoreRejectsBadInput(t *testing.T) {
	store := testCredentialStore(t)

	if err := store.Save(Credential{}); err == nil {
		t.Fatal("Save(empty token) error = nil, want an error")
	}
	if err := store.Save(Credential{Token: strings.Repeat("a", credMaxBlobSize+1)}); err == nil {
		t.Fatal("Save(oversized token) error = nil, want an error")
	}
	if _, err := store.Load(); !errors.Is(err, ErrNoCredential) {
		t.Fatalf("Load() error = %v, want ErrNoCredential after rejected writes", err)
	}
}

func TestEnvCredentialStoreOverride(t *testing.T) {
	inner := &fakeStore{cred: Credential{Token: "stored"}, present: true}
	store := envCredentialStore{inner: inner}

	t.Setenv("TYPHON_API_TOKEN", "env-token")
	cred, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cred.Token != "env-token" {
		t.Errorf("token = %q, want env-token", cred.Token)
	}

	t.Setenv("TYPHON_API_TOKEN", "")
	cred, err = store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cred.Token != "stored" {
		t.Errorf("token = %q, want stored", cred.Token)
	}
}
