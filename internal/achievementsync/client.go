package achievementsync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"typhon/internal/account"
	"typhon/internal/app"
)

const (
	maxResponseBytes = 1 << 16
	requestTimeout   = 30 * time.Second
)

type client struct {
	baseURL    string
	httpClient *http.Client
}

func newClient(baseURL string) (*client, error) {
	base, err := account.ValidateBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	return &client{
		baseURL: base,
		httpClient: &http.Client{
			Timeout: requestTimeout,
			Transport: &http.Transport{
				DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 20 * time.Second,
				ExpectContinueTimeout: 5 * time.Second,
			},
			CheckRedirect: account.CheckRedirect,
		},
	}, nil
}

// report sends one batch. A nil result means the server accepted it; every
// failure is a *PermanentError, a *RetryableError, ErrNotSignedIn or a context
// error.
func (c *client) report(ctx context.Context, tok string, req request) error {
	if tok == "" {
		return ErrNotSignedIn
	}
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("encode achievements report: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+account.APIPrefix+"/me/achievements", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build achievements request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok)
	httpReq.Header.Set("User-Agent", account.UserAgent)
	httpReq.Header.Set("X-Typhon-Version", app.Version)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return &RetryableError{cause: err, Stop: true}
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Debug("close achievements response body", "error", err)
		}
	}()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	code := errorCode(io.LimitReader(resp.Body, maxResponseBytes))
	switch {
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusRequestTimeout, resp.StatusCode == http.StatusTooManyRequests:
		return &RetryableError{Status: resp.StatusCode, Code: code, Stop: true}
	case resp.StatusCode >= 500:
		return &RetryableError{Status: resp.StatusCode, Code: code}
	}
	return &PermanentError{Status: resp.StatusCode, Code: code}
}

func errorCode(body io.Reader) string {
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(body).Decode(&env); err != nil {
		return ""
	}
	return env.Error.Code
}
