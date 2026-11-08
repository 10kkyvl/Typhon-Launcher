package emulators

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"
)

const (
	// closeWait is how long an emulator gets to save and quit on its own.
	closeWait = 10 * time.Second
	closePoll = 100 * time.Millisecond
)

// SingleInstance says whether a second copy of the emulator refuses to start
// while one runs: RPCS3 holds a lock in the user's cache and stops a new copy
// with an error the player has to dismiss.
func (p Platform) SingleInstance() bool {
	return p.singleInstance
}

// copyFinder lists the running copies of a program and closes one, gently or
// not. It is a value so that tests never touch real processes.
type copyFinder struct {
	list  func(ctx context.Context, program string) ([]int, error)
	ask   func(pid int) error
	force func(pid int) error
}

// RunningCopies lists the copies of program that would stop a new one.
func RunningCopies(ctx context.Context, program string) ([]int, error) {
	return systemCopies.list(ctx, program)
}

// CloseRunning asks every copy of program to quit, waits for them, and ends
// the ones that are still there. It returns only when none is left or with an
// error that says which are.
func CloseRunning(ctx context.Context, program string) error {
	return systemCopies.close(ctx, program, closeWait)
}

func (f copyFinder) close(ctx context.Context, program string, wait time.Duration) error {
	pids, err := f.list(ctx, program)
	if err != nil {
		return fmt.Errorf("найти открытый эмулятор: %w", err)
	}
	if len(pids) == 0 {
		return nil
	}
	var asked []error
	for _, pid := range pids {
		if err := f.ask(pid); err != nil {
			asked = append(asked, fmt.Errorf("попросить закрыться %d: %w", pid, err))
		}
	}
	deadline := time.NewTimer(wait)
	defer deadline.Stop()
	tick := time.NewTicker(closePoll)
	defer tick.Stop()
	for {
		left, err := f.list(ctx, program)
		if err != nil {
			return fmt.Errorf("проверить эмулятор: %w", err)
		}
		if len(left) == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.Join(append(asked, ctx.Err())...)
		case <-deadline.C:
			return f.finish(ctx, program, left, asked)
		case <-tick.C:
		}
	}
}

// finish ends the copies that did not quit in time and checks that they are
// gone: a copy that survives is reported, never assumed closed.
func (f copyFinder) finish(ctx context.Context, program string, left []int, asked []error) error {
	var errs []error
	for _, pid := range left {
		if err := f.force(pid); err != nil {
			errs = append(errs, fmt.Errorf("завершить %d: %w", pid, err))
		}
	}
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(closePoll)
	defer tick.Stop()
	for {
		still, err := f.list(ctx, program)
		if err != nil {
			return errors.Join(append(errs, fmt.Errorf("проверить эмулятор: %w", err))...)
		}
		if len(still) == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.Join(append(errs, ctx.Err())...)
		case <-deadline.C:
			errs = append(asked, errs...)
			return errors.Join(append(errs, fmt.Errorf("%s не закрылся: %v", filepath.Base(program), still))...)
		case <-tick.C:
		}
	}
}
