package install

import (
	"errors"
	"path/filepath"
	"strings"

	"typhon/internal/platform"
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
	return platform.Inside(dir, path)
}

func samePath(a, b string) bool {
	return platform.SamePath(a, b)
}
