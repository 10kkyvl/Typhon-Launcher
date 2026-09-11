package diagnostics

import (
	"context"
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
