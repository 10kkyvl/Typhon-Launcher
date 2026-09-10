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
	"runtime"
	"time"

	"typhon/internal/account"
	"typhon/internal/app"
	"typhon/internal/clientid"
	"typhon/internal/telemetrylog"
)

const (
	requestTimeout   = 15 * time.Second
	maxErrorBodySize = 8 << 10
)

type client struct {
	baseURL    string
	httpClient *http.Client
}

func newTransport() *http.Transport {
	return &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 10 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: requestTimeout,
		ExpectContinueTimeout: 5 * time.Second,
	}
}

func newClient(baseURL string) (*client, error) {
	return newClientWithTimeout(baseURL, requestTimeout)
}

// newClientWithTimeout is newClient with an overridable http.Client.Timeout.
// The manual log upload in logsupload.go sends a body many times larger
// than an error batch and needs more room on a slow connection than the
// errors endpoint's fixed requestTimeout allows.
func newClientWithTimeout(baseURL string, timeout time.Duration) (*client, error) {
	base, err := account.ValidateBaseURL(baseURL)
	if err != nil {
		return nil, fmt.Errorf("validate diagnostics base url: %w", err)
	}
	transport := newTransport()
	transport.ResponseHeaderTimeout = timeout
	return &client{
		baseURL: base,
		httpClient: &http.Client{
			Timeout:       timeout,
			Transport:     transport,
			CheckRedirect: account.CheckRedirect,
		},
	}, nil
}

func (c *client) send(ctx context.Context, id clientid.Identity, reports []reportPayload) error {
	// Repair persisted reports from clients that used CodeNone for frontend errors.
	reports = append([]reportPayload(nil), reports...)
	for i := range reports {
		if reports[i].ErrorCode == "" {
			reports[i].ErrorCode = "unknown"
			if reports[i].Component == "frontend" {
				reports[i].ErrorCode = "frontend_error"
			}
		}
	}
	// Bound a flush even if a long outage accumulated more than one server batch.
	if len(reports) > 20 {
		for len(reports) > 0 {
			n := min(len(reports), 20)
			if err := c.send(ctx, id, reports[:n]); err != nil {
				return err
			}
			reports = reports[n:]
		}
		return nil
	}
	path := account.APIPrefix + "/diagnostics/errors"
	body, err := json.Marshal(batchPayload{
		InstallationID: id.InstallationID,
		SessionID:      id.SessionID,
		AppVersion:     app.Version,
		OS:             runtime.GOOS,
		Arch:           runtime.GOARCH,
		Reports:        reports,
	})
	if err != nil {
		return fmt.Errorf("encode %s payload: %w", path, err)
	}
	if len(body) > 256<<10 {
		if len(reports) < 2 {
			return &deliveryError{status: 413, code: "batch_too_large"}
		}
		mid := len(reports) / 2
		if err := c.send(ctx, id, reports[:mid]); err != nil {
			return err
		}
		return c.send(ctx, id, reports[mid:])
	}
	telemetrylog.Record(telemetrylog.KindDiagnostics, path, body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request %s: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			slog.Debug("close response body", "path", path, "error", cerr)
		}
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		limited := io.LimitReader(resp.Body, maxErrorBodySize)
		data, readErr := io.ReadAll(limited)
		if readErr != nil {
			return fmt.Errorf("%s: status %d, read error body: %w", path, resp.StatusCode, readErr)
		}
		var envelope struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(data, &envelope); err != nil {
			return &deliveryError{status: resp.StatusCode, requestID: resp.Header.Get("X-Request-ID")}
		}
		return &deliveryError{status: resp.StatusCode, code: envelope.Error.Code, requestID: resp.Header.Get("X-Request-ID")}
	}
	return nil
}

// Only payload rejections are permanent. Throttling and server/auth availability
// must retain the queued reports for a later attempt.
type deliveryError struct {
	status          int
	code, requestID string
}

func (e *deliveryError) Error() string {
	return fmt.Sprintf("diagnostics delivery: status=%d code=%q request_id=%q", e.status, e.code, e.requestID)
}
func permanentDeliveryError(err error) bool {
	var e *deliveryError
	return errors.As(err, &e) && (e.status == 400 || e.status == 413 || e.status == 422)
}
