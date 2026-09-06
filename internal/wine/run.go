package wine

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Cmd — команда, запускаемая внутри бутыля. Path и WorkDir — windows-пути:
// перевод делает вызывающий через Bottle.ToWindows, потому что только он
// знает, откуда путь взялся.
type Cmd struct {
	Path         string
	Args         []string
	WorkDir      string
	Log          string
	DLLOverrides string
	WinVer       string
	WaitChildren bool
}

// cxstartArgs собирает командную строку cxstart. Аргументы установщика
// передаются массивом после `--`: разбирать и склеивать их обратно нельзя,
// там ключи, которые не переживают ни разбиения, ни экранирования.
func cxstartArgs(b Bottle, c Cmd, wait bool) []string {
	args := []string{"--bottle", b.Name, "--no-gui"}
	switch {
	case !wait:
		args = append(args, "--no-wait")
	case c.WaitChildren:
		args = append(args, "--wait-children")
	default:
		args = append(args, "--wait")
	}
	if c.WorkDir != "" {
		args = append(args, "--workdir", c.WorkDir)
	}
	if c.Log != "" {
		args = append(args, "--cx-log", c.Log)
	}
	if c.DLLOverrides != "" {
		args = append(args, "--dll", c.DLLOverrides)
	}
	if c.WinVer != "" {
		args = append(args, "--winver", c.WinVer)
	}
	args = append(args, "--", c.Path)
	return append(args, c.Args...)
}

// Run ждёт завершения и отдаёт код возврата установщика. Ненулевой код — не
// ошибка Go: разбирать его умеет вызывающий, у которого есть движок и лог.
func (m *Manager) Run(ctx context.Context, b Bottle, c Cmd) (int, error) {
	//nolint:gosec // G204: путь до cxstart получен из Detect, аргументы собраны cxstartArgs
	cmd := exec.CommandContext(ctx, m.rt.CxStart, cxstartArgs(b, c, true)...)
	out, err := cmd.CombinedOutput()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return 0, ctxErr
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode(), nil
	}
	if err != nil {
		return 0, fmt.Errorf("запуск %s в бутыле %s: %w: %s", c.Path, b.Name, err, strings.TrimSpace(string(out)))
	}
	return 0, nil
}

// StartDetached запускает и возвращается сразу: pid игры отдаёт не cxstart, а
// перечисление процессов бутыля, поэтому ждать здесь нечего.
func (m *Manager) StartDetached(ctx context.Context, b Bottle, c Cmd) error {
	//nolint:gosec // G204: путь до cxstart получен из Detect, аргументы собраны cxstartArgs
	cmd := exec.CommandContext(ctx, m.rt.CxStart, cxstartArgs(b, c, false)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("запуск %s в бутыле %s: %w: %s", c.Path, b.Name, err, strings.TrimSpace(string(out)))
	}
	return nil
}
