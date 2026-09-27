package download

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"typhon/internal/uierr"
)

var (
	errFileMissing    = uierr.New("download.file_missing", "файл отсутствует на диске")
	errFileIncomplete = uierr.New("download.file_incomplete", "загрузка не завершена: на диске остался только незавершённый файл (.part)")
	errFileTruncated  = uierr.New("download.file_truncated", "файл на диске меньше ожидаемого размера")
	errFileOversized  = uierr.New("download.file_oversized", "файл на диске больше ожидаемого размера")
	errFileStatFailed = uierr.New("download.file_stat_failed", "не удалось проверить файл на диске")
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
		switch {
		case info.Size() < expected:
			return errFileTruncated
		case info.Size() > expected:
			return errFileOversized
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
