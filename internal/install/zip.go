package install

import (
	"archive/zip"
	"context"
	"os"
)

func extractZip(ctx context.Context, archivePath, dest string, rep *reporter) error {
	rc, err := zip.OpenReader(archivePath)
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
			return err
		}
		err = writeEntry(ctx, target, info.Mode(), src, rep, buf)
		src.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func estimateZip(archivePath string) (int64, error) {
	rc, err := zip.OpenReader(archivePath)
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
		total += int64(entry.UncompressedSize64)
	}
	return total, nil
}
