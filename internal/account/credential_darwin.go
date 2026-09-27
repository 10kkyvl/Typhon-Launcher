//go:build darwin && !devmock

package account

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Секрет хранится в связке ключей macOS — там же, где ему место, а не файлом
// рядом с настройками. Работаем через /usr/bin/security, а не через
// Security.framework: cgo-обёртка над SecItem* дала бы то же самое, но ACL
// записи выписывался бы на конкретный бинарь лаунчера, и каждая пересборка
// вызывала бы у пользователя диалог доступа. У security ACL стабильный.
const (
	keychainService = "app.typhon.launcher"
	keychainAccount = "account"
	keychainLabel   = "Typhon"

	// keychainNotFound — код возврата security, когда записи нет.
	keychainNotFound = 44
)

type keychainStore struct {
	service string
	account string
	// run подменяется в тестах: настоящая связка ключей на машине сборки
	// может быть заперта, а проверять надо разбор ответа.
	run func(args []string, stdin string) (string, error)
}

func newSystemCredentialStore() (CredentialStore, error) {
	return keychainStore{service: keychainService, account: keychainAccount, run: runSecurity}, nil
}

func runSecurity(args []string, stdin string) (string, error) {
	//nolint:gosec // G204: аргументы собирает сам пакет, пользовательских данных в них нет
	cmd := exec.Command("/usr/bin/security", args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	out, err := cmd.Output()
	return string(out), err
}

func (s keychainStore) Load() (Credential, error) {
	out, err := s.run([]string{
		"find-generic-password", "-s", s.service, "-a", s.account, "-w",
	}, "")
	if err != nil {
		if exitCode(err) == keychainNotFound {
			return Credential{}, ErrNoCredential
		}
		return Credential{}, fmt.Errorf("read keychain item %s: %w", s.service, err)
	}
	cred, err := decodeCredential(strings.TrimSpace(out))
	if err != nil {
		return Credential{}, err
	}
	if cred.Token == "" {
		return Credential{}, ErrNoCredential
	}
	return cred, nil
}

func (s keychainStore) Save(cred Credential) error {
	if cred.Token == "" {
		return errors.New("refusing to store an empty token")
	}
	payload, err := encodeCredential(cred)
	if err != nil {
		return err
	}
	// security спрашивает пароль дважды, поэтому и на вход он идёт дважды.
	// Через stdin, а не аргументом: аргументы видны в ps всей машине.
	if _, err := s.run([]string{
		"add-generic-password", "-U", "-s", s.service, "-a", s.account,
		"-l", keychainLabel, "-D", "Typhon account token", "-w",
	}, payload+"\n"+payload+"\n"); err != nil {
		return fmt.Errorf("write keychain item %s: %w", s.service, err)
	}
	return nil
}

func (s keychainStore) Delete() error {
	if _, err := s.run([]string{
		"delete-generic-password", "-s", s.service, "-a", s.account,
	}, ""); err != nil {
		if exitCode(err) == keychainNotFound {
			return nil
		}
		return fmt.Errorf("delete keychain item %s: %w", s.service, err)
	}
	return nil
}

// encodeCredential кладёт в связку base64: security печатает пароль как есть,
// и не-ASCII в имени пользователя вышел бы шестнадцатеричным дампом.
func encodeCredential(cred Credential) (string, error) {
	encoded, err := json.Marshal(cred)
	if err != nil {
		return "", fmt.Errorf("encode credential: %w", err)
	}
	return base64.StdEncoding.EncodeToString(encoded), nil
}

func decodeCredential(payload string) (Credential, error) {
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return Credential{}, fmt.Errorf("decode keychain payload: %w", err)
	}
	var cred Credential
	if err := json.Unmarshal(raw, &cred); err != nil {
		return Credential{}, fmt.Errorf("parse keychain payload: %w", err)
	}
	return cred, nil
}

func exitCode(err error) int {
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	return -1
}
