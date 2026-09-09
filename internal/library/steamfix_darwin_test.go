//go:build darwin && !devmock

package library

import (
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/wine"
)

func TestDetectSteamFix(t *testing.T) {
	dir := t.TempDir()
	config := "\ufeff[Main]\r\nRealAppId=1001270\r\nFakeAppId=480\r\n\r\n[FreeTP]\r\nId=5488\r\n"
	if err := os.WriteFile(filepath.Join(dir, "STEAMFIX.INI"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}

	got, ok := detectSteamFix(filepath.Join(dir, "game.exe"), "")
	if !ok {
		t.Fatal("detectSteamFix did not find a case-insensitive SteamFix.ini")
	}
	if got.Kind != "FreeTP" || got.RealAppID != 1001270 || got.FakeAppID != 480 {
		t.Fatalf("detectSteamFix = %+v", got)
	}
}

func TestDetectSteamFixRejectsConfigWithoutFakeAppID(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SteamFix.ini"), []byte("[Main]\nRealAppId=1001270\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := detectSteamFix(filepath.Join(dir, "game.exe"), ""); ok {
		t.Fatal("detectSteamFix accepted a config without FakeAppId")
	}
}

func TestParseSteamTrackedProcess(t *testing.T) {
	winExe := `Y:\Games\Kebab Chefs\Kebab.exe`
	line := `[2026-09-09 00:22:49] AppID 480 adding PID 2040 as a tracked process ""Y:\Games\Kebab Chefs\Kebab.exe""`
	got, ok := parseSteamTrackedProcess(line, winExe)
	if !ok {
		t.Fatal("parseSteamTrackedProcess did not match the executable")
	}
	if got.AppID != 480 || got.WinePID != 2040 {
		t.Fatalf("parseSteamTrackedProcess = %+v", got)
	}
	if _, ok := parseSteamTrackedProcess(line, `Y:\Games\Other\game.exe`); ok {
		t.Fatal("parseSteamTrackedProcess matched another executable")
	}
}

func TestObserveSteamLogStartsAtLaunchCursorAndKeepsPartialLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gameprocess_log.txt")
	old := "[old] AppID 480 adding PID 1 as a tracked process \"Y:\\Games\\Demo\\game.exe\"\n"
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	cursor := steamLogCursor{Offset: int64(len(old))}
	partial := "[new] AppID 1001270 adding PID 42 as a tracked process \"Y:\\Games\\Demo\\game"
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(partial); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	if _, next, ok, err := observeSteamLog(path, `Y:\Games\Demo\game.exe`, cursor); err != nil || ok || next.Tail == "" {
		t.Fatalf("first observe = ok %v, cursor %+v, err %v", ok, next, err)
	} else {
		cursor = next
	}
	f, err = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(".exe\"\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	got, _, ok, err := observeSteamLog(path, `Y:\Games\Demo\game.exe`, cursor)
	if err != nil || !ok {
		t.Fatalf("second observe = ok %v, err %v", ok, err)
	}
	if got.AppID != 1001270 || got.WinePID != 42 {
		t.Fatalf("second observe = %+v", got)
	}
}

func TestSteamGameProcessLogPathSupportsProgramFiles(t *testing.T) {
	bottle := wine.Bottle{Path: t.TempDir()}
	want := filepath.Join(bottle.Path, "drive_c", "Program Files", "Steam", "logs", "gameprocess_log.txt")
	if err := os.MkdirAll(filepath.Dir(want), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(want, nil, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if got := steamGameProcessLogPath(bottle); got != want {
		t.Fatalf("steamGameProcessLogPath = %q, want %q", got, want)
	}
}

func TestSteamGameProcessLogPathUsesSteamInstallBeforeLogExists(t *testing.T) {
	bottle := wine.Bottle{Path: t.TempDir()}
	steamExe := filepath.Join(bottle.Path, "drive_c", "Program Files", "Steam", "steam.exe")
	if err := os.MkdirAll(filepath.Dir(steamExe), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(steamExe, nil, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	want := filepath.Join(filepath.Dir(steamExe), "logs", "gameprocess_log.txt")
	if got := steamGameProcessLogPath(bottle); got != want {
		t.Fatalf("steamGameProcessLogPath = %q, want %q", got, want)
	}
}
