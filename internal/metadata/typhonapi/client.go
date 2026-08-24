package typhonapi

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
	"strings"
	"time"

	"typhon/internal/account"
	"typhon/internal/metadata"
)

const (
	requestTimeout = 45 * time.Second
	maxBodyBytes   = 4 << 20
	defaultLimit   = 10
	maxLimit       = 25
	maxResolve     = 50
	providerName   = "igdb"
	maxRetryAfter  = 24 * time.Hour
)

var (
	ErrUpstream   = errors.New("сервер метаданных недоступен")
	ErrBadRequest = errors.New("сервер метаданных отклонил запрос")
)

type TokenFunc func() (string, error)

type Client struct {
	baseURL string
	token   TokenFunc
	http    *http.Client
}

func New(baseURL string, token TokenFunc) (*Client, error) {
	base, err := account.ValidateBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	if token == nil {
		return nil, errors.New("не задан источник токена сессии")
	}
	return &Client{
		baseURL: base,
		token:   token,
		http: &http.Client{
			Timeout: requestTimeout,
			Transport: &http.Transport{
				DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second,
				ExpectContinueTimeout: 5 * time.Second,
			},
			CheckRedirect: account.CheckRedirect,
		},
	}, nil
}

func (c *Client) Name() string {
	return providerName
}

func (c *Client) Search(ctx context.Context, query string, limit int) ([]metadata.Candidate, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("%w: пустой запрос", ErrBadRequest)
	}
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	params := url.Values{}
	params.Set("q", query)
	params.Set("limit", strconv.Itoa(limit))

	var payload searchResponse
	if err := c.get(ctx, "/metadata/search?"+params.Encode(), &payload); err != nil {
		return nil, err
	}

	candidates := make([]metadata.Candidate, 0, len(payload.Candidates))
	for _, c := range payload.Candidates {
		title := strings.TrimSpace(c.Title)
		if c.ProviderID == "" || title == "" {
			continue
		}
		candidates = append(candidates, metadata.Candidate{
			ProviderID:  c.ProviderID,
			Title:       title,
			ReleaseYear: c.ReleaseYear,
			Developer:   strings.TrimSpace(c.Developer),
			ThumbURL:    c.ThumbURL,
		})
	}
	return candidates, nil
}

func (c *Client) Get(ctx context.Context, providerID string) (metadata.GameMetadata, error) {
	providerID = strings.TrimSpace(providerID)
	if !numeric(providerID) {
		return metadata.GameMetadata{}, fmt.Errorf("%w: некорректный идентификатор %q", ErrBadRequest, providerID)
	}

	var payload gameResponse
	if err := c.get(ctx, "/metadata/games/"+providerID, &payload); err != nil {
		return metadata.GameMetadata{}, err
	}
	return gameMetadata(payload)
}

func (c *Client) Resolve(ctx context.Context, titles []string) ([]metadata.Resolved, error) {
	wanted := make([]string, 0, len(titles))
	for _, title := range titles {
		title = strings.TrimSpace(title)
		if title == "" {
			continue
		}
		wanted = append(wanted, title)
		if len(wanted) == maxResolve {
			break
		}
	}
	if len(wanted) == 0 {
		return nil, fmt.Errorf("%w: пустой список названий", ErrBadRequest)
	}

	body, err := json.Marshal(resolveRequest{Titles: wanted})
	if err != nil {
		return nil, fmt.Errorf("собрать запрос метаданных: %w", err)
	}

	var payload resolveResponse
	if err := c.post(ctx, "/metadata/resolve", body, &payload); err != nil {
		return nil, err
	}

	resolved := make([]metadata.Resolved, 0, len(payload.Games))
	for _, entry := range payload.Games {
		title := strings.TrimSpace(entry.Title)
		meta, err := gameMetadata(entry.Game)
		if err != nil {
			// Одна испорченная запись не должна отменять всю пачку: остальные
			// игры применяются, эта попадёт в поштучный поиск позже.
			slog.Warn("пропущена запись пачки метаданных", "title", title, "error", err)
			continue
		}
		if title == "" {
			title = meta.Title
		}
		resolved = append(resolved, metadata.Resolved{Title: title, Meta: meta})
	}
	return resolved, nil
}

func gameMetadata(payload gameResponse) (metadata.GameMetadata, error) {
	title := strings.TrimSpace(payload.Title)
	if payload.ProviderID == "" || title == "" {
		return metadata.GameMetadata{}, fmt.Errorf("%w: неполный ответ", ErrUpstream)
	}

	meta := metadata.GameMetadata{
		ProviderID:  payload.ProviderID,
		Title:       title,
		Summary:     payload.Summary,
		ReleaseDate: payload.ReleaseDate,
		Developer:   strings.TrimSpace(payload.Developer),
		Publisher:   strings.TrimSpace(payload.Publisher),
		Genres:      payload.Genres,
		Themes:      payload.Themes,
		Platforms:   payload.Platforms,
	}
	if payload.Cover != nil && payload.Cover.URL != "" {
		meta.Cover = &metadata.ImageRef{URL: payload.Cover.URL, Width: payload.Cover.Width, Height: payload.Cover.Height}
	}
	for _, shot := range payload.Screenshots {
		if shot.URL == "" {
			continue
		}
		meta.Screenshots = append(meta.Screenshots, metadata.ImageRef{URL: shot.URL, Width: shot.Width, Height: shot.Height})
	}
	return meta, nil
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	return c.send(ctx, http.MethodGet, path, nil, out)
}

func (c *Client) post(ctx context.Context, path string, body []byte, out any) error {
	return c.send(ctx, http.MethodPost, path, body, out)
}

func (c *Client) send(ctx context.Context, method, path string, body []byte, out any) error {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("собрать запрос метаданных: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	token, err := c.token()
	if err != nil {
		return fmt.Errorf("получить токен сессии: %w", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUpstream, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Debug("close metadata response body", "error", err)
		}
	}()

	limited := io.LimitReader(resp.Body, maxBodyBytes)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return statusError(resp, limited)
	}
	if err := json.NewDecoder(limited).Decode(out); err != nil {
		return fmt.Errorf("%w: разбор ответа: %w", ErrUpstream, err)
	}
	return nil
}

func statusError(resp *http.Response, body io.Reader) error {
	status := resp.StatusCode
	code := decodeCode(body)
	switch {
	case status == http.StatusServiceUnavailable && code == "metadata_unavailable":
		return fmt.Errorf("%w: провайдер не настроен на сервере", metadata.ErrNotConfigured)
	case status == http.StatusNotFound:
		return fmt.Errorf("%w: %d", metadata.ErrNoMatch, status)
	case status == http.StatusTooManyRequests:
		return &metadata.RateLimitError{RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"), time.Now())}
	case status == http.StatusBadRequest || status == http.StatusUnprocessableEntity:
		return fmt.Errorf("%w: %d %s", ErrBadRequest, status, code)
	default:
		return fmt.Errorf("%w: %d %s", ErrUpstream, status, code)
	}
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	secs, err := strconv.ParseInt(value, 10, 64)
	if err == nil || errors.Is(err, strconv.ErrRange) {
		switch {
		case secs <= 0:
			return 0
		case secs >= int64(maxRetryAfter/time.Second):
			return maxRetryAfter
		default:
			return time.Duration(secs) * time.Second
		}
	}
	when, err := http.ParseTime(value)
	if err != nil {
		return 0
	}
	wait := when.Sub(now)
	if wait <= 0 {
		return 0
	}
	return min(wait, maxRetryAfter)
}

func decodeCode(body io.Reader) string {
	var payload errorResponse
	if err := json.NewDecoder(body).Decode(&payload); err != nil {
		return ""
	}
	return payload.Error.Code
}

func numeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
