package install

import (
	"errors"
	"fmt"
	"path/filepath"
)

var (
	errShellReparsePoint = errors.New("запись снимка ярлыков оказалась ссылкой")
	errShellEscaped      = errors.New("запись снимка ярлыков ведёт за пределы своего каталога")
	errShellKindChanged  = errors.New("запись снимка ярлыков сменила тип")
	errShellDirNotEmpty  = errors.New("каталог ярлыков не пуст")
	errShellRootUnknown  = errors.New("каталог ярлыков не проверен при снимке")
	errShellRootChanged  = errors.New("каталог ярлыков подменён после снимка")

	// Подменяется в тестах: отказ удаления воспроизводится без прав файловой системы.
	removeShellEntry = func(entry *shellEntryFile) error { return entry.remove() }
)

// openShellEntry открывает запись снимка так, чтобы проверка, чтение и
// удаление шли через один открытый объект, а не через повторный разбор пути:
// общий рабочий стол пишется и без прав администратора, и подменить
// промежуточный каталог ссылкой между проверкой и удалением можно извне.
// Корень сверяется с тем, каким его застал снимок snap.
func openShellEntry(snap shellSnapshot, path string, dir bool) (*shellEntryFile, error) {
	root, rel, err := shellRel(snap.roots, path)
	if err != nil {
		return nil, err
	}
	id, ok := snap.rootIDs[root]
	if !ok {
		return nil, fmt.Errorf("%w: %s", errShellRootUnknown, root)
	}
	return openShellEntryIn(root, rel, id, dir)
}

func shellRel(roots []string, path string) (string, string, error) {
	best := ""
	for _, root := range roots {
		if root == "" || samePath(root, path) || !inside(root, path) {
			continue
		}
		if len(root) > len(best) {
			best = root
		}
	}
	if best == "" {
		return "", "", fmt.Errorf("%w: %s", errShellEscaped, path)
	}
	rel, err := filepath.Rel(best, path)
	if err != nil {
		return "", "", fmt.Errorf("%w: %s: %w", errShellEscaped, path, err)
	}
	if !filepath.IsLocal(rel) {
		return "", "", fmt.Errorf("%w: %s", errShellEscaped, path)
	}
	return best, rel, nil
}
