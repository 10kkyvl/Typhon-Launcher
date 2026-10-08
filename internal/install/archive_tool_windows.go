//go:build windows

package install

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
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

func findArchiveTools(ctx context.Context) toolSet {
	return findTools(ctx, systemToolSources)
}

func findTools(ctx context.Context, src toolSources) toolSet {
	var set toolSet
	if path := firstRegularFile(unrarCandidates(src)); path != "" {
		tool, err := newUnrar(ctx, path)
		set.add(tool, err)
	}
	if path := firstRegularFile(sevenZipCandidates(src)); path != "" {
		tool, err := newSevenZip(ctx, path)
		set.add(tool, err)
	}
	return set
}

func unrarCandidates(src toolSources) []string {
	var out []string
	for _, exe := range src.registry(`SOFTWARE\WinRAR`, "exe64", "exe32") {
		out = append(out, filepath.Join(filepath.Dir(exe), "UnRAR.exe"))
	}
	for _, dir := range src.programFiles() {
		out = append(out, filepath.Join(dir, "WinRAR", "UnRAR.exe"))
	}
	return append(out, src.lookPath("UnRAR.exe"))
}

func sevenZipCandidates(src toolSources) []string {
	var out []string
	for _, dir := range src.registry(`SOFTWARE\7-Zip`, "Path64", "Path") {
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
		key, err := registry.OpenKey(root, path, registry.QUERY_VALUE|registry.WOW64_64KEY)
		if err != nil {
			logRegistryMiss(path, err)
			continue
		}
		for _, name := range names {
			value, _, err := key.GetStringValue(name)
			if err != nil {
				logRegistryMiss(path+`\`+name, err)
				continue
			}
			out = append(out, value)
		}
		if err := key.Close(); err != nil {
			slog.Warn("close registry key", "path", path, "error", err)
		}
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
