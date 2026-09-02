//go:build devmock && !windows

package install

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"typhon/internal/storage"
)

const (
	devmockMarkerName = ".typhon-devmock"
	devmockExeSize    = 64 << 10

	devmockElevateEnv            = "TYPHON_DEVMOCK_ELEVATE"
	devmockInstallSecondsEnv     = "TYPHON_DEVMOCK_INSTALL_SECONDS"
	devmockDefaultInstallSeconds = 2
)

var (
	errDevmockUnmarkedDir         = errors.New("devmock: отказ удалить каталог без метки установки")
	errDevmockNoGamesPath         = errors.New("devmock: путь библиотеки игр не задан")
	errDevmockNoInstallName       = errors.New("devmock: не удалось определить имя игры")
	errDevmockInvalidElevateFlag  = errors.New("devmock: TYPHON_DEVMOCK_ELEVATE должен быть 0 или 1")
	errDevmockInvalidInstallDelay = errors.New("devmock: TYPHON_DEVMOCK_INSTALL_SECONDS должен быть неотрицательным целым числом")
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
	if spec.StatePath != "" {
		elevate, err := devmockElevationEnabled()
		if err != nil {
			return 0, err
		}
		if elevate {
			return runElevated(ctx, spec)
		}
	}
	return r.install(ctx, spec)
}

// devmockElevationEnabled — единственный переключатель настоящего цикла
// повышенного воркера в devmock (TYPHON_DEVMOCK_ELEVATE): по умолчанию (пусто
// или "1") тихие установки всегда идут через runElevated, как на Windows, а
// "0" оставляет прямую фейковую установку для тестов, которым сам протокол
// spec/state/cancel не нужен.
func devmockElevationEnabled() (bool, error) {
	switch raw := os.Getenv(devmockElevateEnv); raw {
	case "", "1":
		return true, nil
	case "0":
		return false, nil
	default:
		return false, fmt.Errorf("%w: %q", errDevmockInvalidElevateFlag, raw)
	}
}

// devmockInstallDelay делает фейковую установку наблюдаемой: без задержки
// прогресс и отмена никогда не были бы видны — прямой путь писал файлы
// мгновенно.
func devmockInstallDelay() (time.Duration, error) {
	raw := os.Getenv(devmockInstallSecondsEnv)
	if raw == "" {
		return devmockDefaultInstallSeconds * time.Second, nil
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds < 0 {
		return 0, fmt.Errorf("%w: %q", errDevmockInvalidInstallDelay, raw)
	}
	return time.Duration(seconds) * time.Second, nil
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

func (r mockRunner) install(ctx context.Context, spec runSpec) (int, error) {
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

	delay, err := devmockInstallDelay()
	if err != nil {
		return 0, err
	}
	if delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-timer.C:
		}
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
