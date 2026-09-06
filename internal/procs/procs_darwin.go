//go:build darwin && !devmock

package procs

import (
	"context"
	"sync"

	"typhon/internal/wine"
)

func Supported() bool { return true }

// Менеджер заводится один раз: Detect ходит в файловую систему, а List зовёт
// цикл детекта игр каждые несколько секунд.
var (
	managerOnce sync.Once
	manager     *wine.Manager
)

func sharedManager() *wine.Manager {
	managerOnce.Do(func() {
		rt, err := wine.Detect()
		if err != nil {
			return
		}
		manager = wine.NewManager(rt)
	})
	return manager
}

func List(ctx context.Context) ([]Process, error) {
	return listWith(ctx, sharedManager())
}

// listWith отделён от List, чтобы отсутствие CrossOver проверялось тестом:
// на машине сборки он может быть и установлен, и нет.
func listWith(ctx context.Context, m *wine.Manager) ([]Process, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if m == nil {
		return nil, nil
	}
	found, err := m.AllProcesses(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Process, 0, len(found))
	for _, p := range found {
		//nolint:gosec // G115: pid в macOS укладывается в uint32
		out = append(out, Process{PID: uint32(p.PID), Path: p.Path, CreatedAt: p.CreatedAt})
	}
	return out, nil
}
