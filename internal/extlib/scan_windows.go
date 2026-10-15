//go:build windows

package extlib

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"

	"typhon/internal/library"
)

func Scan(ctx context.Context) ([]ProviderScan, error) {
	return []ProviderScan{
		scanSteamInstalled(ctx),
		scanEpicInstalled(ctx),
		scanGOG(ctx, systemRegistry{}),
	}, ctx.Err()
}

func scanSteamInstalled(ctx context.Context) ProviderScan {
	root, err := steamRoot()
	if err != nil {
		return ProviderScan{Provider: library.ProviderSteam, Err: err}
	}
	return scanSteam(ctx, root)
}

func scanEpicInstalled(ctx context.Context) ProviderScan {
	programData := os.Getenv("ProgramData")
	if programData == "" {
		return ProviderScan{Provider: library.ProviderEpic, Err: errors.New("переменная окружения ProgramData не задана")}
	}
	return scanEpic(ctx, filepath.Join(programData, "Epic", "EpicGamesLauncher", "Data", "Manifests"))
}

func steamRoot() (string, error) {
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

type systemRegistry struct{}

func (systemRegistry) SubKeys(path string) ([]string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.ENUMERATE_SUB_KEYS|registry.WOW64_64KEY)
	if err != nil {
		return nil, registryErr(path, err)
	}
	names, err := k.ReadSubKeyNames(-1)
	closeErr := k.Close()
	if err != nil {
		return nil, registryErr(path, err)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close %s: %w", path, closeErr)
	}
	return names, nil
}

func (systemRegistry) Values(path string) (map[string]string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return nil, registryErr(path, err)
	}
	out, err := readStrings(k)
	closeErr := k.Close()
	if err != nil {
		return nil, registryErr(path, err)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close %s: %w", path, closeErr)
	}
	return out, nil
}

func readStrings(k registry.Key) (map[string]string, error) {
	names, err := k.ReadValueNames(-1)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(names))
	for _, name := range names {
		value, _, err := k.GetStringValue(name)
		if errors.Is(err, registry.ErrUnexpectedType) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out[strings.ToLower(name)] = value
	}
	return out, nil
}
