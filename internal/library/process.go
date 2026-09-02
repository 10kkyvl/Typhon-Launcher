package library

import "os/exec"

type gameProcess interface {
	pid() int
	wait() error
	kill() error
}

type gameStarter func(executable string, args []string, dir string) (gameProcess, error)

type execProcess struct {
	cmd *exec.Cmd
}

func (p *execProcess) pid() int { return p.cmd.Process.Pid }

func (p *execProcess) wait() error { return p.cmd.Wait() }

func (p *execProcess) kill() error { return p.cmd.Process.Kill() }

func execStarter(executable string, args []string, dir string) (gameProcess, error) {
	//nolint:gosec // G204: путь берётся из записи библиотеки и проверен os.Stat в PlayGame (инвариант 32); запуск переменного исполняемого файла — суть лаунчера
	cmd := exec.Command(executable, args...)
	cmd.Dir = dir
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &execProcess{cmd: cmd}, nil
}
