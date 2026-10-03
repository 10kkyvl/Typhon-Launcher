//go:build !windows

package install

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

// Ссылки на unix видны в Mode(), отдельного признака reparse point нет.
func reparsePoint(fs.FileInfo) (bool, error) {
	return false, nil
}

// Обязательных блокировок файла, мешающих fsync, на unix нет.
func osFileBusy(error) bool {
	return false
}

func syncAndCountLinks(path string, _ fs.FileMode) (uint64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	info, statErr := f.Stat()
	syncErr := f.Sync()
	closeErr := f.Close()
	if err := errors.Join(statErr, syncErr, closeErr); err != nil {
		return 0, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("число ссылок файла недоступно: %T", info.Sys())
	}
	return uint64(st.Nlink), nil
}
