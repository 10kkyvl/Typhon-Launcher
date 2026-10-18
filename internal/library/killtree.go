package library

import (
	"errors"
	"fmt"
	"os"

	"typhon/internal/procs"
)

var errNoRootPID = errors.New("у сессии нет pid процесса")

// killTree ends the processes below root, deepest first, and then root
// itself. A launcher wrapper such as a Steam stub hands the work to a child
// that outlives it, so ending only the pid the session is bound to leaves the
// game running. A failure on one process never stops the rest: all of them
// are attempted and the errors come back together.
func killTree(root uint32, parents func() (map[uint32]uint32, error), killPID func(pid uint32) error, killRoot func() error) error {
	if root == 0 {
		return errNoRootPID
	}
	self := uint32(os.Getpid()) //nolint:gosec // G115: PID из ОС укладывается в uint32
	var errs []error
	table, err := parents()
	if err != nil {
		errs = append(errs, fmt.Errorf("список дочерних процессов игры: %w", err))
	} else {
		for _, pid := range procs.Descendants(table, root) {
			if pid == self {
				continue
			}
			if err := killPID(pid); err != nil {
				errs = append(errs, fmt.Errorf("дочерний процесс %d: %w", pid, err))
			}
		}
	}
	if err := killRoot(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
