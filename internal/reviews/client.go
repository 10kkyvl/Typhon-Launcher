package reviews

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
	"strconv"
	"time"

	"typhon/internal/account"
	"typhon/internal/app"
)

const (
	maxResponseBytes = 1 << 20
	requestTimeout   = 30 * time.Second
)

var ErrUnauthorized = errors.New("reviews: not authenticated")

type APIError struct {
	Code   string
	Field  string
	Status int
}

func (e *APIError) Error() string { return e.Code }

type NetworkError struct {
	cause error
}

func (e *NetworkError) Error() string { return fmt.Sprintf("reviews: network error: %v", e.cause) }

func (e *NetworkError) Unwrap() error { return e.cause }

type ServerError struct {
	Status int
}

func (e *ServerError) Error() string {
	return fmt.Sprintf("reviews: server error, status %d", e.Status)
}

type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Field   string `json:"field"`
		Message string `json:"message"`
	} `json:"error"`
}

type putReviewBody struct {
	Recommended bool   `json:"recommended"`
	Body        string `json:"body"`
}

type voteBody struct {
	Vote string `json:"vote"`
}

type reportBody struct {
	Reason string `json:"reason"`
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
		return nil, errors.New("reviews: token resolver is nil")
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

func reviewsPath(igdbID string) string {
	return account.APIPrefix + "/games/" + url.PathEscape(igdbID) + "/reviews"
}

func minePath(igdbID string) string {
	return reviewsPath(igdbID) + "/mine"
}

func votePath(id int64) string {
	return account.APIPrefix + "/reviews/" + strconv.FormatInt(id, 10) + "/vote"
}

func reportPath(id int64) string {
	return account.APIPrefix + "/reviews/" + strconv.FormatInt(id, 10) + "/report"
}

func (c *client) list(ctx context.Context, igdbID, sort, filter, cursor string) (Page, error) {
	query := url.Values{}
	query.Set("sort", sort)
	query.Set("filter", filter)
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	path := reviewsPath(igdbID) + "?" + query.Encode()

	var page Page
	if err := c.do(ctx, http.MethodGet, path, nil, &page, true); err != nil {
		return Page{}, err
	}
	return normalizePage(page), nil
}

func (c *client) mine(ctx context.Context, igdbID string) (Mine, error) {
	var mine Mine
	if err := c.do(ctx, http.MethodGet, minePath(igdbID), nil, &mine, false); err != nil {
		return Mine{}, err
	}
	return mine, nil
}

func (c *client) save(ctx context.Context, igdbID string, recommended bool, body string) (Review, error) {
	var review Review
	reqBody := putReviewBody{Recommended: recommended, Body: body}
	if err := c.do(ctx, http.MethodPut, minePath(igdbID), reqBody, &review, false); err != nil {
		return Review{}, err
	}
	return review, nil
}

func (c *client) delete(ctx context.Context, igdbID string) error {
	return c.do(ctx, http.MethodDelete, minePath(igdbID), nil, nil, false)
}

func (c *client) vote(ctx context.Context, id int64, vote string) (VoteResult, error) {
	var result VoteResult
	if err := c.do(ctx, http.MethodPut, votePath(id), voteBody{Vote: vote}, &result, false); err != nil {
		return VoteResult{}, err
	}
	return result, nil
}

func (c *client) report(ctx context.Context, id int64, reason string) error {
	return c.do(ctx, http.MethodPost, reportPath(id), reportBody{Reason: reason}, nil, false)
}

func normalizePage(page Page) Page {
	if page.Reviews == nil {
		page.Reviews = []Review{}
	}
	return page
}

func (c *client) do(ctx context.Context, method, path string, reqBody, out any, optionalAuth bool) error {
	tok, guest, err := c.resolveToken()
	if err != nil {
		return err
	}
	if guest && !optionalAuth {
		return ErrUnauthorized
	}

	var reader io.Reader
	if reqBody != nil {
		encoded, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("encode reviews request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("build reviews request: %w", err)
	}
	if !guest {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	req.Header.Set("User-Agent", account.UserAgent)
	req.Header.Set("X-Typhon-Version", app.Version)
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &NetworkError{cause: err}
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Debug("close reviews response body", "error", err)
		}
	}()

	limited := io.LimitReader(resp.Body, maxResponseBytes)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeError(resp.StatusCode, limited)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(limited).Decode(out); err != nil {
		return fmt.Errorf("decode reviews response: %w", err)
	}
	return nil
}

// resolveToken распознаёт гостя (нет сохранённых учётных данных) отдельно от
// сбоя получения токена: только первое допускает анонимный GET списка отзывов.
func (c *client) resolveToken() (tok string, guest bool, err error) {
	tok, err = c.token()
	if err != nil {
		if errors.Is(err, account.ErrNoCredential) {
			return "", true, nil
		}
		return "", false, fmt.Errorf("resolve reviews token: %w", err)
	}
	if tok == "" {
		return "", true, nil
	}
	return tok, false, nil
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
