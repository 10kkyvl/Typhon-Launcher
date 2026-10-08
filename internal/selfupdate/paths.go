package selfupdate

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"typhon/internal/uierr"
)

var (
	ErrEmptyConfigDir     = uierr.New("selfupdate.empty_config_dir", "selfupdate: config dir is empty")
	ErrInvalidVersionPath = uierr.New("selfupdate.invalid_version_path", "selfupdate: version is not a safe path component")
)

var errNotCached = errors.New("selfupdate: path is not an artifact inside the selfupdate cache")

const pathInvalidChars = `/\:*?"<>|`

var windowsDeviceNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true, "CONIN$": true, "CONOUT$": true,
}

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
	if isWindowsUnsafeName(s) {
		return ErrInvalidVersionPath
	}
	return nil
}

// isWindowsUnsafeName is checked on every host because the manifest is signed
// once and served to all of them. Windows opens a device instead of a file for
// these names whatever the extension, and silently strips a trailing dot or
// space, so two names the manifest keeps apart would land on one file.
func isWindowsUnsafeName(name string) bool {
	if strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return true
	}
	stem := name
	if i := strings.IndexByte(stem, '.'); i >= 0 {
		stem = stem[:i]
	}
	stem = strings.ToUpper(strings.TrimRight(stem, " "))
	if windowsDeviceNames[stem] {
		return true
	}
	runes := []rune(stem)
	if len(runes) != 4 {
		return false
	}
	if prefix := string(runes[:3]); prefix != "COM" && prefix != "LPT" {
		return false
	}
	last := runes[3]
	return last >= '0' && last <= '9' || last == '¹' || last == '²' || last == '³'
}

// artifactRel accepts only the shape the downloader produces, <version>/<name>
// directly under the cache: state.json is a file the user's own account can
// rewrite, so the path it records is input and not something to delete or
// install from on trust.
func artifactRel(configDir, p string) (cacheDir, rel string, err error) {
	cacheDir, err = CacheDir(configDir)
	if err != nil {
		return "", "", err
	}
	if !filepath.IsAbs(p) || p != filepath.Clean(p) {
		return "", "", errNotCached
	}
	rel, err = filepath.Rel(cacheDir, p)
	if err != nil || !filepath.IsLocal(rel) {
		return "", "", errNotCached
	}
	if dir := filepath.Dir(rel); dir == "." || filepath.Dir(dir) != "." {
		return "", "", errNotCached
	}
	return cacheDir, rel, nil
}

// removeCached deletes through an os.Root scoped to the cache directory, so a
// version directory swapped for a link cannot carry the removal out of it.
func removeCached(configDir, p string) error {
	cacheDir, rel, err := artifactRel(configDir, p)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(cacheDir)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := root.Close(); cerr != nil {
			slog.Warn("close selfupdate cache root", "error", cerr)
		}
	}()
	return root.Remove(rel)
}
