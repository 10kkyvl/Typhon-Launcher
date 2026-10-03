//go:build windows

package installguard

import (
	"fmt"
	"log/slog"

	"golang.org/x/sys/windows"
)

const (
	swHide           = 0
	swShowNoActivate = 4
	wsVisible        = 0x10000000
)

type verifierProc struct {
	passes int
	gaveUp bool
}

// verifierSkip ends QuickSFV when repack verification is switched off. closed
// maps the owning PID to the guard passes elapsed since WM_CLOSE was sent.
// A verifier the guard failed to end is kept with gaveUp set: it is shown
// again and left alone, so a failing kill is not retried every pass.
type verifierSkip struct {
	closed    map[uint32]*verifierProc
	terminate func(pid uint32) error
	exited    func(h windows.Handle) (bool, error)
	show      func(top, command uintptr)
	log       *slog.Logger
}

func newVerifierSkip() *verifierSkip {
	return &verifierSkip{
		closed:    map[uint32]*verifierProc{},
		terminate: terminateVerifier,
		exited:    processExited,
		show:      showVerifierWindow,
		log:       slog.Default(),
	}
}

func (v *verifierSkip) window(top uintptr, pid uint32) {
	tracked, seen := v.closed[pid]
	if seen && tracked.gaveUp {
		if style(top)&wsVisible == 0 {
			v.show(top, swShowNoActivate)
		}
		return
	}
	if style(top)&wsVisible != 0 {
		v.show(top, swHide)
	}
	if nextVerifierAction(false, seen, 0) != verifierClose {
		return
	}
	v.closed[pid] = &verifierProc{}
	v.log.Info("repack verification skipped", "pid", pid)
	message(top, 0x10, 0, 0) // WM_CLOSE
}

// Async because the window belongs to another process and must not block the guard.
func showVerifierWindow(top, command uintptr) {
	//nolint:errcheck,gosec // главный принцип: ошибка не подменяется значением, исход запроса проверяется по WS_VISIBLE на каждом проходе guard, а не по ответу ShowWindowAsync
	showWindow.Call(top, command)
}

// sweep counts passes for every closed verifier and ends the ones that ignored
// WM_CLOSE. The held handle keeps the PID from being reused, so the process
// terminated here is the one whose window was closed.
func (v *verifierSkip) sweep(handles map[uint32]windows.Handle) {
	for pid, tracked := range v.closed {
		held, ok := handles[pid]
		if !ok {
			delete(v.closed, pid)
			continue
		}
		exited, err := v.exited(held)
		if err != nil {
			v.giveUp(pid, tracked, "wait", err)
			continue
		}
		if exited {
			delete(v.closed, pid)
			continue
		}
		if tracked.gaveUp {
			continue
		}
		tracked.passes++
		if nextVerifierAction(false, true, tracked.passes) != verifierTerminate {
			continue
		}
		if err := v.terminate(pid); err != nil {
			v.giveUp(pid, tracked, "terminate", err)
			continue
		}
		v.log.Info("repack verifier terminated", "pid", pid)
		delete(v.closed, pid)
	}
}

func (v *verifierSkip) giveUp(pid uint32, tracked *verifierProc, step string, err error) {
	if tracked.gaveUp {
		return
	}
	tracked.gaveUp = true
	v.log.Warn("repack verifier left running", "pid", pid, "step", step, "error", err)
}

func processExited(h windows.Handle) (bool, error) {
	event, err := windows.WaitForSingleObject(h, 0)
	if err != nil {
		return false, err
	}
	return event == windows.WAIT_OBJECT_0, nil
}

func terminateVerifier(pid uint32) error {
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return fmt.Errorf("open repack verifier %d: %w", pid, err)
	}
	defer func() {
		if err := windows.CloseHandle(h); err != nil {
			slog.Warn("close verifier handle", "pid", pid, "error", err)
		}
	}()
	if err := windows.TerminateProcess(h, 1); err != nil {
		return fmt.Errorf("terminate repack verifier %d: %w", pid, err)
	}
	return nil
}
