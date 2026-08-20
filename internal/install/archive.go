package install

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

const copyBufferSize = 256 * 1024

var (
	errUnsupportedArchive = errors.New("формат архива не поддерживается, распакуйте вручную")
	errNoEstimate         = errors.New("не удалось определить размер распакованного архива")
)

func IsArchive(path string) bool {
	switch archiveExt(path) {
	case ".zip", ".7z", ".rar":
		return true
	}
	return false
}

func archiveExt(path string) string {
	return strings.ToLower(filepath.Ext(path))
}

func archiveType(path string) Type {
	switch archiveExt(path) {
	case ".zip":
		return TypeArchiveZip
	case ".7z":
		return TypeArchive7z
	case ".rar":
		return TypeArchiveRar
	}
	return TypeUnknown
}

func ExtractArchive(ctx context.Context, archivePath, dest string, onProgress func(Progress)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	total, err := EstimateExtracted(archivePath)
	if err != nil {
		total = 0
	}
	rep := newReporter(onProgress, total)

	switch archiveExt(archivePath) {
	case ".zip":
		err = extractZip(ctx, archivePath, dest, rep)
	case ".7z":
		err = extractSevenZip(ctx, archivePath, dest, rep)
	case ".rar":
		err = extractRar(ctx, archivePath, dest, rep)
	default:
		return errUnsupportedArchive
	}
	if err != nil {
		return err
	}
	rep.flush()
	return nil
}

func EstimateExtracted(archivePath string) (int64, error) {
	switch archiveExt(archivePath) {
	case ".zip":
		return estimateZip(archivePath)
	case ".7z":
		return estimateSevenZip(archivePath)
	case ".rar":
		return estimateRar(archivePath)
	}
	return 0, errUnsupportedArchive
}

func skipEntry(archivePath, name string) {
	slog.Warn("skip unsafe archive entry", "archive", archivePath, "entry", name)
}

func writeEntry(ctx context.Context, target string, mode fs.FileMode, src io.Reader, rep *reporter, buf []byte) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, entryMode(mode))
	if err != nil {
		return err
	}
	if err := copyStream(ctx, f, src, rep, buf); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func entryMode(mode fs.FileMode) fs.FileMode {
	if mode.Perm()&0o111 != 0 {
		return 0o755
	}
	return 0o644
}

func copyStream(ctx context.Context, dst io.Writer, src io.Reader, rep *reporter, buf []byte) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, readErr := src.Read(buf)
		if n > 0 {
			if _, err := dst.Write(buf[:n]); err != nil {
				return err
			}
			rep.add(int64(n))
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}
