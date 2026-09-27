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

// launch — всё, что платформенному слою нужно знать о запуске игры.
// Структура, а не список аргументов: набор нужного у платформ разный, и
// каждое новое поле иначе расходилось бы сразу по четырём сигнатурам.
type launch struct {
	installDir string
	executable string
	args       []string
	workDir    string

	// shared — игре разрешено ехать в общий бутыль CrossOver, где живёт
	// windows Steam. Разрешено не значит «поедет»: если общего бутыля на
	// машине нет, игра останется в собственном.
	shared bool
}

// gameStarter получает контекст жизни сервиса: запуск игры на некоторых
// платформах идёт через внешний процесс, и обрывать его надо вместе с
// лаунчером, а не оставлять висеть.
type gameStarter func(ctx context.Context, req launch) (gameProcess, error)

type execProcess struct {
	cmd *exec.Cmd
}

func (p *execProcess) pid() int { return p.cmd.Process.Pid }

func (p *execProcess) wait() error { return p.cmd.Wait() }

func (p *execProcess) kill() error { return p.cmd.Process.Kill() }

// execStarter не пользуется контекстом намеренно: игра переживает лаунчер,
// и завершение лаунчера убивать её не должно.
func execStarter(_ context.Context, req launch) (gameProcess, error) {
	//nolint:gosec // G204: путь берётся из записи библиотеки и проверен os.Stat в PlayGame (инвариант 32); запуск переменного исполняемого файла — суть лаунчера
	cmd := exec.Command(req.executable, req.args...)
	cmd.Dir = req.workDir
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &execProcess{cmd: cmd}, nil
}
