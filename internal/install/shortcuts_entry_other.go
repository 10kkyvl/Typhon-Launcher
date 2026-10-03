//go:build !windows

package install

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

type shellEntryFile struct {
	root string
	rel  string
	id   shellRootID
	path string
	info fs.FileInfo
	file *os.File
}

// shellRootID — сам корень на момент снимка, без перехода по ссылке.
type shellRootID struct {
	info fs.FileInfo
}

func identifyShellRoot(root string) (shellRootID, error) {
	info, err := os.Lstat(root)
	if err != nil {
		return shellRootID{}, err
	}
	return shellRootID{info: info}, nil
}

func openShellEntryIn(root, rel string, id shellRootID, dir bool) (*shellEntryFile, error) {
	info, err := lstatShellChain(root, rel, id, dir)
	if err != nil {
		return nil, err
	}
	entry := &shellEntryFile{root: root, rel: rel, id: id, path: filepath.Join(root, rel), info: info}
	if dir {
		return entry, nil
	}
	file, err := os.OpenFile(entry.path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	opened, err := file.Stat()
	if err == nil && !os.SameFile(info, opened) {
		err = fmt.Errorf("%w: %s", errShellKindChanged, entry.path)
	}
	if err != nil {
		return nil, errors.Join(err, file.Close())
	}
	entry.file = file
	return entry, nil
}

// lstatShellChain проходит путь от корня без перехода по ссылкам: корень
// обязан остаться тем, что застал снимок, и ни один каталог между ним и
// записью не может оказаться символической ссылкой.
func lstatShellChain(root, rel string, id shellRootID, dir bool) (fs.FileInfo, error) {
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	if rootInfo.Mode()&fs.ModeSymlink != 0 {
		return nil, fmt.Errorf("%w: %s", errShellReparsePoint, root)
	}
	if id.info == nil || !os.SameFile(id.info, rootInfo) {
		return nil, fmt.Errorf("%w: %s", errShellRootChanged, root)
	}
	parts := strings.Split(rel, string(filepath.Separator))
	current := root
	var info fs.FileInfo
	for i, part := range parts {
		current = filepath.Join(current, part)
		if info, err = os.Lstat(current); err != nil {
			return nil, err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return nil, fmt.Errorf("%w: %s", errShellReparsePoint, current)
		}
		if i < len(parts)-1 && !info.IsDir() {
			return nil, fmt.Errorf("%w: %s", errShellKindChanged, current)
		}
	}
	if info.IsDir() != dir || (!dir && !info.Mode().IsRegular()) {
		return nil, fmt.Errorf("%w: %s", errShellKindChanged, current)
	}
	return info, nil
}

func (e *shellEntryFile) read(limit int64) ([]byte, error) {
	if e.file == nil {
		return nil, fmt.Errorf("%w: %s", errShellKindChanged, e.path)
	}
	return io.ReadAll(io.LimitReader(e.file, limit))
}

func (e *shellEntryFile) remove() error {
	current, err := lstatShellChain(e.root, e.rel, e.id, e.file == nil)
	if err != nil {
		return err
	}
	if !os.SameFile(e.info, current) {
		return fmt.Errorf("%w: %s", errShellKindChanged, e.path)
	}
	err = os.Remove(e.path)
	if errors.Is(err, syscall.ENOTEMPTY) || errors.Is(err, syscall.EEXIST) {
		return fmt.Errorf("%w: %s", errShellDirNotEmpty, e.path)
	}
	return err
}

func (e *shellEntryFile) close() error {
	if e.file == nil {
		return nil
	}
	return e.file.Close()
}
