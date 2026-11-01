//go:build windows

package protocolreg

import (
	"errors"
	"fmt"
	"log/slog"

	"golang.org/x/sys/windows/registry"
)

// Register points typhon:// at exe for the current user.
func Register(exe string) error {
	return apply(regStore{base: classKey}, exe)
}

// InstallDir reads the directory the installer recorded; found is false when
// nothing installed the launcher for this user.
func InstallDir() (string, bool, error) {
	data, found, err := regStore{base: installKey}.get("", installDirValue)
	if err != nil {
		return "", false, fmt.Errorf("read install dir: %w", err)
	}
	return data, found, nil
}

type regStore struct {
	base string
}

func (r regStore) path(key string) string {
	if key == "" {
		return r.base
	}
	return r.base + `\` + key
}

func closeKey(k registry.Key) {
	if err := k.Close(); err != nil {
		slog.Debug("close protocol registry key", "error", err)
	}
}

func (r regStore) get(key, name string) (string, bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, r.path(key), registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("open key: %w", err)
	}
	defer closeKey(k)
	data, _, err := k.GetStringValue(name)
	// A value of another type is as wrong as a missing one: set replaces it.
	if errors.Is(err, registry.ErrNotExist) || errors.Is(err, registry.ErrUnexpectedType) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read value: %w", err)
	}
	return data, true, nil
}

func (r regStore) set(key, name, data string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, r.path(key), registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("create key: %w", err)
	}
	defer closeKey(k)
	if err := k.SetStringValue(name, data); err != nil {
		return fmt.Errorf("set value: %w", err)
	}
	return nil
}
