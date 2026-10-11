package usagestats

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"net/url"
	"strings"
	"syscall"

	"typhon/internal/uierr"
)

const (
	CodeNone             = ""
	CodeCancelled        = "cancelled"
	CodeTimeout          = "timeout"
	CodeNotFound         = "not_found"
	CodePermissionDenied = "permission_denied"
	CodeDiskFull         = "disk_full"
	CodeNetwork          = "network"
	CodeUnknown          = "unknown"
)

// Classify returns an error_code that fits the usage event pattern. A generic
// cause (cancel, timeout, disk, network) wins over a uierr code: the code only
// names the failure when nothing more specific is known about it.
func Classify(err error) string {
	code := Class(err)
	if code != CodeUnknown {
		return code
	}
	return eventCode(uierr.Code(err))
}

// eventCode maps a uierr code, which may contain dots, onto the event
// error_code alphabet. A code outside it is dropped rather than shipped: the
// server rejects the whole event over one bad field.
func eventCode(code string) string {
	code = strings.ReplaceAll(code, ".", "_")
	if !errorCodePattern.MatchString(code) {
		return CodeUnknown
	}
	return code
}

func Class(err error) string {
	if err == nil {
		return CodeNone
	}
	if errors.Is(err, context.Canceled) {
		return CodeCancelled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return CodeTimeout
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return CodeTimeout
	}
	if errors.Is(err, fs.ErrNotExist) {
		return CodeNotFound
	}
	if errors.Is(err, fs.ErrPermission) {
		return CodePermissionDenied
	}
	if errors.Is(err, syscall.ENOSPC) {
		return CodeDiskFull
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return CodeNetwork
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return CodeNetwork
	}
	return CodeUnknown
}
