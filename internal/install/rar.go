package install

import (
	"context"
	"errors"
	"io"
	"os"

	rardecode "github.com/nwaples/rardecode/v2"
)

func extractRar(ctx context.Context, archivePath, dest string, rep *reporter) error {
	rc, err := rardecode.OpenReader(archivePath)
	if err != nil {
		return errUnsupportedArchive
	}
	defer rc.Close()

	buf := make([]byte, copyBufferSize)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, err := rc.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return errUnsupportedArchive
		}
		if !header.IsDir && !header.Mode().IsRegular() {
			continue
		}
		target, err := safeJoin(dest, header.Name)
		if err != nil {
			skipEntry(archivePath, header.Name)
			continue
		}
		if header.IsDir {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		rep.setFile(header.Name)
		if err := writeEntry(ctx, target, header.Mode(), rc, rep, buf); err != nil {
			return err
		}
	}
}

func estimateRar(archivePath string) (int64, error) {
	files, err := rardecode.List(archivePath)
	if err != nil {
		return 0, errUnsupportedArchive
	}
	var total int64
	for _, f := range files {
		if f.IsDir {
			continue
		}
		if f.UnKnownSize {
			return 0, errNoEstimate
		}
		total += f.UnPackedSize
	}
	return total, nil
}
