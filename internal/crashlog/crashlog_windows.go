//go:build windows

package crashlog

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	evtQueryChannelPath      = 0x1
	evtQueryReverseDirection = 0x200
	evtRenderEventXML        = 1
	evtBatch                 = 16
	evtNextTimeoutMS         = 5000
	maxEventsPerQuery        = 500
	timeLayout               = "2006-01-02T15:04:05.000Z"
)

var (
	wevtapi       = windows.NewLazySystemDLL("wevtapi.dll")
	procEvtQuery  = wevtapi.NewProc("EvtQuery")
	procEvtNext   = wevtapi.NewProc("EvtNext")
	procEvtRender = wevtapi.NewProc("EvtRender")
	procEvtClose  = wevtapi.NewProc("EvtClose")
)

func buildXPath(q Query) string {
	return fmt.Sprintf("*[System[((Provider[@Name='%s'] and EventID=1000) or (Provider[@Name='%s'] and EventID=1001)) "+
		"and TimeCreated[@SystemTime>='%s' and @SystemTime<='%s']]]",
		SourceAppError, SourceWER, q.From.UTC().Format(timeLayout), q.To.UTC().Format(timeLayout))
}

func read(ctx context.Context, q Query) ([]Fault, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := wevtapi.Load(); err != nil {
		return nil, fmt.Errorf("load wevtapi: %w", err)
	}
	channel, err := windows.UTF16PtrFromString("Application")
	if err != nil {
		return nil, err
	}
	query, err := windows.UTF16PtrFromString(buildXPath(q))
	if err != nil {
		return nil, err
	}
	//nolint:gosec // G103: uintptr(unsafe.Pointer(..)) прямо в аргументах Call — единственная безопасная форма для wevtapi; буферы локальные.
	h, _, callErr := procEvtQuery.Call(0, uintptr(unsafe.Pointer(channel)), uintptr(unsafe.Pointer(query)), uintptr(evtQueryChannelPath|evtQueryReverseDirection))
	if h == 0 {
		return nil, fmt.Errorf("EvtQuery: %w", callErr)
	}
	defer closeHandle(h)

	var faults []Fault
	var errs []error
	seen := 0
	for seen < maxEventsPerQuery {
		if err := ctx.Err(); err != nil {
			return faults, err
		}
		var handles [evtBatch]uintptr
		var returned uint32
		//nolint:gosec // G103: uintptr(unsafe.Pointer(..)) прямо в аргументах Call — единственная безопасная форма для wevtapi; буферы локальные.
		r, _, callErr := procEvtNext.Call(h, uintptr(evtBatch), uintptr(unsafe.Pointer(&handles[0])), uintptr(evtNextTimeoutMS), 0, uintptr(unsafe.Pointer(&returned)))
		if r == 0 {
			if errors.Is(callErr, windows.ERROR_NO_MORE_ITEMS) {
				break
			}
			return faults, errors.Join(append(errs, fmt.Errorf("EvtNext: %w", callErr))...)
		}
		for i := uint32(0); i < returned; i++ {
			seen++
			xml, rerr := renderXML(handles[i])
			closeHandle(handles[i])
			if rerr != nil {
				errs = append(errs, rerr)
				continue
			}
			f, perr := ParseEvent(xml)
			switch {
			case perr == nil:
				if !f.Time.IsZero() && (f.Time.Before(q.From) || f.Time.After(q.To)) {
					continue
				}
				faults = append(faults, f)
			case errors.Is(perr, ErrNotCrashEvent):
			default:
				errs = append(errs, perr)
			}
		}
	}
	return faults, errors.Join(errs...)
}

func renderXML(ev uintptr) ([]byte, error) {
	var used, props uint32
	buf := make([]uint16, 4096)
	for attempt := 0; attempt < 2; attempt++ {
		//nolint:gosec // G103: uintptr(unsafe.Pointer(..)) прямо в аргументах Call — единственная безопасная форма для wevtapi; буферы локальные.
		r, _, callErr := procEvtRender.Call(0, ev, uintptr(evtRenderEventXML), uintptr(len(buf)*2), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&used)), uintptr(unsafe.Pointer(&props)))
		if r != 0 {
			return []byte(windows.UTF16ToString(buf)), nil
		}
		if !errors.Is(callErr, windows.ERROR_INSUFFICIENT_BUFFER) {
			return nil, fmt.Errorf("EvtRender: %w", callErr)
		}
		if used > maxEventXML*2 {
			return nil, ErrEventTooLarge
		}
		buf = make([]uint16, used/2+1)
	}
	return nil, fmt.Errorf("EvtRender: %w", windows.ERROR_INSUFFICIENT_BUFFER)
}

func closeHandle(h uintptr) {
	if r, _, err := procEvtClose.Call(h); r == 0 {
		slog.Warn("close event log handle", "error", err)
	}
}

func CrashDumpsDir() (string, error) {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return "", errors.New("LOCALAPPDATA is not set")
	}
	return filepath.Join(base, "CrashDumps"), nil
}
