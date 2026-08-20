package install

import (
	"errors"
	"path/filepath"
	"runtime"
	"strings"
)

var errUnsafePath = errors.New("недопустимый путь внутри архива")

func safeJoin(dest, name string) (string, error) {
	slashed := strings.ReplaceAll(name, `\`, "/")
	if slashed == "" || strings.HasPrefix(slashed, "/") || strings.Contains(slashed, ":") {
		return "", errUnsafePath
	}
	trimmed := strings.TrimRight(slashed, "/")
	if trimmed == "" || trimmed == "." {
		return "", errUnsafePath
	}
	for _, part := range strings.Split(trimmed, "/") {
		if part == ".." {
			return "", errUnsafePath
		}
	}
	rel := filepath.FromSlash(trimmed)
	if !filepath.IsLocal(rel) {
		return "", errUnsafePath
	}
	target := filepath.Join(dest, rel)
	if !inside(dest, target) {
		return "", errUnsafePath
	}
	return target, nil
}

func inside(dir, path string) bool {
	root, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	root = filepath.Clean(root)
	abs = filepath.Clean(abs)
	if abs == root {
		return true
	}
	prefix := root + string(filepath.Separator)
	if runtime.GOOS == "windows" {
		return strings.HasPrefix(strings.ToLower(abs), strings.ToLower(prefix))
	}
	return strings.HasPrefix(abs, prefix)
}
