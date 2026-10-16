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
	"typhon/internal/steamlink"
)

func Scan(ctx context.Context) ([]ProviderScan, error) {
	return []ProviderScan{
		scanSteamInstalled(ctx),
		scanEpicInstalled(ctx),
		scanGOG(ctx, systemRegistry{}),
	}, ctx.Err()
}

func scanSteamInstalled(ctx context.Context) ProviderScan {
	root, err := steamlink.Root()
	if errors.Is(err, steamlink.ErrNotInstalled) {
		return ProviderScan{Provider: library.ProviderSteam, Err: ErrNotInstalled}
	}
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
