package wine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Cmd — команда, запускаемая внутри бутыля. Path и WorkDir — windows-пути:
// перевод делает вызывающий через Bottle.ToWindows, потому что только он
// знает, откуда путь взялся.
type Cmd struct {
	Path          string
	Args          []string
	WorkDir       string
	Log           string
	DLLOverrides  string
	DebugMessages string
	WinVer        string
	WaitChildren  bool
	// InstallerGuard is interpreted by the installer runner, never game launches.
	InstallerGuard         bool
	HideProgress           bool
	Limit32BitAddressSpace bool
	// Additional executables launched by a bridge, for cancellation in a shared bottle.
	StopPaths  []string
	CancelFile string
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
	if c.DebugMessages != "" {
		args = append(args, "--debugmsg", c.DebugMessages)
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

// ErrTreeNotStopped значит: ctx отменён, но Kill не смог подтвердить, что
// все процессы бутыля остановлены. cxstart подменяет себя winewrapper'ом —
// прямым потомком лаунчера, — и завершение только его не гасит установщик,
// который тот запустил внутри wine; вызывающий не может считать писателя
// мёртвым, пока Kill не подтвердил остановку бутыля целиком.
var ErrTreeNotStopped = errors.New("wine: не удалось остановить процессы в бутыле")

// Run ждёт завершения и отдаёт код возврата установщика. Ненулевой код — не
// ошибка Go: разбирать его умеет вызывающий, у которого есть движок и лог.
func (m *Manager) Run(ctx context.Context, b Bottle, c Cmd) (int, error) {
	//nolint:gosec // G204: путь до cxstart получен из Detect, аргументы собраны cxstartArgs
	cmd := exec.CommandContext(ctx, m.rt.CxStart, cxstartArgs(b, c, true)...)
	// Never let inherited output pipes postpone cancellation indefinitely.
	cmd.WaitDelay = 5 * time.Second
	if c.CancelFile != "" {
		//nolint:forbidigo // fresh private temporary helper file/IPC marker, never persistent user state.
		cmd.Cancel = func() error { return os.WriteFile(c.CancelFile, []byte("cancel\n"), 0600) }
	}
	out, err := cmd.CombinedOutput()
	if os.Getenv("TYPHON_INSTALLGUARD_TRACE") == "1" {
		slog.Info("installer bridge diagnostic", "output", string(out))
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		if c.CancelFile != "" {
			ack, readErr := os.ReadFile(c.CancelFile + ".stopped")
			if readErr == nil && string(ack) == "stopped\n" {
				return 0, ctxErr
			}
			if stopErr := m.stopBottle(b, append([]string{c.Path}, c.StopPaths...)...); stopErr != nil {
				slog.Warn("installer fallback stop failed", "error", stopErr)
			}
			return 0, fmt.Errorf("%w: %w: installer bridge did not confirm termination", ErrTreeNotStopped, ctxErr)
		}
		// exec.CommandContext убивает только прямого потомка (cxstart, он же
		// winewrapper): установщик внутри бутыля остаётся жив, поэтому
		// требуется свалить бутыль целиком. Общий бутыль — исключение: там
		// гасятся только процессы этой установки, иначе отмена установки
		// уронила бы Steam и все чужие игры того же префикса.
		if killErr := m.stopBottle(b, append([]string{c.Path}, c.StopPaths...)...); killErr != nil {
			return 0, fmt.Errorf("%w: %w: %w", ErrTreeNotStopped, ctxErr, killErr)
		}
		return 0, ctxErr
	}

	if c.CancelFile != "" && cmd.ProcessState != nil {
		ack, readErr := os.ReadFile(c.CancelFile + ".stopped")
		if readErr != nil || string(ack) != "stopped\n" {
			return 0, errors.Join(fmt.Errorf("%w: installer bridge exited without confirming completion", ErrTreeNotStopped), err)
		}
		// The bridge has confirmed that no installer writers remain. Wine services
		// may retain a diagnostic pipe after a successful wrapper exit.
		if errors.Is(err, exec.ErrWaitDelay) && cmd.ProcessState.Success() {
			return 0, nil
		}
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		// Keep the child diagnostic: bridge/startup failures otherwise look like
		// an unexplained installer exit code.
		tail := out
		if len(tail) > 4096 {
			tail = tail[len(tail)-4096:]
		}
		slog.Warn("wine process exited", "path", c.Path, "code", exit.ExitCode(), "output", strings.TrimSpace(string(tail)))
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
	// Путь и launch options нужны для диагностики, но сами аргументы игры
	// могут содержать токены или пароль и в журнал попадать не должны.
	slog.Info("cxstart", "bottle", b.Name, "path", c.Path, "workDir", c.WorkDir,
		"dllOverrides", c.DLLOverrides, "argCount", len(c.Args))
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
