//go:build darwin && !devmock

package account

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

type fakeSecurity struct {
	out   string
	err   error
	calls [][]string
	stdin []string
}

func (f *fakeSecurity) run(args []string, stdin string) (string, error) {
	f.calls = append(f.calls, args)
	f.stdin = append(f.stdin, stdin)
	return f.out, f.err
}

func notFoundError(t *testing.T) error {
	t.Helper()
	// Настоящий код 44 берём у самой утилиты: выдумывать ExitError руками
	// значит проверять свою выдумку, а не поведение security.
	err := exec.Command("/usr/bin/security", "find-generic-password",
		"-s", "typhon-test-definitely-absent-item", "-a", "nobody", "-w").Run()
	if err == nil {
		t.Fatal("security нашёл заведомо отсутствующую запись")
	}
	return err
}

func TestKeychainLoadMissingIsErrNoCredential(t *testing.T) {
	fake := &fakeSecurity{err: notFoundError(t)}
	store := keychainStore{service: "s", account: "a", run: fake.run}

	if _, err := store.Load(); !errors.Is(err, ErrNoCredential) {
		t.Fatalf("err = %v, want ErrNoCredential", err)
	}
}

func TestKeychainDeleteMissingIsNoError(t *testing.T) {
	fake := &fakeSecurity{err: notFoundError(t)}
	store := keychainStore{service: "s", account: "a", run: fake.run}

	if err := store.Delete(); err != nil {
		t.Fatalf("Delete on a missing item: %v", err)
	}
}

func TestKeychainSaveKeepsTheSecretOutOfArgv(t *testing.T) {
	fake := &fakeSecurity{}
	store := keychainStore{service: "s", account: "a", run: fake.run}

	if err := store.Save(Credential{Token: "top-secret-token", Username: "егор"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(fake.calls))
	}
	joined := strings.Join(fake.calls[0], " ")
	if strings.Contains(joined, "top-secret-token") {
		t.Fatalf("секрет попал в аргументы: %q", joined)
	}
	// Пароль подаётся дважды: security спрашивает его и подтверждение.
	lines := strings.Split(strings.TrimSuffix(fake.stdin[0], "\n"), "\n")
	if len(lines) != 2 || lines[0] != lines[1] || lines[0] == "" {
		t.Fatalf("stdin = %q, want the payload twice", fake.stdin[0])
	}
}

func TestKeychainSaveRefusesEmptyToken(t *testing.T) {
	store := keychainStore{service: "s", account: "a", run: (&fakeSecurity{}).run}
	if err := store.Save(Credential{Username: "егор"}); err == nil {
		t.Fatal("Save with an empty token: want error")
	}
}

func TestCredentialPayloadRoundTrip(t *testing.T) {
	want := Credential{Token: "t0ken", Username: "Егор Рипа"}

	encoded, err := encodeCredential(want)
	if err != nil {
		t.Fatalf("encodeCredential: %v", err)
	}
	if strings.ContainsAny(encoded, "\n\r ") {
		t.Fatalf("payload %q must be a single ASCII line", encoded)
	}
	got, err := decodeCredential(encoded)
	if err != nil {
		t.Fatalf("decodeCredential: %v", err)
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestKeychainLoadRejectsGarbage(t *testing.T) {
	store := keychainStore{service: "s", account: "a", run: (&fakeSecurity{out: "not base64!!"}).run}
	if _, err := store.Load(); err == nil {
		t.Fatal("Load on a corrupt payload: want error")
	}
}

// Живая проверка против настоящей связки ключей: пишет и тут же сносит свою
// запись под уникальным именем. По умолчанию выключена — на CI связка бывает
// заперта, а трогать чужую связку молча нельзя.
func TestLiveKeychainRoundTrip(t *testing.T) {
	if os.Getenv("TYPHON_KEYCHAIN_LIVE") == "" {
		t.Skip("живые проверки связки ключей выключены: установите TYPHON_KEYCHAIN_LIVE=1")
	}
	store := keychainStore{service: "app.typhon.launcher.test", account: "test", run: runSecurity}
	t.Cleanup(func() {
		if err := store.Delete(); err != nil {
			t.Errorf("cleanup delete: %v", err)
		}
	})

	if _, err := store.Load(); !errors.Is(err, ErrNoCredential) {
		t.Fatalf("Load before Save = %v, want ErrNoCredential", err)
	}
	want := Credential{Token: "live-token-123", Username: "Егор"}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if err := store.Delete(); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Load(); !errors.Is(err, ErrNoCredential) {
		t.Fatalf("Load after Delete = %v, want ErrNoCredential", err)
	}
}
