package emulators

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	storageRoot  = "ux0"
	storageGames = "app"

	storageConfig = "config.yml"
	storageKey    = "pref-path"
	// The emulator's marker of a portable install. Where such an install keeps
	// its games is not established, so its storage counts as unknown.
	portableMark = "portable"
	// The real file is under 5 KB.
	maxStorageConfig = 1 << 20

	utf8BOM = "\xef\xbb\xbf"
)

var storageData = []string{"patch", "addcont"}

// Storage is where an emulator keeps the games it installed. Known is false when
// that could not be established, which is not the same as the default location.
type Storage struct {
	Root  string
	Known bool
}

// Placement says what a game's folder is to the emulator.
type Placement int

const (
	// PlaceOutside is any other folder: the emulator can take it as content.
	PlaceOutside Placement = iota
	// PlaceInstalled is the emulator's own copy. Installing removes the title's
	// folder before it copies, so as content it would delete the only copy.
	PlaceInstalled
	// PlaceData is an update or an add-on in the emulator's storage.
	PlaceData
)

// StorageHome is where the emulator reads its config and where it keeps games
// when the config names no folder. An empty ConfigDir is the program's folder.
type StorageHome struct {
	ConfigDir string
	Root      string
}

// DefaultStorageHome gives the locations seen on real runs. On Windows the config
// sits beside Vita3K.exe. On macOS it is in the user's Application Support,
// outside the signed bundle, and a fresh install writes pref-path as its fs
// folder there.
func DefaultStorageHome() (StorageHome, error) {
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		return StorageHome{}, errors.New("папка эмулятора по умолчанию на этой системе неизвестна")
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return StorageHome{}, fmt.Errorf("папка настроек пользователя: %w", err)
	}
	pref := filepath.Join(dir, "Vita3K", "Vita3K")
	if runtime.GOOS == "darwin" {
		return StorageHome{ConfigDir: pref, Root: filepath.Join(pref, "fs")}, nil
	}
	return StorageHome{Root: pref}, nil
}

// FindStorage reads the emulator's config and never writes it. Only a config
// that is absent or says nothing means the default location. Without a home the
// config is still looked for beside the program, since it may name the folder.
func (p Platform) FindStorage(program string, home func() (StorageHome, error)) Storage {
	if !p.RemembersInstall() {
		return Storage{}
	}
	dir := filepath.Dir(program)
	if _, err := os.Lstat(filepath.Join(dir, portableMark)); !errors.Is(err, fs.ErrNotExist) {
		return Storage{}
	}
	where, homeErr := home()
	configDir := dir
	if homeErr == nil && where.ConfigDir != "" {
		if !filepath.IsAbs(where.ConfigDir) {
			return Storage{}
		}
		configDir = where.ConfigDir
	}
	data, err := readStorageConfig(filepath.Join(configDir, storageConfig))
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return Storage{}
	default:
		root, ok := prefPath(data)
		if !ok {
			return Storage{}
		}
		if root != "" {
			return Storage{Root: filepath.Clean(root), Known: true}
		}
	}
	if homeErr != nil || !filepath.IsAbs(where.Root) {
		return Storage{}
	}
	return Storage{Root: where.Root, Known: true}
}

func readStorageConfig(path string) (data []byte, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()
	data, err = io.ReadAll(io.LimitReader(f, maxStorageConfig+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxStorageConfig {
		return nil, fmt.Errorf("%s: больше %d байт", storageConfig, maxStorageConfig)
	}
	return data, nil
}

// prefPath finds the one top-level key the launcher needs in a YAML file without
// parsing the rest. An empty result means the key is absent or empty; ok is false
// for anything that cannot be read as an absolute path with certainty.
func prefPath(data []byte) (string, bool) {
	text := strings.TrimPrefix(string(data), utf8BOM)
	if !utf8.ValidString(text) {
		return "", false
	}
	value, found := "", false
	for _, line := range strings.Split(text, "\n") {
		key, rest, ok := strings.Cut(line, ":")
		if !ok || key != storageKey {
			continue
		}
		if found {
			return "", false
		}
		found = true
		if value, ok = yamlScalar(strings.TrimSpace(rest)); !ok {
			return "", false
		}
	}
	if value != "" && (!filepath.IsAbs(value) || strings.ContainsFunc(value, unicode.IsControl)) {
		return "", false
	}
	return value, true
}

// The file writes a path as a plain scalar, backslashes included. The quoted
// forms are read as far as paths need them; any other escape is refused.
func yamlScalar(raw string) (string, bool) {
	switch {
	case len(raw) >= 2 && raw[0] == '\'' && raw[len(raw)-1] == '\'':
		inner := raw[1 : len(raw)-1]
		if strings.Contains(strings.ReplaceAll(inner, "''", ""), "'") {
			return "", false
		}
		return strings.ReplaceAll(inner, "''", "'"), true
	case len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"':
		return doubleQuoted(raw[1 : len(raw)-1])
	case strings.HasPrefix(raw, "'"), strings.HasPrefix(raw, `"`), strings.Contains(raw, " #"):
		return "", false
	}
	return raw, true
}

func doubleQuoted(inner string) (string, bool) {
	var out strings.Builder
	for i := 0; i < len(inner); i++ {
		c := inner[i]
		if c == '"' {
			return "", false
		}
		if c == '\\' {
			i++
			if i == len(inner) || (inner[i] != '\\' && inner[i] != '"') {
				return "", false
			}
			c = inner[i]
		}
		out.WriteByte(c)
	}
	return out.String(), true
}

// Place decides by identity, so a link, a substituted drive or a short name all
// come to the same answer. Names are the last resort, for a storage that is not
// known, and there any doubt goes to starting by id: an emulator that does not
// have the game only says so, while content given by mistake deletes it.
func (p Platform) Place(image, titleID string, storage Storage) Placement {
	if !p.RemembersInstall() {
		return PlaceOutside
	}
	if storage.Known {
		if place, ok := placeByIdentity(image, titleID, storage.Root); ok {
			return place
		}
	}
	switch {
	case p.StoredTitle(image) != "":
		return PlaceInstalled
	case p.InEmulatorData(image):
		return PlaceData
	}
	return PlaceOutside
}

func placeByIdentity(image, titleID, root string) (Placement, bool) {
	var folders []fs.FileInfo
	for dir := filepath.Dir(image); ; dir = filepath.Dir(dir) {
		info, err := os.Stat(dir)
		if err != nil {
			return PlaceOutside, false
		}
		folders = append(folders, info)
		if dir == filepath.Dir(dir) {
			break
		}
	}
	check := func(kind string) (bool, bool) {
		info, err := os.Stat(filepath.Join(root, storageRoot, kind, titleID))
		if errors.Is(err, fs.ErrNotExist) {
			return false, true
		}
		if err != nil {
			return false, false
		}
		for _, folder := range folders {
			if os.SameFile(info, folder) {
				return true, true
			}
		}
		return false, true
	}
	same, ok := check(storageGames)
	if !ok {
		return PlaceOutside, false
	}
	if same {
		return PlaceInstalled, true
	}
	for _, kind := range storageData {
		same, ok := check(kind)
		if !ok {
			return PlaceOutside, false
		}
		if same {
			return PlaceData, true
		}
	}
	return PlaceOutside, true
}

// storageFolder finds "ux0/<kind>/<name>/..." on the path by whole folder names
// and returns kind and name. The file itself is never the name. Windows drops
// trailing dots and spaces, so «ux0.\app» is the same folder.
func storageFolder(image string) (kind, name string, ok bool) {
	parts := strings.FieldsFunc(image, func(r rune) bool { return r == '/' || r == '\\' })
	for i := range parts {
		parts[i] = strings.TrimRight(parts[i], ". ")
	}
	for i := len(parts) - 3; i >= 0; i-- {
		if strings.EqualFold(parts[i], storageRoot) {
			return strings.ToLower(parts[i+1]), parts[i+2], i+3 < len(parts)
		}
	}
	return "", "", false
}

// StoredTitle is the folder name when the path looks like the storage of an
// emulator that installs games.
func (p Platform) StoredTitle(image string) string {
	if kind, name, ok := storageFolder(image); ok && p.RemembersInstall() && kind == storageGames {
		return name
	}
	return ""
}

// InEmulatorData says whether the path looks like the updates or add-ons in the
// emulator's storage: they belong to a game there and start nothing by themselves.
func (p Platform) InEmulatorData(image string) bool {
	kind, _, ok := storageFolder(image)
	return ok && p.RemembersInstall() && (kind == storageData[0] || kind == storageData[1])
}
