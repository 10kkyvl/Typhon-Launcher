//go:build windows

package install

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	shellEntryAccess = windows.DELETE | windows.FILE_READ_DATA | windows.FILE_READ_ATTRIBUTES | windows.SYNCHRONIZE
	// BACKUP_SEMANTICS открывает и файл, и каталог: тип сверяется по атрибутам
	// уже открытого хэндла, иначе junction с именем ярлыка давал бы отказ в
	// доступе вместо отказа по ссылке.
	shellEntryFlags         = windows.FILE_FLAG_OPEN_REPARSE_POINT | windows.FILE_FLAG_BACKUP_SEMANTICS
	finalPathDOS            = 0
	reparseTagNameSurrogate = 0x20000000
)

type shellEntryFile struct {
	handle windows.Handle
	path   string
	denied error
}

// shellRootID — итоговый путь корня на момент снимка.
type shellRootID struct {
	final string
}

type fileDispositionInfoEx struct {
	Flags uint32
}

type fileDispositionInfo struct {
	DeleteFile bool
}

type fileAttributeTagInfo struct {
	FileAttributes uint32
	ReparseTag     uint32
}

func openShellEntryIn(root, rel string, id shellRootID, dir bool) (*shellEntryFile, error) {
	path := filepath.Join(root, rel)
	name, err := windows.UTF16PtrFromString(extendedPath(path))
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: path, Err: err}
	}
	entry := &shellEntryFile{path: path}
	// Без FILE_SHARE_DELETE никто другой не переименует и не удалит запись,
	// пока хэндл открыт.
	handle, err := windows.CreateFile(name, shellEntryAccess, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, shellEntryFlags, 0)
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		// Лаунчер без прав администратора не получит DELETE в общем каталоге,
		// но решить по содержимому, что ярлык не его, обязан и тогда: отказ
		// всплывёт, только если ярлык действительно надо удалить.
		entry.denied = &fs.PathError{Op: "remove", Path: path, Err: err}
		handle, err = windows.CreateFile(name, shellEntryAccess&^windows.DELETE, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, shellEntryFlags, 0)
	}
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: path, Err: err}
	}
	entry.handle = handle
	if err := entry.verify(rel, id, dir); err != nil {
		return nil, errors.Join(err, entry.close())
	}
	return entry, nil
}

// extendedPath переводит абсолютный путь в форму \\?\: без неё CreateFile
// упирается в MAX_PATH там, где длинные пути в системе выключены, и
// нормализует имя (срезает точки и пробелы в конце), то есть открывает не ту
// запись, что дал обход каталога, и отказ превращается в «файла нет».
func extendedPath(path string) string {
	if strings.HasPrefix(path, `\\?\`) || strings.HasPrefix(path, `\\.\`) || !filepath.IsAbs(path) {
		return path
	}
	clean := filepath.Clean(path)
	if strings.HasPrefix(clean, `\\`) {
		return `\\?\UNC\` + clean[2:]
	}
	return `\\?\` + clean
}

func (e *shellEntryFile) verify(rel string, id shellRootID, dir bool) error {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(e.handle, &info); err != nil {
		return &fs.PathError{Op: "stat", Path: e.path, Err: err}
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		surrogate, err := e.nameSurrogate()
		if err != nil {
			return err
		}
		if surrogate {
			return fmt.Errorf("%w: %s", errShellReparsePoint, e.path)
		}
	}
	if (info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != dir {
		return fmt.Errorf("%w: %s", errShellKindChanged, e.path)
	}
	got, err := finalPath(e.handle)
	if err != nil {
		return &fs.PathError{Op: "resolve", Path: e.path, Err: err}
	}
	// Итоговый путь хэндла ловит junction в любом каталоге от корня до записи
	// и сам корень, подменённый после снимка: по нему видно, куда открытие
	// пришло на самом деле.
	if want := strings.TrimSuffix(id.final, `\`) + `\` + rel; !strings.EqualFold(got, want) {
		return fmt.Errorf("%w: %s открылся как %s", errShellEscaped, e.path, got)
	}
	return nil
}

// nameSurrogate отличает ссылку (symlink, junction, точка монтирования) от
// reparse point, который имя не перенаправляет: файлы OneDrive на рабочем
// столе и дедуплицированные файлы — обычные ярлыки, и отказ по ним оставил бы
// ярлык сайта на месте.
func (e *shellEntryFile) nameSurrogate() (bool, error) {
	var tag fileAttributeTagInfo
	//nolint:gosec // G103: GetFileInformationByHandleEx пишет только в локальную структуру, живущую весь вызов; тип ссылки по хэндлу нужен инварианту 11
	if err := windows.GetFileInformationByHandleEx(e.handle, windows.FileAttributeTagInfo, (*byte)(unsafe.Pointer(&tag)), uint32(unsafe.Sizeof(tag))); err != nil {
		return false, &fs.PathError{Op: "stat", Path: e.path, Err: err}
	}
	return tag.ReparseTag&reparseTagNameSurrogate != 0, nil
}

// identifyShellRoot запоминает, куда корень разрешался в момент снимка; подмену
// корня после этого ловит сверка итогового пути каждой записи с этим значением.
func identifyShellRoot(root string) (shellRootID, error) {
	name, err := windows.UTF16PtrFromString(extendedPath(root))
	if err != nil {
		return shellRootID{}, &fs.PathError{Op: "open", Path: root, Err: err}
	}
	share := uint32(windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE)
	handle, err := windows.CreateFile(name, windows.FILE_READ_ATTRIBUTES, share, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return shellRootID{}, &fs.PathError{Op: "open", Path: root, Err: err}
	}
	path, err := finalPath(handle)
	if closeErr := windows.CloseHandle(handle); closeErr != nil {
		return shellRootID{}, errors.Join(err, &fs.PathError{Op: "close", Path: root, Err: closeErr})
	}
	if err != nil {
		return shellRootID{}, &fs.PathError{Op: "resolve", Path: root, Err: err}
	}
	return shellRootID{final: path}, nil
}

func finalPath(handle windows.Handle) (string, error) {
	size := uint32(windows.MAX_PATH)
	for {
		buf := make([]uint16, size)
		n, err := windows.GetFinalPathNameByHandle(handle, &buf[0], size, finalPathDOS)
		if err != nil {
			return "", err
		}
		if n < size {
			return windows.UTF16ToString(buf[:n]), nil
		}
		size = n
	}
}

func (e *shellEntryFile) read(limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(handleReader(e.handle), limit))
	if err != nil {
		return nil, &fs.PathError{Op: "read", Path: e.path, Err: err}
	}
	return data, nil
}

type handleReader windows.Handle

func (h handleReader) Read(p []byte) (int, error) {
	var n uint32
	if err := windows.ReadFile(windows.Handle(h), p, &n, nil); err != nil {
		return int(n), err
	}
	if n == 0 && len(p) > 0 {
		return 0, io.EOF
	}
	return int(n), nil
}

// remove удаляет через тот же хэндл. Пустоту каталога проверяет сама файловая
// система: непустой каталог она не пометит на удаление, и проверить это
// атомарнее, чем перечислить содержимое и удалить следом, нельзя.
func (e *shellEntryFile) remove() error {
	if e.denied != nil {
		return e.denied
	}
	ex := fileDispositionInfoEx{Flags: windows.FILE_DISPOSITION_DELETE | windows.FILE_DISPOSITION_POSIX_SEMANTICS | windows.FILE_DISPOSITION_IGNORE_READONLY_ATTRIBUTE}
	//nolint:gosec // G103: SetFileInformationByHandle читает только локальную структуру, живущую весь вызов; удаление по проверенному хэндлу — инвариант 11
	err := windows.SetFileInformationByHandle(e.handle, windows.FileDispositionInfoEx, (*byte)(unsafe.Pointer(&ex)), uint32(unsafe.Sizeof(ex)))
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) || errors.Is(err, windows.ERROR_NOT_SUPPORTED) || errors.Is(err, windows.ERROR_INVALID_FUNCTION) {
		// FileDispositionInfoEx появился в Windows 10 1607, а POSIX-удаление
		// не поддерживают FAT и exFAT: там остаётся классическая пометка.
		legacy := fileDispositionInfo{DeleteFile: true}
		//nolint:gosec // G103: SetFileInformationByHandle читает только локальную структуру, живущую весь вызов; удаление по проверенному хэндлу — инвариант 11
		err = windows.SetFileInformationByHandle(e.handle, windows.FileDispositionInfo, (*byte)(unsafe.Pointer(&legacy)), uint32(unsafe.Sizeof(legacy)))
	}
	switch {
	case errors.Is(err, windows.ERROR_DIR_NOT_EMPTY):
		return fmt.Errorf("%w: %s", errShellDirNotEmpty, e.path)
	case err != nil:
		return &fs.PathError{Op: "remove", Path: e.path, Err: err}
	}
	return nil
}

func (e *shellEntryFile) close() error {
	if err := windows.CloseHandle(e.handle); err != nil {
		return &fs.PathError{Op: "close", Path: e.path, Err: err}
	}
	return nil
}
