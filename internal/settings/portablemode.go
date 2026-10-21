package settings

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"
)

const (
	PortableMarkerName = "portable.txt"
	// PortableEnv hands the data directory to the update worker: it runs from a
	// copy of the launcher in the cache, where no marker sits next to it.
	PortableEnv        = "TYPHON_PORTABLE_DIR"
	portableDefaultDir = "data"
	portableMarkerMax  = 4 << 10
)

var ErrPortableMarkerInvalid = errors.New("portable marker is invalid")

var executablePath = os.Executable

type portableResult struct {
	dir string
	ok  bool
	err error
}

var (
	portableOnce   sync.Once
	portableCached portableResult
)

func PortableDir() (dir string, ok bool, err error) {
	portableOnce.Do(func() {
		portableCached.dir, portableCached.ok, portableCached.err = resolvePortable()
	})
	return portableCached.dir, portableCached.ok, portableCached.err
}

func IsPortable() bool {
	_, ok, err := PortableDir()
	return err == nil && ok
}

func PortableError() error {
	_, _, err := PortableDir()
	return err
}

func resolvePortable() (string, bool, error) {
	exe, err := executablePath()
	if err != nil {
		return "", false, fmt.Errorf("locate launcher executable: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return "", false, fmt.Errorf("resolve launcher executable %q: %w", exe, err)
	}
	exe = resolved
	exeDir := filepath.Dir(exe)
	marker := filepath.Join(exeDir, PortableMarkerName)
	raw, err := readPortableMarker(marker)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return portableFromEnv()
		}
		return "", false, err
	}
	rel, err := portableRelative(raw)
	if err != nil {
		return "", false, fmt.Errorf("%s: %w", marker, err)
	}
	return filepath.Join(exeDir, rel), true, nil
}

func portableFromEnv() (string, bool, error) {
	dir := os.Getenv(PortableEnv)
	if dir == "" {
		return "", false, nil
	}
	if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir {
		return "", false, fmt.Errorf("%s=%q: %w: want a clean absolute path", PortableEnv, dir, ErrPortableMarkerInvalid)
	}
	return dir, true, nil
}

func readPortableMarker(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open portable marker: %w", err)
	}
	raw, readErr := io.ReadAll(io.LimitReader(f, portableMarkerMax+1))
	closeErr := f.Close()
	if readErr != nil {
		return "", fmt.Errorf("read portable marker %s: %w", path, readErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("close portable marker %s: %w", path, closeErr)
	}
	if len(raw) > portableMarkerMax {
		return "", fmt.Errorf("%s: %w: longer than %d bytes", path, ErrPortableMarkerInvalid, portableMarkerMax)
	}
	return string(raw), nil
}

func portableRelative(content string) (string, error) {
	text := strings.TrimSpace(strings.TrimPrefix(content, string(rune(0xFEFF))))
	if text == "" {
		return portableDefaultDir, nil
	}
	if strings.ContainsAny(text, "\r\n") {
		return "", fmt.Errorf("%w: more than one line", ErrPortableMarkerInvalid)
	}
	if filepath.IsAbs(text) || filepath.VolumeName(text) != "" || text[0] == '/' || text[0] == '\\' {
		return "", fmt.Errorf("%w: %q is not a relative path", ErrPortableMarkerInvalid, text)
	}
	segments := strings.FieldsFunc(text, func(r rune) bool { return r == '/' || r == '\\' })
	if len(segments) != strings.Count(text, "/")+strings.Count(text, "\\")+1 {
		return "", fmt.Errorf("%w: %q has an empty segment", ErrPortableMarkerInvalid, text)
	}
	for _, seg := range segments {
		if strings.Trim(seg, ". ") == "" {
			return "", fmt.Errorf("%w: %q contains the segment %q", ErrPortableMarkerInvalid, text, seg)
		}
		if strings.ContainsFunc(seg, func(r rune) bool { return r == ':' || unicode.IsControl(r) }) {
			return "", fmt.Errorf("%w: %q contains a forbidden character", ErrPortableMarkerInvalid, text)
		}
	}
	return filepath.Join(segments...), nil
}
