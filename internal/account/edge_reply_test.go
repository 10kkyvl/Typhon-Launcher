package account

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
)

type levelRecorder struct {
	mu     sync.Mutex
	levels map[string]slog.Level
}

func (r *levelRecorder) Enabled(context.Context, slog.Level) bool { return true }
func (r *levelRecorder) WithAttrs([]slog.Attr) slog.Handler       { return r }
func (r *levelRecorder) WithGroup(string) slog.Handler            { return r }
func (r *levelRecorder) Handle(_ context.Context, rec slog.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.levels[rec.Message] = rec.Level
	return nil
}

// TestEdgeRepliesAreNotErrorReports fails against a client that logs every
// reply without the API envelope at ERROR: the reverse proxy's own 429 then
// reached diagnostics as a fault (reports from 1.2.0 and 1.3.1).
func TestEdgeRepliesAreNotErrorReports(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })
	const msg = "account api replied outside the error contract"

	for _, tc := range []struct {
		status int
		level  slog.Level
		code   string
	}{
		{http.StatusTooManyRequests, slog.LevelWarn, CodeRateLimited},
		{http.StatusBadGateway, slog.LevelWarn, CodeServer},
		{http.StatusServiceUnavailable, slog.LevelWarn, CodeServer},
		{http.StatusGatewayTimeout, slog.LevelWarn, CodeServer},
		{http.StatusNotFound, slog.LevelError, CodeServer},
		{http.StatusInternalServerError, slog.LevelError, CodeServer},
	} {
		rec := &levelRecorder{levels: map[string]slog.Level{}}
		slog.SetDefault(slog.New(rec))
		err := decodeError(tc.status, strings.NewReader("<html>proxy</html>"))
		var apiErr *Error
		if !errors.As(err, &apiErr) || apiErr.Code != tc.code || apiErr.Status != tc.status {
			t.Fatalf("status %d: error = %#v, want code %q", tc.status, err, tc.code)
		}
		level, logged := rec.levels[msg]
		if !logged || level != tc.level {
			t.Fatalf("status %d: logged=%v level=%v, want %v", tc.status, logged, level, tc.level)
		}
	}

	// A reply in the API's own envelope is not logged at all.
	rec := &levelRecorder{levels: map[string]slog.Level{}}
	slog.SetDefault(slog.New(rec))
	err := decodeError(http.StatusTooManyRequests, strings.NewReader(`{"error":{"code":"rate_limited"}}`))
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != CodeRateLimited {
		t.Fatalf("enveloped 429: error = %#v", err)
	}
	if _, logged := rec.levels[msg]; logged {
		t.Fatal("an enveloped reply was logged as outside the contract")
	}
}
