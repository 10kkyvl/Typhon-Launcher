package usagestats

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/url"
	"syscall"
	"testing"
	"time"
)

type fakeTimeoutError struct{ msg string }

func (e fakeTimeoutError) Error() string   { return e.msg }
func (e fakeTimeoutError) Timeout() bool   { return true }
func (e fakeTimeoutError) Temporary() bool { return true }

func TestClassify(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, CodeNone},
		{"context_canceled", context.Canceled, CodeCancelled},
		{"wrapped_context_canceled", fmt.Errorf("op: %w", context.Canceled), CodeCancelled},
		{"context_deadline_exceeded", context.DeadlineExceeded, CodeTimeout},
		{"net_timeout_error", fakeTimeoutError{msg: "dial timeout"}, CodeTimeout},
		{"fs_not_exist", fs.ErrNotExist, CodeNotFound},
		{"wrapped_not_exist", fmt.Errorf("open x: %w", fs.ErrNotExist), CodeNotFound},
		{"fs_permission", fs.ErrPermission, CodePermissionDenied},
		{"enospc", syscall.ENOSPC, CodeDiskFull},
		{"wrapped_enospc", fmt.Errorf("write: %w", syscall.ENOSPC), CodeDiskFull},
		{"net_op_error", &net.OpError{Op: "dial", Err: errors.New("refused")}, CodeNetwork},
		{"url_error", &url.Error{Op: "Get", URL: "http://x", Err: errors.New("boom")}, CodeNetwork},
		{"plain_russian_text", errors.New("файл не найден, но это не fs.ErrNotExist"), CodeUnknown},
		{"generic_unknown", errors.New("something else entirely"), CodeUnknown},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Classify(tc.err); got != tc.want {
				t.Fatalf("Classify(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}

func TestClassifyRealDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	<-ctx.Done()
	if got := Classify(ctx.Err()); got != CodeTimeout {
		t.Fatalf("Classify(ctx.Err()) = %q, want %q", got, CodeTimeout)
	}
}

func TestClassifyRealCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := Classify(ctx.Err()); got != CodeCancelled {
		t.Fatalf("Classify(ctx.Err()) = %q, want %q", got, CodeCancelled)
	}
}
