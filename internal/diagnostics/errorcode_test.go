package diagnostics

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"typhon/internal/uierr"
)

func TestLogUploadRejectionHasSpecificDiagnosticCode(t *testing.T) {
	if got := diagnosticCode(uierr.New(ErrCodeLogUploadRejected, "status 403")); got != ErrCodeLogUploadRejected {
		t.Fatal(got)
	}
	if got := diagnosticCode(context.DeadlineExceeded); got != "timeout" {
		t.Fatal(got)
	}
}

func TestDiagnosticCodeKeepsUIErrorCodeVerbatim(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"dotted_code", uierr.New("install.archive_tool_failed", "x"), "install.archive_tool_failed"},
		{"wrapped_code", fmt.Errorf("run: %w", uierr.Wrap("install.installer_failed", errors.New("exit 1"))), "install.installer_failed"},
		{"cancel_beats_code", uierr.Wrap("install.installer_failed", context.Canceled), "cancelled"},
		{"path_as_code", uierr.New(`C:\Users\egor\Games`, "x"), "unknown"},
		{"uppercase_code", uierr.New("Install.Failed", "x"), "unknown"},
		{"too_long_code", uierr.New("install."+strings.Repeat("a", 60), "x"), "unknown"},
		{"no_code", errors.New("plain"), "unknown"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := diagnosticCode(tc.err); got != tc.want {
				t.Fatalf("diagnosticCode(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}
