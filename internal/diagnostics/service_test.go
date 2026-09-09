package diagnostics

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	svc, err := newServiceAt(t.TempDir(), testIdentity(), enabled)
	if err != nil {
		t.Fatalf("newServiceAt: %v", err)
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

// The server rejects a whole batch that carries a control character, and a
// message built from a subprocess's output can carry one. Stripping them here
// costs nothing readable and keeps a real report from being refused at the
// border.
func TestSanitizeReportStripsControlCharacters(t *testing.T) {
	out, err := sanitizeReport(Report{
		Component: "download",
		Operation: "fetch",
		Message:   "reset \x1b[31mred\x1b[0m\x07 done",
		Stack:     "download.fetch\n\tdownload.retry\x1b[0m",
	})
	if err != nil {
		t.Fatalf("sanitizeReport: %v", err)
	}
	for _, field := range []string{out.Message, out.Stack} {
		for _, r := range field {
			if r == '\n' || r == '\r' || r == '\t' {
				continue
			}
			if r < 0x20 || r == 0x7f {
				t.Fatalf("control character %#U survived sanitization: %q", r, field)
			}
		}
	}
	if !strings.Contains(out.Message, "reset") || !strings.Contains(out.Message, "done") {
		t.Fatalf("sanitization ate the readable text: %q", out.Message)
	}
	if !strings.Contains(out.Stack, "download.fetch\n\tdownload.retry") {
		t.Fatalf("sanitization broke the stack shape: %q", out.Stack)
	}
}

func TestNewServiceRejectsInvalidInputs(t *testing.T) {
	if _, err := NewService(testIdentity(), nil); err == nil {
		t.Fatal("expected error for nil enabled callback")
	}
	if _, err := NewService(clientid.Identity{}, func() bool { return true }); err == nil {
		t.Fatal("expected error for empty identity")
	}
}

func TestNewServiceAtRejectsEmptyConfigDir(t *testing.T) {
	if _, err := newServiceAt("", testIdentity(), func() bool { return true }); err == nil {
		t.Fatal("expected error for an empty config dir")
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

func TestFlushSpillsFailedBatchToDisk(t *testing.T) {
	badSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer badSrv.Close()

	svc := newTestService(t, badSrv, nil)
	svc.Capture("download", "start", errors.New("boom"), false)
	svc.flush(context.Background())

	entries, err := os.ReadDir(svc.pendingDir)
	if err != nil {
		t.Fatalf("read pending dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("pending files = %d, want 1", len(entries))
	}

	svc.mu.Lock()
	qlen := len(svc.queue)
	svc.mu.Unlock()
	if qlen != 0 {
		t.Fatalf("expected the failed batch to leave the live queue empty, got %d", qlen)
	}
}

func TestFlushDrainsPendingBeforeLiveQueue(t *testing.T) {
	badSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer badSrv.Close()

	svc := newTestService(t, badSrv, nil)
	svc.dedupWindow = 0
	svc.Capture("download", "start", errors.New("first failure"), false)
	svc.flush(context.Background())

	entries, err := os.ReadDir(svc.pendingDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected 1 pending file after the failed flush, got %d (err=%v)", len(entries), err)
	}

	goodSrv, reqs := newCapturingServer(t, http.StatusNoContent)
	cl, err := newClient(goodSrv.URL)
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	svc.client = cl

	svc.Capture("download", "second", errors.New("second"), false)
	svc.flush(context.Background())

	first := decodeSingleReport(t, waitFor(t, reqs, "/diagnostics/errors"))
	if first.Message != "first failure" {
		t.Fatalf("first drained report = %q, want the pending batch sent first", first.Message)
	}
	second := decodeSingleReport(t, waitFor(t, reqs, "/diagnostics/errors"))
	if second.Message != "second" {
		t.Fatalf("second report = %q, want the live batch sent after pending", second.Message)
	}

	entries, err = os.ReadDir(svc.pendingDir)
	if err != nil {
		t.Fatalf("read pending dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("pending files = %d after a successful drain, want 0", len(entries))
	}
}

func TestFlushRemovesCorruptPendingFileWithoutSending(t *testing.T) {
	srv, reqs := newCapturingServer(t, http.StatusNoContent)
	svc := newTestService(t, srv, nil)

	if err := os.MkdirAll(svc.pendingDir, 0o755); err != nil {
		t.Fatalf("mkdir pending dir: %v", err)
	}
	corrupt := filepath.Join(svc.pendingDir, "1.json")
	if err := os.WriteFile(corrupt, []byte("{ not json"), 0o600); err != nil {
		t.Fatalf("write corrupt pending file: %v", err)
	}

	svc.flush(context.Background())

	if _, err := os.Stat(corrupt); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("corrupt pending file should have been removed")
	}
	select {
	case req := <-reqs:
		t.Fatalf("expected no send attempt for a corrupt pending file, got %s", req.body)
	default:
	}
}

// A pending file that cannot be read is not a corrupt one: an antivirus lock
// or a failing disk makes ReadFile fail on a perfectly good report, and
// deleting it there destroys the only copy.
func TestFlushKeepsUnreadablePendingFileForRetry(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads a 0000 file regardless of its mode")
	}
	badSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer badSrv.Close()

	svc := newTestService(t, badSrv, nil)
	svc.Capture("download", "start", errors.New("boom"), false)
	svc.flush(context.Background())

	names, err := listPendingFiles(svc.pendingDir)
	if err != nil || len(names) != 1 {
		t.Fatalf("listPendingFiles = %v, %v, want exactly one spilled file", names, err)
	}
	path := filepath.Join(svc.pendingDir, names[0])
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("chmod pending file: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(path, 0o600); err != nil {
			t.Errorf("restore pending file mode: %v", err)
		}
	})

	svc.flush(context.Background())

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("unreadable pending file was dropped instead of kept for retry: %v", err)
	}
}

// Removing a sent file can fail on its own (a locked directory), and the
// report inside it has already reached the server. Sending it again on every
// tick would duplicate one report indefinitely.
func TestFlushDoesNotResendAPendingFileItCouldNotRemove(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root removes files from a read-only directory")
	}
	badSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	svc := newTestService(t, badSrv, nil)
	svc.Capture("download", "start", errors.New("boom"), false)
	svc.flush(context.Background())
	badSrv.Close()

	srv, reqs := newCapturingServer(t, http.StatusNoContent)
	cl, err := newClient(srv.URL)
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	svc.client = cl

	//nolint:gosec // G302: the point of the test is a directory that cannot be written to, so os.Remove fails the way a locked one does.
	if err := os.Chmod(svc.pendingDir, 0o500); err != nil {
		t.Fatalf("chmod pending dir: %v", err)
	}
	t.Cleanup(func() {
		//nolint:gosec // G302: restoring the directory to the mode t.TempDir gave it, so the cleanup can delete it.
		if err := os.Chmod(svc.pendingDir, 0o700); err != nil {
			t.Errorf("restore pending dir mode: %v", err)
		}
	})

	svc.flush(context.Background())
	svc.flush(context.Background())

	sent := 0
	for {
		select {
		case <-reqs:
			sent++
			continue
		default:
		}
		break
	}
	if sent != 1 {
		t.Fatalf("the same pending report was sent %d times, want 1", sent)
	}
}

// Opting out while a batch is in flight used to leave the report on disk: the
// spill ran after RemoveAll and recreated the directory it had just removed.
func TestOptOutDuringAnInFlightSendLeavesNothingOnDisk(t *testing.T) {
	release := make(chan struct{})
	arrived := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		select {
		case arrived <- struct{}{}:
		default:
		}
		<-release
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	svc := newTestService(t, srv, nil)
	svc.Capture("download", "start", errors.New("boom"), false)

	done := make(chan struct{})
	go func() {
		defer close(done)
		svc.flush(context.Background())
	}()

	<-arrived
	svc.SetEnabled(false)
	close(release)
	<-done

	if _, err := os.Stat(svc.pendingDir); !errors.Is(err, fs.ErrNotExist) {
		names, listErr := listPendingFiles(svc.pendingDir)
		t.Fatalf("a report survived opt-out on disk: %v (stat err %v, list err %v)", names, err, listErr)
	}
}

// capture() reads the opt-out flag, then enqueue() takes the lock again to
// store the report. A user who opts out in between must not end up with a
// report sitting in the queue, waiting for the next opt-in to send it.
func TestEnqueueRefusesAfterOptOut(t *testing.T) {
	srv, _ := newCapturingServer(t, http.StatusNoContent)
	svc := newTestService(t, srv, nil)

	svc.SetEnabled(false)
	svc.enqueue(reportPayload{Component: "download", Operation: "start"}, "fingerprint")

	svc.mu.Lock()
	queued := len(svc.queue)
	svc.mu.Unlock()
	if queued != 0 {
		t.Fatalf("queue holds %d reports after opt-out, want 0", queued)
	}
}

func TestSetEnabledFalseRemovesPendingDir(t *testing.T) {
	badSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer badSrv.Close()

	svc := newTestService(t, badSrv, nil)
	svc.Capture("download", "start", errors.New("boom"), false)
	svc.flush(context.Background())

	if _, err := os.Stat(svc.pendingDir); err != nil {
		t.Fatalf("expected a pending file before opt-out: %v", err)
	}

	svc.SetEnabled(false)

	if _, err := os.Stat(svc.pendingDir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("pending dir should be removed on opt-out")
	}
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
