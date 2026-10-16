//go:build windows

package steamlink

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows/registry"

	"typhon/internal/procs"
)

const activeProcessKey = `Software\Valve\Steam\ActiveProcess`

func Supported() bool { return true }

// Root returns the Steam installation directory, or ErrNotInstalled when the
// registry points nowhere.
func Root() (string, error) {
	candidates := []struct {
		hive   registry.Key
		path   string
		name   string
		access uint32
	}{
		{registry.CURRENT_USER, `Software\Valve\Steam`, "SteamPath", registry.QUERY_VALUE},
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Valve\Steam`, "InstallPath", registry.QUERY_VALUE | registry.WOW64_64KEY},
	}
	for _, c := range candidates {
		value, err := readString(c.hive, c.path, c.name, c.access)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		root := filepath.Clean(filepath.FromSlash(strings.TrimSpace(value)))
		info, err := os.Stat(root)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("stat %s: %w", root, err)
		}
		if info.IsDir() {
			return root, nil
		}
	}
	return "", ErrNotInstalled
}

func readString(hive registry.Key, path, name string, access uint32) (string, error) {
	k, err := registry.OpenKey(hive, path, access)
	if err != nil {
		return "", registryErr(path, err)
	}
	value, _, err := k.GetStringValue(name)
	closeErr := k.Close()
	if err != nil {
		return "", registryErr(path+`\`+name, err)
	}
	if closeErr != nil {
		return "", fmt.Errorf("close %s: %w", path, closeErr)
	}
	return value, nil
}

func registryErr(path string, err error) error {
	if errors.Is(err, registry.ErrNotExist) {
		return fmt.Errorf("%s: %w", path, fs.ErrNotExist)
	}
	return fmt.Errorf("registry %s: %w", path, err)
}

func steamRunning(ctx context.Context) (bool, error) {
	pid, err := activePID(activeProcessKey)
	if err != nil {
		return false, err
	}
	return runningFromList(ctx, pid, procs.List)
}

// activePID reads the pid Steam records while it runs. A missing key or value
// means Steam never ran or shut down cleanly, which is a legitimate "not
// running"; any other failure is reported.
func activePID(path string) (uint32, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("registry %s: %w", path, err)
	}
	value, _, err := k.GetIntegerValue("pid")
	closeErr := k.Close()
	if errors.Is(err, registry.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("registry %s, значение pid: %w", path, err)
	}
	if closeErr != nil {
		return 0, fmt.Errorf("close %s: %w", path, closeErr)
	}
	if value > 0xFFFFFFFF {
		return 0, fmt.Errorf("registry %s, значение pid %d вне диапазона", path, value)
	}
	return uint32(value), nil
}

func JoinArgs(args []string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = syscall.EscapeArg(a)
	}
	return strings.Join(parts, " ")
}
