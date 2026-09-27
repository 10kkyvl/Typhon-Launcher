package diagnostics

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"

	"typhon/internal/app"
	"typhon/internal/clientid"
	"typhon/internal/uierr"
)

func gunzip(t *testing.T, data []byte) []byte {
	t.Helper()
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read gzip: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("close gzip reader: %v", err)
	}
	return out
}

func TestPostLogBundlePostsGzipAndReturnsID(t *testing.T) {
	raw := []byte("a fake zip archive, not gzip-compressed yet")
	var gotMethod, gotPath, gotEncoding, gotInstallation, gotSession string
	var gotBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotEncoding = r.Header.Get("Content-Encoding")
		gotInstallation = r.Header.Get("X-Installation-Id")
		gotSession = r.Header.Get("X-Session-Id")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		gotBody = body
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"id":"abc123"}`)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer srv.Close()

	gz, err := gzipForTest(t, raw)
	if err != nil {
		t.Fatalf("gzipForTest: %v", err)
	}

	id, err := postLogBundle(context.Background(), srv.Client(), srv.URL, clientid.Identity{InstallationID: "install-1", SessionID: "session-1"}, gz)
	if err != nil {
		t.Fatalf("postLogBundle: %v", err)
	}
	if id != "abc123" {
		t.Fatalf("id = %q, want %q", id, "abc123")
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/v1/diagnostics/logs" {
		t.Fatalf("path = %q, want /v1/diagnostics/logs", gotPath)
	}
	if gotEncoding != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", gotEncoding)
	}
	if gotInstallation != "install-1" || gotSession != "session-1" {
		t.Fatalf("identity headers = %q/%q, want install-1/session-1", gotInstallation, gotSession)
	}
	if string(gunzip(t, gotBody)) != string(raw) {
		t.Fatalf("uploaded body did not gunzip back to the original archive")
	}
}

func TestPostLogBundleTooLarge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		if _, err := w.Write([]byte("payload too large")); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer srv.Close()

	_, err := postLogBundle(context.Background(), srv.Client(), srv.URL, clientid.Identity{}, []byte("x"))
	if err == nil {
		t.Fatal("expected an error for a 413 response")
	}
	if code := uierr.Code(err); code != ErrCodeLogUploadTooLarge {
		t.Fatalf("code = %q, want %q", code, ErrCodeLogUploadTooLarge)
	}
}

func TestPostLogBundleRateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	_, err := postLogBundle(context.Background(), srv.Client(), srv.URL, clientid.Identity{}, []byte("x"))
	if err == nil {
		t.Fatal("expected an error for a 429 response")
	}
	if code := uierr.Code(err); code != ErrCodeLogUploadRateLimited {
		t.Fatalf("code = %q, want %q", code, ErrCodeLogUploadRateLimited)
	}
}

// "Try again later" and "this archive will never be accepted" are different
// answers for the user, so a temporary server failure and a rejected request
// must not arrive under the same code.
func TestPostLogBundleSeparatesTemporaryFailuresFromRejections(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   string
	}{
		{"server error", http.StatusInternalServerError, ErrCodeLogUploadUnavailable},
		{"storage unavailable", http.StatusServiceUnavailable, ErrCodeLogUploadUnavailable},
		{"bad request", http.StatusBadRequest, ErrCodeLogUploadRejected},
		{"unauthorized", http.StatusUnauthorized, ErrCodeLogUploadRejected},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			}))
			defer srv.Close()

			_, err := postLogBundle(context.Background(), srv.Client(), srv.URL, clientid.Identity{}, []byte("x"))
			if err == nil {
				t.Fatalf("expected an error for a %d response", tt.status)
			}
			if code := uierr.Code(err); code != tt.want {
				t.Fatalf("code = %q, want %q", code, tt.want)
			}
		})
	}
}

func TestPostLogBundleUnreachableServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	_, err := postLogBundle(context.Background(), srv.Client(), url, clientid.Identity{}, []byte("x"))
	if err == nil {
		t.Fatal("expected an error for an unreachable server")
	}
	if code := uierr.Code(err); code != ErrCodeLogUploadNetwork {
		t.Fatalf("code = %q, want %q", code, ErrCodeLogUploadNetwork)
	}
}

func TestPostLogBundleRespectsCancelledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"id":"abc123"}`)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := postLogBundle(ctx, srv.Client(), srv.URL, clientid.Identity{}, []byte("x"))
	if err == nil {
		t.Fatal("expected an error for a cancelled context")
	}
	if code := uierr.Code(err); code != ErrCodeLogUploadCancelled {
		t.Fatalf("code = %q, want %q", code, ErrCodeLogUploadCancelled)
	}
}

func TestPostLogBundleReportsOnlyRealBodyProgress(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Errorf("read body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"id":"progress-id"}`)); err != nil {
			t.Error(err)
		}
	}))
	defer srv.Close()

	progress := make([][2]int64, 0, 2)
	raw := bytes.Repeat([]byte("log line\n"), 20_000)
	gz, err := gzipForTest(t, raw)
	if err != nil {
		t.Fatalf("gzipForTest: %v", err)
	}
	_, err = postLogBundleWithProgress(context.Background(), srv.Client(), srv.URL, clientid.Identity{}, gz, func(sent, total int64) {
		progress = append(progress, [2]int64{sent, total})
	})
	if err != nil {
		t.Fatalf("postLogBundleWithProgress: %v", err)
	}
	if len(progress) == 0 || progress[0] != [2]int64{0, int64(len(gz))} {
		t.Fatalf("progress start = %v, want zero of %d", progress, len(gz))
	}
	last := progress[len(progress)-1]
	if last[0] != int64(len(gz)) || last[1] != int64(len(gz)) {
		t.Fatalf("progress end = %v, want %d/%d", last, len(gz), len(gz))
	}
}

func TestPostLogBundleReportsWaitingAfterBodyBeforeResponse(t *testing.T) {
	release := make(chan struct{})
	bodyRead := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength != 4 {
			t.Errorf("Content-Length = %d, want 4", r.ContentLength)
		}
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Error(err)
			return
		}
		close(bodyRead)
		<-release
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"id":"waiting-id"}`)); err != nil {
			t.Error(err)
		}
	}))
	defer srv.Close()

	waiting := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := postLogBundleWithCallbacks(context.Background(), srv.Client(), srv.URL, clientid.Identity{}, []byte("logs"), nil, func() { close(waiting) })
		done <- err
	}()
	<-bodyRead
	select {
	case <-waiting:
	case <-time.After(time.Second):
		t.Fatal("waiting status was not emitted after the request body")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestPostLogBundleTimeoutReturnsTimeoutError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-time.After(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := postLogBundle(ctx, srv.Client(), srv.URL, clientid.Identity{}, []byte("x"))
	if err == nil || uierr.Code(err) != ErrCodeLogUploadTimeout {
		t.Fatalf("error = %v, code = %q, want timeout error", err, uierr.Code(err))
	}
}

func TestPostLogBundleEmptyIDInResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{}`)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer srv.Close()

	_, err := postLogBundle(context.Background(), srv.Client(), srv.URL, clientid.Identity{}, []byte("x"))
	if err == nil {
		t.Fatal("expected an error for a response with no id")
	}
	if code := uierr.Code(err); code != ErrCodeLogUploadFailed {
		t.Fatalf("code = %q, want %q", code, ErrCodeLogUploadFailed)
	}
}

func TestSendLogsRequiresTheServiceToBeStarted(t *testing.T) {
	dir := t.TempDir()
	svc, err := newServiceAt(dir, clientid.Identity{InstallationID: "i", SessionID: "s"}, func() bool { return true })
	if err != nil {
		t.Fatalf("newServiceAt: %v", err)
	}

	if _, err := svc.SendLogs(); !errIsNotStarted(err) {
		t.Fatalf("SendLogs error = %v, want errDiagnosticsNotStarted", err)
	}
}

func TestSendLogsRejectsConcurrentUpload(t *testing.T) {
	dir := t.TempDir()
	svc, err := newServiceAt(dir, clientid.Identity{InstallationID: "i", SessionID: "s"}, func() bool { return true })
	if err != nil {
		t.Fatalf("newServiceAt: %v", err)
	}
	svc.logUploadMu.Lock()
	defer svc.logUploadMu.Unlock()
	if _, err := svc.SendLogs(); !errors.Is(err, errLogUploadBusy) {
		t.Fatalf("SendLogs error = %v, want busy", err)
	}
}

func errIsNotStarted(err error) bool {
	return errors.Is(err, errDiagnosticsNotStarted)
}

func gzipForTest(t *testing.T, raw []byte) ([]byte, error) {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(raw); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Бэкенд принимает непрозрачный gzip, поэтому версию и ОС взять больше
// неоткуда — они обязаны ехать заголовками, иначе приём отвечает 400.
func TestPostLogBundleSendsVersionAndOS(t *testing.T) {
	var gotVersion, gotOS string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotVersion = r.Header.Get("X-Typhon-Version")
		gotOS = r.Header.Get("X-Typhon-OS")
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"id":"abc123"}`)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer srv.Close()

	id := clientid.Identity{InstallationID: "inst", SessionID: "sess"}
	if _, err := postLogBundle(context.Background(), srv.Client(), srv.URL, id, []byte("gz")); err != nil {
		t.Fatalf("postLogBundle: %v", err)
	}
	if gotVersion != app.Version {
		t.Errorf("X-Typhon-Version = %q, want %q", gotVersion, app.Version)
	}
	if gotOS != runtime.GOOS {
		t.Errorf("X-Typhon-OS = %q, want %q", gotOS, runtime.GOOS)
	}
}
