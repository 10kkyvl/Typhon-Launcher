//go:build windows

package procs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// maxImagePathRunes bounds the QueryFullProcessImageName retry loop: paths
// can exceed MAX_PATH, but the buffer must not grow without limit.
const maxImagePathRunes = 32768

func Supported() bool { return true }

func List(ctx context.Context) ([]Process, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
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

	seen := make(map[uint32]struct{})
	var out []Process
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if _, dup := seen[entry.ProcessID]; !dup {
			seen[entry.ProcessID] = struct{}{}
			out = append(out, inspectProcess(entry.ProcessID))
		}
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
				break
			}
			return nil, fmt.Errorf("enumerate next process: %w", err)
		}
	}
	return out, nil
}

// inspectProcess is factored out of List's loop so its deferred
// CloseHandle runs at the end of every iteration instead of accumulating
// thousands of open handles until List itself returns.
func inspectProcess(pid uint32) Process {
	p := Process{PID: pid}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		p.PathUnknown = true
		p.CreatedAtUnknown = true
		return p
	}
	defer func() {
		if cerr := windows.CloseHandle(handle); cerr != nil {
			slog.Warn("close process handle", "pid", pid, "error", cerr)
		}
	}()

	path, err := queryImagePath(handle)
	if err != nil {
		p.PathUnknown = true
	} else {
		p.Path = path
	}

	created, err := queryCreatedAt(handle)
	if err != nil {
		p.CreatedAtUnknown = true
	} else {
		p.CreatedAt = created
	}
	return p
}

func queryImagePath(handle windows.Handle) (string, error) {
	size := uint32(windows.MAX_PATH)
	for {
		buf := make([]uint16, size)
		n := size
		err := windows.QueryFullProcessImageName(handle, 0, &buf[0], &n)
		if err == nil {
			return windows.UTF16ToString(buf[:n]), nil
		}
		if !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) || size >= maxImagePathRunes {
			return "", fmt.Errorf("query image name: %w", err)
		}
		size *= 2
		if size > maxImagePathRunes {
			size = maxImagePathRunes
		}
	}
}

func queryCreatedAt(handle windows.Handle) (time.Time, error) {
	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(handle, &creation, &exit, &kernel, &user); err != nil {
		return time.Time{}, fmt.Errorf("get process times: %w", err)
	}
	return time.Unix(0, creation.Nanoseconds()), nil
}
