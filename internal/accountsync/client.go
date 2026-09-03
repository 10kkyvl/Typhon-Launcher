package accountsync

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
	"typhon/internal/settings"
)

const (
	maxSyncResponseBytes = 1 << 20
	syncRequestTimeout   = 30 * time.Second
	syncPath             = account.APIPrefix + "/me/sync"
)

var (
	ErrUnauthorized = errors.New("accountsync: not authenticated")
	ErrConflict     = errors.New("accountsync: settings revision conflict")
	ErrTooManyGames = errors.New("accountsync: too many games in request")
	ErrDeviceLimit  = errors.New("accountsync: device limit reached")
)

type ValidationError struct {
	Code  string
	Field string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("accountsync: request rejected (%s, field %q)", e.Code, e.Field)
}

type NetworkError struct {
	cause error
}

func (e *NetworkError) Error() string { return fmt.Sprintf("accountsync: network error: %v", e.cause) }
func (e *NetworkError) Unwrap() error { return e.cause }

type ServerError struct {
	Status int
}

func (e *ServerError) Error() string {
	return fmt.Sprintf("accountsync: server error, status %d", e.Status)
}

type wireGame struct {
	IGDBID          int64      `json:"igdbId"`
	Owned           bool       `json:"owned"`
	Favorite        bool       `json:"favorite"`
	Status          string     `json:"status"`
	StatusAt        *time.Time `json:"statusAt"`
	LastPlayedAt    *time.Time `json:"lastPlayedAt"`
	PlaytimeSeconds int64      `json:"playtimeSeconds"`
}

type snapshotBody struct {
	Settings         settings.Portable `json:"settings"`
	SettingsRevision int64             `json:"settingsRevision"`
	Games            []wireGame        `json:"games"`
}

type applyBody struct {
	snapshotBody
	Skipped []int64 `json:"skipped"`
}

type putRequest struct {
	DeviceID         string             `json:"deviceId"`
	SettingsRevision int64              `json:"settingsRevision"`
	Settings         *settings.Portable `json:"settings,omitempty"`
	Games            []wireGame         `json:"games"`
}

type errorEnvelope struct {
	Error struct {
		Code  string `json:"code"`
		Field string `json:"field"`
	} `json:"error"`
}

type httpClient struct {
	baseURL string
	token   func() (string, error)
	client  *http.Client
}

func newHTTPClient(baseURL string, token func() (string, error)) (*httpClient, error) {
	base, err := account.ValidateBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	return &httpClient{
		baseURL: base,
		token:   token,
		client: &http.Client{
			Timeout: syncRequestTimeout,
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

func (c *httpClient) get(ctx context.Context) (snapshotBody, error) {
	var body snapshotBody
	if err := c.do(ctx, http.MethodGet, nil, &body); err != nil {
		return snapshotBody{}, err
	}
	return body, nil
}

func (c *httpClient) put(ctx context.Context, req putRequest) (applyBody, error) {
	var body applyBody
	if err := c.do(ctx, http.MethodPut, req, &body); err != nil {
		return applyBody{}, err
	}
	return body, nil
}

func (c *httpClient) remove(ctx context.Context) error {
	return c.do(ctx, http.MethodDelete, nil, nil)
}

func (c *httpClient) do(ctx context.Context, method string, reqBody, out any) error {
	tok, err := c.resolveToken()
	if err != nil {
		return err
	}

	var reader io.Reader
	if reqBody != nil {
		encoded, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("encode sync request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, c.baseURL+syncPath, reader)
	if err != nil {
		return fmt.Errorf("build sync request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok)
	httpReq.Header.Set("User-Agent", account.UserAgent)
	if reqBody != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return &NetworkError{cause: err}
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Debug("close sync response body", "error", err)
		}
	}()

	limited := io.LimitReader(resp.Body, maxSyncResponseBytes)

	if resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeSyncError(resp.StatusCode, limited)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(limited).Decode(out); err != nil {
		return fmt.Errorf("decode sync response: %w", err)
	}
	return nil
}

func (c *httpClient) resolveToken() (string, error) {
	tok, err := c.token()
	if err != nil {
		if errors.Is(err, account.ErrNoCredential) {
			return "", ErrUnauthorized
		}
		return "", fmt.Errorf("resolve sync token: %w", err)
	}
	if tok == "" {
		return "", ErrUnauthorized
	}
	return tok, nil
}

func decodeSyncError(status int, body io.Reader) error {
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
	case status == http.StatusConflict && env.Error.Code == "sync_conflict":
		return ErrConflict
	case env.Error.Code == "sync_too_many_games":
		return ErrTooManyGames
	case env.Error.Code == "sync_device_limit":
		return ErrDeviceLimit
	case status == http.StatusUnauthorized:
		return ErrUnauthorized
	case status >= 500:
		return &ServerError{Status: status}
	default:
		return &ValidationError{Code: env.Error.Code, Field: env.Error.Field}
	}
}
