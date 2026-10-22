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
	set.find(ctx, "UnRAR", toolCandidates("unrar"), newUnrar)
	set.find(ctx, "7-Zip", append(toolCandidates("7zz"), toolCandidates("7z")...), newSevenZip)
	return set
}

func toolCandidates(name string) []string {
	out := []string{lookPathAbs(name)}
	for _, dir := range toolSearchDirs {
		out = append(out, filepath.Join(dir, name))
	}
	return out
}
