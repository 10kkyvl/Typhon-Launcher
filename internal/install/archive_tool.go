package install

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"typhon/internal/uierr"
)

const (
	toolOutputTail       = 1024
	toolWaitDelay        = 5 * time.Second
	toolCommandLineError = 7
)

var (
	errToolExit     = errors.New("распаковщик завершился с ошибкой")
	errToolOutdated = errors.New("распаковщик не принял параметры безопасной распаковки, обновите 7-Zip или WinRAR")
	errToolWrite    = uierr.New("install.archive_tool_write_failed", "распаковщик не смог записать файлы на диск")
	toolPercentExp  = regexp.MustCompile(`(\d{1,3})%`)
)

type archiveTool struct {
	name string
	path string
	args func(archive, dest string) []string
	// Коды выхода с известной причиной. Ошибку записи или пароль следующий
	// распаковщик не исправит, поэтому по ним перебор останавливается.
	exitCauses map[int]error
	// Предупреждение распаковщика — не отказ: годен ли результат, решает
	// сверка с оглавлением архива.
	warnExits map[int]bool
	// 7-Zip отвечает на любую ошибку записи кодом 2, как и на битый архив, а
	// текст системной причины зависит от языка Windows, поэтому по тексту
	// опознаются только его собственные фразы.
	writeMarkers []string
	// Обрезанный архив оба распаковщика называют одной фразой, но кодом выхода
	// это не отличить от повреждённых данных.
	incompleteMarkers []string
	// UnRAR переименовывает имя, недопустимое в Windows, и при этом отвечает
	// кодом ошибки создания файла; это ошибка имени, а не диска.
	renameMarkers []string
}

// -snl- заставляет 7-Zip 25.01 не создавать ссылку из записи перенаправления,
// о которой rardecode не знает, а распаковать её как обычный каталог. Без него
// 7-Zip сам пробует создать ссылку и писать за ней. Старый 7-Zip переключатель
// отвергает кодом 7: такой распаковщик отклоняется, а не запускается заново без
// -snl-.
func sevenZipTool(path string) archiveTool {
	return archiveTool{
		name: "7-Zip",
		path: path,
		args: func(archive, dest string) []string {
			return []string{"x", "-y", "-aoa", "-snl-", "-bso0", "-bsp1", "-sccUTF-8", "-o" + dest, "--", archive}
		},
		exitCauses: map[int]error{toolCommandLineError: errToolOutdated},
		warnExits:  map[int]bool{1: true},
		writeMarkers: []string{
			"cannot open output file", "cannot delete output file", "cannot delete output folder", "cannot create folder",
		},
		incompleteMarkers: []string{"unexpected end of archive"},
	}
}

// -ol- запрещает UnRAR создавать символические ссылки, но существует только с
// версии 7.00: старый UnRAR может отвергнуть его или принять за -ol, то есть
// включить обработку ссылок. Для него переключатель не передаётся, и защитой
// остаются пропуск небезопасных ссылок самим UnRAR и verifyExtracted. На жёсткие
// ссылки переключателя нет ни у одной версии.
func unrarTool(path string, linkSwitch bool) archiveTool {
	return archiveTool{
		name: "UnRAR",
		path: path,
		args: func(archive, dest string) []string {
			args := []string{"x", "-y", "-o+", "-p-", "-idc"}
			if linkSwitch {
				args = append(args, "-ol-")
			}
			return append(args, "--", archive, dest+string(filepath.Separator))
		},
		// 3 и 13 — «Invalid checksum. Data is damaged» и «Bad archive» (Rar.txt,
		// раздел Exit values): эталонный декодер подтвердил, что данные повреждены.
		exitCauses: map[int]error{
			3: errArchiveCorrupt, 5: errToolWrite, toolCommandLineError: errToolOutdated,
			9: errToolWrite, 11: errArchiveEncrypted, 13: errArchiveCorrupt,
		},
		incompleteMarkers: []string{"unexpected end of archive"},
		renameMarkers:     []string{"attempting to correct the invalid file or directory name"},
	}
}

func (t archiveTool) extract(ctx context.Context, archive, dest string, onPercent func(int)) error {
	//nolint:gosec // G204: t.path is an absolute path to an existing file found by findArchiveTools (invariant 33); archive and dest come from the install record and follow "--"
	cmd := exec.CommandContext(ctx, t.path, t.args(archive, dest)...)
	cmd.SysProcAttr = toolProcAttr()
	cmd.WaitDelay = toolWaitDelay
	stderr := &tailBuffer{scan: newMarkerScan(t.renameMarkers, t.incompleteMarkers, t.writeMarkers)}
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("%s: %w", t.name, err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("запуск %s: %w", t.name, err)
	}
	stdoutTail := &tailBuffer{scan: newMarkerScan(t.renameMarkers, t.incompleteMarkers, t.writeMarkers)}
	scanErr := scanToolOutput(stdout, stdoutTail, onPercent)
	waitErr := cmd.Wait()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if waitErr != nil {
		detail := stderr.text()
		if detail == "" {
			detail = stdoutTail.text()
		}
		var exitErr *exec.ExitError
		if !errors.As(waitErr, &exitErr) {
			return fmt.Errorf("%s: %w: %s", t.name, waitErr, detail)
		}
		code := exitErr.ExitCode()
		if !t.warnExits[code] {
			stderr.scan.absorb(stdoutTail.scan)
			return t.exitError(code, detail, stderr.scan)
		}
		slog.Warn("external tool finished with a warning", "tool", t.name, "code", code, "output", detail)
	}
	if scanErr != nil {
		return fmt.Errorf("%s: чтение вывода: %w", t.name, scanErr)
	}
	return nil
}

// Фраза про обрезанный архив и переименование имени точнее кода выхода: UnRAR
// сообщает обрыв кодом «данные повреждены», а переименование — кодом ошибки
// создания файла.
func (t archiveTool) exitError(code int, detail string, seen *markerScan) error {
	switch {
	case seen.found(t.renameMarkers):
		return fmt.Errorf("%s: %w: %w (код %d): %s", t.name, errArchiveMismatch, errArchiveUnlisted, code, detail)
	case seen.found(t.incompleteMarkers):
		return fmt.Errorf("%s: %w (код %d): %s", t.name, errArchiveIncomplete, code, detail)
	}
	cause, ok := t.exitCauses[code]
	if !ok {
		cause = errToolExit
		if seen.found(t.writeMarkers) {
			cause = errToolWrite
		}
	}
	return fmt.Errorf("%s: %w (код %d): %s", t.name, cause, code, detail)
}

func scanToolOutput(r io.Reader, tail *tailBuffer, onPercent func(int)) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	sc.Split(splitToolOutput)
	for sc.Scan() {
		token := sc.Bytes()
		tail.scan.feed(string(token) + "\n")
		matches := toolPercentExp.FindAllSubmatch(token, -1)
		if len(matches) == 0 {
			tail.line(token)
			continue
		}
		pct, err := strconv.Atoi(string(matches[len(matches)-1][1]))
		if err != nil {
			return err
		}
		onPercent(pct)
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return nil
}

// 7-Zip и UnRAR перерисовывают прогресс на месте через \r и \b, поэтому
// строкой считается кусок между любыми из этих символов.
func splitToolOutput(data []byte, atEOF bool) (int, []byte, error) {
	start := 0
	for start < len(data) && isToolBreak(data[start]) {
		start++
	}
	for i := start; i < len(data); i++ {
		if isToolBreak(data[i]) {
			return i + 1, data[start:i], nil
		}
	}
	if atEOF && start < len(data) {
		return len(data), data[start:], nil
	}
	return start, nil, nil
}

func isToolBreak(c byte) bool {
	return c == '\n' || c == '\r' || c == '\b'
}

type tailBuffer struct {
	buf  []byte
	scan *markerScan
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.scan.feed(string(p))
	b.buf = append(b.buf, p...)
	if len(b.buf) > toolOutputTail {
		b.buf = b.buf[len(b.buf)-toolOutputTail:]
	}
	return len(p), nil
}

func (b *tailBuffer) line(p []byte) {
	if len(bytes.TrimSpace(p)) == 0 {
		return
	}
	b.buf = append(b.buf, p...)
	b.buf = append(b.buf, '\n')
	if len(b.buf) > toolOutputTail {
		b.buf = b.buf[len(b.buf)-toolOutputTail:]
	}
}

func (b *tailBuffer) text() string {
	fields := strings.Fields(strings.ToValidUTF8(string(b.buf), "?"))
	return sanitizeText(strings.Join(fields, " "), toolOutputTail)
}

// Кандидаты — догадки о месте установки: отсутствующий или нечитаемый путь
// значит лишь, что распаковщика там нет, и проверяется следующий.
func firstRegularFile(candidates []string) string {
	for _, path := range candidates {
		if path == "" || !filepath.IsAbs(path) {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				slog.Warn("stat archive tool candidate", "path", path, "error", err)
			}
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}
		return path
	}
	return ""
}

// Ненайденный в PATH распаковщик — штатный случай «не установлен», а
// относительный результат (ErrDot) запускать нельзя (инвариант 33).
func lookPathAbs(name string) string {
	path, err := exec.LookPath(name)
	if err != nil || !filepath.IsAbs(path) {
		return ""
	}
	return path
}
