//go:build devmock && !windows

package shortcut

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"typhon/internal/storage"
)

func Supported() bool { return true }

func DesktopDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("shortcut: домашний каталог: %w", err)
	}
	dir := filepath.Join(home, "Desktop")
	info, err := os.Stat(dir)
	if err != nil {
		return "", fmt.Errorf("shortcut: каталог Desktop %s: %w", dir, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("shortcut: %s не является каталогом", dir)
	}
	return dir, nil
}

func Create(path string, link Link) error {
	if path == "" {
		return errors.New("shortcut: путь ярлыка пуст")
	}
	if link.Target == "" {
		return errors.New("shortcut: цель ярлыка (Target) пуста")
	}
	dir := filepath.Dir(path)
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("shortcut: каталог назначения %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("shortcut: %s не является каталогом", dir)
	}

	encoded, err := json.MarshalIndent(link, "", "  ")
	if err != nil {
		return fmt.Errorf("shortcut: сериализация %s: %w", path, err)
	}
	if err := storage.WriteAtomic(path, encoded); err != nil {
		return fmt.Errorf("shortcut: запись %s: %w", path, err)
	}
	return nil
}
