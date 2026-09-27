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

// List returns the current process table. The bool result reports whether
// the enumeration ran to completion (a full, authoritative snapshot of every
// live pid) as opposed to being cut short mid-walk; a caller must not treat
// an absent pid as proof a process exited unless this is true. It is
// distinct from err: a hard failure of the snapshot itself (nothing usable
// obtained) is always an error, while a walk interrupted partway through
// (some pids seen, some not) is reported as incomplete without an error,
// since the pids collected so far are still real. Per-pid inspection
// failures (no permission to open a specific process) are normal and do not
// affect completeness — see inspectProcess.
func List(ctx context.Context) ([]Process, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, false, fmt.Errorf("create process snapshot: %w", err)
	}
	defer func() {
		if cerr := windows.CloseHandle(snapshot); cerr != nil {
			slog.Warn("close process snapshot handle", "error", cerr)
		}
	}()

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(snapshot, &entry); err != nil {
		return nil, false, fmt.Errorf("enumerate first process: %w", err)
	}

	seen := make(map[uint32]struct{})
	var out []Process
	for {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		if _, dup := seen[entry.ProcessID]; !dup {
			seen[entry.ProcessID] = struct{}{}
			out = append(out, inspectProcess(entry.ProcessID, imgCache))
		}
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
				break
			}
			// Process32Next itself faulted instead of running out of
			// entries: out holds every pid visited before the fault, which
			// is real data, just not exhaustive — an incomplete answer, not
			// the hard failure a caller would discard entirely.
			return out, false, nil
		}
	}
	imgCache.prune(pidSet(out))
	return out, true, nil
}

func pidSet(list []Process) map[uint32]struct{} {
	set := make(map[uint32]struct{}, len(list))
	for _, p := range list {
		set[p.PID] = struct{}{}
	}
	return set
}

// imgCache remembers each pid's resolved image path across ticks, keyed
// together with its creation time so a recycled pid never returns a stale
// path — see imageCache in procs.go.
var imgCache = newImageCache()

// inspectProcess is factored out of List's loop so its deferred
// CloseHandle runs at the end of every iteration instead of accumulating
// thousands of open handles until List itself returns.
//
// The image path is the expensive half of inspecting a process
// (QueryFullProcessImageName translates a device path and can retry with a
// growing buffer); the creation time is comparatively cheap. So the
// creation time is always read fresh — it both answers CreatedAt and
// validates a cache hit — while the path is resolved again only for a pid
// not already in cache under that same creation time, i.e. a genuinely new
// or recycled pid.
func inspectProcess(pid uint32, cache *imageCache) Process {
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

	created, err := queryCreatedAt(handle)
	if err != nil {
		p.CreatedAtUnknown = true
	} else {
		p.CreatedAt = created
	}

	if !p.CreatedAtUnknown {
		if path, ok := cache.lookup(pid, created); ok {
			p.Path = path
			return p
		}
	}

	path, err := queryImagePath(handle)
	if err != nil {
		p.PathUnknown = true
		return p
	}
	p.Path = path
	if !p.CreatedAtUnknown {
		cache.store(pid, created, path)
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
