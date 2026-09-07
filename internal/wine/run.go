package wine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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

// StartDetached запускает и возвращается сразу. Ни Run, ни CombinedOutput
// здесь не годятся, и обе ловушки проверены на живой игре.
//
// CombinedOutput ждёт не завершения cxstart, а закрытия пайпов, которые
// наследует запущенная игра, — то есть до выхода из игры.
//
// Run ждёт немногим меньше: cxstart подменяет себя winewrapper'ом, тот
// остаётся прямым потомком лаунчера и живёт всё время игры. Ожидание вешало
// PlayGame целиком, вместе с мьютексом библиотеки.
//
// Поэтому запускаем и отпускаем, а состояние узнаём из перечисления процессов
// бутыля: Wait в фоне нужен только чтобы не оставлять зомби. Диагностика идёт
// в файл через Cmd.Log (--cx-log), а не через перехват потоков.
func (m *Manager) StartDetached(ctx context.Context, b Bottle, c Cmd) error {
	//nolint:gosec // G204: путь до cxstart получен из Detect, аргументы собраны cxstartArgs
	cmd := exec.CommandContext(ctx, m.rt.CxStart, cxstartArgs(b, c, false)...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("запуск %s в бутыле %s: %w", c.Path, b.Name, err)
	}
	go func() {
		// Ошибка тут не про запуск игры, а про судьбу обёртки: сам факт
		// запуска подтверждает появление процесса в бутыле. Записываем её на
		// debug, чтобы «игра закрылась сама» осталось объяснимым.
		if err := cmd.Wait(); err != nil {
			slog.Debug("cxstart wrapper exited", "bottle", b.Name, "path", c.Path, "error", err)
		}
	}()
	return nil
}

// Boot прогревает свежий бутыль. Первый запуск инициализирует префикс —
// поднимает services.exe, разворачивает реестр, — и на живой игре это заняло
// минуты. Делать это в момент, когда пользователь нажал «Играть», нельзя:
// прогрев уходит туда, где ожидание уместно, то есть в установку.
func (m *Manager) Boot(ctx context.Context, b Bottle) error {
	//nolint:gosec // G204: путь до cxstart получен из Detect, имя бутыля из нашей метки
	cmd := exec.CommandContext(ctx, m.rt.CxStart, "--bottle", b.Name, "--no-gui", "--wait", "--", "wineboot", "-u")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("прогрев бутыля %s: %w: %s", b.Name, err, strings.TrimSpace(string(out)))
	}
	return nil
}
