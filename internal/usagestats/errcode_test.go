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

	"typhon/internal/uierr"
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

func TestClassifyUIErrorCode(t *testing.T) {
	longest := "download.net_interface_host_check_failed"
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"direct", uierr.New("install.archive_tool_failed", "detail"), "install_archive_tool_failed"},
		{"wrapped_plain_cause", uierr.Wrap("install.installer_failed", errors.New("exit 1")), "install_installer_failed"},
		{"wrapped_by_fmt", fmt.Errorf("extract: %w", uierr.New("install.archive_corrupt", "bad")), "install_archive_corrupt"},
		{"joined", errors.Join(errors.New("tool"), uierr.New("install.archive_tool_missing", "none")), "install_archive_tool_missing"},
		{
			"decoder_and_tool_failure",
			fmt.Errorf("%w: builtin: %w; %w", uierr.New("install.archive_tool_failed", "tool failed"), errors.New("rardecode: decoded file too short"), errors.New("exit code 2")),
			"install_archive_tool_failed",
		},
		{"longest_real_code", uierr.New(longest, "x"), "download_net_interface_host_check_failed"},
		{"cancel_beats_code", uierr.Wrap("catalog.save_failed", context.Canceled), CodeCancelled},
		{"deadline_beats_code", uierr.Wrap("catalog.save_failed", context.DeadlineExceeded), CodeTimeout},
		{"disk_full_beats_code", uierr.Wrap("catalog.save_failed", syscall.ENOSPC), CodeDiskFull},
		{"empty_code", uierr.New("", "x"), CodeUnknown},
		{"path_as_code", uierr.New(`C:\Users\egor\Games`, "x"), CodeUnknown},
		{"uppercase_code", uierr.New("Install.Failed", "x"), CodeUnknown},
		{"space_in_code", uierr.New("install failed", "x"), CodeUnknown},
		{"too_long_code", uierr.New(longest+"_x", "x"), CodeUnknown},
		{"no_code", errors.New("typhon:install.archive_tool_failed: text only"), CodeUnknown},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(tc.err)
			if got != tc.want {
				t.Fatalf("Classify(%v) = %q, want %q", tc.err, got, tc.want)
			}
			if got != CodeNone && !errorCodePattern.MatchString(got) {
				t.Fatalf("Classify(%v) = %q does not fit the usage event error_code pattern", tc.err, got)
			}
		})
	}
}

func TestClassifyUIErrorCodePassesEventValidation(t *testing.T) {
	ev := Event{
		Type:      TypeInstallFailed,
		Timestamp: time.Now(),
		Properties: Properties{
			GameID:        "1",
			InstallerType: "unknown",
			ErrorCode:     Classify(uierr.New("install.archive_tool_failed", "detail")),
		},
	}
	if err := validate(ev); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if ev.Properties.ErrorCode == CodeUnknown {
		t.Fatal("the uierr code was dropped")
	}
}
