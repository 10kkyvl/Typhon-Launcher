//go:build windows

package installguard

import (
	"log/slog"

	"golang.org/x/sys/windows"
)

// verifierSkip ends QuickSFV when repack verification is switched off. closed
// maps the owning PID to the guard passes elapsed since WM_CLOSE was sent.
type verifierSkip struct {
	closed map[uint32]int
}

func newVerifierSkip() *verifierSkip {
	return &verifierSkip{closed: map[uint32]int{}}
}

func (v *verifierSkip) window(top uintptr, pid uint32) {
	if style(top)&0x10000000 != 0 {
		//nolint:errcheck,gosec // Win32/COM result is carried by the return value or output buffer; last-error is not authoritative here. G104: native result/output is used; last-error is not authoritative, cleanup is best effort.
		showWindow.Call(top, 0)
	}
	_, seen := v.closed[pid]
	if nextVerifierAction(false, seen, 0) != verifierClose {
		return
	}
	v.closed[pid] = 0
	slog.Info("repack verification skipped", "pid", pid)
	message(top, 0x10, 0, 0) // WM_CLOSE
}

// sweep counts passes for every closed verifier and ends the ones that ignored
// WM_CLOSE. The held handle keeps the PID from being reused, so the process
// terminated here is the one whose window was closed.
func (v *verifierSkip) sweep(handles map[uint32]windows.Handle) {
	for pid, waited := range v.closed {
		held, ok := handles[pid]
		if !ok {
			delete(v.closed, pid)
			continue
		}
		exited, err := processExited(held)
		if err != nil {
			slog.Warn("wait for repack verifier", "pid", pid, "error", err)
			delete(v.closed, pid)
			continue
		}
		if exited {
			delete(v.closed, pid)
			continue
		}
		v.closed[pid] = waited + 1
		if nextVerifierAction(false, true, waited+1) != verifierTerminate {
			continue
		}
		terminateVerifier(pid)
		delete(v.closed, pid)
	}
}

func processExited(h windows.Handle) (bool, error) {
	event, err := windows.WaitForSingleObject(h, 0)
	if err != nil {
		return false, err
	}
	return event == windows.WAIT_OBJECT_0, nil
}

func terminateVerifier(pid uint32) {
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		slog.Warn("repack verifier did not exit and cannot be terminated", "pid", pid, "error", err)
		return
	}
	defer func() {
		if err := windows.CloseHandle(h); err != nil {
			slog.Warn("close verifier handle", "pid", pid, "error", err)
		}
	}()
	if err := windows.TerminateProcess(h, 1); err != nil {
		slog.Warn("terminate repack verifier", "pid", pid, "error", err)
		return
	}
	slog.Info("repack verifier terminated", "pid", pid)
}
