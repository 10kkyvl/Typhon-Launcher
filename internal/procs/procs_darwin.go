//go:build darwin && !devmock

package procs

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sys/unix"

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

// List's bool result always reports true on success: the ps snapshot of the
// bottles and the kernel's process table either both come back whole or the
// call fails outright (below), there is no partial mode to report.
func List(ctx context.Context) ([]Process, bool, error) {
	return listWith(ctx, sharedManager(), nativeTable)
}

// listWith отделён от List, чтобы отсутствие CrossOver проверялось тестом:
// на машине сборки он может быть и установлен, и нет.
//
// Процессы бутылей приходят с путём, остальные — только с pid и временем
// старта. Без них полный снимок не видел эмулятор или нативную игру, которые
// лаунчер запустил сам, и detectTick закрывал их сессию на следующем тике.
// Путь у них неизвестен намеренно: сессию по ним цикл детекта не открывает.
func listWith(ctx context.Context, m *wine.Manager, native func() ([]Process, error)) ([]Process, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	var out []Process
	seen := make(map[uint32]bool)
	if m != nil {
		found, err := m.AllProcesses(ctx)
		if err != nil {
			return nil, false, err
		}
		out = make([]Process, 0, len(found))
		for _, p := range found {
			//nolint:gosec // G115: pid в macOS укладывается в uint32
			pid := uint32(p.PID)
			out = append(out, Process{PID: pid, Path: p.Path, CreatedAt: p.CreatedAt})
			seen[pid] = true
		}
	}
	table, err := native()
	if err != nil {
		return nil, false, err
	}
	for _, p := range table {
		if !seen[p.PID] {
			out = append(out, p)
		}
	}
	return out, true, nil
}

// sZomb is SZOMB of <sys/proc.h>: the process has exited and waits to be reaped.
const sZomb = 5

func nativeTable() ([]Process, error) {
	kinfos, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil, fmt.Errorf("таблица процессов: %w", err)
	}
	return nativeProcesses(kinfos), nil
}

func nativeProcesses(kinfos []unix.KinfoProc) []Process {
	out := make([]Process, 0, len(kinfos))
	for _, k := range kinfos {
		if k.Proc.P_pid <= 0 || k.Proc.P_stat == sZomb {
			continue
		}
		// Секунды, как у lstart в ps бутылей: один и тот же процесс из двух
		// источников должен получить одно время старта, иначе сверка личности
		// в detectTick примет его за другой процесс с тем же pid.
		started := time.Unix(k.Proc.P_starttime.Sec, 0)
		out = append(out, Process{PID: uint32(k.Proc.P_pid), PathUnknown: true, CreatedAt: started})
	}
	return out
}
