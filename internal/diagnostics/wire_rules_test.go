package diagnostics

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

const (
	serverMaxMessage = 4096
	serverMaxStack   = 16 << 10
	serverPast       = 30 * 24 * time.Hour
	serverFuture     = 24 * time.Hour
)

func serverCheckPrintable(field, value string, maxLen int) error {
	if value == "" {
		return fmt.Errorf("%s is empty", field)
	}
	if len(value) > maxLen {
		return fmt.Errorf("%s is %d bytes, limit is %d", field, len(value), maxLen)
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("%s contains a control character", field)
		}
	}
	return nil
}

func serverCheckText(field, value string, maxLen int) error {
	if len(value) > maxLen {
		return fmt.Errorf("%s is %d bytes, limit is %d", field, len(value), maxLen)
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("%s is not valid utf-8", field)
	}
	for _, r := range value {
		if r == '\n' || r == '\r' || r == '\t' {
			continue
		}
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("%s contains a control character", field)
		}
	}
	return nil
}

func serverCheckContext(fields map[string]string) error {
	if len(fields) > MaxContextFields {
		return errors.New("too many context fields")
	}
	for key, value := range fields {
		if !contextKeyAllowed(key) {
			return fmt.Errorf("unsupported context key %q", key)
		}
		if err := serverCheckText("context", value, MaxContextValueLen); err != nil {
			return err
		}
	}
	return nil
}

// serverValidate mirrors Report.validate and Details.validate of the backend:
// the report goes through JSON the way it travels, so a byte the encoder
// widens is measured at its wire size.
func serverValidate(rp reportPayload, now time.Time) error {
	body, err := json.Marshal(batchPayload{Reports: []reportPayload{rp}})
	if err != nil {
		return err
	}
	var decoded batchPayload
	if err := json.Unmarshal(body, &decoded); err != nil {
		return err
	}
	r := decoded.Reports[0]
	if err := serverCheckPrintable("component", r.Component, 64); err != nil {
		return err
	}
	if err := serverCheckPrintable("operation", r.Operation, 64); err != nil {
		return err
	}
	if err := serverCheckText("message", r.Message, serverMaxMessage); err != nil {
		return err
	}
	if err := serverCheckText("stack", r.Stack, serverMaxStack); err != nil {
		return err
	}
	if r.Timestamp.IsZero() || r.Timestamp.Before(now.Add(-serverPast)) || r.Timestamp.After(now.Add(serverFuture)) {
		return fmt.Errorf("timestamp %s outside the window", r.Timestamp)
	}
	d := r.Details
	if d == nil {
		return nil
	}
	encoded, err := json.Marshal(d)
	if err != nil || len(encoded) > MaxDetailsBytes {
		return errors.New("details too large")
	}
	if err := serverCheckContext(d.Context); err != nil {
		return err
	}
	if len(d.Breadcrumbs) > MaxBreadcrumbs {
		return errors.New("too many breadcrumbs")
	}
	var previous time.Time
	for _, b := range d.Breadcrumbs {
		if b.Timestamp.IsZero() || b.Timestamp.Before(r.Timestamp.Add(-5*time.Minute)) || b.Timestamp.After(r.Timestamp) || b.Timestamp.Before(previous) {
			return fmt.Errorf("invalid breadcrumb timestamp %s for report %s", b.Timestamp, r.Timestamp)
		}
		previous = b.Timestamp
		if err := serverCheckPrintable("breadcrumb component", b.Component, 64); err != nil {
			return err
		}
		if err := serverCheckText("breadcrumb message", b.Message, MaxBreadcrumbMessageLen); err != nil {
			return err
		}
		if err := serverCheckContext(b.Context); err != nil {
			return err
		}
	}
	return nil
}

func sanitizedPayload(t *testing.T, r Report) reportPayload {
	t.Helper()
	out, err := sanitizeReport(r)
	if err != nil {
		t.Fatalf("sanitizeReport: %v", err)
	}
	return toPayload(out)
}

func TestSingleLineFieldsAreStrippedOfEveryControlCharacter(t *testing.T) {
	cases := []struct {
		name, component, operation string
	}{
		{"newline in operation", "download", "torrent\nstate changed"},
		{"carriage return in operation", "download", "line\r"},
		{"tab in operation", "download", "a\tb"},
		{"newline in component", "down\nload", "fetch"},
		{"tab in component", "\tdownload", "fetch"},
		{"escape and DEL", "down\x1bload\x7f", "fe\x00tch"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rp := sanitizedPayload(t, Report{Component: tc.component, Operation: tc.operation, Message: "m", Timestamp: time.Now()})
			if err := serverValidate(rp, time.Now()); err != nil {
				t.Fatalf("server would reject %q/%q: %v", rp.Component, rp.Operation, err)
			}
		})
	}
}

func TestBreadcrumbComponentIsStrippedOfEveryControlCharacter(t *testing.T) {
	at := time.Now()
	for _, component := range []string{"down\nload", "down\rload", "down\tload", "down\x1bload"} {
		t.Run(fmt.Sprintf("%q", component), func(t *testing.T) {
			rp := sanitizedPayload(t, Report{Component: "download", Operation: "x", Timestamp: at, Details: &Details{
				Breadcrumbs: []Breadcrumb{{Timestamp: at.Add(-time.Second), Level: "INFO", Component: component, Message: "m"}},
			}})
			if err := serverValidate(rp, at); err != nil {
				t.Fatalf("server would reject breadcrumb component %q: %v", rp.Details.Breadcrumbs[0].Component, err)
			}
		})
	}
}

func TestInvalidUTF8NeverWidensPastServerLimits(t *testing.T) {
	cp1251 := func(n int) string { return strings.Repeat("\xcf\xf0\xe8", n) }
	cases := []struct {
		name string
		r    Report
	}{
		{"message of cp1251 bytes", Report{Component: "install", Operation: "run", Message: cp1251(2000)}},
		{"message of lone continuation bytes", Report{Component: "install", Operation: "run", Message: strings.Repeat("\x80", 5000)}},
		{"stack of cp866 bytes", Report{Component: "install", Operation: "run", Message: "m", Stack: strings.Repeat("\xcf\xf0\xe8", 7000)}},
		{"component and operation", Report{Component: strings.Repeat("\xff", 60), Operation: strings.Repeat("\xfe", 60), Message: "m"}},
		{"breadcrumb message", Report{Component: "install", Operation: "run", Message: "m", Details: &Details{Breadcrumbs: []Breadcrumb{
			{Level: "INFO", Component: "install", Message: strings.Repeat("\xe9", 900)},
		}}}},
		{"context value", Report{Component: "install", Operation: "run", Message: "m", Details: &Details{Context: map[string]string{"stage": strings.Repeat("\xe9", 600)}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now()
			tc.r.Timestamp = now
			if tc.r.Details != nil {
				for i := range tc.r.Details.Breadcrumbs {
					tc.r.Details.Breadcrumbs[i].Timestamp = now.Add(-time.Second)
				}
			}
			rp := sanitizedPayload(t, tc.r)
			if err := serverValidate(rp, now); err != nil {
				t.Fatalf("server would reject the report: %v", err)
			}
		})
	}
}

func TestEmptyComponentAndOperationAreNeverSent(t *testing.T) {
	cases := []struct {
		name, component, operation, wantComponent, wantOperation string
	}{
		{"both empty", "", "", "launcher", "unknown"},
		{"only control characters", "\n\t", "\r\x1b", "launcher", "unknown"},
		{"empty component keeps operation", "", "fetch", "launcher", "fetch"},
		{"empty operation keeps component", "download", "", "download", "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rp := sanitizedPayload(t, Report{Component: tc.component, Operation: tc.operation, Message: "m", Timestamp: time.Now()})
			if rp.Component != tc.wantComponent || rp.Operation != tc.wantOperation {
				t.Fatalf("component/operation = %q/%q, want %q/%q", rp.Component, rp.Operation, tc.wantComponent, tc.wantOperation)
			}
			if err := serverValidate(rp, time.Now()); err != nil {
				t.Fatalf("server would reject: %v", err)
			}
		})
	}
}

func TestBreadcrumbEmptyComponentFallsBackToLauncher(t *testing.T) {
	at := time.Now()
	rp := sanitizedPayload(t, Report{Component: "download", Operation: "x", Timestamp: at, Details: &Details{
		Breadcrumbs: []Breadcrumb{{Timestamp: at.Add(-time.Second), Level: "INFO", Component: "\n", Message: "m"}},
	}})
	if got := rp.Details.Breadcrumbs[0].Component; got != "launcher" {
		t.Fatalf("breadcrumb component = %q, want launcher", got)
	}
	if err := serverValidate(rp, at); err != nil {
		t.Fatalf("server would reject: %v", err)
	}
}

func captureThroughHandler(t *testing.T, svc *Service, log func(*slog.Logger)) reportPayload {
	t.Helper()
	logger := slog.New(NewLogHandler(slog.NewTextHandler(&bytes.Buffer{}, nil), svc)).With("component", "download")
	log(logger)
	select {
	case e := <-svc.logEvents:
		svc.captureEvent(e, false)
	default:
		t.Fatal("handler queued no event")
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if len(svc.queue) != 1 {
		t.Fatalf("queue has %d reports, want 1", len(svc.queue))
	}
	return svc.queue[0]
}

func TestLogHandlerReportsPassServerRules(t *testing.T) {
	srv, _ := newCapturingServer(t, http.StatusNoContent)
	cases := []struct {
		name string
		log  func(*slog.Logger)
	}{
		{"multi-line message becomes the operation", func(l *slog.Logger) {
			l.Error("peer closed\r\n\tconnection reset", "err", errors.New("boom"))
		}},
		{"empty component attr", func(l *slog.Logger) { l.Error("failed", "component", "") }},
		{"empty operation attr", func(l *slog.Logger) { l.Error("failed", "operation", "") }},
		{"empty message and attrs", func(l *slog.Logger) { l.Error("", "component", "", "operation", "") }},
		{"control characters in attrs", func(l *slog.Logger) { l.Error("failed", "component", "dow\nn", "operation", "a\tb") }},
		{"invalid utf-8 message", func(l *slog.Logger) { l.Error("failed", "err", errors.New(strings.Repeat("\xcf\xf0", 3000))) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(t, srv, nil)
			logger := slog.New(NewLogHandler(slog.NewTextHandler(&bytes.Buffer{}, nil), svc))
			logger.Info("earlier", "component", "launcher")
			rp := captureThroughHandler(t, svc, tc.log)
			if err := serverValidate(rp, time.Now()); err != nil {
				t.Fatalf("server would reject component=%q operation=%q: %v", rp.Component, rp.Operation, err)
			}
		})
	}
}

func TestOperationFallsBackToMessageWhenAttrIsEmpty(t *testing.T) {
	srv, _ := newCapturingServer(t, http.StatusNoContent)
	svc := newTestService(t, srv, nil)
	rp := captureThroughHandler(t, svc, func(l *slog.Logger) { l.Error("disk full", "operation", "", "component", "") })
	if rp.Operation != "disk full" || rp.Component == "" {
		t.Fatalf("component/operation = %q/%q, want a non-empty component and the message as operation", rp.Component, rp.Operation)
	}
}

func TestEmittedTimestampsCarryNoMonotonicReading(t *testing.T) {
	srv, _ := newCapturingServer(t, http.StatusNoContent)
	svc := newTestService(t, srv, nil)
	logger := slog.New(NewLogHandler(slog.NewTextHandler(&bytes.Buffer{}, nil), svc))
	logger.Warn("earlier", "component", "download")
	logger.Warn("later", "component", "download")
	svc.Capture("download", "fetch", errors.New("failed"), false)

	svc.mu.Lock()
	defer svc.mu.Unlock()
	rp := svc.queue[0]
	if strings.Contains(rp.Timestamp.String(), "m=") {
		t.Fatalf("report timestamp %s keeps a monotonic reading", rp.Timestamp)
	}
	if len(rp.Details.Breadcrumbs) != 2 {
		t.Fatalf("breadcrumbs = %d, want 2", len(rp.Details.Breadcrumbs))
	}
	for _, b := range rp.Details.Breadcrumbs {
		if strings.Contains(b.Timestamp.String(), "m=") {
			t.Fatalf("breadcrumb timestamp %s keeps a monotonic reading", b.Timestamp)
		}
	}
}

func TestBreadcrumbsAreFilteredAndOrderedOnWallClock(t *testing.T) {
	at := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	crumb := func(offset time.Duration, msg string) Breadcrumb {
		return Breadcrumb{Timestamp: at.Add(offset), Level: "INFO", Component: "download", Message: msg}
	}
	cases := []struct {
		name  string
		crumb []Breadcrumb
		want  []string
	}{
		{"after the report is dropped", []Breadcrumb{crumb(-time.Second, "a"), crumb(time.Nanosecond, "late")}, []string{"a"}},
		{"older than five minutes is dropped", []Breadcrumb{crumb(-5*time.Minute-time.Nanosecond, "old"), crumb(-5*time.Minute, "edge")}, []string{"edge"}},
		{"unordered input is sorted", []Breadcrumb{crumb(-time.Second, "b"), crumb(-3*time.Second, "a")}, []string{"a", "b"}},
		{"the report moment itself is kept", []Breadcrumb{crumb(0, "now")}, []string{"now"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rp := sanitizedPayload(t, Report{Component: "download", Operation: "x", Message: "m", Timestamp: at, Details: &Details{Breadcrumbs: tc.crumb}})
			var got []string
			for _, b := range rp.Details.Breadcrumbs {
				got = append(got, b.Message)
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("breadcrumbs = %v, want %v", got, tc.want)
			}
			if err := serverValidate(rp, at); err != nil {
				t.Fatalf("server would reject: %v", err)
			}
		})
	}
}

func pendingReport(id string, at time.Time) reportPayload {
	return reportPayload{
		ErrorID: id, AppVersion: "1.0.0", OS: "windows", Arch: "amd64",
		Component: "download", Operation: "fetch", ErrorCode: "unknown", Message: "m", Timestamp: at,
	}
}

func TestPendingReportsOutsideServerWindowAreDroppedNotSent(t *testing.T) {
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name     string
		stamps   []time.Time
		wantSent []string
	}{
		{"older than thirty days", []time.Time{now.Add(-serverPast - time.Hour)}, nil},
		{"more than a day ahead", []time.Time{now.Add(serverFuture + time.Hour)}, nil},
		{"only the stale one of a mixed batch", []time.Time{now.Add(-serverPast - time.Hour), now.Add(-time.Hour)}, []string{"id-1"}},
		{"fresh batch is untouched", []time.Time{now.Add(-time.Hour), now.Add(time.Minute)}, []string{"id-0", "id-1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, reqs := newCapturingServer(t, http.StatusNoContent)
			svc := newTestService(t, srv, nil)
			svc.clock = func() time.Time { return now }
			var batch []reportPayload
			for i, at := range tc.stamps {
				batch = append(batch, pendingReport(fmt.Sprintf("id-%d", i), at))
			}
			if err := savePending(svc.pendingDir, now, batch); err != nil {
				t.Fatal(err)
			}

			svc.flush(context.Background())

			var sent []string
			for len(reqs) > 0 {
				var decoded batchPayload
				if err := json.Unmarshal((<-reqs).body, &decoded); err != nil {
					t.Fatal(err)
				}
				for _, r := range decoded.Reports {
					sent = append(sent, r.ErrorID)
					if err := serverValidate(r, now); err != nil {
						t.Fatalf("sent a report the server rejects: %v", err)
					}
				}
			}
			if strings.Join(sent, ",") != strings.Join(tc.wantSent, ",") {
				t.Fatalf("sent %v, want %v", sent, tc.wantSent)
			}
			left, err := filepath.Glob(filepath.Join(svc.pendingDir, "*.json"))
			if err != nil {
				t.Fatal(err)
			}
			if len(left) != 0 {
				t.Fatalf("pending files left after flush: %v", left)
			}
			if _, err := os.Stat(filepath.Join(svc.pendingDir, "rejected")); err == nil {
				t.Fatal("a locally dropped report must not be quarantined as a server rejection")
			}
		})
	}
}
