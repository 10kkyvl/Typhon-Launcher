package selfupdate

import (
	"errors"
	"path/filepath"
	"strings"
)

var (
	ErrEmptyConfigDir     = errors.New("selfupdate: config dir is empty")
	ErrInvalidVersionPath = errors.New("selfupdate: version is not a safe path component")
)

const pathInvalidChars = `/\:*?"<>|`

func CacheDir(configDir string) (string, error) {
	if configDir == "" {
		return "", ErrEmptyConfigDir
	}
	return filepath.Join(configDir, "selfupdate"), nil
}

func VersionDir(configDir, version string) (string, error) {
	base, err := CacheDir(configDir)
	if err != nil {
		return "", err
	}
	if err := validatePathSegment(version); err != nil {
		return "", err
	}
	return filepath.Join(base, version), nil
}

func ArtifactPath(configDir, version, name string) (string, error) {
	dir, err := VersionDir(configDir, version)
	if err != nil {
		return "", err
	}
	if err := validateArtifactName(name); err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

func validatePathSegment(s string) error {
	if s == "" || s == "." || s == ".." {
		return ErrInvalidVersionPath
	}
	if strings.ContainsAny(s, pathInvalidChars) {
		return ErrInvalidVersionPath
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return ErrInvalidVersionPath
		}
	}
	return nil
}
