package extlib

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"typhon/internal/library"
	"typhon/internal/platform"
)

var errEpicManifest = errors.New("некорректный манифест Epic")

type epicItem struct {
	DisplayName      string `json:"DisplayName"`
	InstallLocation  string `json:"InstallLocation"`
	AppName          string `json:"AppName"`
	CatalogNamespace string `json:"CatalogNamespace"`
	CatalogItemID    string `json:"CatalogItemId"`
	LaunchExecutable string `json:"LaunchExecutable"`
	MainGameAppName  string `json:"MainGameAppName"`
	Incomplete       bool   `json:"bIsIncompleteInstall"`
}

func parseEpicItem(data []byte) (epicItem, error) {
	var item epicItem
	if err := json.Unmarshal(data, &item); err != nil {
		return epicItem{}, fmt.Errorf("%w: %w", errEpicManifest, err)
	}
	return item, nil
}

func epicLaunchURI(item epicItem) string {
	return "com.epicgames.launcher://apps/" + item.CatalogNamespace + "%3A" + item.CatalogItemID + "%3A" + item.AppName +
		"?action=launch&silent=true"
}

func scanEpic(ctx context.Context, manifests string) ProviderScan {
	res := ProviderScan{Provider: library.ProviderEpic}
	entries, err := os.ReadDir(manifests)
	if errors.Is(err, fs.ErrNotExist) {
		res.Err = ErrNotInstalled
		return res
	}
	if err != nil {
		res.Err = fmt.Errorf("read %s: %w", manifests, err)
		return res
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			res.Err = err
			return res
		}
		name := entry.Name()
		if !entry.Type().IsRegular() || !strings.EqualFold(filepath.Ext(name), ".item") {
			continue
		}
		data, err := readLimited(filepath.Join(manifests, name))
		if err != nil {
			res.Skipped = append(res.Skipped, Skipped{Title: name, Reason: err.Error()})
			continue
		}
		item, err := parseEpicItem(data)
		if err != nil {
			res.Skipped = append(res.Skipped, Skipped{Title: name, Reason: err.Error()})
			continue
		}
		if item.MainGameAppName != "" && item.MainGameAppName != item.AppName {
			continue
		}
		title := cleanTitle(item.DisplayName)
		if title == "" {
			title = name
		}
		cand, err := epicCandidate(item, title)
		if err != nil {
			res.Skipped = append(res.Skipped, Skipped{Title: title, Reason: err.Error()})
			continue
		}
		if seen[cand.AppID] {
			continue
		}
		seen[cand.AppID] = true
		res.Games = append(res.Games, cand)
	}
	return res
}

func epicCandidate(item epicItem, title string) (Candidate, error) {
	if item.Incomplete {
		return Candidate{}, errors.New("установка в Epic Games не завершена")
	}
	if !isToken(item.AppName) || !isToken(item.CatalogNamespace) || !isToken(item.CatalogItemID) {
		return Candidate{}, fmt.Errorf("%w: недопустимые идентификаторы приложения", errEpicManifest)
	}
	dir, err := checkDir(item.InstallLocation)
	if err != nil {
		return Candidate{}, err
	}
	cand := Candidate{
		Provider:   library.ProviderEpic,
		AppID:      item.AppName,
		Title:      title,
		InstallDir: dir,
		LaunchURI:  epicLaunchURI(item),
	}
	if rel := strings.TrimSpace(item.LaunchExecutable); rel != "" && !filepath.IsAbs(rel) && !hasDotDot(rel) {
		exe := filepath.Join(dir, rel)
		info, err := os.Stat(exe)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			// LaunchExecutable не обязан существовать: запуск идёт по URI, пустой Executable ему не мешает.
		case err != nil:
			return Candidate{}, fmt.Errorf("stat %s: %w", exe, err)
		case info.Mode().IsRegular() && platform.Inside(dir, exe):
			cand.Executable = exe
		}
	}
	return cand, nil
}
