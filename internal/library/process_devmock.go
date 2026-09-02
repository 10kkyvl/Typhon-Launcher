//go:build devmock && !windows

package library

import "typhon/internal/devmock"

type devmockProcess struct {
	proc *devmock.Process
}

func (p *devmockProcess) pid() int { return int(p.proc.PID()) }

func (p *devmockProcess) wait() error { return p.proc.Wait() }

func (p *devmockProcess) kill() error { return p.proc.Kill() }

func newGameStarter() gameStarter {
	return func(executable string, args []string, dir string) (gameProcess, error) {
		proc, err := devmock.Start(executable, args, dir)
		if err != nil {
			return nil, err
		}
		return &devmockProcess{proc: proc}, nil
	}
}
