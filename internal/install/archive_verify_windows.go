//go:build windows

package install

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"syscall"

	"golang.org/x/sys/windows"
)

// Как Mode() показывает reparse point, зависит от версии Go и GODEBUG
// winsymlink, а ссылки нельзя пропустить, поэтому признак берётся из
// атрибутов файла напрямую.
func reparsePoint(info fs.FileInfo) (bool, error) {
	attrs, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return false, fmt.Errorf("атрибуты файла недоступны: %T", info.Sys())
	}
	return attrs.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0, nil
}

func osFileBusy(err error) bool {
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_LOCK_VIOLATION)
}

func syncAndCountLinks(path string, mode fs.FileMode) (uint64, error) {
	if mode.Perm()&0o200 != 0 {
		return syncWritable(path)
	}
	return syncReadOnly(path, mode)
}

// FlushFileBuffers требует дескриптор на запись, а атрибут «только чтение» его
// не даёт, поэтому на время сброса он снимается и затем возвращается.
func syncReadOnly(path string, mode fs.FileMode) (links uint64, err error) {
	if err := os.Chmod(path, mode.Perm()|0o200); err != nil {
		return 0, err
	}
	defer func() {
		if restoreErr := os.Chmod(path, mode.Perm()); restoreErr != nil {
			err = errors.Join(err, restoreErr)
		}
	}()
	return syncWritable(path)
}

func syncWritable(path string) (uint64, error) {
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return 0, err
	}
	links, linkErr := countLinks(f)
	syncErr := f.Sync()
	closeErr := f.Close()
	if err := errors.Join(linkErr, syncErr, closeErr); err != nil {
		return 0, err
	}
	return links, nil
}

func countLinks(f *os.File) (uint64, error) {
	raw, err := f.SyscallConn()
	if err != nil {
		return 0, err
	}
	var (
		info    windows.ByHandleFileInformation
		callErr error
	)
	if err := raw.Control(func(fd uintptr) {
		callErr = windows.GetFileInformationByHandle(windows.Handle(fd), &info)
	}); err != nil {
		return 0, err
	}
	if callErr != nil {
		return 0, fmt.Errorf("GetFileInformationByHandle: %w", callErr)
	}
	return uint64(info.NumberOfLinks), nil
}
