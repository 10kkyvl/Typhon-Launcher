package install

import (
	"context"
	"fmt"
	"io/fs"
	"math"
	"path/filepath"

	rardecode "github.com/nwaples/rardecode/v2"
)

var archiveFreeSpace = CheckFreeSpace

// 7-Zip отвечает на нехватку места тем же кодом, что и на битый архив, а текст
// причины зависит от языка системы, поэтому причину ищет сравнение остатка со
// свободным местом. CheckFreeSpace отказывает и когда места мало, и когда его не
// удалось узнать; запрос на ноль байт отказывает только во втором случае.
func diagnoseToolFailure(ctx context.Context, dest string, files []*rardecode.File, cause error) error {
	remaining, err := bytesLeft(ctx, dest, files)
	if err != nil {
		return fmt.Errorf("%w; остаток распаковки не посчитан: %w", cause, err)
	}
	if remaining <= 0 {
		return cause
	}
	if err := archiveFreeSpace(dest, 0); err != nil {
		return fmt.Errorf("%w; свободное место не определено: %w", cause, err)
	}
	if err := archiveFreeSpace(dest, remaining); err != nil {
		return fmt.Errorf("%w: %w: %w", errNotEnoughSpace, err, fmt.Errorf("%w: %w", errToolWrite, cause))
	}
	return cause
}

func bytesLeft(ctx context.Context, dest string, files []*rardecode.File) (int64, error) {
	var total int64
	for _, f := range files {
		if f.IsDir {
			continue
		}
		if f.UnPackedSize < 0 || total > math.MaxInt64-f.UnPackedSize {
			return 0, errArchiveSize
		}
		total += f.UnPackedSize
	}
	written, err := writtenBytes(ctx, dest)
	if err != nil {
		return 0, err
	}
	return total - written, nil
}

func writtenBytes(ctx context.Context, dest string) (int64, error) {
	var total int64
	err := filepath.WalkDir(dest, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("размер распакованного: %w", err)
	}
	return total, nil
}
