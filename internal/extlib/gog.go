package extlib

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"typhon/internal/library"
	"typhon/internal/platform"
)

const gogGamesKey = `SOFTWARE\WOW6432Node\GOG.com\Games`

type registryReader interface {
	SubKeys(path string) ([]string, error)
	Values(path string) (map[string]string, error)
}

func scanGOG(ctx context.Context, reg registryReader) ProviderScan {
	res := ProviderScan{Provider: library.ProviderGOG}
	ids, err := reg.SubKeys(gogGamesKey)
	if errors.Is(err, fs.ErrNotExist) {
		res.Err = ErrNotInstalled
		return res
	}
	if err != nil {
		res.Err = fmt.Errorf("read %s: %w", gogGamesKey, err)
		return res
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			res.Err = err
			return res
		}
		values, err := reg.Values(gogGamesKey + `\` + id)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			res.Err = fmt.Errorf("read %s\\%s: %w", gogGamesKey, id, err)
			return res
		}
		title := cleanTitle(values["gamename"])
		if title == "" {
			title = id
		}
		cand, err := gogCandidate(id, title, values)
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

func gogCandidate(id, title string, values map[string]string) (Candidate, error) {
	appID := strings.TrimSpace(values["gameid"])
	if appID == "" {
		appID = id
	}
	if !isDigits(appID, 20) {
		return Candidate{}, fmt.Errorf("недопустимый идентификатор GOG %q", appID)
	}
	dir, err := checkDir(values["path"])
	if err != nil {
		return Candidate{}, err
	}
	exe := strings.TrimSpace(values["exe"])
	if exe == "" {
		file := strings.TrimSpace(values["exefile"])
		if file == "" {
			return Candidate{}, errors.New("в реестре GOG не указан исполняемый файл")
		}
		exe = filepath.Join(dir, file)
	}
	if hasDotDot(exe) {
		return Candidate{}, fmt.Errorf("путь к исполняемому файлу содержит '..': %s", exe)
	}
	if !filepath.IsAbs(exe) {
		exe = filepath.Join(dir, exe)
	}
	exe = filepath.Clean(exe)
	info, err := os.Stat(exe)
	if err != nil {
		return Candidate{}, fmt.Errorf("исполняемый файл недоступен: %w", err)
	}
	if !info.Mode().IsRegular() {
		return Candidate{}, fmt.Errorf("не файл: %s", exe)
	}
	if !platform.Inside(dir, exe) {
		return Candidate{}, fmt.Errorf("исполняемый файл вне каталога игры: %s", exe)
	}
	return Candidate{
		Provider:   library.ProviderGOG,
		AppID:      appID,
		Title:      title,
		InstallDir: dir,
		Executable: exe,
	}, nil
}
