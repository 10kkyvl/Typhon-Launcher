package usagestats

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"net/url"
	"syscall"
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

func Classify(err error) string {
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
