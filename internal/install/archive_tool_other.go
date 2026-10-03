//go:build !windows

package install

import (
	"context"
	"path/filepath"
	"syscall"
)

func toolProcAttr() *syscall.SysProcAttr {
	return nil
}

// Приложение, запущенное из Finder, не видит PATH из профиля оболочки, поэтому
// каталоги Homebrew перечислены явно.
var toolSearchDirs = []string{"/opt/homebrew/bin", "/usr/local/bin", "/usr/bin"}

// WhatsNew.txt WinRAR: уязвимости пути в 7.12 и 7.13 касаются только
// Windows-версий, Unix-версии не затронуты, поэтому нижней границы здесь нет.
var unrarFloor toolVersion

func findArchiveTools(ctx context.Context) toolSet {
	var set toolSet
	if path := firstRegularFile(toolCandidates("unrar")); path != "" {
		tool, err := newUnrar(ctx, path)
		set.add(tool, err)
	}
	if path := firstRegularFile(append(toolCandidates("7zz"), toolCandidates("7z")...)); path != "" {
		tool, err := newSevenZip(ctx, path)
		set.add(tool, err)
	}
	return set
}

func toolCandidates(name string) []string {
	out := []string{lookPathAbs(name)}
	for _, dir := range toolSearchDirs {
		out = append(out, filepath.Join(dir, name))
	}
	return out
}
