//go:build darwin

package emulators

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

// sZomb is SZOMB of <sys/proc.h>: a process that has exited and waits to be reaped.
const sZomb = 5

// maxComm is MAXCOMLEN: the kernel keeps this much of a program's file name.
const maxComm = 16

// On macOS every copy shares the lock in ~/Library/Caches/rpcs3, so a copy of
// this user counts by its name, wherever its bundle lies.
var systemCopies = copyFinder{list: darwinCopies, ask: darwinSignal(syscall.SIGTERM), force: darwinSignal(syscall.SIGKILL)}

func darwinCopies(ctx context.Context, program string) ([]int, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	kinfos, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil, fmt.Errorf("таблица процессов: %w", err)
	}
	return copiesNamed(kinfos, filepath.Base(program), os.Getuid(), os.Getpid()), nil
}

func copiesNamed(kinfos []unix.KinfoProc, name string, uid, self int) []int {
	if len(name) > maxComm {
		name = name[:maxComm]
	}
	var out []int
	for _, k := range kinfos {
		pid := int(k.Proc.P_pid)
		if pid <= 0 || pid == self || k.Proc.P_stat == sZomb || int(k.Eproc.Ucred.Uid) != uid {
			continue
		}
		if unix.ByteSliceToString(k.Proc.P_comm[:]) == name {
			out = append(out, pid)
		}
	}
	return out
}

func darwinSignal(sig syscall.Signal) func(pid int) error {
	return func(pid int) error {
		if err := syscall.Kill(pid, sig); err != nil && err != syscall.ESRCH {
			return err
		}
		return nil
	}
}
