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

// List's bool result always reports true on success: a single ps snapshot
// either yields the whole process table for our bottles or fails outright
// (below), there is no partial mode to report.
func List(ctx context.Context) ([]Process, bool, error) {
	return listWith(ctx, sharedManager())
}

// listWith отделён от List, чтобы отсутствие CrossOver проверялось тестом:
// на машине сборки он может быть и установлен, и нет.
func listWith(ctx context.Context, m *wine.Manager) ([]Process, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if m == nil {
		return nil, true, nil
	}
	found, err := m.AllProcesses(ctx)
	if err != nil {
		return nil, false, err
	}
	out := make([]Process, 0, len(found))
	for _, p := range found {
		//nolint:gosec // G115: pid в macOS укладывается в uint32
		out = append(out, Process{PID: uint32(p.PID), Path: p.Path, CreatedAt: p.CreatedAt})
	}
	return out, true, nil
}
