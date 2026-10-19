package extlib

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"typhon/internal/library"
	"typhon/internal/platform"
)

const steamFullyInstalled = 4

var errSteamManifest = errors.New("некорректный манифест Steam")

type steamApp struct {
	AppID      string
	Name       string
	InstallDir string
	Flags      int
}

func parseLibraryFolders(data []byte) ([]string, error) {
	doc, err := parseVDF(data)
	if err != nil {
		return nil, err
	}
	folders := doc.child("libraryfolders")
	if folders == nil || folders.children == nil {
		return nil, fmt.Errorf("%w: нет блока libraryfolders", errVDF)
	}
	var out []string
	for key, entry := range folders.children {
		if !isDigits(key, 6) {
			continue
		}
		path := entry.value
		if entry.children != nil {
			path = entry.str("path")
		}
		if strings.TrimSpace(path) != "" {
			out = append(out, path)
		}
	}
	return out, nil
}

func parseAppManifest(data []byte) (steamApp, error) {
	doc, err := parseVDF(data)
	if err != nil {
		return steamApp{}, err
	}
	state := doc.child("AppState")
	if state == nil || state.children == nil {
		return steamApp{}, fmt.Errorf("%w: нет блока AppState", errSteamManifest)
	}
	app := steamApp{
		AppID:      strings.TrimSpace(state.str("appid")),
		Name:       state.str("name"),
		InstallDir: state.str("installdir"),
	}
	if !isDigits(app.AppID, 12) {
		return steamApp{}, fmt.Errorf("%w: appid %q", errSteamManifest, app.AppID)
	}
	if raw := strings.TrimSpace(state.str("StateFlags")); raw != "" {
		flags, err := strconv.Atoi(raw)
		if err != nil {
			return steamApp{}, fmt.Errorf("%w: StateFlags %q", errSteamManifest, raw)
		}
		app.Flags = flags
	}
	return app, nil
}

func isSteamTool(app steamApp) bool {
	if app.AppID == "228980" {
		return true
	}
	for _, prefix := range []string{"Proton ", "Steam Linux Runtime", "Steamworks Shared"} {
		if strings.HasPrefix(app.Name, prefix) {
			return true
		}
	}
	return false
}

func steamLibraryPaths(root string) ([]string, error) {
	libs := []string{root}
	data, err := readLimited(filepath.Join(root, "steamapps", "libraryfolders.vdf"))
	if errors.Is(err, fs.ErrNotExist) {
		return libs, nil
	}
	if err != nil {
		return nil, fmt.Errorf("libraryfolders.vdf: %w", err)
	}
	extra, err := parseLibraryFolders(data)
	if err != nil {
		return nil, fmt.Errorf("libraryfolders.vdf: %w", err)
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(libs)+len(extra))
	for _, p := range append(libs, extra...) {
		key, err := platform.PathKey(p)
		if err != nil {
			out = append(out, p)
			continue
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, p)
	}
	return out, nil
}

func isSteamGameType(typ string) bool {
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "game", "demo", "beta", "mod":
		return true
	}
	return false
}

func steamManifestIDs(libs []string) (map[uint32]bool, error) {
	ids := map[uint32]bool{}
	for _, lib := range libs {
		libDir, err := checkDir(lib)
		if err != nil {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(libDir, "steamapps"))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", lib, err)
		}
		for _, e := range entries {
			name := e.Name()
			if !strings.HasPrefix(name, "appmanifest_") || !strings.HasSuffix(name, ".acf") {
				continue
			}
			raw := strings.TrimSuffix(strings.TrimPrefix(name, "appmanifest_"), ".acf")
			if !isDigits(raw, 10) {
				continue
			}
			id, err := strconv.ParseUint(raw, 10, 32)
			if err != nil {
				continue
			}
			ids[uint32(id)] = true
		}
	}
	return ids, nil
}

func steamAppTypes(ctx context.Context, cache *appTypeCache, root string, libs []string) (map[string]string, error) {
	ids, err := steamManifestIDs(libs)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	types, err := cache.get(ctx, filepath.Join(root, "appcache", "appinfo.vdf"), ids)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("appinfo.vdf: %w", err)
	}
	return types, nil
}

func scanSteam(ctx context.Context, root string) ProviderScan {
	return scanSteamWith(ctx, steamTypeCache, root)
}

func scanSteamWith(ctx context.Context, cache *appTypeCache, root string) ProviderScan {
	res := ProviderScan{Provider: library.ProviderSteam}
	libs, err := steamLibraryPaths(root)
	if err != nil {
		res.Err = err
		return res
	}
	types, err := steamAppTypes(ctx, cache, root, libs)
	if err != nil {
		res.Err = err
		return res
	}
	seen := map[string]bool{}
	for _, lib := range libs {
		libDir, err := checkDir(lib)
		if err != nil {
			res.Skipped = append(res.Skipped, Skipped{Title: lib, Reason: "библиотека Steam недоступна: " + err.Error()})
			continue
		}
		if err := scanSteamLibrary(ctx, libDir, types, seen, &res); err != nil {
			res.Err = err
			return res
		}
	}
	return res
}

func scanSteamLibrary(ctx context.Context, libDir string, types map[string]string, seen map[string]bool, res *ProviderScan) error {
	apps := filepath.Join(libDir, "steamapps")
	entries, err := os.ReadDir(apps)
	if errors.Is(err, fs.ErrNotExist) {
		res.Skipped = append(res.Skipped, Skipped{Title: libDir, Reason: "в библиотеке Steam нет каталога steamapps"})
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", apps, err)
	}
	common := filepath.Join(apps, "common")
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "appmanifest_") || !strings.HasSuffix(name, ".acf") {
			continue
		}
		if !entry.Type().IsRegular() {
			res.Skipped = append(res.Skipped, Skipped{Title: name, Reason: "манифест не является обычным файлом"})
			continue
		}
		data, err := readLimited(filepath.Join(apps, name))
		if err != nil {
			res.Skipped = append(res.Skipped, Skipped{Title: name, Reason: err.Error()})
			continue
		}
		app, err := parseAppManifest(data)
		if err != nil {
			res.Skipped = append(res.Skipped, Skipped{Title: name, Reason: err.Error()})
			continue
		}
		if isSteamTool(app) || seen[app.AppID] {
			continue
		}
		if typ, known := types[app.AppID]; known && !isSteamGameType(typ) {
			seen[app.AppID] = true
			res.NotGames = append(res.NotGames, app.AppID)
			continue
		}
		title := cleanTitle(app.Name)
		if title == "" {
			title = name
		}
		if app.Flags&steamFullyInstalled == 0 {
			res.Skipped = append(res.Skipped, Skipped{Title: title, Reason: "установка в Steam не завершена", AppID: app.AppID})
			continue
		}
		dir, err := steamInstallDir(common, app.InstallDir)
		if err != nil {
			res.Skipped = append(res.Skipped, Skipped{Title: title, Reason: err.Error(), AppID: app.AppID})
			continue
		}
		seen[app.AppID] = true
		res.Games = append(res.Games, Candidate{
			Provider:   library.ProviderSteam,
			AppID:      app.AppID,
			Title:      title,
			InstallDir: dir,
			LaunchURI:  "steam://rungameid/" + app.AppID,
		})
	}
	return nil
}

func steamInstallDir(common, installdir string) (string, error) {
	installdir = strings.TrimSpace(installdir)
	if installdir == "" {
		return "", errors.New("в манифесте не указан installdir")
	}
	if filepath.IsAbs(installdir) || hasDotDot(installdir) {
		return "", fmt.Errorf("недопустимый installdir %q", installdir)
	}
	full := filepath.Join(common, installdir)
	if !platform.Inside(common, full) {
		return "", fmt.Errorf("installdir %q вне каталога common", installdir)
	}
	return checkDir(full)
}
