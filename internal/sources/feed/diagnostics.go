package feed

import (
	"strconv"
	"time"
)

// operationError preserves the original user-facing message and error chain,
// while making the failed phase available to the common diagnostics handler.
type operationError struct {
	err     error
	stage   string
	status  int
	timeout time.Duration
}

func (e *operationError) Error() string { return e.err.Error() }
func (e *operationError) Unwrap() error { return e.err }
func (e *operationError) DiagnosticContext() map[string]string {
	fields := map[string]string{"stage": e.stage}
	if e.timeout > 0 {
		fields["http_timeout_ms"] = strconv.FormatInt(e.timeout.Milliseconds(), 10)
	}
	if e.status != 0 {
		fields["http_status"] = strconv.Itoa(e.status)
	}
	return fields
}
