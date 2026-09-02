//go:build devmock && !windows

package install

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"typhon/internal/storage"
)

const (
	devmockMarkerName = ".typhon-devmock"
	devmockExeSize    = 64 << 10
)

var (
	errDevmockUnmarkedDir   = errors.New("devmock: отказ удалить каталог без метки установки")
	errDevmockNoGamesPath   = errors.New("devmock: путь библиотеки игр не задан")
	errDevmockNoInstallName = errors.New("devmock: не удалось определить имя игры")
)

var devmockSetupSuffixes = []string{"-setup", "_setup", " setup", "setup"}

type mockRunner struct {
	gamesPath func() string
}

func newRunner(gamesPath func() string) runner { return mockRunner{gamesPath: gamesPath} }

func (r mockRunner) run(ctx context.Context, spec runSpec) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if devmockIsProductCodeUninstall(spec.Args) {
		return 0, nil
	}
	dir, marked, err := devmockUninstallTarget(spec.Path)
	if err != nil {
		return 0, err
	}
	if marked {
		if err := os.RemoveAll(dir); err != nil {
			return 0, fmt.Errorf("remove %s: %w", dir, err)
		}
		return 0, nil
	}
	if devmockLooksLikeUninstaller(spec.Path) {
		return 0, fmt.Errorf("%w: %s", errDevmockUnmarkedDir, dir)
	}
	return r.install(spec)
}

func devmockIsProductCodeUninstall(args []string) bool {
	for _, arg := range args {
		if strings.EqualFold(arg, "/x") {
			return true
		}
	}
	return false
}

func devmockLooksLikeUninstaller(path string) bool {
	return strings.HasPrefix(strings.ToLower(filepath.Base(path)), "unins")
}

// devmockUninstallTarget reports the directory an uninstall spec would remove
// and whether it carries the marker the mock installer leaves behind: the
// real Windows uninstall path is driven by the uninstaller's own location
// (removal.go, uninstallSpec), so the mock has to derive the same directory
// rather than a value passed separately.
func devmockUninstallTarget(path string) (dir string, marked bool, err error) {
	dir = filepath.Dir(path)
	marker := filepath.Join(dir, devmockMarkerName)
	if _, statErr := os.Stat(marker); statErr == nil {
		return dir, true, nil
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return dir, false, fmt.Errorf("stat %s: %w", marker, statErr)
	}
	return dir, false, nil
}

func (r mockRunner) install(spec runSpec) (int, error) {
	name, err := devmockGameName(spec)
	if err != nil {
		return 0, err
	}
	dest := spec.Destination
	if dest == "" {
		gamesPath := ""
		if r.gamesPath != nil {
			gamesPath = r.gamesPath()
		}
		if gamesPath == "" {
			return 0, errDevmockNoGamesPath
		}
		dest = filepath.Join(gamesPath, name)
	}
	// mock only: a real installer's package format decides directory
	// permissions (invariant 8); the mock has no package to take them from.
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return 0, fmt.Errorf("create %s: %w", dest, err)
	}
	if err := storage.WriteAtomic(filepath.Join(dest, name+".exe"), devmockExePayload()); err != nil {
		return 0, err
	}
	installer := spec.InstallerPath
	if installer == "" {
		installer = spec.Path
	}
	if err := storage.WriteAtomic(filepath.Join(dest, devmockMarkerName), []byte(installer)); err != nil {
		return 0, err
	}
	if spec.LogPath != "" {
		if err := os.MkdirAll(filepath.Dir(spec.LogPath), 0o755); err != nil {
			return 0, fmt.Errorf("create %s: %w", filepath.Dir(spec.LogPath), err)
		}
		if err := storage.WriteAtomic(spec.LogPath, []byte("devmock install: "+installer+"\n")); err != nil {
			return 0, err
		}
	}
	return 0, nil
}

func devmockGameName(spec runSpec) (string, error) {
	source := spec.InstallerPath
	if source == "" {
		source = spec.Path
	}
	base := filepath.Base(source)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	lower := strings.ToLower(base)
	for _, suffix := range devmockSetupSuffixes {
		if strings.HasSuffix(lower, suffix) {
			base = base[:len(base)-len(suffix)]
			break
		}
	}
	name := strings.TrimSpace(base)
	if name == "" {
		return "", errDevmockNoInstallName
	}
	return name, nil
}

func devmockExePayload() []byte {
	const header = "typhon-devmock-executable\n"
	payload := make([]byte, devmockExeSize)
	copy(payload, header)
	return payload
}
