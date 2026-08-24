package download

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

var (
	errFileMissing    = errors.New("файл отсутствует на диске")
	errFileIncomplete = errors.New("загрузка не завершена: на диске остался только незавершённый файл (.part)")
	errFileTruncated  = errors.New("файл на диске меньше ожидаемого размера")
	errFileStatFailed = errors.New("не удалось проверить файл на диске")
	errBlockedByAV    = errors.New("файл заблокирован или удалён антивирусом Windows")
)

func verifyFilesOnDisk(ctx context.Context, files []FileState, paths []string) error {
	for i, f := range files {
		if !f.Selected {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if i >= len(paths) {
			return fmt.Errorf("%s: %w", f.Path, errFileMissing)
		}
		if err := verifyOneFile(paths[i], f.Size); err != nil {
			return fmt.Errorf("%s: %w", f.Path, err)
		}
	}
	return nil
}

func verifyOneFile(path string, expected int64) error {
	info, err := os.Stat(path)
	if err == nil {
		if info.Size() < expected {
			return errFileTruncated
		}
		return nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		_, partErr := os.Stat(path + PartFileSuffix)
		switch {
		case partErr == nil:
			return errFileIncomplete
		case errors.Is(partErr, fs.ErrNotExist):
			return errFileMissing
		default:
			return fmt.Errorf("%w: %w", errFileStatFailed, partErr)
		}
	}
	if avErr := classifyAntivirusError(err); avErr != nil {
		return avErr
	}
	return fmt.Errorf("%w: %w", errFileStatFailed, err)
}
