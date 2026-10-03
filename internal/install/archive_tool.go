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
	toolOutputTail = 1024
	toolWaitDelay  = 5 * time.Second
)

var (
	errToolExit    = errors.New("распаковщик завершился с ошибкой")
	errToolWrite   = uierr.New("install.archive_tool_write_failed", "распаковщик не смог записать файлы на диск")
	toolPercentExp = regexp.MustCompile(`(\d{1,3})%`)
)

type archiveTool struct {
	name string
	path string
	args func(archive, dest string) []string
	// Коды выхода с известной причиной. Ошибку записи или пароль следующий
	// распаковщик не исправит, поэтому по ним перебор останавливается.
	exitCauses map[int]error
}

func sevenZipTool(path string) archiveTool {
	return archiveTool{name: "7-Zip", path: path, args: func(archive, dest string) []string {
		return []string{"x", "-y", "-aoa", "-bso0", "-bsp1", "-sccUTF-8", "-o" + dest, "--", archive}
	}}
}

func unrarTool(path string) archiveTool {
	return archiveTool{
		name: "UnRAR",
		path: path,
		args: func(archive, dest string) []string {
			return []string{"x", "-y", "-o+", "-p-", "-idc", "--", archive, dest + string(filepath.Separator)}
		},
		exitCauses: map[int]error{5: errToolWrite, 9: errToolWrite, 11: errArchiveEncrypted},
	}
}

func (t archiveTool) extract(ctx context.Context, archive, dest string, onPercent func(int)) error {
	//nolint:gosec // G204: t.path is an absolute path to an existing file found by findArchiveTools (invariant 33); archive and dest come from the install record and follow "--"
	cmd := exec.CommandContext(ctx, t.path, t.args(archive, dest)...)
	cmd.SysProcAttr = toolProcAttr()
	cmd.WaitDelay = toolWaitDelay
	stderr := &tailBuffer{}
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("%s: %w", t.name, err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("запуск %s: %w", t.name, err)
	}
	stdoutTail := &tailBuffer{}
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
		if errors.As(waitErr, &exitErr) {
			cause, ok := t.exitCauses[exitErr.ExitCode()]
			if !ok {
				cause = errToolExit
			}
			return fmt.Errorf("%s: %w (код %d): %s", t.name, cause, exitErr.ExitCode(), detail)
		}
		return fmt.Errorf("%s: %w: %s", t.name, waitErr, detail)
	}
	if scanErr != nil {
		return fmt.Errorf("%s: чтение вывода: %w", t.name, scanErr)
	}
	return nil
}

func scanToolOutput(r io.Reader, tail *tailBuffer, onPercent func(int)) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	sc.Split(splitToolOutput)
	for sc.Scan() {
		token := sc.Bytes()
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
	buf []byte
}

func (b *tailBuffer) Write(p []byte) (int, error) {
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
	return strings.Join(fields, " ")
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
