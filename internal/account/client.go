package account

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
	"net/url"
	"strings"
	"time"

	"typhon/internal/app"
)

const (
	maxResponseBodySize = 1 << 20
	maxAvatarSize       = 10 << 20
	maxRedirects        = 5
)

type CurrentUser struct {
	ID          string          `json:"id"`
	Username    string          `json:"username"`
	DisplayName string          `json:"displayName"`
	Email       string          `json:"email"`
	AvatarURL   string          `json:"avatarUrl"`
	Profile     ProfileSettings `json:"profile"`
	CreatedAt   time.Time       `json:"createdAt"`
}

type Patch struct {
	Username    *string          `json:"username,omitempty"`
	DisplayName *string          `json:"displayName,omitempty"`
	Profile     *ProfileSettings `json:"profile,omitempty"`
}

type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Field   string `json:"field"`
		Message string `json:"message"`
	} `json:"error"`
}

type Client struct {
	baseURL    string
	token      func() (string, error)
	httpClient *http.Client
	uploadHTTP *http.Client
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

func CheckRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("stopped after %d redirects", maxRedirects)
	}
	if err := checkURLScheme(req.URL); err != nil {
		return err
	}
	prev := via[len(via)-1].URL
	if req.URL.Scheme != prev.Scheme || req.URL.Host != prev.Host {
		req.Header.Del("Authorization")
	}
	return nil
}

func NewClient(baseURL string, token func() (string, error)) (*Client, error) {
	base, err := ValidateBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	return &Client{
		baseURL: base,
		token:   token,
		httpClient: &http.Client{
			Timeout:       30 * time.Second,
			Transport:     newTransport(),
			CheckRedirect: CheckRedirect,
		},
		uploadHTTP: &http.Client{
			Timeout:       120 * time.Second,
			Transport:     newTransport(),
			CheckRedirect: CheckRedirect,
		},
	}, nil
}

func (c *Client) Me(ctx context.Context) (CurrentUser, error) {
	return c.doUser(ctx, http.MethodGet, APIPrefix+"/me", nil, "", c.httpClient)
}

func (c *Client) UpdateProfile(ctx context.Context, patch Patch) (CurrentUser, error) {
	body, err := json.Marshal(patch)
	if err != nil {
		return CurrentUser{}, fmt.Errorf("encode profile patch: %w", err)
	}
	return c.doUser(ctx, http.MethodPatch, APIPrefix+"/me", bytes.NewReader(body), "application/json", c.httpClient)
}

func (c *Client) UploadAvatar(ctx context.Context, data []byte) (CurrentUser, error) {
	if len(data) == 0 {
		return CurrentUser{}, &Error{Code: CodeInvalidAvatar}
	}
	if len(data) > maxAvatarSize {
		return CurrentUser{}, &Error{Code: CodeAvatarTooLarge}
	}
	return c.doUser(ctx, http.MethodPut, APIPrefix+"/me/avatar", bytes.NewReader(data), "application/octet-stream", c.uploadHTTP)
}

func (c *Client) RemoveAvatar(ctx context.Context) (CurrentUser, error) {
	return c.doUser(ctx, http.MethodDelete, APIPrefix+"/me/avatar", nil, "", c.httpClient)
}

func (c *Client) FetchAvatar(ctx context.Context, rawURL string) (AvatarImage, error) {
	if rawURL == "" {
		return AvatarImage{}, &Error{Code: CodeBadRequest, cause: errors.New("empty avatar url")}
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return AvatarImage{}, &Error{Code: CodeBadRequest, cause: fmt.Errorf("parse avatar url: %w", err)}
	}
	if err := checkURLScheme(parsed); err != nil {
		return AvatarImage{}, &Error{Code: CodeBadRequest, cause: err}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return AvatarImage{}, fmt.Errorf("build avatar request: %w", err)
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("X-Typhon-Version", app.Version)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		slog.Debug("fetch avatar failed", "url", rawURL, "error", err)
		return AvatarImage{}, &Error{Code: CodeNetwork, cause: err}
	}
	defer closeBody(resp)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return AvatarImage{}, &Error{Code: CodeServer, Status: resp.StatusCode, cause: fmt.Errorf("fetch avatar: unexpected status %d", resp.StatusCode)}
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAvatarSize+1))
	if err != nil {
		return AvatarImage{}, fmt.Errorf("read avatar body: %w", err)
	}
	if len(data) > maxAvatarSize {
		return AvatarImage{}, &Error{Code: CodeAvatarTooLarge}
	}
	return avatarImage(data)
}

func (c *Client) doUser(
	ctx context.Context,
	method, path string,
	body io.Reader,
	contentType string,
	hc *http.Client,
) (CurrentUser, error) {
	tok, err := c.resolveToken()
	if err != nil {
		return CurrentUser{}, err
	}

	resp, err := c.do(ctx, method, path, body, contentType, hc, tok)
	if err != nil {
		return CurrentUser{}, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Debug("close response body", "error", err)
		}
	}()

	limited := io.LimitReader(resp.Body, maxResponseBodySize)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return CurrentUser{}, decodeError(resp.StatusCode, limited)
	}

	var user CurrentUser
	if err := json.NewDecoder(limited).Decode(&user); err != nil {
		return CurrentUser{}, &Error{Code: CodeServer, Status: resp.StatusCode, cause: fmt.Errorf("decode response body: %w", err)}
	}
	return withProfileDefaults(user), nil
}

func (c *Client) resolveToken() (string, error) {
	tok, err := c.token()
	if err != nil {
		if errors.Is(err, ErrNoCredential) {
			return "", &Error{Code: CodeUnauthenticated, Status: http.StatusUnauthorized}
		}
		slog.Error("resolve api token", "error", err)
		return "", &Error{Code: CodeServer, cause: err}
	}
	return tok, nil
}

func (c *Client) do(
	ctx context.Context,
	method, path string,
	body io.Reader,
	contentType string,
	hc *http.Client,
	token string,
) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("build request %s %s: %w", method, path, err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("X-Typhon-Version", app.Version)

	resp, err := hc.Do(req)
	if err != nil {
		slog.Debug("account api request failed", "method", method, "path", path, "error", err)
		return nil, &Error{Code: CodeNetwork, cause: err}
	}
	return resp, nil
}

func snippet(data []byte) string {
	const max = 120
	s := strings.TrimSpace(string(data))
	if len(s) > max {
		return s[:max]
	}
	return s
}

func decodeError(status int, body io.Reader) error {
	data, readErr := io.ReadAll(body)
	if readErr != nil {
		return &Error{Code: CodeServer, Status: status, cause: fmt.Errorf("read error body: %w", readErr)}
	}

	code := CodeServer
	field := ""
	message := ""

	var env errorEnvelope
	if err := json.Unmarshal(data, &env); err == nil && env.Error.Code != "" {
		code = env.Error.Code
		field = env.Error.Field
		message = env.Error.Message
	} else {
		// A reply without our envelope never reached the API: it comes from
		// whatever sits in front of it, and only the status says what happened.
		slog.Error("account api replied outside the error contract",
			"status", status, "bytes", len(data), "body", snippet(data))
	}

	if code == CodeServer && status == http.StatusUnauthorized {
		code = CodeUnauthenticated
	}

	if code == CodeServer && status == http.StatusTooManyRequests {
		code = CodeRateLimited
	}

	if code == CodeServer && status == http.StatusForbidden {
		code = CodeBlocked
	}

	if code == CodeServer && status == http.StatusUpgradeRequired {
		code = CodeOutdated
	}

	return &Error{Code: code, Field: field, Status: status, Message: message}
}
