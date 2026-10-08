//go:build devmock && !windows

package account

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFileCredentialStoreAbsent(t *testing.T) {
	store := newFileCredentialStore(filepath.Join(t.TempDir(), "credential.json"))
	if _, err := store.Load(); !errors.Is(err, ErrNoCredential) {
		t.Fatalf("Load() err = %v, want ErrNoCredential", err)
	}
}

func TestFileCredentialStoreRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credential.json")
	store := newFileCredentialStore(path)
	want := Credential{Token: "tok-123", Username: "player"}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save() err = %v", err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load() err = %v", err)
	}
	if got != want {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
}

func TestFileCredentialStoreDelete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credential.json")
	store := newFileCredentialStore(path)
	if err := store.Save(Credential{Token: "tok"}); err != nil {
		t.Fatalf("Save() err = %v", err)
	}
	if err := store.Delete(); err != nil {
		t.Fatalf("Delete() err = %v", err)
	}
	if _, err := store.Load(); !errors.Is(err, ErrNoCredential) {
		t.Fatalf("Load() after delete err = %v, want ErrNoCredential", err)
	}
}

func TestFileCredentialStoreDeleteAbsent(t *testing.T) {
	store := newFileCredentialStore(filepath.Join(t.TempDir(), "credential.json"))
	if err := store.Delete(); err != nil {
		t.Fatalf("Delete() of absent file err = %v, want nil", err)
	}
}

func TestFileCredentialStoreCorruptJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credential.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := newFileCredentialStore(path)
	_, err := store.Load()
	if err == nil {
		t.Fatal("Load() err = nil, want error")
	}
	if errors.Is(err, ErrNoCredential) {
		t.Fatalf("Load() err = %v, want anything but ErrNoCredential", err)
	}
}

func TestFileCredentialStoreFileMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credential.json")
	store := newFileCredentialStore(path)
	if err := store.Save(Credential{Token: "tok"}); err != nil {
		t.Fatalf("Save() err = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("file mode = %o, want 0600", perm)
	}
}

func TestFileCredentialStoreSaveUnwritableParent(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root, permission checks are not enforced")
	}
	parent := t.TempDir()
	dir := filepath.Join(parent, "locked")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // G302: каталог, а не файл: без бита x он не читается; тесту нужен read-only каталог, а не 0600
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		//nolint:gosec // G302: каталогу возвращается исходный режим (инвариант 8), иначе t.TempDir() не сможет его удалить
		if err := os.Chmod(dir, 0o700); err != nil {
			t.Errorf("restore dir mode: %v", err)
		}
	})
	store := newFileCredentialStore(filepath.Join(dir, "credential.json"))
	if err := store.Save(Credential{Token: "tok"}); err == nil {
		t.Fatal("Save() err = nil, want error for unwritable parent")
	}
}

func TestNamedCredentialStoresDoNotShareAFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TYPHON_TEST_DATA_DIR", dir)
	first, err := NewNamedCredentialStore("Typhon Launcher Proxy")
	if err != nil {
		t.Fatalf("NewNamedCredentialStore(first) err = %v", err)
	}
	second, err := NewNamedCredentialStore("Another Store")
	if err != nil {
		t.Fatalf("NewNamedCredentialStore(second) err = %v", err)
	}
	if err := first.Save(Credential{Token: "one"}); err != nil {
		t.Fatalf("Save() err = %v", err)
	}
	if _, err := second.Load(); !errors.Is(err, ErrNoCredential) {
		t.Fatalf("second store Load() err = %v, want ErrNoCredential", err)
	}
}

func TestCredentialSlug(t *testing.T) {
	cases := map[string]string{
		"Typhon Launcher Proxy": "typhon-launcher-proxy",
		`a/b\c`:                 "a-b-c",
		"Ключ":                  "----",
	}
	for in, want := range cases {
		if got := credentialSlug(in); got != want {
			t.Errorf("credentialSlug(%q) = %q, want %q", in, got, want)
		}
	}
}
