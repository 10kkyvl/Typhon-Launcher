package diagnostics

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"typhon/internal/clientid"
	"typhon/internal/redact"
	"typhon/internal/usagestats"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type capturedRequest struct {
	path string
	body []byte
}

func newCapturingServer(t *testing.T, status int) (*httptest.Server, chan capturedRequest) {
	t.Helper()
	ch := make(chan capturedRequest, 256)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		ch <- capturedRequest{path: r.URL.Path, body: body}
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv, ch
}

func waitFor(t *testing.T, ch chan capturedRequest, pathSuffix string) capturedRequest {
	t.Helper()
	for {
		select {
		case req := <-ch:
			if strings.HasSuffix(req.path, pathSuffix) {
				return req
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for request to %s", pathSuffix)
		}
	}
}

func testIdentity() clientid.Identity {
	return clientid.Identity{
		InstallationID: "11111111-1111-1111-1111-111111111111",
		SessionID:      "22222222-2222-2222-2222-222222222222",
	}
}

func newTestService(t *testing.T, srv *httptest.Server, enabled func() bool) *Service {
	t.Helper()
	if enabled == nil {
		enabled = func() bool { return true }
	}
	svc, err := NewService(testIdentity(), enabled)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	cl, err := newClient(srv.URL)
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	svc.client = cl
	return svc
}

func decodeSingleReport(t *testing.T, req capturedRequest) reportPayload {
	t.Helper()
	var decoded batchPayload
	if err := json.Unmarshal(req.body, &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(decoded.Reports) != 1 {
		t.Fatalf("expected exactly 1 report, got %d: %+v", len(decoded.Reports), decoded.Reports)
	}
	return decoded.Reports[0]
}

func TestNewServiceRejectsInvalidInputs(t *testing.T) {
	if _, err := NewService(testIdentity(), nil); err == nil {
		t.Fatal("expected error for nil enabled callback")
	}
	if _, err := NewService(clientid.Identity{}, func() bool { return true }); err == nil {
		t.Fatal("expected error for empty identity")
	}
}

func TestCaptureNoopWhenDisabled(t *testing.T) {
	srv, reqs := newCapturingServer(t, http.StatusNoContent)
	svc := newTestService(t, srv, func() bool { return false })

	svc.Capture("download", "start", errors.New("boom"), false)

	svc.mu.Lock()
	qlen := len(svc.queue)
	svc.mu.Unlock()
	if qlen != 0 {
		t.Fatalf("expected nothing queued while disabled, got %d", qlen)
	}
	select {
	case req := <-reqs:
		t.Fatalf("expected no request while disabled, got %s", req.body)
	default:
	}
}

func TestCaptureNilErrorIsNoop(t *testing.T) {
	srv, _ := newCapturingServer(t, http.StatusNoContent)
	svc := newTestService(t, srv, nil)

	svc.Capture("download", "start", nil, false)

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if len(svc.queue) != 0 {
		t.Fatalf("expected nil error to be a no-op, got %d queued", len(svc.queue))
	}
}

func TestCaptureSendsReportWhenEnabled(t *testing.T) {
	srv, reqs := newCapturingServer(t, http.StatusNoContent)
	svc := newTestService(t, srv, nil)

	svc.Capture("download", "start", errors.New("disk full"), false)
	svc.flush(context.Background())

	req := waitFor(t, reqs, "/diagnostics/errors")
	report := decodeSingleReport(t, req)
	if report.Component != "download" || report.Operation != "start" {
		t.Fatalf("unexpected component/operation: %+v", report)
	}
	if report.Message != "disk full" {
		t.Fatalf("message = %q, want %q", report.Message, "disk full")
	}
	if report.ErrorCode != usagestats.CodeUnknown {
		t.Fatalf("error code = %q, want %q", report.ErrorCode, usagestats.CodeUnknown)
	}
	if report.Fatal {
		t.Fatal("expected Fatal = false")
	}
	if report.ErrorID == "" {
		t.Fatal("expected a non-empty error id")
	}
}

func TestSetEnabledFalseClearsQueueAndBlocksSend(t *testing.T) {
	srv, reqs := newCapturingServer(t, http.StatusNoContent)
	svc := newTestService(t, srv, nil)

	svc.Capture("download", "start", errors.New("boom"), false)
	svc.mu.Lock()
	qlen := len(svc.queue)
	svc.mu.Unlock()
	if qlen == 0 {
		t.Fatal("expected report queued before opting out")
	}

	svc.SetEnabled(false)

	svc.mu.Lock()
	qlen = len(svc.queue)
	svc.mu.Unlock()
	if qlen != 0 {
		t.Fatalf("expected queue cleared after opt-out, got %d", qlen)
	}

	svc.flush(context.Background())
	select {
	case req := <-reqs:
		t.Fatalf("expected no request after opt-out, got %s", req.body)
	default:
	}

	svc.Capture("download", "start", errors.New("boom 2"), false)
	svc.mu.Lock()
	qlen = len(svc.queue)
	svc.mu.Unlock()
	if qlen != 0 {
		t.Fatal("expected Capture to stay a no-op while disabled")
	}

	svc.SetEnabled(true)
	svc.Capture("download", "start", errors.New("boom 3"), false)
	svc.mu.Lock()
	qlen = len(svc.queue)
	svc.mu.Unlock()
	if qlen != 1 {
		t.Fatalf("expected Capture to resume after SetEnabled(true), queue has %d", qlen)
	}
}

func TestWindowsPathRedacted(t *testing.T) {
	srv, reqs := newCapturingServer(t, http.StatusNoContent)
	svc := newTestService(t, srv, nil)

	svc.Capture("install", "extract", errors.New(`open C:\Users\10kk\AppData\Local\Typhon\state.json: access denied`), false)
	svc.flush(context.Background())

	report := decodeSingleReport(t, waitFor(t, reqs, "/diagnostics/errors"))
	for _, leak := range []string{"10kk", "AppData", `C:\`} {
		if strings.Contains(report.Message, leak) {
			t.Fatalf("message leaked %q: %q", leak, report.Message)
		}
	}
	if !strings.Contains(report.Message, redact.Path) {
		t.Fatalf("message missing redaction marker: %q", report.Message)
	}
}

func TestUnixPathRedacted(t *testing.T) {
	srv, reqs := newCapturingServer(t, http.StatusNoContent)
	svc := newTestService(t, srv, nil)

	svc.Capture("install", "extract", errors.New("open /home/egor/.config/typhon/state.json: permission denied"), false)
	svc.flush(context.Background())

	report := decodeSingleReport(t, waitFor(t, reqs, "/diagnostics/errors"))
	for _, leak := range []string{"egor", ".config"} {
		if strings.Contains(report.Message, leak) {
			t.Fatalf("message leaked %q: %q", leak, report.Message)
		}
	}
	if !strings.Contains(report.Message, redact.Path) {
		t.Fatalf("message missing redaction marker: %q", report.Message)
	}
}

func TestMagnetRedacted(t *testing.T) {
	srv, reqs := newCapturingServer(t, http.StatusNoContent)
	svc := newTestService(t, srv, nil)

	magnet := "magnet:?xt=urn:btih:a748597437835a2fd0d2e06f8edd86fee316a84d&dn=Startup+Panic&tr=udp%3A%2F%2Ftracker.example%3A80"
	svc.Capture("download", "add", errors.New("parse "+magnet+": bad metainfo"), false)
	svc.flush(context.Background())

	report := decodeSingleReport(t, waitFor(t, reqs, "/diagnostics/errors"))
	for _, leak := range []string{"btih", "a748597437835a2fd0d2e06f8edd86fee316a84d", "tracker.example", "Startup"} {
		if strings.Contains(report.Message, leak) {
			t.Fatalf("message leaked %q: %q", leak, report.Message)
		}
	}
	if !strings.Contains(report.Message, redact.Magnet) {
		t.Fatalf("message missing redaction marker: %q", report.Message)
	}
}

func TestInfohashRedacted(t *testing.T) {
	srv, reqs := newCapturingServer(t, http.StatusNoContent)
	svc := newTestService(t, srv, nil)

	svc.Capture("download", "verify", errors.New("torrent a748597437835a2fd0d2e06f8edd86fee316a84d stalled"), false)
	svc.flush(context.Background())

	report := decodeSingleReport(t, waitFor(t, reqs, "/diagnostics/errors"))
	if strings.Contains(report.Message, "a748597437835a2fd0d2e06f8edd86fee316a84d") {
		t.Fatalf("message leaked the infohash: %q", report.Message)
	}
	if !strings.Contains(report.Message, redact.Hash) {
		t.Fatalf("message missing redaction marker: %q", report.Message)
	}
}

func TestAuthTokenRedacted(t *testing.T) {
	srv, reqs := newCapturingServer(t, http.StatusNoContent)
	svc := newTestService(t, srv, nil)

	svc.Capture("account", "refresh", errors.New("request failed: Bearer eyJhbGciOiJIUzI1NiJ9.abcdefgh.signature rejected"), false)
	svc.flush(context.Background())

	report := decodeSingleReport(t, waitFor(t, reqs, "/diagnostics/errors"))
	for _, leak := range []string{"eyJhbGciOiJIUzI1NiJ9", "signature rejected"} {
		if strings.Contains(report.Message, leak) {
			t.Fatalf("message leaked %q: %q", leak, report.Message)
		}
	}
	if !strings.Contains(report.Message, redact.Token) {
		t.Fatalf("message missing redaction marker: %q", report.Message)
	}
}

func TestSourceURLRedacted(t *testing.T) {
	srv, reqs := newCapturingServer(t, http.StatusNoContent)
	svc := newTestService(t, srv, nil)

	svc.Capture("sources", "refresh", errors.New("fetch https://feed.example/list.json?token=s3cret failed"), false)
	svc.flush(context.Background())

	report := decodeSingleReport(t, waitFor(t, reqs, "/diagnostics/errors"))
	for _, leak := range []string{"s3cret", "list.json"} {
		if strings.Contains(report.Message, leak) {
			t.Fatalf("message leaked %q: %q", leak, report.Message)
		}
	}
	if !strings.Contains(report.Message, "https://feed.example") {
		t.Fatalf("message lost the host: %q", report.Message)
	}
}

func TestMessageLengthCap(t *testing.T) {
	srv, reqs := newCapturingServer(t, http.StatusNoContent)
	svc := newTestService(t, srv, nil)

	svc.Capture("install", "extract", errors.New(strings.Repeat("a", redact.MaxMessage*2)), false)
	svc.flush(context.Background())

	report := decodeSingleReport(t, waitFor(t, reqs, "/diagnostics/errors"))
	if len(report.Message) > redact.MaxMessage+4 {
		t.Fatalf("message length = %d, want <= %d", len(report.Message), redact.MaxMessage+4)
	}
}

func TestStackLengthCap(t *testing.T) {
	srv, reqs := newCapturingServer(t, http.StatusNoContent)
	svc := newTestService(t, srv, nil)

	svc.CapturePanic("launcher", "boom", []byte(strings.Repeat("frame\n", redact.MaxStack)))
	svc.flush(context.Background())

	report := decodeSingleReport(t, waitFor(t, reqs, "/diagnostics/errors"))
	if len(report.Stack) > redact.MaxStack+4 {
		t.Fatalf("stack length = %d, want <= %d", len(report.Stack), redact.MaxStack+4)
	}
}

func TestRateLimit(t *testing.T) {
	srv, _ := newCapturingServer(t, http.StatusNoContent)
	svc := newTestService(t, srv, nil)
	svc.ratePerMinute = 3
	svc.dedupWindow = 0

	for i := 0; i < 10; i++ {
		svc.Capture("download", "op"+strconv.Itoa(i), errors.New("distinct "+strconv.Itoa(i)), false)
	}

	svc.mu.Lock()
	qlen := len(svc.queue)
	svc.mu.Unlock()
	if qlen != 3 {
		t.Fatalf("expected rate limit to cap the queue at 3, got %d", qlen)
	}
}

func TestRateLimitWindowResets(t *testing.T) {
	srv, _ := newCapturingServer(t, http.StatusNoContent)
	svc := newTestService(t, srv, nil)
	svc.ratePerMinute = 1
	svc.dedupWindow = 0
	svc.rateWindow = time.Minute

	now := time.Now()
	svc.clock = func() time.Time { return now }

	svc.Capture("download", "a", errors.New("first"), false)
	svc.Capture("download", "b", errors.New("second"), false)
	svc.mu.Lock()
	qlen := len(svc.queue)
	svc.mu.Unlock()
	if qlen != 1 {
		t.Fatalf("expected second report to be rate limited, queue has %d", qlen)
	}

	now = now.Add(2 * time.Minute)
	svc.Capture("download", "c", errors.New("third"), false)
	svc.mu.Lock()
	qlen = len(svc.queue)
	svc.mu.Unlock()
	if qlen != 2 {
		t.Fatalf("expected a fresh rate window to admit another report, queue has %d", qlen)
	}
}

func TestFingerprintGroupingCollapsesRepeats(t *testing.T) {
	srv, _ := newCapturingServer(t, http.StatusNoContent)
	svc := newTestService(t, srv, nil)

	for i := 0; i < 5; i++ {
		svc.Capture("download", "start", errors.New("disk full"), false)
	}

	svc.mu.Lock()
	qlen := len(svc.queue)
	svc.mu.Unlock()
	if qlen != 1 {
		t.Fatalf("expected identical errors to collapse into 1 queued report, got %d", qlen)
	}
}

func TestFingerprintGroupingDifferentComponentNotCollapsed(t *testing.T) {
	srv, _ := newCapturingServer(t, http.StatusNoContent)
	svc := newTestService(t, srv, nil)

	svc.Capture("download", "start", errors.New("disk full"), false)
	svc.Capture("install", "start", errors.New("disk full"), false)

	svc.mu.Lock()
	qlen := len(svc.queue)
	svc.mu.Unlock()
	if qlen != 2 {
		t.Fatalf("expected different components to stay separate reports, got %d", qlen)
	}
}

func TestDedupWindowExpires(t *testing.T) {
	srv, _ := newCapturingServer(t, http.StatusNoContent)
	svc := newTestService(t, srv, nil)
	svc.ratePerMinute = 100
	svc.dedupWindow = time.Minute

	now := time.Now()
	svc.clock = func() time.Time { return now }

	svc.Capture("download", "start", errors.New("disk full"), false)
	svc.Capture("download", "start", errors.New("disk full"), false)
	svc.mu.Lock()
	qlen := len(svc.queue)
	svc.mu.Unlock()
	if qlen != 1 {
		t.Fatalf("expected the second identical report inside the window to be collapsed, got %d", qlen)
	}

	now = now.Add(2 * time.Minute)
	svc.Capture("download", "start", errors.New("disk full"), false)
	svc.mu.Lock()
	qlen = len(svc.queue)
	svc.mu.Unlock()
	if qlen != 2 {
		t.Fatalf("expected the report to be re-admitted once the dedup window passed, got %d", qlen)
	}
}

func TestSanitizerFailureDropsReport(t *testing.T) {
	srv, reqs := newCapturingServer(t, http.StatusNoContent)
	svc := newTestService(t, srv, nil)

	orig := sanitizeText
	forceErr := errors.New("forced sanitize failure")
	sanitizeText = func(string) (string, error) { return "", forceErr }
	t.Cleanup(func() { sanitizeText = orig })

	svc.Capture("download", "start", errors.New("disk full"), false)

	svc.mu.Lock()
	qlen := len(svc.queue)
	svc.mu.Unlock()
	if qlen != 0 {
		t.Fatalf("expected a sanitizer failure to drop the report, got %d queued", qlen)
	}

	svc.flush(context.Background())
	select {
	case req := <-reqs:
		t.Fatalf("expected no request for a dropped report, got %s", req.body)
	default:
	}
}

func TestSanitizeReportRecoversFromPanic(t *testing.T) {
	orig := sanitizeText
	sanitizeText = func(string) (string, error) { panic("boom") }
	t.Cleanup(func() { sanitizeText = orig })

	_, err := sanitizeReport(Report{Message: "boom", Component: "download"})
	if err == nil {
		t.Fatal("expected sanitizeReport to convert a panic into an error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error = %v, want it to mention the panic value", err)
	}
}

func TestCapturePanicSendsFatalReport(t *testing.T) {
	srv, reqs := newCapturingServer(t, http.StatusNoContent)
	svc := newTestService(t, srv, nil)

	svc.CapturePanic("launcher", "nil pointer dereference", []byte("main.main()\n\t/path/main.go:1 +0x1\n"))
	svc.flush(context.Background())

	report := decodeSingleReport(t, waitFor(t, reqs, "/diagnostics/errors"))
	if !report.Fatal {
		t.Fatal("expected CapturePanic to produce a Fatal report")
	}
	if report.Component != "launcher" || report.Operation != "panic" {
		t.Fatalf("unexpected component/operation: %+v", report)
	}
	if !strings.Contains(report.Message, "nil pointer dereference") {
		t.Fatalf("message = %q, want it to mention the recovered value", report.Message)
	}
}

func TestReportClientErrorSanitizesFrontendInput(t *testing.T) {
	srv, reqs := newCapturingServer(t, http.StatusNoContent)
	svc := newTestService(t, srv, nil)

	err := svc.ReportClientError("ui", "render", `TypeError at C:\Users\10kk\AppData\Local\Typhon`, "at Foo (app.js:1:1)", true)
	if err != nil {
		t.Fatalf("ReportClientError: %v", err)
	}
	svc.flush(context.Background())

	report := decodeSingleReport(t, waitFor(t, reqs, "/diagnostics/errors"))
	if strings.Contains(report.Message, "10kk") {
		t.Fatalf("frontend message leaked: %q", report.Message)
	}
	if !report.Fatal {
		t.Fatal("expected Fatal = true to survive")
	}
}

func TestFlushRespectsCancelledContext(t *testing.T) {
	srv, reqs := newCapturingServer(t, http.StatusNoContent)
	svc := newTestService(t, srv, nil)

	svc.Capture("download", "start", errors.New("boom"), false)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	svc.flush(ctx)

	select {
	case req := <-reqs:
		t.Fatalf("expected no request with a cancelled context, got %s", req.body)
	default:
	}

	svc.mu.Lock()
	qlen := len(svc.queue)
	svc.mu.Unlock()
	if qlen != 0 {
		t.Fatalf("expected the failed batch to be dropped rather than requeued, got %d", qlen)
	}
}

func TestFlushHappensAtThresholdWithoutWaitingForTicker(t *testing.T) {
	srv, reqs := newCapturingServer(t, http.StatusNoContent)
	svc := newTestService(t, srv, nil)
	svc.flushThreshold = 2
	svc.flushInterval = time.Hour
	svc.dedupWindow = 0

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	defer func() {
		if err := svc.ServiceShutdown(); err != nil {
			t.Fatalf("ServiceShutdown: %v", err)
		}
	}()

	svc.Capture("download", "a", errors.New("first"), false)
	svc.Capture("download", "b", errors.New("second"), false)

	waitFor(t, reqs, "/diagnostics/errors")
}

func TestServiceShutdownFlushesFinalBatch(t *testing.T) {
	srv, reqs := newCapturingServer(t, http.StatusNoContent)
	svc := newTestService(t, srv, nil)
	svc.flushThreshold = 1000
	svc.flushInterval = time.Hour

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	svc.Capture("download", "a", errors.New("first"), false)

	if err := svc.ServiceShutdown(); err != nil {
		t.Fatalf("ServiceShutdown: %v", err)
	}

	waitFor(t, reqs, "/diagnostics/errors")
}

func TestQueueOverflowDropsOldestNotNewest(t *testing.T) {
	srv, _ := newCapturingServer(t, http.StatusNoContent)
	svc := newTestService(t, srv, nil)
	svc.maxQueue = 3
	svc.ratePerMinute = 1000
	svc.dedupWindow = 0

	for i := 0; i < 5; i++ {
		svc.Capture("download", "op"+strconv.Itoa(i), errors.New("distinct "+strconv.Itoa(i)), false)
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if len(svc.queue) != 3 {
		t.Fatalf("expected queue capped at 3, got %d", len(svc.queue))
	}
	want := []string{"op2", "op3", "op4"}
	for i, w := range want {
		if svc.queue[i].Operation != w {
			t.Fatalf("expected newest reports retained in order %v, got %+v", want, svc.queue)
		}
	}
}

func TestConcurrentCaptureAndSetEnabled(t *testing.T) {
	srv, reqs := newCapturingServer(t, http.StatusNoContent)
	svc := newTestService(t, srv, nil)
	svc.flushThreshold = 4
	svc.flushInterval = 10 * time.Millisecond
	svc.ratePerMinute = 1000
	svc.dedupWindow = 0

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			svc.Capture("download", "op"+strconv.Itoa(n), errors.New("distinct"), false)
		}(i)
	}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			svc.SetEnabled(n%2 == 0)
		}(i)
	}
	wg.Wait()
	svc.SetEnabled(true)

	if err := svc.ServiceShutdown(); err != nil {
		t.Fatalf("ServiceShutdown: %v", err)
	}
	drain(reqs)
}

func drain(ch chan capturedRequest) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}
