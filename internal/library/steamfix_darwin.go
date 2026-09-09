//go:build darwin && !devmock

package library

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"typhon/internal/wine"
)

const (
	steamFixObserveInterval = 500 * time.Millisecond
	steamFixObserveTimeout  = 30 * time.Second
	steamLogReadLimit       = 1 << 20
)

type steamFixInfo struct {
	Kind      string
	Path      string
	RealAppID uint64
	FakeAppID uint64
}

type steamLogCursor struct {
	Offset int64
	Tail   string
}

type steamAppObservation struct {
	AppID   uint64
	WinePID int
}

var steamTrackedProcess = regexp.MustCompile(`(?i)AppID\s+(\d+)\s+adding PID\s+(\d+)\s+as a tracked process`)

// detectSteamFix находит конфигурацию именно рядом с запускаемой программой.
// Поиск по всей установке здесь был бы ложноположительным: в репаке могут
// остаться неиспользуемые фиксы от другого exe.
func detectSteamFix(executable, workDir string) (steamFixInfo, bool) {
	dirs := []string{filepath.Dir(executable)}
	if workDir != "" && !strings.EqualFold(filepath.Clean(workDir), filepath.Clean(dirs[0])) {
		dirs = append(dirs, workDir)
	}
	for _, dir := range dirs {
		path, ok := localFile(dir, "SteamFix.ini")
		if !ok {
			continue
		}
		info, err := readSteamFix(path)
		if err != nil {
			slog.Warn("read steam fix config", "path", path, "error", err)
			continue
		}
		return info, true
	}
	return steamFixInfo{}, false
}

func localFile(dir, name string) (string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(entry.Name(), name) {
			return filepath.Join(dir, entry.Name()), true
		}
	}
	return "", false
}

func readSteamFix(path string) (steamFixInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return steamFixInfo{}, err
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			slog.Debug("close steam fix config", "path", path, "error", closeErr)
		}
	}()

	info := steamFixInfo{Kind: "SteamFix", Path: path}
	section := ""
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "\ufeff"))
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			if section == "freetp" {
				info.Kind = "FreeTP"
			}
			continue
		}
		if section != "main" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		id, parseErr := strconv.ParseUint(strings.TrimSpace(value), 10, 32)
		if parseErr != nil {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "realappid":
			info.RealAppID = id
		case "fakeappid":
			info.FakeAppID = id
		}
	}
	if err := scanner.Err(); err != nil {
		return steamFixInfo{}, err
	}
	if info.FakeAppID == 0 {
		return steamFixInfo{}, fmt.Errorf("FakeAppId не задан")
	}
	return info, nil
}

func steamGameProcessLogPath(bottle wine.Bottle) string {
	root := filepath.Join(bottle.Path, "drive_c")
	programFiles := []string{"Program Files (x86)", "Program Files"}
	for _, dir := range programFiles {
		path := filepath.Join(root, dir, "Steam", "logs", "gameprocess_log.txt")
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	for _, dir := range programFiles {
		steamRoot := filepath.Join(root, dir, "Steam")
		if _, err := os.Stat(filepath.Join(steamRoot, "steam.exe")); err == nil {
			return filepath.Join(steamRoot, "logs", "gameprocess_log.txt")
		}
	}
	return filepath.Join(root, "Program Files (x86)", "Steam", "logs", "gameprocess_log.txt")
}

func steamGameProcessLogCursor(bottle wine.Bottle) steamLogCursor {
	stat, err := os.Stat(steamGameProcessLogPath(bottle))
	if err != nil {
		return steamLogCursor{}
	}
	return steamLogCursor{Offset: stat.Size()}
}

// monitorSteamAppID не доверяет глобальному RunningAppID в реестре: при
// SteamFix он бывает намеренно равен RealAppId. Источник истины — новая
// запись Steam, которая связывает конкретный путь exe с отслеживаемым AppID.
func monitorSteamAppID(
	ctx context.Context,
	bottle wine.Bottle,
	winExe string,
	fix steamFixInfo,
	cursor steamLogCursor,
) {
	ticker := time.NewTicker(steamFixObserveInterval)
	defer ticker.Stop()
	timer := time.NewTimer(steamFixObserveTimeout)
	defer timer.Stop()
	logPath := steamGameProcessLogPath(bottle)
	for {
		observation, next, ok, err := observeSteamLog(logPath, winExe, cursor)
		cursor = next
		if err != nil && !os.IsNotExist(err) {
			slog.Debug("read steam game process log", "path", logPath, "error", err)
		}
		if ok {
			fields := []any{
				"kind", fix.Kind, "executable", winExe, "winePID", observation.WinePID,
				"actualAppID", observation.AppID, "expectedAppID", fix.FakeAppID,
				"realAppID", fix.RealAppID, "config", fix.Path,
			}
			switch observation.AppID {
			case fix.FakeAppID:
				slog.Info("steam app id verified", fields...)
			case fix.RealAppID:
				slog.Warn("steam app id stayed real; steam fix did not redirect it", fields...)
			default:
				slog.Warn("steam app id is unexpected", fields...)
			}
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			slog.Warn("steam api registration not observed",
				"kind", fix.Kind, "executable", winExe,
				"expectedAppID", fix.FakeAppID, "realAppID", fix.RealAppID,
				"config", fix.Path, "log", logPath)
			return
		case <-ticker.C:
		}
	}
}

func observeSteamLog(path, winExe string, cursor steamLogCursor) (steamAppObservation, steamLogCursor, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return steamAppObservation{}, cursor, false, err
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			slog.Debug("close steam game process log", "path", path, "error", closeErr)
		}
	}()

	stat, err := f.Stat()
	if err != nil {
		return steamAppObservation{}, cursor, false, err
	}
	if stat.Size() < cursor.Offset {
		// Steam rotated/truncated the log after the launch cursor was captured.
		cursor = steamLogCursor{}
	}
	if _, err := f.Seek(cursor.Offset, io.SeekStart); err != nil {
		return steamAppObservation{}, cursor, false, err
	}
	raw, err := io.ReadAll(io.LimitReader(f, steamLogReadLimit))
	if err != nil {
		return steamAppObservation{}, cursor, false, err
	}
	cursor.Offset += int64(len(raw))
	text := cursor.Tail + string(raw)
	lines := strings.Split(text, "\n")
	if !strings.HasSuffix(text, "\n") {
		cursor.Tail = lines[len(lines)-1]
		lines = lines[:len(lines)-1]
	} else {
		cursor.Tail = ""
	}
	if len(cursor.Tail) > 16<<10 {
		cursor.Tail = cursor.Tail[len(cursor.Tail)-(16<<10):]
	}
	for _, line := range lines {
		observation, ok := parseSteamTrackedProcess(line, winExe)
		if ok {
			return observation, cursor, true, nil
		}
	}
	return steamAppObservation{}, cursor, false, nil
}

func parseSteamTrackedProcess(line, winExe string) (steamAppObservation, bool) {
	if !strings.Contains(strings.ToLower(line), strings.ToLower(winExe)) {
		return steamAppObservation{}, false
	}
	match := steamTrackedProcess.FindStringSubmatch(line)
	if match == nil {
		return steamAppObservation{}, false
	}
	appID, appErr := strconv.ParseUint(match[1], 10, 32)
	pid, pidErr := strconv.Atoi(match[2])
	if appErr != nil || pidErr != nil {
		return steamAppObservation{}, false
	}
	return steamAppObservation{AppID: appID, WinePID: pid}, true
}
