//go:build windows

package procs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Parents returns a pid-to-parent snapshot of the whole process table.
func Parents(ctx context.Context) (map[uint32]uint32, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return ParentsNow()
}

// ParentsNow is Parents for a caller that has no context to give and must not
// be cancelled, such as ending a game process. A walk cut short by
// Process32Next is an error, not a partial answer: a tree lookup built on
// half a table would miss exactly the child it is after.
func ParentsNow() (map[uint32]uint32, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("create process snapshot: %w", err)
	}
	defer func() {
		if cerr := windows.CloseHandle(snapshot); cerr != nil {
			slog.Warn("close process snapshot handle", "error", cerr)
		}
	}()

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(snapshot, &entry); err != nil {
		return nil, fmt.Errorf("enumerate first process: %w", err)
	}
	out := make(map[uint32]uint32)
	for {
		out[entry.ProcessID] = entry.ParentProcessID
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
				return out, nil
			}
			return nil, fmt.Errorf("enumerate next process: %w", err)
		}
	}
}
