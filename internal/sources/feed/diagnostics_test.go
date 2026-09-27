package feed

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPFailureDiagnosticStagePreservesCause(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	client := &http.Client{Timeout: 20 * time.Millisecond}
	_, err := Fetch(context.Background(), client, server.URL, Conditional{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout cause lost: %v", err)
	}
	var contextual interface{ DiagnosticContext() map[string]string }
	if !errors.As(err, &contextual) || (contextual.DiagnosticContext()["stage"] != "http_request" || contextual.DiagnosticContext()["http_timeout_ms"] != "20") {
		t.Fatalf("stage: %v", err)
	}
}

func TestHTTPStatusDiagnosticContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	defer server.Close()
	_, err := Fetch(context.Background(), server.Client(), server.URL, Conditional{})
	var status *StatusError
	if !errors.As(err, &status) || status.StatusCode != 503 {
		t.Fatalf("HTTP cause lost: %v", err)
	}
	var contextual interface{ DiagnosticContext() map[string]string }
	if !errors.As(err, &contextual) || contextual.DiagnosticContext()["http_status"] != "503" || contextual.DiagnosticContext()["stage"] != "http_response" {
		t.Fatalf("context: %v", err)
	}
}
