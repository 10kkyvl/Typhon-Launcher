//go:build devmock && !windows

package library

import (
	"context"

	"typhon/internal/devmock"
)

type devmockProcess struct {
	proc *devmock.Process
}

func (p *devmockProcess) pid() int { return int(p.proc.PID()) }

func (p *devmockProcess) wait() error { return p.proc.Wait() }

func (p *devmockProcess) kill() error { return p.proc.Kill() }

func newGameStarter() gameStarter {
	return func(_ context.Context, req launch) (gameProcess, error) {
		proc, err := devmock.Start(req.executable, req.args, req.workDir)
		if err != nil {
			return nil, err
		}
		return &devmockProcess{proc: proc}, nil
	}
}
