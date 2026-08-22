package account

import (
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	credTypeGeneric         = 1
	credPersistLocalMachine = 2
	credMaxBlobSize         = 5 * 512
)

var (
	advapi32        = windows.NewLazySystemDLL("advapi32.dll")
	procCredWriteW  = advapi32.NewProc("CredWriteW")
	procCredReadW   = advapi32.NewProc("CredReadW")
	procCredDeleteW = advapi32.NewProc("CredDeleteW")
	procCredFree    = advapi32.NewProc("CredFree")
)

type credentialW struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        windows.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

type windowsCredentialStore struct {
	name   string
	target *uint16
}

func newSystemCredentialStore() (CredentialStore, error) {
	return newWindowsCredentialStore(storeTarget)
}

func newWindowsCredentialStore(name string) (CredentialStore, error) {
	target, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, fmt.Errorf("encode credential target: %w", err)
	}
	return windowsCredentialStore{name: name, target: target}, nil
}

func (s windowsCredentialStore) Load() (Credential, error) {
	var raw *credentialW
	//nolint:gosec // G103: инвариант 10 требует OS credential storage; CredReadW вызывается только через unsafe.Pointer.
	ret, _, err := procCredReadW.Call(
		uintptr(unsafe.Pointer(s.target)),
		credTypeGeneric,
		0,
		uintptr(unsafe.Pointer(&raw)),
	)
	if ret == 0 {
		if errors.Is(err, windows.ERROR_NOT_FOUND) {
			return Credential{}, ErrNoCredential
		}
		return Credential{}, fmt.Errorf("read credential %q: %w", s.name, err)
	}
	defer freeCredentialBuffer(raw)

	if raw.CredentialBlob == nil || raw.CredentialBlobSize == 0 {
		return Credential{}, ErrNoCredential
	}

	//nolint:gosec // G103: блоб приходит от CredReadW, длина берётся из того же результата.
	blob := unsafe.Slice(raw.CredentialBlob, raw.CredentialBlobSize)
	cred := Credential{Token: string(blob)}
	if raw.UserName != nil {
		cred.Username = windows.UTF16PtrToString(raw.UserName)
	}
	if cred.Token == "" {
		return Credential{}, ErrNoCredential
	}
	return cred, nil
}

func freeCredentialBuffer(raw *credentialW) {
	// CredFree returns void; Call reports the thread's last OS error, which this call does not set.
	//nolint:gosec // G103: освобождение буфера, выделенного CredReadW.
	if _, _, err := procCredFree.Call(uintptr(unsafe.Pointer(raw))); !errors.Is(err, windows.ERROR_SUCCESS) {
		slog.Debug("free credential buffer", "error", err)
	}
}

func (s windowsCredentialStore) Save(cred Credential) error {
	if cred.Token == "" {
		return errors.New("refusing to store an empty token")
	}

	blob := []byte(cred.Token)
	if len(blob) > credMaxBlobSize {
		return fmt.Errorf("token is %d bytes, credential manager allows %d", len(blob), credMaxBlobSize)
	}

	entry := credentialW{
		Type:       credTypeGeneric,
		TargetName: s.target,
		//nolint:gosec // G115: длина проверена против credMaxBlobSize строкой выше, переполнение невозможно.
		CredentialBlobSize: uint32(len(blob)),
		CredentialBlob:     &blob[0],
		Persist:            credPersistLocalMachine,
	}

	var username *uint16
	if cred.Username != "" {
		encoded, err := windows.UTF16PtrFromString(cred.Username)
		if err != nil {
			return fmt.Errorf("encode credential user name: %w", err)
		}
		username = encoded
		entry.UserName = username
	}

	//nolint:gosec // G103: инвариант 10 требует OS credential storage; CredWriteW принимает только указатель.
	ret, _, err := procCredWriteW.Call(uintptr(unsafe.Pointer(&entry)), 0)
	runtime.KeepAlive(blob)
	runtime.KeepAlive(username)
	runtime.KeepAlive(s.target)
	if ret == 0 {
		return fmt.Errorf("write credential %q: %w", s.name, err)
	}
	return nil
}

func (s windowsCredentialStore) Delete() error {
	//nolint:gosec // G103: инвариант 10 требует OS credential storage; CredDeleteW принимает только указатель.
	ret, _, err := procCredDeleteW.Call(uintptr(unsafe.Pointer(s.target)), credTypeGeneric, 0)
	runtime.KeepAlive(s.target)
	if ret == 0 {
		if errors.Is(err, windows.ERROR_NOT_FOUND) {
			return nil
		}
		return fmt.Errorf("delete credential %q: %w", s.name, err)
	}
	return nil
}
