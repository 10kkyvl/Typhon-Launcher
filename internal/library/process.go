package library

import (
	"context"
	"os/exec"
)

type gameProcess interface {
	pid() int
	wait() error
	kill() error
}

// gameStarter получает контекст жизни сервиса: запуск игры на некоторых
// платформах идёт через внешний процесс, и обрывать его надо вместе с
// лаунчером, а не оставлять висеть.
type gameStarter func(ctx context.Context, executable string, args []string, dir string) (gameProcess, error)

type execProcess struct {
	cmd *exec.Cmd
}

func (p *execProcess) pid() int { return p.cmd.Process.Pid }

func (p *execProcess) wait() error { return p.cmd.Wait() }

func (p *execProcess) kill() error { return p.cmd.Process.Kill() }

// execStarter не пользуется контекстом намеренно: игра переживает лаунчер,
// и завершение лаунчера убивать её не должно.
func execStarter(_ context.Context, executable string, args []string, dir string) (gameProcess, error) {
	//nolint:gosec // G204: путь берётся из записи библиотеки и проверен os.Stat в PlayGame (инвариант 32); запуск переменного исполняемого файла — суть лаунчера
	cmd := exec.Command(executable, args...)
	cmd.Dir = dir
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &execProcess{cmd: cmd}, nil
}
