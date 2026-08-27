package diagnostics

import (
	"bytes"
	"context"
	"encoding/json"
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
		ResponseHeaderTimeout: 20 * time.Second,
		ExpectContinueTimeout: 5 * time.Second,
	}
}

func newClient(baseURL string) (*client, error) {
	base, err := account.ValidateBaseURL(baseURL)
	if err != nil {
		return nil, fmt.Errorf("validate diagnostics base url: %w", err)
	}
	return &client{
		baseURL: base,
		httpClient: &http.Client{
			Timeout:       requestTimeout,
			Transport:     newTransport(),
			CheckRedirect: account.CheckRedirect,
		},
	}, nil
}

func (c *client) send(ctx context.Context, id clientid.Identity, reports []reportPayload) error {
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
		return fmt.Errorf("%s: unexpected status %d: %s", path, resp.StatusCode, string(data))
	}
	return nil
}
