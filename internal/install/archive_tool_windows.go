//go:build windows

package install

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Консольный распаковщик, запущенный из GUI-процесса, иначе открывает
// собственное окно консоли поверх лаунчера.
func toolProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
}

type toolSources struct {
	registry     func(path string, names ...string) []string
	programFiles func() []string
	lookPath     func(name string) string
}

var systemToolSources = toolSources{registry: registryValues, programFiles: programFilesDirs, lookPath: lookPathAbs}

// WhatsNew.txt WinRAR: в 7.12 и 7.13 закрыты две уязвимости, из-за которых
// Windows-версии RAR и UnRAR берут путь из специально собранного архива вместо
// заданного пользователем; записать за пределы каталога можно, не оставив
// следа внутри него. Unix-версии не затронуты.
var unrarFloor = toolVersion{major: 7, minor: 13}

const (
	appPathsKey  = `SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\`
	uninstallKey = `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\`
)

func findArchiveTools(ctx context.Context) toolSet {
	return findTools(ctx, systemToolSources)
}

func findTools(ctx context.Context, src toolSources) toolSet {
	var set toolSet
	set.find(ctx, "UnRAR", unrarCandidates(src), newUnrar)
	set.find(ctx, "7-Zip", sevenZipCandidates(src), newSevenZip)
	return set
}

// Значения реестра — чужой ввод (инвариант 32): запускается только существующий
// обычный файл, найденный по ним, а значение, не ставшее абсолютным путём,
// отбрасывается при проверке кандидата (regularFiles).
func registryDirs(src toolSources, path string, names ...string) []string {
	var out []string
	for _, value := range src.registry(path, names...) {
		if dir := cleanRegistryPath(value); dir != "" {
			out = append(out, dir)
		}
	}
	return out
}

func registryExeDirs(src toolSources, path string, names ...string) []string {
	var out []string
	for _, value := range src.registry(path, names...) {
		if exe := cleanRegistryPath(value); exe != "" {
			out = append(out, filepath.Dir(exe))
		}
	}
	return out
}

// DisplayIcon хранит путь с индексом значка и иногда в кавычках, например
// "C:\Program Files\WinRAR\WinRAR.exe",0.
func cleanRegistryPath(value string) string {
	value = strings.TrimSpace(value)
	if i := strings.LastIndex(value, ","); i >= 0 {
		if _, err := strconv.Atoi(strings.TrimSpace(value[i+1:])); err == nil {
			value = strings.TrimSpace(value[:i])
		}
	}
	return strings.TrimSpace(strings.Trim(value, `"`))
}

func unrarCandidates(src toolSources) []string {
	var dirs []string
	dirs = append(dirs, registryExeDirs(src, appPathsKey+"WinRAR.exe", "")...)
	dirs = append(dirs, registryExeDirs(src, `SOFTWARE\WinRAR`, "exe64", "exe32")...)
	dirs = append(dirs, registryDirs(src, uninstallKey+"WinRAR archiver", "InstallLocation")...)
	dirs = append(dirs, registryExeDirs(src, uninstallKey+"WinRAR archiver", "DisplayIcon")...)
	var out []string
	for _, dir := range dirs {
		out = append(out, filepath.Join(dir, "UnRAR.exe"))
	}
	for _, dir := range src.programFiles() {
		out = append(out, filepath.Join(dir, "WinRAR", "UnRAR.exe"))
	}
	return append(out, src.lookPath("UnRAR.exe"))
}

func sevenZipCandidates(src toolSources) []string {
	var dirs []string
	dirs = append(dirs, registryDirs(src, `SOFTWARE\7-Zip`, "Path64", "Path")...)
	dirs = append(dirs, registryExeDirs(src, appPathsKey+"7zFM.exe", "")...)
	dirs = append(dirs, registryDirs(src, uninstallKey+"7-Zip", "InstallLocation")...)
	var out []string
	for _, dir := range dirs {
		out = append(out, filepath.Join(dir, "7z.exe"))
	}
	for _, dir := range src.programFiles() {
		out = append(out, filepath.Join(dir, "7-Zip", "7z.exe"))
	}
	return append(out, src.lookPath("7z.exe"))
}

func programFilesDirs() []string {
	var out []string
	for _, name := range []string{"ProgramW6432", "ProgramFiles", "ProgramFiles(x86)"} {
		if dir := os.Getenv(name); dir != "" {
			out = append(out, dir)
		}
	}
	return out
}

func registryValues(path string, names ...string) []string {
	var out []string
	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		for _, view := range []uint32{registry.WOW64_64KEY, registry.WOW64_32KEY} {
			out = append(out, registryViewValues(root, view, path, names)...)
		}
	}
	return out
}

func registryViewValues(root registry.Key, view uint32, path string, names []string) []string {
	key, err := registry.OpenKey(root, path, registry.QUERY_VALUE|view)
	if err != nil {
		logRegistryMiss(path, err)
		return nil
	}
	var out []string
	for _, name := range names {
		value, _, err := key.GetStringValue(name)
		if err != nil {
			logRegistryMiss(path+`\`+name, err)
			continue
		}
		if strings.Contains(value, "%") {
			expanded, err := registry.ExpandString(value)
			if err != nil {
				slog.Warn("expand archive tool registry value", "path", path, "error", err)
				continue
			}
			value = expanded
		}
		out = append(out, value)
	}
	if err := key.Close(); err != nil {
		slog.Warn("close registry key", "path", path, "error", err)
	}
	return out
}

// Отсутствующий ключ — обычный «не установлено»; остальное (нет прав) тоже
// лишь исключает этот источник из поиска, но оставляет след в логе.
func logRegistryMiss(path string, err error) {
	if errors.Is(err, registry.ErrNotExist) {
		return
	}
	slog.Warn("read archive tool registry entry", "path", path, "error", err)
}
