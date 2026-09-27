package diagnostics

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptrace"
	"runtime"
	"sync"
	"time"

	"github.com/google/uuid"
	"typhon/internal/account"
	"typhon/internal/app"
	"typhon/internal/clientid"
	"typhon/internal/redact"
	"typhon/internal/uierr"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	logUploadPath = account.APIPrefix + "/diagnostics/logs"

	// logUploadHTTPTimeout is longer than the errors endpoint's
	// requestTimeout: an upload can carry up to maxLogUploadGzipBytes,
	// several orders of magnitude bigger than an error batch, and needs
	// room on a slow connection.
	logUploadHTTPTimeout = 60 * time.Second

	// maxLogUploadGzipBytes bounds the gzip-compressed body of a manual log
	// upload. Rotation keeps typhon.log plus up to 5 backups at 10 MiB
	// each (internal/app/logging.go), so a full set is at most ~60 MiB raw;
	// plain text runs about tenfold through gzip, landing a full set near
	// 6 MiB compressed. 8 MiB leaves headroom for a still-growing current
	// log without ever needing to drop a backup in ordinary use.
	maxLogUploadGzipBytes int64 = 8 << 20

	maxLogUploadErrorBody = 4 << 10
)

// Error codes surfaced to the frontend for a manual log upload. They travel
// as typhon:<code> in the error text (see internal/uierr) so the UI can show
// a specific reason instead of raw request/response text.
const (
	ErrCodeLogUploadTooLarge    = "diagnostics.log_upload_too_large"
	ErrCodeLogUploadRateLimited = "diagnostics.log_upload_rate_limited"
	ErrCodeLogUploadNetwork     = "diagnostics.log_upload_network"
	ErrCodeLogUploadUnavailable = "diagnostics.log_upload_unavailable"
	ErrCodeLogUploadRejected    = "diagnostics.log_upload_rejected"
	ErrCodeLogUploadFailed      = "diagnostics.log_upload_failed"
	ErrCodeLogUploadTimeout     = "diagnostics.log_upload_timeout"
	ErrCodeLogUploadCancelled   = "diagnostics.log_upload_cancelled"
	ErrCodeLogUploadBusy        = "diagnostics.log_upload_busy"
)

var errDiagnosticsNotStarted = errors.New("diagnostics service is not started")

var errLogUploadBusy = uierr.New(ErrCodeLogUploadBusy, "a log upload is already in progress")

const logUploadStatusEvent = "diagnostics:logs_status"

// LogUploadStatus is emitted while a manual upload is running. Byte counts
// are only populated for the network phase, where the HTTP request body has a
// known compressed length; preparation intentionally stays indeterminate.
type LogUploadStatus struct {
	State      string `json:"state"` // preparing, sending, waiting, success, error
	SentBytes  int64  `json:"sentBytes,omitempty"`
	TotalBytes int64  `json:"totalBytes,omitempty"`
}

func emitLogUploadStatus(status LogUploadStatus) {
	if a := application.Get(); a != nil {
		a.Event.Emit(logUploadStatusEvent, status)
	}
}

// SendLogsResult is what a manual log upload hands back to the frontend: the
// short id support can look the bundle up by, plus the names of any old log
// rotations that did not fit under the client-side size cap and were left
// out of what was actually sent.
type SendLogsResult struct {
	ID      string   `json:"id"`
	Dropped []string `json:"dropped"`
}

type logUploadResponse struct {
	ID string `json:"id"`
}

// SendLogs uploads exactly the bundle ExportLogs would write to disk — gzip
// compressed and capped client-side, dropping the oldest rotations first if
// it would not otherwise fit (see app.BuildLogUpload). It only ever runs
// from a direct user action: there is no automatic retry, no queue, and a
// failed upload here never touches the diagnostics/pending spill directory
// the automatic error-batch pipeline above uses.
//
// The identity travels the way clientid.Identity always does elsewhere in
// this package, just over headers instead of a JSON body field: the
// request body here is the opaque gzip archive itself, so there is no JSON
// envelope to carry installation_id/session_id inside.
func (s *Service) SendLogs() (result SendLogsResult, err error) {
	if !s.logUploadMu.TryLock() {
		return SendLogsResult{}, errLogUploadBusy
	}
	defer s.logUploadMu.Unlock()

	emitLogUploadStatus(LogUploadStatus{State: "preparing"})
	defer func() {
		if err != nil {
			emitLogUploadStatus(LogUploadStatus{State: "error"})
			slog.Warn("log upload failed", "component", "diagnostics", "error_code", uierr.Code(err), "error", redact.Text(err.Error()))
			s.Capture("diagnostics_upload", "send", err, false)
		}
	}()

	s.mu.Lock()
	base := s.ctx
	s.mu.Unlock()
	if base == nil {
		return SendLogsResult{}, errDiagnosticsNotStarted
	}

	data, dropped, err := app.BuildLogUpload(maxLogUploadGzipBytes)
	if err != nil {
		return SendLogsResult{}, err
	}
	emitLogUploadStatus(LogUploadStatus{State: "sending", TotalBytes: int64(len(data))})

	cl, err := newClientWithTimeout(account.BaseURL(), logUploadHTTPTimeout)
	if err != nil {
		return SendLogsResult{}, err
	}

	ctx, cancel := context.WithTimeout(base, logUploadHTTPTimeout)
	defer cancel()

	id, err := postLogBundleWithCallbacks(ctx, cl.httpClient, cl.baseURL, s.identity, data,
		func(sent, total int64) {
			emitLogUploadStatus(LogUploadStatus{State: "sending", SentBytes: sent, TotalBytes: total})
		}, func() {
			emitLogUploadStatus(LogUploadStatus{State: "waiting"})
		})
	if err != nil {
		return SendLogsResult{}, err
	}
	emitLogUploadStatus(LogUploadStatus{State: "success"})
	return SendLogsResult{ID: id, Dropped: dropped}, nil
}

// postLogBundle POSTs an already gzip-compressed log bundle and returns the
// short id the backend hands back. Every non-2xx status becomes a coded
// uierr so the frontend can show a specific reason instead of raw text.
func postLogBundle(ctx context.Context, httpClient *http.Client, baseURL string, id clientid.Identity, gzipped []byte) (string, error) {
	return postLogBundleWithProgress(ctx, httpClient, baseURL, id, gzipped, nil)
}

type logUploadReader struct {
	r          *bytes.Reader
	total      int64
	sent       int64
	lastSent   int64
	lastEmit   time.Time
	onProgress func(sent, total int64)
}

func (r *logUploadReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	r.sent += int64(n)
	if n > 0 && r.onProgress != nil && (r.sent == r.total || r.sent-r.lastSent >= 64<<10 || time.Since(r.lastEmit) >= 100*time.Millisecond) {
		r.lastSent = r.sent
		r.lastEmit = time.Now()
		r.onProgress(r.sent, r.total)
	}
	return n, err
}

func postLogBundleWithProgress(ctx context.Context, httpClient *http.Client, baseURL string, id clientid.Identity, gzipped []byte, onProgress func(sent, total int64)) (string, error) {
	return postLogBundleWithCallbacks(ctx, httpClient, baseURL, id, gzipped, onProgress, nil)
}

func postLogBundleWithCallbacks(ctx context.Context, httpClient *http.Client, baseURL string, id clientid.Identity, gzipped []byte, onProgress func(sent, total int64), onWaiting func()) (bundleID string, resultErr error) {
	uploadID := uuid.NewString()
	started := time.Now()
	serverRequestID := ""
	slog.Info("log upload started", "component", "diagnostics", "upload_id", uploadID, "bytes", len(gzipped))
	defer func() {
		slog.Info("log upload finished", "component", "diagnostics", "upload_id", uploadID, "request_id", serverRequestID, "duration_ms", time.Since(started).Milliseconds(), "success", resultErr == nil, "error_code", uierr.Code(resultErr))
	}()
	// Transport callbacks may outlive Do when a server rejects the body early.
	var callbackMu sync.Mutex
	callbacksClosed := false
	defer func() { callbackMu.Lock(); callbacksClosed = true; callbackMu.Unlock() }()
	progress := func(sent, total int64) {
		callbackMu.Lock()
		defer callbackMu.Unlock()
		if !callbacksClosed && onProgress != nil {
			onProgress(sent, total)
		}
	}
	trace := &httptrace.ClientTrace{WroteRequest: func(info httptrace.WroteRequestInfo) {
		callbackMu.Lock()
		defer callbackMu.Unlock()
		if info.Err == nil && !callbacksClosed {
			slog.Info("log upload waiting for response", "component", "diagnostics", "upload_id", uploadID, "duration_ms", time.Since(started).Milliseconds())
			if onWaiting != nil {
				onWaiting()
			}
		}
	}}
	ctx = httptrace.WithClientTrace(ctx, trace)
	reqBody := &logUploadReader{r: bytes.NewReader(gzipped), total: int64(len(gzipped)), onProgress: progress}
	if onProgress != nil {
		onProgress(0, reqBody.total)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+logUploadPath, reqBody)
	if err != nil {
		return "", uierr.Wrap(ErrCodeLogUploadFailed, fmt.Errorf("build request %s: %w", logUploadPath, err))
	}
	req.ContentLength = reqBody.total
	req.Header.Set("X-Typhon-Upload-ID", uploadID)
	req.Header.Set("Content-Type", "application/zip")
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("X-Installation-Id", id.InstallationID)
	req.Header.Set("X-Session-Id", id.SessionID)
	// Тело — непрозрачный gzip, поле в него не воткнуть, поэтому версия и ОС
	// едут заголовками: без них приём на бэкенде отвечает 400.
	req.Header.Set("X-Typhon-Version", app.Version)
	req.Header.Set("X-Typhon-OS", runtime.GOOS)

	resp, err := httpClient.Do(req)
	if err != nil {
		code := ErrCodeLogUploadNetwork
		var netErr net.Error
		if errors.Is(err, context.Canceled) {
			code = ErrCodeLogUploadCancelled
		} else if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
			code = ErrCodeLogUploadTimeout
		}
		return "", uierr.Wrap(code, fmt.Errorf("upload %s: %w", uploadID, err))
	}
	serverRequestID = resp.Header.Get("X-Request-ID")
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			slog.Debug("close log upload response body", "error", cerr)
		}
	}()

	if resp.StatusCode == http.StatusOK {
		var decoded logUploadResponse
		limited := io.LimitReader(resp.Body, maxLogUploadErrorBody)
		if err := json.NewDecoder(limited).Decode(&decoded); err != nil {
			return "", uierr.Wrap(ErrCodeLogUploadFailed, fmt.Errorf("decode %s response: %w", logUploadPath, err))
		}
		if decoded.ID == "" {
			return "", uierr.New(ErrCodeLogUploadFailed, fmt.Sprintf("%s: empty id in response", logUploadPath))
		}
		return decoded.ID, nil
	}

	limited := io.LimitReader(resp.Body, maxLogUploadErrorBody)
	body, readErr := io.ReadAll(limited)
	if readErr != nil {
		return "", uierr.Wrap(ErrCodeLogUploadFailed, fmt.Errorf("%s: status %d, read error body: %w", logUploadPath, resp.StatusCode, readErr))
	}

	// The user gets a different sentence for each class, so the classes must
	// not collapse into one code: a 5xx is worth retrying later, a 4xx never
	// is, and the two named statuses have advice of their own.
	switch {
	case resp.StatusCode == http.StatusRequestEntityTooLarge:
		return "", uierr.New(ErrCodeLogUploadTooLarge, fmt.Sprintf("%s: %d %s", logUploadPath, resp.StatusCode, string(body)))
	case resp.StatusCode == http.StatusTooManyRequests:
		return "", uierr.New(ErrCodeLogUploadRateLimited, fmt.Sprintf("%s: %d %s", logUploadPath, resp.StatusCode, string(body)))
	case resp.StatusCode == http.StatusGatewayTimeout || resp.StatusCode == http.StatusRequestTimeout:
		return "", uierr.New(ErrCodeLogUploadTimeout, fmt.Sprintf("upload %s: server timeout, request_id=%s", uploadID, serverRequestID))
	case resp.StatusCode >= 500:
		return "", uierr.New(ErrCodeLogUploadUnavailable, fmt.Sprintf("%s: status %d: %s", logUploadPath, resp.StatusCode, string(body)))
	case resp.StatusCode >= 400:
		return "", uierr.New(ErrCodeLogUploadRejected, fmt.Sprintf("%s: status %d: %s", logUploadPath, resp.StatusCode, string(body)))
	default:
		return "", uierr.New(ErrCodeLogUploadFailed, fmt.Sprintf("%s: unexpected status %d: %s", logUploadPath, resp.StatusCode, string(body)))
	}
}
