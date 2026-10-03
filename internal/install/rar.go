package install

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"

	rardecode "github.com/nwaples/rardecode/v2"

	"typhon/internal/uierr"
)

var (
	errArchiveToolMissing = uierr.New("install.archive_tool_missing", "встроенный распаковщик не справился с архивом: установите 7-Zip или WinRAR либо распакуйте вручную")
	errArchiveToolFailed  = uierr.New("install.archive_tool_failed", "архив не распаковался ни встроенным распаковщиком, ни 7-Zip или WinRAR")
	errArchiveUnsafe      = uierr.New("install.archive_unsafe_entries", "в архиве есть ссылки или пути за пределы папки, распакуйте его вручную")
	errArchiveEncrypted   = uierr.New("install.archive_encrypted", "архив защищён паролем, распакуйте его вручную")
	errArchiveIncomplete  = uierr.New("install.archive_incomplete", "архив обрезан или не хватает следующего тома")
	errArchiveHasLinks    = errors.New("ссылка или нестандартная запись")
)

var lookupArchiveTools = findArchiveTools

type rarDecodeError struct{ err error }

func (e *rarDecodeError) Error() string { return e.err.Error() }
func (e *rarDecodeError) Unwrap() error { return e.err }

// В отказ декодера попадает только то, с чем внешний распаковщик может
// справиться: ошибку чтения самого файла (нет прав, обрыв диска), пароль и
// обрезанный архив или недостающий том он не исправит.
func asDecodeError(err error) error {
	var pathErr *fs.PathError
	switch {
	case err == nil || errors.Is(err, io.EOF) || errors.As(err, &pathErr):
		return err
	case errors.Is(err, rardecode.ErrArchiveEncrypted), errors.Is(err, rardecode.ErrArchivedFileEncrypted), errors.Is(err, rardecode.ErrBadPassword):
		return fmt.Errorf("%w: %w", errArchiveEncrypted, err)
	case errors.Is(err, rardecode.ErrMultiVolume), errors.Is(err, rardecode.ErrUnexpectedArcEnd), errors.Is(err, rardecode.ErrBadVolumeNumber):
		return fmt.Errorf("%w: %w", errArchiveIncomplete, err)
	}
	return &rarDecodeError{err: err}
}

type rarSource struct{ r io.Reader }

func (s rarSource) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	return n, asDecodeError(err)
}

func extractRar(ctx context.Context, archivePath, dest string, rep *reporter) error {
	err := decodeRar(ctx, archivePath, dest, rep)
	var decodeErr *rarDecodeError
	if !errors.As(err, &decodeErr) {
		return err
	}
	return extractRarWithTool(ctx, archivePath, dest, rep, decodeErr)
}

func decodeRar(ctx context.Context, archivePath, dest string, rep *reporter) error {
	rc, err := rardecode.OpenReader(archivePath)
	if err != nil {
		return errUnsupportedArchive
	}
	defer closeReadOnly(archivePath, rc)

	src := rarSource{r: rc}
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
			return asDecodeError(err)
		}
		if !header.IsDir && !header.Mode().IsRegular() {
			skipIrregular(archivePath, header.Name)
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
		if err := writeEntry(ctx, target, header.Mode(), src, rep, buf); err != nil {
			return err
		}
	}
}

func extractRarWithTool(ctx context.Context, archivePath, dest string, rep *reporter, decodeErr error) error {
	tools := lookupArchiveTools()
	if len(tools) == 0 {
		return fmt.Errorf("%w: %w", errArchiveToolMissing, decodeErr)
	}
	if err := checkToolEntries(archivePath, dest); err != nil {
		return fmt.Errorf("%w: %w", err, decodeErr)
	}
	slog.Warn("builtin rar decoder failed, trying external tools", "archive", archivePath, "error", decodeErr)
	var failures []error
	for _, tool := range tools {
		rep.restart()
		slog.Info("extract with external tool", "tool", tool.name, "path", tool.path, "archive", archivePath)
		err := tool.extract(ctx, archivePath, dest, rep.setPercent)
		if err == nil {
			rep.setPercent(100)
			return nil
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		slog.Warn("external tool failed", "tool", tool.name, "archive", archivePath, "error", err)
		if errors.Is(err, errToolWrite) || errors.Is(err, errArchiveEncrypted) {
			return fmt.Errorf("%w; встроенный: %w", err, decodeErr)
		}
		failures = append(failures, err)
	}
	return fmt.Errorf("%w: встроенный: %w; %w", errArchiveToolFailed, decodeErr, errors.Join(failures...))
}

// Внешний распаковщик пишет записи сам, мимо safeJoin и пропуска ссылок,
// поэтому имена проверяются заранее по оглавлению архива (инвариант 32).
func checkToolEntries(archivePath, dest string) error {
	files, err := rardecode.List(archivePath)
	if err != nil {
		return classifyArchiveError(archivePath, err)
	}
	for _, f := range files {
		if !f.IsDir && !f.Mode().IsRegular() {
			return fmt.Errorf("%w: %w: %q", errArchiveUnsafe, errArchiveHasLinks, f.Name)
		}
		if _, err := safeJoin(dest, f.Name); err != nil {
			return fmt.Errorf("%w: %w: %q", errArchiveUnsafe, err, f.Name)
		}
	}
	return nil
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
