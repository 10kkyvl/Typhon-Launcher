package achievements

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type scanner struct {
	getenv  func(string) string
	profile bool
	ls      func(string) ([]fs.DirEntry, error)
}

func newScanner() scanner {
	return scanner{getenv: os.Getenv, profile: runtime.GOOS == "windows", ls: os.ReadDir}
}

type scanResult struct {
	Unlocked map[string]time.Time
	Found    bool
	// Partial is set when some place or file could not be read in full, so
	// the absence of a key does not prove the achievement is locked.
	Partial  bool
	Failures []error
}

func validAppID(appID string) bool {
	if appID == "" || len(appID) > 12 {
		return false
	}
	for _, r := range appID {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (sc scanner) scan(ctx context.Context, ref GameRef) (scanResult, error) {
	res := scanResult{Unlocked: map[string]time.Time{}}
	if !validAppID(ref.AppID) {
		return res, ErrNoSteamID
	}
	dirs, err := sc.progressDirs(ctx, ref, &res)
	if err != nil {
		return res, err
	}
	for _, dir := range dirs {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		if err := sc.readProgressDir(ctx, dir, &res); err != nil {
			return res, err
		}
	}
	return res, nil
}

// progressDirs lists the directories that may hold progress files. The
// wildcard roots are matched by shape, never by the name of what wrote them.
func (sc scanner) progressDirs(ctx context.Context, ref GameRef, res *scanResult) ([]string, error) {
	var dirs []string
	seen := map[string]struct{}{}
	add := func(dir string) {
		dir = filepath.Clean(dir)
		if _, dup := seen[dir]; dup {
			return
		}
		seen[dir] = struct{}{}
		dirs = append(dirs, dir)
	}
	wildcard := func(parent string) error {
		entries, err := sc.readDir(parent, res)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			if !entry.IsDir() {
				continue
			}
			add(filepath.Join(parent, entry.Name(), ref.AppID))
		}
		return nil
	}

	if sc.profile {
		if public := sc.getenv("PUBLIC"); public != "" {
			if err := wildcard(filepath.Join(public, "Documents", "Steam")); err != nil {
				return nil, err
			}
		}
		if appData := sc.getenv("APPDATA"); appData != "" {
			if err := wildcard(appData); err != nil {
				return nil, err
			}
		}
		if programData := sc.getenv("PROGRAMDATA"); programData != "" {
			if err := wildcard(filepath.Join(programData, "Steam")); err != nil {
				return nil, err
			}
		}
	}
	if install := strings.TrimSpace(ref.InstallDir); install != "" && filepath.IsAbs(install) {
		add(filepath.Join(install, "steam_settings"))
		add(filepath.Join(install, ref.AppID))
	}
	return dirs, nil
}

func (sc scanner) readProgressDir(ctx context.Context, dir string, res *scanResult) error {
	info, err := os.Lstat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case errors.Is(err, fs.ErrPermission):
		res.Partial = true
		return nil
	case err != nil:
		return fmt.Errorf("проверка каталога прогресса: %w", err)
	case !info.IsDir():
		return nil
	}
	entries, err := sc.readDir(dir, res)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !entry.Type().IsRegular() {
			continue
		}
		name := strings.ToLower(entry.Name())
		if name != "achievements.json" && name != "achievements.ini" {
			continue
		}
		if err := readProgressFile(filepath.Join(dir, entry.Name()), strings.HasSuffix(name, ".json"), res); err != nil {
			return err
		}
	}
	return nil
}

func readProgressFile(path string, isJSON bool, res *scanResult) error {
	f, err := os.Open(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case errors.Is(err, fs.ErrPermission):
		res.Partial = true
		return nil
	case err != nil:
		return fmt.Errorf("открытие файла прогресса: %w", err)
	}
	parse := ParseINI
	if isJSON {
		parse = ParseJSON
	}
	found, parseErr := parse(f)
	if err := f.Close(); err != nil {
		return fmt.Errorf("закрытие файла прогресса: %w", err)
	}
	switch {
	case errors.Is(parseErr, ErrNotProgress):
		return nil
	case errors.Is(parseErr, ErrBadJSON), errors.Is(parseErr, ErrBadINI), errors.Is(parseErr, ErrFileTooBig):
		res.Partial = true
		res.Failures = append(res.Failures, fmt.Errorf("%s: %w", path, parseErr))
		return nil
	case parseErr != nil:
		return fmt.Errorf("%s: %w", path, parseErr)
	}
	res.Found = true
	mergeUnlocks(res.Unlocked, found)
	return nil
}

// readDir treats a missing directory as nothing to read and a denied one as a
// partial result; any other failure is the caller's error.
func (sc scanner) readDir(path string, res *scanResult) ([]fs.DirEntry, error) {
	entries, err := sc.ls(path)
	switch {
	case err == nil:
		return entries, nil
	case errors.Is(err, fs.ErrNotExist):
		return nil, nil
	case errors.Is(err, fs.ErrPermission):
		res.Partial = true
		return nil, nil
	}
	return nil, fmt.Errorf("чтение каталога %s: %w", path, err)
}
