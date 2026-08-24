package platform

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

func Normalize(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", ErrEmptyPath
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", path, err)
	}
	return filepath.Clean(abs), nil
}

// PathKey даёт ключ сравнения путей: на Windows файловая система
// регистронезависима, поэтому один и тот же каталог приходит из разных мест
// в разном регистре.
func PathKey(path string) (string, error) {
	normalized, err := Normalize(path)
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "windows" {
		return strings.ToLower(normalized), nil
	}
	return normalized, nil
}

func SamePath(a, b string) bool {
	first, err := PathKey(a)
	if err != nil {
		return false
	}
	second, err := PathKey(b)
	if err != nil {
		return false
	}
	return first == second
}

func Inside(dir, path string) bool {
	root, err := PathKey(dir)
	if err != nil {
		return false
	}
	target, err := PathKey(path)
	if err != nil {
		return false
	}
	if root == target {
		return true
	}
	return strings.HasPrefix(target, root+string(filepath.Separator))
}
