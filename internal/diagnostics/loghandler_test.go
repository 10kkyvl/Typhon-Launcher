package diagnostics

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	alog "github.com/anacrolix/log"
)

func TestDHTBridgedErrorsProduceNoReport(t *testing.T) {
	srv, reqs := newCapturingServer(t, http.StatusNoContent)
	svc := newTestService(t, srv, nil)
	var local bytes.Buffer
	handler := NewLogHandler(slog.NewTextHandler(&local, nil), svc).WithAttrs([]slog.Attr{slog.String("component", "download")})

	base := alog.NewLogger().WithDefaultLevel(alog.Debug)
	base.SetHandlers(alog.SlogHandlerAsHandler{SlogHandler: handler})
	logger := base.WithNames("dht", "0.0.0.0:42815")

	logger.Levelf(alog.Error, "error bootstrapping during bucket refresh: %v", errors.New("getting starting nodes: nothing resolved"))

	if n := len(svc.logEvents); n != 0 {
		t.Fatalf("expected the DHT error to be filtered before reaching the report queue, got %d queued events", n)
	}
	if !strings.Contains(local.String(), "error bootstrapping during bucket refresh") {
		t.Fatalf("expected the DHT log line to still reach the local log, got %q", local.String())
	}

	svc.flush(context.Background())
	select {
	case req := <-reqs:
		t.Fatalf("expected no diagnostics report for a DHT log, got %s", req.body)
	default:
	}
}

func TestNonDHTAnacrolixErrorsStillProduceReport(t *testing.T) {
	srv, reqs := newCapturingServer(t, http.StatusNoContent)
	svc := newTestService(t, srv, nil)
	logger := slog.New(NewLogHandler(slog.NewTextHandler(&bytes.Buffer{}, nil), svc)).With("component", "download")

	logger.Error("error marking piece complete", "piece", 3, "err", errors.New("error promoting part file: access is denied"))

	e := <-svc.logEvents
	svc.captureEvent(e, false)
	svc.flush(context.Background())

	report := decodeSingleReport(t, waitFor(t, reqs, "/diagnostics/errors"))
	if !strings.Contains(report.Message, "error promoting part file") {
		t.Fatalf("report message = %q, want it to mention the promotion failure", report.Message)
	}
}

func TestTyphonInternalErrorsStillProduceReport(t *testing.T) {
	srv, reqs := newCapturingServer(t, http.StatusNoContent)
	svc := newTestService(t, srv, nil)
	logger := slog.New(NewLogHandler(slog.NewTextHandler(&bytes.Buffer{}, nil), svc)).With("component", "install")

	logger.Error("disk full", "operation", "extract")

	e := <-svc.logEvents
	svc.captureEvent(e, false)
	svc.flush(context.Background())

	report := decodeSingleReport(t, waitFor(t, reqs, "/diagnostics/errors"))
	if !strings.Contains(report.Message, "disk full") {
		t.Fatalf("report message = %q, want it to mention the original error", report.Message)
	}
}
