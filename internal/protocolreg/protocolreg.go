// Package protocolreg ties the typhon:// scheme to the launcher in the current
// user's registry. macOS and Linux declare it in the bundle and the desktop
// entry instead, so Register does nothing there.
package protocolreg

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	classKey = `Software\Classes\typhon`

	// Written by build/windows/nsis/project.nsi and by selfupdate.
	installKey      = `Software\Typhon`
	installDirValue = "InstallDir"

	foldCase = runtime.GOOS == "windows"
)

var (
	ErrEmptyPath  = errors.New("путь лаунчера пустой")
	ErrRelative   = errors.New("путь лаунчера не абсолютный")
	ErrQuotedPath = errors.New("в пути лаунчера есть кавычка")
)

// RunsFrom reports whether exe lives directly in installDir. A record that is
// empty, relative or points at a removed directory describes no installation.
func RunsFrom(exe, installDir string) (bool, error) {
	if installDir == "" || !filepath.IsAbs(installDir) {
		return false, nil
	}
	dir, err := filepath.EvalSymlinks(installDir)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("resolve install dir: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return false, fmt.Errorf("resolve launcher path: %w", err)
	}
	return samePath(filepath.Dir(resolved), dir, foldCase), nil
}

func samePath(a, b string, fold bool) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if fold {
		return strings.EqualFold(a, b)
	}
	return a == b
}

type value struct {
	key, name, data string
}

type store interface {
	// get reports found=false for a value or key that does not exist.
	get(key, name string) (data string, found bool, err error)
	set(key, name, data string) error
}

// values lists the command first and "URL Protocol" last: an interrupted write
// must not leave a class that Windows offers but cannot run.
func values(exe string) ([]value, error) {
	if exe == "" {
		return nil, ErrEmptyPath
	}
	if !filepath.IsAbs(exe) {
		return nil, ErrRelative
	}
	if strings.Contains(exe, `"`) {
		return nil, ErrQuotedPath
	}
	quoted := `"` + exe + `"`
	return []value{
		{`shell\open\command`, "", quoted + ` "%1"`},
		{`DefaultIcon`, "", quoted + ",0"},
		{``, "", "URL:Typhon Protocol"},
		{``, "URL Protocol", ""},
	}, nil
}

// apply writes only what differs, so a start with nothing to change leaves the
// registry untouched. Nothing is written unless the path passed validation.
func apply(s store, exe string) error {
	wanted, err := values(exe)
	if err != nil {
		return err
	}
	for _, v := range wanted {
		current, found, err := s.get(v.key, v.name)
		if err != nil {
			return fmt.Errorf("read protocol value %q %q: %w", v.key, v.name, err)
		}
		if found && current == v.data {
			continue
		}
		if err := s.set(v.key, v.name, v.data); err != nil {
			return fmt.Errorf("write protocol value %q %q: %w", v.key, v.name, err)
		}
	}
	return nil
}
