package online

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
	"time"

	"typhon/internal/account"
	"typhon/internal/app"
)

const (
	maxResponseBytes = 8 << 10
	requestTimeout   = 10 * time.Second
)

var (
	ErrSignedOut    = errors.New("online: not signed in")
	ErrUnauthorized = errors.New("online: not authenticated")
)

type APIError struct {
	Code   string
	Field  string
	Status int
}

func (e *APIError) Error() string { return e.Code }

type NetworkError struct {
	cause error
}

func (e *NetworkError) Error() string { return fmt.Sprintf("online: network error: %v", e.cause) }

func (e *NetworkError) Unwrap() error { return e.cause }

type ServerError struct {
	Status int
}

func (e *ServerError) Error() string { return fmt.Sprintf("online: server error, status %d", e.Status) }

type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Field   string `json:"field"`
		Message string `json:"message"`
	} `json:"error"`
}

type payload struct {
	Status     string `json:"status"`
	GameID     string `json:"gameId"`
	AppVersion string `json:"appVersion"`
}

type client struct {
	baseURL    string
	token      func() (string, error)
	httpClient *http.Client
}

func newClient(baseURL string, token func() (string, error)) (*client, error) {
	base, err := account.ValidateBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	if token == nil {
		return nil, errors.New("online: token resolver is nil")
	}
	return &client{
		baseURL: base,
		token:   token,
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

func (c *client) report(ctx context.Context, p payload) error {
	return c.do(ctx, http.MethodPut, &p)
}

func (c *client) clear(ctx context.Context) error {
	return c.do(ctx, http.MethodDelete, nil)
}

func (c *client) do(ctx context.Context, method string, body *payload) error {
	tok, err := c.resolveToken()
	if err != nil {
		return err
	}

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode presence payload: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+account.APIPrefix+"/me/presence", reader)
	if err != nil {
		return fmt.Errorf("build presence request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("User-Agent", account.UserAgent)
	req.Header.Set("X-Typhon-Version", app.Version)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &NetworkError{cause: err}
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Debug("close presence response body", "error", err)
		}
	}()

	limited := io.LimitReader(resp.Body, maxResponseBytes)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeError(resp.StatusCode, limited)
	}
	return nil
}

func (c *client) resolveToken() (string, error) {
	tok, err := c.token()
	if err != nil {
		if errors.Is(err, account.ErrNoCredential) {
			return "", ErrSignedOut
		}
		return "", fmt.Errorf("resolve presence token: %w", err)
	}
	if tok == "" {
		return "", ErrSignedOut
	}
	return tok, nil
}

func decodeError(status int, body io.Reader) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return &ServerError{Status: status}
	}

	var env errorEnvelope
	if err := json.Unmarshal(data, &env); err != nil || env.Error.Code == "" {
		if status == http.StatusUnauthorized {
			return ErrUnauthorized
		}
		return &ServerError{Status: status}
	}

	switch {
	case status == http.StatusUnauthorized:
		return ErrUnauthorized
	case status >= 500:
		return &ServerError{Status: status}
	default:
		return &APIError{Code: env.Error.Code, Field: env.Error.Field, Status: status}
	}
}
