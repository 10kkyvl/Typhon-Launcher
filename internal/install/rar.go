package install

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"slices"

	rardecode "github.com/nwaples/rardecode/v2"

	"typhon/internal/uierr"
)

var (
	errArchiveToolMissing  = uierr.New("install.archive_tool_missing", "встроенный распаковщик не справился с архивом: установите 7-Zip или WinRAR либо распакуйте вручную")
	errArchiveToolFailed   = uierr.New("install.archive_tool_failed", "архив не распаковался ни встроенным распаковщиком, ни 7-Zip или WinRAR")
	errArchiveUnsafe       = uierr.New("install.archive_unsafe_entries", "в архиве есть ссылки или пути за пределы папки, распакуйте его вручную")
	errArchiveEncrypted    = uierr.New("install.archive_encrypted", "архив защищён паролем, распакуйте его вручную")
	errArchiveIncomplete   = uierr.New("install.archive_incomplete", "архив обрезан или не хватает следующего тома")
	errArchiveMismatch     = uierr.New("install.archive_result_mismatch", "распакованные файлы не совпадают с оглавлением архива")
	errArchiveVerify       = uierr.New("install.archive_verify_failed", "не удалось проверить распакованные файлы")
	errArchiveCorrupt      = uierr.New("install.archive_corrupt", "архив повреждён: контрольная сумма не сошлась и во внешнем распаковщике, скачайте его заново")
	errArchiveToolOutdated = uierr.New("install.archive_tool_outdated", "установленный WinRAR или 7-Zip слишком старый или не опознан: обновите его либо распакуйте архив вручную")
	errArchiveHasLinks     = errors.New("ссылка или нестандартная запись")
)

const (
	winAttrReparsePoint = 0x400
	unixTypeMask        = 0xF000
	unixTypeRegular     = 0x8000
	unixTypeDir         = 0x4000
)

var lookupArchiveTools = findArchiveTools

type rarDecodeError struct{ err error }

func (e *rarDecodeError) Error() string { return e.err.Error() }
func (e *rarDecodeError) Unwrap() error { return e.err }

// Пароль, обрезанный архив и недостающий том внешний распаковщик не исправит.
func rarCause(archivePath string, err error) error {
	switch {
	case errors.Is(err, rardecode.ErrArchiveEncrypted), errors.Is(err, rardecode.ErrArchivedFileEncrypted), errors.Is(err, rardecode.ErrBadPassword):
		return fmt.Errorf("%w: %w", errArchiveEncrypted, err)
	case errors.Is(err, rardecode.ErrMultiVolume), errors.Is(err, rardecode.ErrUnexpectedArcEnd), errors.Is(err, rardecode.ErrBadVolumeNumber), missingVolume(archivePath, err):
		return fmt.Errorf("%w: %w", errArchiveIncomplete, err)
	}
	return nil
}

// Недостающий том rardecode сообщает обычным *fs.PathError на файл рядом с
// архивом, и от отсутствия самого архива это отличает только путь.
func missingVolume(archivePath string, err error) bool {
	var pathErr *fs.PathError
	return errors.As(err, &pathErr) && errors.Is(err, fs.ErrNotExist) && !samePath(pathErr.Path, archivePath)
}

// В отказ декодера попадает только то, с чем внешний распаковщик может
// справиться: ошибку чтения самого файла (нет прав, обрыв диска) он не
// исправит. io.ErrUnexpectedEOF при разборе данных отдаёт и декодер RAR 2.9
// над уже прочитанной записью, поэтому обрывом файла он считается, лишь если
// файл тома и правда кончился (atEOF).
func asDecodeError(archivePath string, err error, atEOF bool) error {
	if err == nil || errors.Is(err, io.EOF) {
		return err
	}
	if cause := rarCause(archivePath, err); cause != nil {
		return cause
	}
	if atEOF && errors.Is(err, io.ErrUnexpectedEOF) {
		return fmt.Errorf("%w: %w", errArchiveIncomplete, err)
	}
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return err
	}
	return &rarDecodeError{err: err}
}

// В заголовках io.ErrUnexpectedEOF — всегда обрыв файла: они читаются прямо с
// диска, без декодера.
func rarListError(archivePath string, err error) error {
	if cause := rarCause(archivePath, err); cause != nil {
		return cause
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return fmt.Errorf("%w: %w", errArchiveIncomplete, err)
	}
	return err
}

// volumeEOF помнит, дошло ли чтение текущего файла тома до его конца: так
// обрезанный файл отличается от ошибки декодера, у которой тот же
// io.ErrUnexpectedEOF. Читает один поток, поэтому без синхронизации.
type volumeEOF struct{ hit bool }

func (v *volumeEOF) reached() bool { return v != nil && v.hit }

type eofFS struct{ eof *volumeEOF }

func (f eofFS) Open(name string) (fs.File, error) {
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	f.eof.hit = false
	return &eofFile{File: file, eof: f.eof}, nil
}

type eofFile struct {
	*os.File
	eof *volumeEOF
}

func (f *eofFile) Read(p []byte) (int, error) {
	n, err := f.File.Read(p)
	if errors.Is(err, io.EOF) {
		f.eof.hit = true
	}
	return n, err
}

type rarSource struct {
	r       io.Reader
	archive string
	eof     *volumeEOF
}

func (s rarSource) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	return n, asDecodeError(s.archive, err, s.eof.reached())
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
	eof := &volumeEOF{}
	rc, err := rardecode.OpenReader(archivePath, rardecode.FileSystem(eofFS{eof: eof}))
	if err != nil {
		return classifyArchiveError(archivePath, rarListError(archivePath, err))
	}
	defer closeReadOnly(archivePath, rc)

	src := rarSource{r: rc, archive: archivePath, eof: eof}
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
			return asDecodeError(archivePath, err, eof.reached())
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
	set := lookupArchiveTools(ctx)
	if len(set.tools) == 0 {
		if len(set.refused) > 0 {
			return fmt.Errorf("%w: %w; встроенный: %w", errArchiveToolOutdated, errors.Join(set.refused...), decodeErr)
		}
		return fmt.Errorf("%w: %w", errArchiveToolMissing, decodeErr)
	}
	listing, err := checkToolEntries(archivePath, dest)
	if err != nil {
		return fmt.Errorf("%w: %w", err, decodeErr)
	}
	slog.Warn("builtin rar decoder failed, trying external tools", "archive", archivePath, "error", decodeErr)
	failures := slices.Clone(set.refused)
	for _, tool := range set.tools {
		rep.restart()
		slog.Info("extract with external tool", "tool", tool.name, "path", tool.path, "archive", archivePath)
		err := runArchiveTool(ctx, tool, archivePath, dest, listing, rep)
		if err == nil {
			rep.setPercent(100)
			return nil
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		slog.Warn("external tool failed", "tool", tool.name, "archive", archivePath, "error", err)
		if stopsToolChain(err) {
			return fmt.Errorf("%w; встроенный: %w", err, decodeErr)
		}
		failures = append(failures, err)
	}
	return fmt.Errorf("%w: встроенный: %w; %w", errArchiveToolFailed, decodeErr, errors.Join(failures...))
}

// Следующий распаковщик встретит ту же причину: диск, пароль, повреждённый или
// обрезанный архив, ссылки в результате, а проверка результата смотрит на то,
// как окружение обошлось с записанным, а не на распаковщик.
func stopsToolChain(err error) bool {
	for _, stop := range []error{
		errToolWrite, errArchiveEncrypted, errArchiveIncomplete, errArchiveCorrupt,
		errArchiveUnsafe, errArchiveMismatch, errArchiveVerify,
	} {
		if errors.Is(err, stop) {
			return true
		}
	}
	return false
}

func runArchiveTool(ctx context.Context, tool archiveTool, archivePath, dest string, listing []*rardecode.File, rep *reporter) error {
	err := tool.extract(ctx, archivePath, dest, rep.setPercent)
	if err == nil {
		return verifyExtracted(ctx, dest, listing, rep)
	}
	if ctx.Err() != nil || stopsToolChain(err) {
		return err
	}
	return diagnoseToolFailure(ctx, dest, listing, err)
}

// Внешний распаковщик пишет записи сам, мимо safeJoin и пропуска ссылок,
// поэтому имена и признаки ссылок проверяются заранее по оглавлению архива
// (инвариант 32). Это лишь первая линия: rardecode не разбирает записи
// перенаправления RAR5, и ссылки, которых нет в оглавлении, ловит проверка
// результата в verifyExtracted.
func checkToolEntries(archivePath, dest string) ([]*rardecode.File, error) {
	files, err := rardecode.List(archivePath)
	if err != nil {
		return nil, classifyArchiveError(archivePath, rarListError(archivePath, err))
	}
	for _, f := range files {
		if isLinkEntry(&f.FileHeader) {
			return nil, fmt.Errorf("%w: %w: %s", errArchiveUnsafe, errArchiveHasLinks, entryLabel(f.Name))
		}
		if _, err := safeJoin(dest, f.Name); err != nil {
			return nil, fmt.Errorf("%w: %w: %s", errArchiveUnsafe, err, entryLabel(f.Name))
		}
	}
	return files, nil
}

// У записей, созданных на Windows, ссылку выдаёт атрибут reparse point, а Mode()
// их не различает; у unix-записей тип лежит в старших битах режима.
func isLinkEntry(h *rardecode.FileHeader) bool {
	switch h.HostOS {
	case rardecode.HostOSWindows:
		return h.Attributes&winAttrReparsePoint != 0
	case rardecode.HostOSUnix:
		kind := h.Attributes & unixTypeMask
		return kind != 0 && kind != unixTypeRegular && kind != unixTypeDir
	}
	return !h.IsDir && !h.Mode().IsRegular()
}

func estimateRar(archivePath string) (int64, error) {
	files, err := rardecode.List(archivePath)
	if err != nil {
		return 0, rarListError(archivePath, err)
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
