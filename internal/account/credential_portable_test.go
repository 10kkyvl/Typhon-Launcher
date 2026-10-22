package account

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func usePortableDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	prev := portableDir
	portableDir = func() (string, bool, error) { return dir, true, nil }
	t.Cleanup(func() { portableDir = prev })
	return dir
}

func portableStores(t *testing.T) (account, proxy CredentialStore) {
	t.Helper()
	t.Setenv("TYPHON_API_TOKEN", "")
	account, err := NewCredentialStore()
	if err != nil {
		t.Fatalf("NewCredentialStore: %v", err)
	}
	proxy, err = NewNamedCredentialStore("Typhon Launcher Proxy")
	if err != nil {
		t.Fatalf("NewNamedCredentialStore: %v", err)
	}
	return account, proxy
}

func TestPortableStoreRoundTripKeepsNamesApart(t *testing.T) {
	dir := usePortableDir(t)
	account, proxy := portableStores(t)

	if err := account.Save(Credential{Token: "account-token", Username: "egor"}); err != nil {
		t.Fatal(err)
	}
	if err := proxy.Save(Credential{Token: "proxy-password", Username: "proxy-user"}); err != nil {
		t.Fatal(err)
	}

	gotAccount, err := account.Load()
	if err != nil || gotAccount != (Credential{Token: "account-token", Username: "egor"}) {
		t.Fatalf("account Load() = %+v, %v", gotAccount, err)
	}
	gotProxy, err := proxy.Load()
	if err != nil || gotProxy != (Credential{Token: "proxy-password", Username: "proxy-user"}) {
		t.Fatalf("proxy Load() = %+v, %v", gotProxy, err)
	}
	if _, err := os.Stat(filepath.Join(dir, portableCredentialFile)); err != nil {
		t.Fatalf("credentials file: %v", err)
	}

	if err := account.Delete(); err != nil {
		t.Fatal(err)
	}
	if _, err := account.Load(); !errors.Is(err, ErrNoCredential) {
		t.Fatalf("account Load() after Delete error = %v, want ErrNoCredential", err)
	}
	if got, err := proxy.Load(); err != nil || got.Token != "proxy-password" {
		t.Fatalf("proxy Load() after account Delete = %+v, %v", got, err)
	}
}

func TestPortableStoreOverwriteReplacesTheEntry(t *testing.T) {
	usePortableDir(t)
	account, _ := portableStores(t)
	for _, token := range []string{"first", "second"} {
		if err := account.Save(Credential{Token: token}); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := account.Load(); err != nil || got.Token != "second" {
		t.Fatalf("Load() = %+v, %v; want second", got, err)
	}
}

func TestPortableStoreWithoutFileIsEmpty(t *testing.T) {
	usePortableDir(t)
	account, _ := portableStores(t)
	if _, err := account.Load(); !errors.Is(err, ErrNoCredential) {
		t.Fatalf("Load() error = %v, want ErrNoCredential", err)
	}
	if err := account.Delete(); err != nil {
		t.Fatalf("Delete() without a file = %v, want nil", err)
	}
}

func TestPortableStoreRefusesAnEmptyToken(t *testing.T) {
	dir := usePortableDir(t)
	account, _ := portableStores(t)
	if err := account.Save(Credential{Username: "egor"}); err == nil {
		t.Fatal("Save() with an empty token succeeded")
	}
	if _, err := os.Stat(filepath.Join(dir, portableCredentialFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("credentials file after a refused Save: %v", err)
	}
}

func TestPortableStoreCorruptFileIsNotAGuest(t *testing.T) {
	cases := map[string]string{
		"not json":         "{not json",
		"truncated":        `{"version":1,"data":{"entries":{"account":{"token":"abc`,
		"future version":   `{"version":99,"data":{"entries":{}}}`,
		"wrong entry type": `{"version":1,"data":{"entries":{"account":"abc"}}}`,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			dir := usePortableDir(t)
			path := filepath.Join(dir, portableCredentialFile)
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			account, proxy := portableStores(t)

			if _, err := account.Load(); err == nil || errors.Is(err, ErrNoCredential) {
				t.Fatalf("Load() error = %v, want a read error that is not ErrNoCredential", err)
			}
			if err := proxy.Save(Credential{Token: "new"}); err == nil {
				t.Fatal("Save() over a corrupt file succeeded")
			}
			if err := account.Delete(); err == nil {
				t.Fatal("Delete() over a corrupt file succeeded")
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, []byte(content)) {
				t.Fatalf("the corrupt file was rewritten: %q", got)
			}
		})
	}
}

func TestPortableStoreUnreadableFileIsNotAGuest(t *testing.T) {
	dir := usePortableDir(t)
	if err := os.Mkdir(filepath.Join(dir, portableCredentialFile), 0o700); err != nil {
		t.Fatal(err)
	}
	account, _ := portableStores(t)
	if _, err := account.Load(); err == nil || errors.Is(err, ErrNoCredential) {
		t.Fatalf("Load() error = %v, want a read error that is not ErrNoCredential", err)
	}
	if err := account.Save(Credential{Token: "abc"}); err == nil {
		t.Fatal("Save() over an unreadable file succeeded")
	}
}

func TestPortableStoreFileIsOwnerOnly(t *testing.T) {
	dir := usePortableDir(t)
	account, _ := portableStores(t)
	if err := account.Save(Credential{Token: "abc"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, portableCredentialFile))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("credentials file mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestPortableStoreEnvTokenStillWins(t *testing.T) {
	usePortableDir(t)
	account, _ := portableStores(t)
	if err := account.Save(Credential{Token: "stored"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TYPHON_API_TOKEN", "from-env")
	fromEnv, err := NewCredentialStore()
	if err != nil {
		t.Fatal(err)
	}
	if got, err := fromEnv.Load(); err != nil || got.Token != "from-env" {
		t.Fatalf("Load() = %+v, %v; want the environment token", got, err)
	}
}

func TestPortableStoreMarkerErrorDoesNotFallBackToTheSystemStore(t *testing.T) {
	boom := errors.New("bad marker")
	prev := portableDir
	portableDir = func() (string, bool, error) { return "", false, boom }
	t.Cleanup(func() { portableDir = prev })

	if store, err := NewCredentialStore(); !errors.Is(err, boom) || store != nil {
		t.Fatalf("NewCredentialStore() = %v, %v; want the marker error", store, err)
	}
	if store, err := NewNamedCredentialStore("proxy"); !errors.Is(err, boom) || store != nil {
		t.Fatalf("NewNamedCredentialStore() = %v, %v; want the marker error", store, err)
	}
}

func TestOpenCredentialStoreUsesTheSystemStoreOutsidePortableMode(t *testing.T) {
	prev := portableDir
	portableDir = func() (string, bool, error) { return "", false, nil }
	t.Cleanup(func() { portableDir = prev })

	called := false
	want := portableCredentialStore{key: "marker"}
	got, err := openCredentialStore("account", func() (CredentialStore, error) {
		called = true
		return want, nil
	})
	if err != nil || !called || got != CredentialStore(want) {
		t.Fatalf("openCredentialStore() = %v, %v, system called = %v", got, err, called)
	}
}

func TestPortableStoresSavingTogetherLoseNothing(t *testing.T) {
	usePortableDir(t)
	account, proxy := portableStores(t)

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, store := range []CredentialStore{account, proxy} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 25; i++ {
				if err := store.Save(Credential{Token: "token"}); err != nil {
					errs <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	for name, store := range map[string]CredentialStore{"account": account, "proxy": proxy} {
		if _, err := store.Load(); err != nil {
			t.Fatalf("%s Load() after concurrent saves: %v", name, err)
		}
	}
}
