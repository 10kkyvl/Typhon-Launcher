//go:build windows

package install

import (
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

func findArchiveTools() []archiveTool {
	var tools []archiveTool
	if path := firstRegularFile(unrarCandidates()); path != "" {
		tools = append(tools, unrarTool(path))
	}
	if path := firstRegularFile(sevenZipCandidates()); path != "" {
		tools = append(tools, sevenZipTool(path))
	}
	return tools
}

func unrarCandidates() []string {
	var out []string
	for _, exe := range registryValues(`SOFTWARE\WinRAR`, "exe64", "exe32") {
		out = append(out, filepath.Join(filepath.Dir(exe), "UnRAR.exe"))
	}
	for _, dir := range programFilesDirs() {
		out = append(out, filepath.Join(dir, "WinRAR", "UnRAR.exe"))
	}
	return append(out, lookPathAbs("UnRAR.exe"))
}

func sevenZipCandidates() []string {
	var out []string
	for _, dir := range registryValues(`SOFTWARE\7-Zip`, "Path64", "Path") {
		out = append(out, filepath.Join(dir, "7z.exe"))
	}
	for _, dir := range programFilesDirs() {
		out = append(out, filepath.Join(dir, "7-Zip", "7z.exe"))
	}
	return append(out, lookPathAbs("7z.exe"))
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
