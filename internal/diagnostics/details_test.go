package diagnostics

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/wailsapp/wails/v3/pkg/application"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"typhon/internal/catalog"
	"typhon/internal/sources"

	"typhon/internal/clientid"
)

func assertDetailsSanitized(t *testing.T) {
	t.Helper()
	in := Report{Details: &Details{Context: map[string]string{"stage": poison, "password": "top-secret"}, Breadcrumbs: []Breadcrumb{{Timestamp: time.Now(), Level: "INFO", Component: poison, Message: poison, Context: map[string]string{"stage": poison, "token": "top-secret"}}}}}
	out, err := sanitizeReport(in)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(out.Details)
	if err != nil {
		t.Fatal(err)
	}
	if out.Details == nil || len(out.Details.Breadcrumbs) != 1 || strings.Contains(string(data), poisonMarker) || strings.Contains(string(data), "top-secret") {
		t.Fatalf("unsafe details %s", data)
	}
}

func TestLogContextSnapshotAndPrivacy(t *testing.T) {
	s, err := newServiceAt(t.TempDir(), testIdentity(), func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	var local bytes.Buffer
	logger := slog.New(NewLogHandler(slog.NewTextHandler(&local, nil), s)).With("component", "download")
	logger.Info("download started", "download_id", "job-1", "url", "https://secret.example/private?token=secret", "password", "secret")
	logger.Warn("write retry", "attempt", 2, "path", poison)
	logger.Error("write failed", "error", fmt.Errorf("write: %w", os.ErrPermission), "download_id", "job-1", "stage", "write", "duration_ms", 1250)
	e := <-s.logEvents
	logger.Info("unrelated later event")
	s.captureEvent(e, false)
	if len(s.queue) != 1 {
		t.Fatal("missing report")
	}
	r := s.queue[0]
	if !r.Timestamp.Equal(e.at) || r.ErrorCode != "permission_denied" || r.Details.Context["stage"] != "write" || r.Details.Context["download_id"] != "job-1" {
		t.Fatalf("report: %+v", r)
	}
	if len(r.Details.Breadcrumbs) != 2 || r.Details.Breadcrumbs[1].Context["attempt"] != "2" {
		t.Fatalf("breadcrumbs: %+v", r.Details)
	}
	data, _ := json.Marshal(r)
	if strings.Contains(string(data), "secret") || strings.Contains(string(data), "later event") || strings.Contains(string(data), poisonMarker) {
		t.Fatalf("unsafe or late context: %s", data)
	}
	if !strings.Contains(local.String(), "secret") {
		t.Fatal("local logger was changed")
	}
}

func TestBreadcrumbWindowConsentAndBoundedness(t *testing.T) {
	s, err := newServiceAt(t.TempDir(), testIdentity(), func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	h := NewLogHandler(slog.NewTextHandler(&bytes.Buffer{}, nil), s).WithAttrs([]slog.Attr{slog.String("component", "install")})
	now := time.Now()
	for i := 0; i < 100; i++ {
		r := slog.NewRecord(now.Add(-time.Duration(100-i)*time.Second), slog.LevelInfo, fmt.Sprint(i), 0)
		_ = h.Handle(context.Background(), r)
	}
	if len(s.breadcrumbs) != breadcrumbCapacity {
		t.Fatal("unbounded breadcrumbs")
	}
	s.Capture("install", "apply", errors.New("failed"), false)
	if len(s.queue[0].Details.Breadcrumbs) != MaxBreadcrumbs {
		t.Fatal("wrong snapshot bound")
	}
	s.Capture("sources", "fetch", errors.New("failed"), false)
	if len(s.queue[1].Details.Breadcrumbs) != 0 {
		t.Fatal("unrelated component included")
	}
	old := slog.NewRecord(now.Add(-6*time.Minute), slog.LevelInfo, "stale", 0)
	_ = h.Handle(context.Background(), old)
	errRecord := slog.NewRecord(now, slog.LevelError, "failed", 0)
	_ = h.Handle(context.Background(), errRecord)
	e := <-s.logEvents
	s.SetEnabled(false)
	s.SetEnabled(true)
	s.captureEvent(e, false)
	if len(s.queue) != 0 || len(s.breadcrumbs) != 0 {
		t.Fatal("old consent data survived re-enable")
	}
}

func TestDetailsWireSizeAndOptOutDuringSanitize(t *testing.T) {
	d := &Details{Context: map[string]string{"stage": strings.Repeat("<", 256)}}
	for i := 0; i < 50; i++ {
		d.Breadcrumbs = append(d.Breadcrumbs, Breadcrumb{Timestamp: time.Now(), Level: "INFO", Component: "download", Message: strings.Repeat("<", 2000)})
	}
	clean := sanitizeDetails(d)
	data, _ := json.Marshal(clean)
	if len(data) > MaxDetailsBytes || len(clean.Breadcrumbs) > MaxBreadcrumbs {
		t.Fatal("wire limit exceeded")
	}
	s, err := newServiceAt(t.TempDir(), testIdentity(), func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	original := sanitizeText
	t.Cleanup(func() { sanitizeText = original })
	switched := false
	sanitizeText = func(raw string) (string, error) {
		if !switched {
			switched = true
			s.SetEnabled(false)
			s.SetEnabled(true)
		}
		return original(raw)
	}
	s.Capture("download", "write", errors.New("failed"), false)
	if len(s.queue) != 0 {
		t.Fatal("in-flight old consent report enqueued after re-enable")
	}
}

func TestClientLegacyFallbackAndRetryKeepsDetails(t *testing.T) {
	for _, code := range []string{"bad_request", "invalid_batch", "storage_unavailable"} {
		t.Run(code, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var b batchPayload
				if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
					t.Error(err)
				}
				if calls == 1 {
					if b.Reports[0].Details == nil {
						t.Error("missing details")
					}
					w.WriteHeader(http.StatusBadRequest)
					fmt.Fprintf(w, `{"error":{"code":%q}}`, code)
					return
				}
				if b.Reports[0].Details != nil || b.Reports[0].ErrorID != "same-id" {
					t.Error("invalid legacy fallback")
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			defer srv.Close()
			cl, err := newClient(srv.URL)
			if err != nil {
				t.Fatal(err)
			}
			reports := []reportPayload{{ErrorID: "same-id", ErrorCode: "unknown", Details: &Details{Context: map[string]string{"stage": "write"}}}}
			err = cl.send(context.Background(), clientid.Identity{}, reports)
			if code == "bad_request" {
				if err != nil || calls != 2 {
					t.Fatalf("fallback: %v, calls %d", err, calls)
				}
			} else if err == nil || calls != 1 {
				t.Fatalf("unexpected downgrade: %v, calls %d", err, calls)
			}
			if reports[0].Details == nil {
				t.Fatal("original details were destroyed")
			}
		})
	}
}

func TestDetailsSurviveDiskRetry(t *testing.T) {
	srv, ch := newCapturingServer(t, http.StatusServiceUnavailable)
	s := newTestService(t, srv, nil)
	s.Capture("download", "write", errors.New("failed"), false)
	want := s.queue[0]
	s.flush(context.Background())
	<-ch
	names, err := listPendingFiles(s.pendingDir)
	if err != nil || len(names) != 1 {
		t.Fatalf("pending: %v %v", names, err)
	}
	good, received := newCapturingServer(t, http.StatusNoContent)
	s.client, err = newClient(good.URL)
	if err != nil {
		t.Fatal(err)
	}
	s.flush(context.Background())
	var b batchPayload
	if err := json.Unmarshal((<-received).body, &b); err != nil {
		t.Fatal(err)
	}
	if b.Reports[0].ErrorID != want.ErrorID || b.Reports[0].Details.Context["error_type"] != want.Details.Context["error_type"] {
		t.Fatal("retry lost report context")
	}
}

func TestSourceFailureCarriesContext(t *testing.T) {
	s, err := newServiceAt(t.TempDir(), testIdentity(), func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	slog.SetDefault(slog.New(NewLogHandler(slog.NewTextHandler(&bytes.Buffer{}, nil), s)))
	cat, err := catalog.NewServiceAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sourceService, err := sources.NewServiceAt(t.TempDir(), cat)
	if err != nil {
		t.Fatal(err)
	}
	if err := sourceService.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sourceService.ServiceShutdown(); err != nil {
			t.Error(err)
		}
	})
	if _, err := sourceService.AddSourceFile(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("expected file error")
	}
	e := <-s.logEvents
	s.captureEvent(e, false)
	report := s.queue[0]
	fields := report.Details.Context
	if report.ErrorCode != "not_found" || fields["source_type"] != "file" || fields["stage"] != "open_file" || fields["retry_attempt"] != "1" || fields["scheduled"] != "false" || fields["duration_ms"] == "" {
		t.Fatalf("source context: %+v", report)
	}
	if len(report.Details.Breadcrumbs) < 2 {
		t.Fatal("source lifecycle events missing")
	}
}

func TestShutdownDeliversBufferedLogContext(t *testing.T) {
	server, received := newCapturingServer(t, http.StatusNoContent)
	s := newTestService(t, server, nil)
	logger := slog.New(NewLogHandler(slog.NewTextHandler(&bytes.Buffer{}, nil), s)).With("component", "install")
	logger.Info("apply started", "stage", "apply")
	logger.Error("final write failed", "stage", "write")
	// No worker consumes this event: simulate its cancellation winning the select.
	if len(s.logEvents) != 1 {
		t.Fatal("fixture must leave buffered event")
	}
	if err := s.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
	var batch batchPayload
	if err := json.Unmarshal((<-received).body, &batch); err != nil {
		t.Fatal(err)
	}
	if len(batch.Reports) != 1 || batch.Reports[0].Details.Context["stage"] != "write" || len(batch.Reports[0].Details.Breadcrumbs) != 1 {
		t.Fatalf("shutdown lost context: %+v", batch)
	}
}
