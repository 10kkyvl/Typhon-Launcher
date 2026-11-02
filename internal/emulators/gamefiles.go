package emulators

import (
	"archive/zip"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	packageExt = ".vpk"
	// A param.sfo is a few kilobytes. The limit only stops a file that is not one.
	maxSFOSize = 1 << 20
	// Real files hold tens of entries with short keys. Both limits matter on their
	// own: a table without a NUL gives one long key per byte of the table.
	maxSFOEntries = 256
	maxSFOKey     = 64

	sfoHeaderSize = 20
	sfoEntrySize  = 16
	sfoMagic      = "\x00PSF"
	sfoUTF8       = 0x0204
	sfoUTF8Raw    = 0x0004
)

// Homebrew has ids of its own, such as VITASHELL.
var validTitleID = regexp.MustCompile(`^[A-Z0-9]{9}$`)

// gameFile is the file that starts an unpacked game. It counts only with proof
// beside it: "eboot.bin" alone says nothing, and neither does any other .bin.
type gameFile struct {
	name string
	// parent is the folder the file must lie in; the game's folder is then one
	// level up. Empty means the file lies right in the game's folder.
	parent string
	// proof is the path from the game's folder to the file that must exist.
	proof []string
	// discFolder and discMark are the layout of a disc: the game's folder is
	// discFolder, and the folder with discMark above it is the whole disc.
	discFolder string
	discMark   string
	// startsFolder means the emulator is given the folder, not the file.
	startsFolder bool
}

// GameInfo is what a game says about itself in its param.sfo.
type GameInfo struct {
	TitleID  string
	Category string
}

// ImageNames are the exact file names this console takes besides Extensions.
func (p Platform) ImageNames() []string {
	if p.game.name == "" {
		return []string{}
	}
	return []string{p.game.name}
}

// HasGameFiles says whether a game of this console is told apart by what it says
// about itself rather than by the shape of its files.
func (p Platform) HasGameFiles() bool {
	return p.game.name != ""
}

// IsGameFile looks at the name only.
func (p Platform) IsGameFile(image string) bool {
	if p.game.name == "" || !strings.EqualFold(filepath.Base(image), p.game.name) {
		return false
	}
	return p.game.parent == "" || strings.EqualFold(filepath.Base(filepath.Dir(image)), p.game.parent)
}

// SkipsFolder says whether a folder of this name can be left unread.
func (p Platform) SkipsFolder(name string) bool {
	return slices.ContainsFunc(p.skippedFolders, func(skipped string) bool { return strings.EqualFold(skipped, name) })
}

// GameDir is the folder that holds the game's proof, or the image's own folder
// when the image is a single file.
func (p Platform) GameDir(image string) string {
	dir := filepath.Dir(image)
	if p.IsGameFile(image) && p.game.parent != "" {
		return filepath.Dir(dir)
	}
	return dir
}

// InDiscFolder says whether the image is in the layout of a disc.
func (p Platform) InDiscFolder(image string) bool {
	return p.game.discFolder != "" && p.IsGameFile(image) && strings.EqualFold(filepath.Base(p.GameDir(image)), p.game.discFolder)
}

// GameFolder is the folder that holds the whole game. It checks the proof of an
// unpacked game: ErrImageIncomplete means it is not there, and anything else that
// it could not be looked at. A disc is the folder above discFolder only when the
// disc's mark says so: otherwise that folder can be Downloads or a whole drive.
func (p Platform) GameFolder(image string) (string, error) {
	if !p.IsGameFile(image) {
		return filepath.Dir(image), nil
	}
	dir := p.GameDir(image)
	if _, err := findFile(dir, p.game.proof); err != nil {
		return "", err
	}
	if !p.InDiscFolder(image) {
		return dir, nil
	}
	disc := filepath.Dir(dir)
	_, err := findFile(disc, []string{p.game.discMark})
	switch {
	case err == nil:
		return disc, nil
	case errors.Is(err, ErrImageIncomplete):
		return dir, nil
	default:
		return "", err
	}
}

// findFile walks a path ignoring case: dumps of the same console differ in it,
// and a case-sensitive disk must not lose a game over that. A link does not count.
func findFile(dir string, parts []string) (string, error) {
	for i, part := range parts {
		entries, err := os.ReadDir(dir)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return "", fmt.Errorf("%w: %s", ErrImageIncomplete, part)
		case err != nil:
			return "", fmt.Errorf("%w: %w", ErrImageUnreadable, err)
		}
		last := i == len(parts)-1
		at := slices.IndexFunc(entries, func(entry fs.DirEntry) bool {
			if !strings.EqualFold(entry.Name(), part) {
				return false
			}
			return (last && entry.Type().IsRegular()) || (!last && entry.IsDir())
		})
		if at < 0 {
			return "", fmt.Errorf("%w: %s", ErrImageIncomplete, part)
		}
		dir = filepath.Join(dir, entries[at].Name())
	}
	return dir, nil
}

// IsUpdate says whether the game's data describes an update to a game.
func (p Platform) IsUpdate(info GameInfo) bool {
	return slices.Contains(p.updateCategories, info.Category)
}

// IsAddon says whether the game's data describes an add-on to a game.
func (p Platform) IsAddon(info GameInfo) bool {
	return slices.Contains(p.addonCategories, info.Category)
}

// ReadGameInfo reads the param.sfo of an image: from the package for a .vpk, from
// beside the file for an unpacked game. On a console that starts games by title
// id a missing or malformed id is an error.
func (p Platform) ReadGameInfo(ctx context.Context, image string) (GameInfo, error) {
	if err := ctx.Err(); err != nil {
		return GameInfo{}, err
	}
	var data []byte
	var err error
	if p.IsGameFile(image) {
		data, err = p.readProof(image)
	} else {
		data, err = readPackageSFO(image)
	}
	if err != nil {
		return GameInfo{}, err
	}
	info, err := parseSFO(data)
	if err != nil {
		return GameInfo{}, err
	}
	if p.RemembersInstall() && !validTitleID.MatchString(info.TitleID) {
		return GameInfo{}, fmt.Errorf("%w: %q", ErrTitleIDInvalid, info.TitleID)
	}
	return info, nil
}

func (p Platform) readProof(image string) (data []byte, err error) {
	proof, err := findFile(p.GameDir(image), p.game.proof)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(proof)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrImageUnreadable, err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("%w: %w", ErrImageUnreadable, closeErr)
		}
	}()
	data, err = readSFOBytes(f)
	if err != nil && !errors.Is(err, ErrSFOInvalid) {
		return nil, fmt.Errorf("%w: %w", ErrImageUnreadable, err)
	}
	return data, err
}

// readPackageSFO looks through a whole package and takes nothing out of it but
// the one file. A name that climbs out of the package refuses the package, not
// just that entry: the emulator unpacks every entry of it.
func readPackageSFO(image string) (data []byte, err error) {
	archive, err := zip.OpenReader(image)
	var pathErr *fs.PathError
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, fmt.Errorf("%w: %s", ErrImageMissing, image)
	case errors.As(err, &pathErr):
		return nil, fmt.Errorf("%w: %w", ErrImageUnreadable, err)
	case err != nil && !errors.Is(err, zip.ErrInsecurePath):
		return nil, fmt.Errorf("%w: %w", ErrPackageInvalid, err)
	}
	defer func() {
		if closeErr := archive.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("%w: %w", ErrImageUnreadable, closeErr)
		}
	}()
	var sfo *zip.File
	for _, file := range archive.File {
		name := strings.ReplaceAll(file.Name, `\`, "/")
		if !filepath.IsLocal(filepath.FromSlash(name)) {
			return nil, fmt.Errorf("%w: запись %q", ErrPackageInvalid, file.Name)
		}
		if !strings.EqualFold(path.Clean(name), "sce_sys/param.sfo") {
			continue
		}
		if sfo != nil {
			return nil, fmt.Errorf("%w: два файла param.sfo", ErrPackageInvalid)
		}
		sfo = file
	}
	if sfo == nil {
		return nil, fmt.Errorf("%w: %s", ErrSFOMissing, filepath.Base(image))
	}
	if sfo.UncompressedSize64 > maxSFOSize {
		return nil, fmt.Errorf("%w: %d байт", ErrSFOInvalid, sfo.UncompressedSize64)
	}
	entry, err := sfo.Open()
	if err != nil {
		return nil, packageReadError(err)
	}
	defer func() {
		if closeErr := entry.Close(); closeErr != nil && err == nil {
			err = packageReadError(closeErr)
		}
	}()
	data, err = readSFOBytes(entry)
	if err != nil && !errors.Is(err, ErrSFOInvalid) {
		return nil, packageReadError(err)
	}
	return data, err
}

// packageReadError tells a disk that failed from a package that is broken: only
// the second one is the file's fault.
func packageReadError(err error) error {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return fmt.Errorf("%w: %w", ErrImageUnreadable, err)
	}
	return fmt.Errorf("%w: %w", ErrPackageInvalid, err)
}

// The size in a package header is only a claim, so the limit holds here as well.
func readSFOBytes(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxSFOSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxSFOSize {
		return nil, fmt.Errorf("%w: больше %d байт", ErrSFOInvalid, maxSFOSize)
	}
	return data, nil
}

// parseSFO reads the PSF format: a header, a table of entries, a table of key
// names and a table of values. Every offset comes from the file and is checked
// against the file, each entry's included even when its key is of no interest.
func parseSFO(data []byte) (GameInfo, error) {
	if len(data) < sfoHeaderSize || string(data[:4]) != sfoMagic {
		return GameInfo{}, fmt.Errorf("%w: нет заголовка", ErrSFOInvalid)
	}
	size := uint64(len(data))
	keys := uint64(binary.LittleEndian.Uint32(data[8:]))
	values := uint64(binary.LittleEndian.Uint32(data[12:]))
	count := uint64(binary.LittleEndian.Uint32(data[16:]))
	if count > maxSFOEntries {
		return GameInfo{}, fmt.Errorf("%w: записей %d", ErrSFOInvalid, count)
	}
	if sfoHeaderSize+count*sfoEntrySize > keys || keys > values || values > size {
		return GameInfo{}, fmt.Errorf("%w: таблицы вне файла", ErrSFOInvalid)
	}
	var info GameInfo
	seen := map[string]bool{}
	for i := range count {
		entry := data[sfoHeaderSize+i*sfoEntrySize:]
		key, err := sfoKey(data[keys:values], uint64(binary.LittleEndian.Uint16(entry)))
		if err != nil {
			return GameInfo{}, err
		}
		if seen[key] {
			return GameInfo{}, fmt.Errorf("%w: ключ %s дважды", ErrSFOInvalid, key)
		}
		seen[key] = true
		format := binary.LittleEndian.Uint16(entry[2:])
		length := uint64(binary.LittleEndian.Uint32(entry[4:]))
		limit := uint64(binary.LittleEndian.Uint32(entry[8:]))
		offset := uint64(binary.LittleEndian.Uint32(entry[12:]))
		if length > limit || values+offset+length > size {
			return GameInfo{}, fmt.Errorf("%w: значение %s вне файла", ErrSFOInvalid, key)
		}
		if key != "TITLE_ID" && key != "CATEGORY" {
			continue
		}
		text, err := sfoText(format, data[values+offset:values+offset+length])
		if err != nil {
			return GameInfo{}, fmt.Errorf("%w: %s", err, key)
		}
		if key == "TITLE_ID" {
			info.TitleID = text
		} else {
			info.Category = text
		}
	}
	return info, nil
}

func sfoKey(table []byte, offset uint64) (string, error) {
	if offset >= uint64(len(table)) {
		return "", fmt.Errorf("%w: ключ вне таблицы", ErrSFOInvalid)
	}
	rest := table[offset:]
	end := slices.Index(rest[:min(len(rest), maxSFOKey+1)], 0)
	if end <= 0 {
		return "", fmt.Errorf("%w: ключ без конца", ErrSFOInvalid)
	}
	return string(rest[:end]), nil
}

func sfoText(format uint16, value []byte) (string, error) {
	if format != sfoUTF8 && format != sfoUTF8Raw {
		return "", fmt.Errorf("%w: значение не строка", ErrSFOInvalid)
	}
	if end := slices.Index(value, 0); end >= 0 {
		value = value[:end]
	}
	text := string(value)
	if !utf8.ValidString(text) || strings.ContainsFunc(text, unicode.IsControl) {
		return "", fmt.Errorf("%w: недопустимые символы", ErrSFOInvalid)
	}
	return text, nil
}
