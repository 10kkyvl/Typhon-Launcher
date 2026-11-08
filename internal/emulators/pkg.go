package emulators

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

const (
	pkgExt = ".pkg"
	// A PS3 package header: magic, revision, type, offsets and sizes, then the
	// content id at 0x30, such as "UP0001-NP0APOLLO_00-0000000000000000".
	pkgMagic       = "\x7fPKG"
	pkgHeaderSize  = 0x30 + 36
	pkgTypeOffset  = 6
	pkgTypePS3     = 1
	contentIDStart = 0x30
	// The title id sits after the publisher prefix "UP0001-".
	titleIDStart = 7
	titleIDEnd   = titleIDStart + 9

	// RPCS3's mount table is a few hundred bytes.
	maxVFSConfig = 1 << 20
	hddMount     = "/dev_hdd0/"
	emulatorDir  = "$(EmulatorDir)"
)

// AcceptsPackage says whether the file is a package the emulator installs for
// itself, which the launcher can hand it and then start from where it went.
func (p Platform) AcceptsPackage(path string) bool {
	return slices.Contains(p.packages, strings.ToLower(filepath.Ext(path)))
}

// PackageTitleID reads the title id from the header of a PS3 package. The
// header is not encrypted, so a package that cannot be installed yet still
// names its game.
func PackageTitleID(path string) (id string, err error) {
	f, err := os.Open(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "", fmt.Errorf("%w: %s", ErrImageMissing, path)
	case err != nil:
		return "", fmt.Errorf("%w: %w", ErrImageUnreadable, err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("%w: %w", ErrImageUnreadable, closeErr)
		}
	}()
	header := make([]byte, pkgHeaderSize)
	if _, err := io.ReadFull(f, header); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return "", fmt.Errorf("%w: короче заголовка", ErrPKGInvalid)
		}
		return "", fmt.Errorf("%w: %w", ErrImageUnreadable, err)
	}
	if string(header[:4]) != pkgMagic {
		return "", fmt.Errorf("%w: нет заголовка PKG", ErrPKGInvalid)
	}
	if kind := int(header[pkgTypeOffset])<<8 | int(header[pkgTypeOffset+1]); kind != pkgTypePS3 {
		return "", fmt.Errorf("%w: пакет не для PS3 (тип %d)", ErrPKGInvalid, kind)
	}
	id = string(header[contentIDStart+titleIDStart : contentIDStart+titleIDEnd])
	if !validTitleID.MatchString(id) {
		return "", fmt.Errorf("%w: %q", ErrTitleIDInvalid, id)
	}
	return id, nil
}

// InstalledPackageGame finds the game RPCS3 installed from a package: its
// EBOOT.BIN in dev_hdd0/game/<title id>. A folder without the program, which
// RPCS3 also leaves for data, does not count.
func (p Platform) InstalledPackageGame(program, titleID, goos string, env BIOSEnv) (string, bool) {
	if p.hddRoots == nil || !validTitleID.MatchString(titleID) {
		return "", false
	}
	hdd, ok := rpcs3HDD(p.hddRoots(program, goos, env))
	if !ok {
		return "", false
	}
	eboot, err := findFile(filepath.Join(hdd, "game", titleID), []string{"USRDIR", "EBOOT.BIN"})
	if err != nil {
		return "", false
	}
	return eboot, true
}

// rpcs3Roots are the folders RPCS3 keeps its data in: beside the program on
// Windows, in the user's config folder elsewhere.
func rpcs3Roots(program, goos string, env BIOSEnv) []string {
	if goos == "windows" {
		if filepath.IsAbs(program) {
			return []string{filepath.Dir(program)}
		}
		return nil
	}
	return joinAll(env.ConfigDir, "rpcs3")
}

// rpcs3HDD is dev_hdd0 of the first data folder that exists, moved where the
// mount table says. A table that cannot be read leaves the place unknown.
func rpcs3HDD(roots []string) (string, bool) {
	for _, root := range roots {
		info, err := os.Stat(root)
		if err != nil || !info.IsDir() {
			continue
		}
		for _, table := range []string{filepath.Join(root, "config", "vfs.yml"), filepath.Join(root, "vfs.yml")} {
			data, err := readVFS(table)
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				return "", false
			}
			mount, found, err := vfsMount(data, hddMount)
			if err != nil {
				return "", false
			}
			if found && mount != "" {
				mount = strings.ReplaceAll(mount, emulatorDir, root+string(filepath.Separator))
				if !filepath.IsAbs(mount) {
					return "", false
				}
				return filepath.Clean(mount), true
			}
		}
		return filepath.Join(root, "dev_hdd0"), true
	}
	return "", false
}

func readVFS(path string) (data []byte, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close %s: %w", path, closeErr)
		}
	}()
	data, err = io.ReadAll(io.LimitReader(f, maxVFSConfig+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if len(data) > maxVFSConfig {
		return nil, fmt.Errorf("%s: больше %d байт", path, maxVFSConfig)
	}
	return data, nil
}

// vfsMount reads one top-level entry of RPCS3's vfs.yml, "/dev_hdd0/: path".
func vfsMount(data []byte, key string) (string, bool, error) {
	scanner := bufio.NewScanner(bytes.NewReader(bytes.TrimPrefix(data, []byte(utf8BOM))))
	scanner.Buffer(make([]byte, 0, 4<<10), maxVFSConfig)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, key+":") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, key+":"))
		quoted, ok := yamlScalar(value)
		if !ok {
			return "", false, fmt.Errorf("mount %s: значение не читается", key)
		}
		return quoted, true, nil
	}
	if err := scanner.Err(); err != nil {
		return "", false, fmt.Errorf("read mount table: %w", err)
	}
	return "", false, nil
}

// InstallPackage hands a package to the emulator's own installer. The player
// confirms it in the emulator's window; the launcher neither waits for it nor
// tracks it as a game, and the process outlives the launcher if need be.
func InstallPackage(program, pkg string) error {
	cmd := exec.Command(program, "--installpkg", pkg) //nolint:gosec // G204: the configured emulator, given the package the player added
	cmd.Dir = filepath.Dir(program)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("запустить установщик: %w", err)
	}
	// The wait only reaps the child: its outcome is the emulator's to show.
	go func() {
		if err := cmd.Wait(); err != nil {
			slog.Warn("package installer exited", "program", program, "package", pkg, "error", err)
		}
	}()
	return nil
}
