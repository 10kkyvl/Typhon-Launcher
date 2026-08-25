package install

import (
	"context"
	"errors"
	"math"
	"os"

	"github.com/bodgit/sevenzip"
)

var errArchiveSize = errors.New("архив сообщает недостоверный размер содержимого")

func extractSevenZip(ctx context.Context, archivePath, dest string, rep *reporter) error {
	rc, err := sevenzip.OpenReader(archivePath)
	if err != nil {
		return errUnsupportedArchive
	}
	defer rc.Close()

	buf := make([]byte, copyBufferSize)
	for _, entry := range rc.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		info := entry.FileInfo()
		if !info.IsDir() && !info.Mode().IsRegular() {
			continue
		}
		target, err := safeJoin(dest, entry.Name)
		if err != nil {
			skipEntry(archivePath, entry.Name)
			continue
		}
		if info.IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		rep.setFile(entry.Name)
		src, err := entry.Open()
		if err != nil {
			return errUnsupportedArchive
		}
		err = writeEntry(ctx, target, info.Mode(), src, rep, buf)
		src.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func estimateSevenZip(archivePath string) (int64, error) {
	rc, err := sevenzip.OpenReader(archivePath)
	if err != nil {
		return 0, err
	}
	defer rc.Close()

	var total int64
	for _, entry := range rc.File {
		info := entry.FileInfo()
		if info.IsDir() || !info.Mode().IsRegular() {
			continue
		}
		next, err := addEntrySize(total, entry.UncompressedSize)
		if err != nil {
			return 0, err
		}
		total = next
	}
	return total, nil
}

func addEntrySize(total int64, size uint64) (int64, error) {
	if size > math.MaxInt64 {
		return 0, errArchiveSize
	}
	//nolint:gosec // G115: size <= MaxInt64 проверено выше
	signed := int64(size)
	if total > math.MaxInt64-signed {
		return 0, errArchiveSize
	}
	return total + signed, nil
}
