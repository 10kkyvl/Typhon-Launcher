package emulators

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// The settings file of DuckStation is a few kilobytes, that of PCSX2 a few dozen.
const maxBIOSConfig = 4 << 20

// biosLayout is where an emulator of an older console looks for the BIOS image
// the player dumped. The launcher never provides one: it only says before the
// start that the emulator would find none, since in batch mode both emulators
// quit without a word when it is missing.
type biosLayout struct {
	// portableMarks beside the program mean the program's folder is its data.
	portableMarks []string
	// roots name the data folder per system; the first one that exists counts.
	roots func(goos string, env BIOSEnv) []string
	// config is the settings file inside the data folder, section and key the
	// entry that moves the BIOS folder away from dir.
	config  []string
	section string
	key     string
	dir     string
	// minSize and maxSize bound a file that may be a BIOS image.
	minSize int64
	maxSize int64
}

// BIOSEnv is where the user's folders are. It is a value so that tests never
// read the player's real data.
type BIOSEnv struct {
	ConfigDir  string // os.UserConfigDir
	CacheDir   string // os.UserCacheDir: LocalAppData on Windows
	HomeDir    string
	XDGDataDir string
}

// DefaultBIOSEnv reads the folders of the current user. A folder that cannot be
// told stays empty and is not looked at.
func DefaultBIOSEnv() BIOSEnv {
	var env BIOSEnv
	if dir, err := os.UserConfigDir(); err == nil {
		env.ConfigDir = dir
	}
	if dir, err := os.UserCacheDir(); err == nil {
		env.CacheDir = dir
	}
	if dir, err := os.UserHomeDir(); err == nil {
		env.HomeDir = dir
	}
	env.XDGDataDir = os.Getenv("XDG_DATA_HOME")
	return env
}

// BIOSState is what the launcher knows about the BIOS of a console.
type BIOSState struct {
	// Missing is said only when the folder the emulator reads is known and
	// holds nothing that can be a BIOS image. Anything less certain lets the
	// game start: the emulator's own setup then speaks for itself.
	Missing bool
	// Dir is the folder the emulator reads, when known.
	Dir string
}

func duckStationRoots(goos string, env BIOSEnv) []string {
	switch goos {
	case "darwin":
		return joinAll(env.ConfigDir, "DuckStation")
	case "windows":
		return append(joinAll(env.CacheDir, "DuckStation"), joinAll(env.HomeDir, "Documents", "DuckStation")...)
	default:
		if filepath.IsAbs(env.XDGDataDir) {
			return joinAll(env.XDGDataDir, "duckstation")
		}
		return joinAll(env.HomeDir, ".local", "share", "duckstation")
	}
}

func pcsx2Roots(goos string, env BIOSEnv) []string {
	if goos == "windows" {
		return joinAll(env.HomeDir, "Documents", "PCSX2")
	}
	return joinAll(env.ConfigDir, "PCSX2")
}

// joinAll gives nothing for an unknown base instead of a relative path.
func joinAll(base string, parts ...string) []string {
	if !filepath.IsAbs(base) {
		return nil
	}
	return []string{filepath.Join(append([]string{base}, parts...)...)}
}

// CheckBIOS says whether the emulator at program would find a BIOS image.
func (p Platform) CheckBIOS(program, goos string, env BIOSEnv) BIOSState {
	layout := p.bios
	if layout.roots == nil {
		return BIOSState{}
	}
	root, ok := biosRoot(layout, program, goos, env)
	if !ok {
		return BIOSState{}
	}
	dir, ok := biosDir(layout, root)
	if !ok {
		return BIOSState{}
	}
	entries, err := os.ReadDir(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return BIOSState{Missing: true, Dir: dir}
	case err != nil:
		return BIOSState{Dir: dir}
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		// Stat follows a link: a player may keep the dumps elsewhere.
		info, err := os.Stat(filepath.Join(dir, entry.Name()))
		if err != nil {
			// A file the launcher cannot read may still be one the
			// emulator can: that is not proof of absence.
			return BIOSState{Dir: dir}
		}
		if info.Mode().IsRegular() && info.Size() >= layout.minSize && info.Size() <= layout.maxSize {
			return BIOSState{Dir: dir}
		}
	}
	return BIOSState{Missing: true, Dir: dir}
}

// BIOSDir is the folder the emulator reads, for the player to put a dump into.
func (p Platform) BIOSDir(program, goos string, env BIOSEnv) (string, bool) {
	if p.bios.roots == nil {
		return "", false
	}
	root, ok := biosRoot(p.bios, program, goos, env)
	if !ok {
		return "", false
	}
	return biosDir(p.bios, root)
}

// biosRoot is the data folder in use. None that exists means the emulator has
// never run: its first start asks for the BIOS itself.
func biosRoot(layout biosLayout, program, goos string, env BIOSEnv) (string, bool) {
	if filepath.IsAbs(program) {
		exeDir := filepath.Dir(program)
		for _, mark := range layout.portableMarks {
			if _, err := os.Lstat(filepath.Join(exeDir, mark)); err == nil {
				return exeDir, true
			}
		}
	}
	for _, root := range layout.roots(goos, env) {
		info, err := os.Stat(root)
		if err == nil && info.IsDir() {
			return root, true
		}
	}
	return "", false
}

// biosDir reads the folder from the settings; a settings file that is absent or
// names none means the default folder, one that cannot be read means unknown.
func biosDir(layout biosLayout, root string) (string, bool) {
	data, err := readBIOSConfig(filepath.Join(append([]string{root}, layout.config...)...))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return filepath.Join(root, layout.dir), true
	case err != nil:
		return "", false
	}
	value, found, err := iniValue(data, layout.section, layout.key)
	if err != nil {
		return "", false
	}
	if !found || value == "" {
		return filepath.Join(root, layout.dir), true
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value), true
	}
	return filepath.Join(root, value), true
}

func readBIOSConfig(path string) (data []byte, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close %s: %w", path, closeErr)
		}
	}()
	data, err = io.ReadAll(io.LimitReader(f, maxBIOSConfig+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if len(data) > maxBIOSConfig {
		return nil, fmt.Errorf("%s: больше %d байт", path, maxBIOSConfig)
	}
	return data, nil
}

// iniValue finds key in section of an INI file as both emulators write it:
// "key = value", sections in brackets, comments after ; or #.
func iniValue(data []byte, section, key string) (string, bool, error) {
	data = bytes.TrimPrefix(data, []byte(utf8BOM))
	in := false
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64<<10), maxBIOSConfig)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == ';' || line[0] == '#' {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			in = strings.EqualFold(strings.TrimSpace(line[1:len(line)-1]), section)
			continue
		}
		if !in {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(name), key) {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
			value = value[1 : len(value)-1]
		}
		return value, true, nil
	}
	if err := scanner.Err(); err != nil {
		return "", false, fmt.Errorf("read settings: %w", err)
	}
	return "", false, nil
}
