//go:build !windows

package install

import (
	"path/filepath"
	"syscall"
)

func toolProcAttr() *syscall.SysProcAttr {
	return nil
}

// Приложение, запущенное из Finder, не видит PATH из профиля оболочки, поэтому
// каталоги Homebrew перечислены явно.
var toolSearchDirs = []string{"/opt/homebrew/bin", "/usr/local/bin", "/usr/bin"}

func findArchiveTools() []archiveTool {
	var tools []archiveTool
	if path := firstRegularFile(toolCandidates("unrar")); path != "" {
		tools = append(tools, unrarTool(path))
	}
	if path := firstRegularFile(append(toolCandidates("7zz"), toolCandidates("7z")...)); path != "" {
		tools = append(tools, sevenZipTool(path))
	}
	return tools
}

func toolCandidates(name string) []string {
	out := []string{lookPathAbs(name)}
	for _, dir := range toolSearchDirs {
		out = append(out, filepath.Join(dir, name))
	}
	return out
}
