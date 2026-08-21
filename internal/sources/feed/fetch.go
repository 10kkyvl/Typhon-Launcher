package feed

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strings"
)

var (
	ErrBadScheme      = errors.New("недопустимая схема URL: разрешены только http и https")
	ErrTooLarge       = errors.New("размер ответа превышает допустимый лимит")
	ErrBadContentType = errors.New("недопустимый Content-Type ответа")
)

type StatusError struct {
	StatusCode int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("сервер вернул статус %d", e.StatusCode)
}

type Conditional struct {
	ETag         string
	LastModified string
}

type Result struct {
	NotModified  bool
	Feed         Feed
	ETag         string
	LastModified string
	Bytes        int64
}

func ValidateURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("некорректный URL: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", ErrBadScheme
	}
	if u.Host == "" {
		return "", errors.New("URL не содержит хост")
	}
	u.Scheme = scheme
	u.Host = strings.ToLower(u.Host)
	return u.String(), nil
}

func acceptableContentType(ct string) bool {
	if strings.TrimSpace(ct) == "" {
		return true
	}
	mt, _, err := mime.ParseMediaType(ct)
	if err != nil {
		mt = strings.ToLower(strings.TrimSpace(strings.SplitN(ct, ";", 2)[0]))
	}
	mt = strings.ToLower(mt)
	switch mt {
	case "application/json", "text/json", "text/plain":
		return true
	}
	if strings.HasPrefix(mt, "application/") && strings.HasSuffix(mt, "+json") {
		return true
	}
	return false
}

func Fetch(ctx context.Context, client *http.Client, raw string, cond Conditional) (Result, error) {
	normalized, err := ValidateURL(raw)
	if err != nil {
		return Result{}, err
	}

	if client == nil {
		client = &http.Client{
			Timeout: FetchTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= MaxRedirects {
					return fmt.Errorf("слишком много редиректов (лимит %d)", MaxRedirects)
				}
				return nil
			},
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, normalized, nil)
	if err != nil {
		return Result{}, fmt.Errorf("не удалось создать запрос: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Typhon/1.0")
	if cond.ETag != "" {
		req.Header.Set("If-None-Match", cond.ETag)
	}
	if cond.LastModified != "" {
		req.Header.Set("If-Modified-Since", cond.LastModified)
	}

	resp, err := client.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("ошибка запроса фида: %w", err)
	}
	defer resp.Body.Close()

	etag := resp.Header.Get("ETag")
	lastMod := resp.Header.Get("Last-Modified")

	if resp.StatusCode == http.StatusNotModified {
		if etag == "" {
			etag = cond.ETag
		}
		if lastMod == "" {
			lastMod = cond.LastModified
		}
		return Result{NotModified: true, ETag: etag, LastModified: lastMod}, nil
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{}, &StatusError{StatusCode: resp.StatusCode}
	}

	if !acceptableContentType(resp.Header.Get("Content-Type")) {
		return Result{}, ErrBadContentType
	}

	if resp.ContentLength > MaxBytes {
		return Result{}, ErrTooLarge
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxBytes+1))
	if err != nil {
		return Result{}, fmt.Errorf("ошибка чтения тела ответа: %w", err)
	}
	if int64(len(body)) > MaxBytes {
		return Result{}, ErrTooLarge
	}

	parsed, err := Parse(body)
	if err != nil {
		return Result{}, err
	}

	slog.Info("feed fetched", "url", normalized, "bytes", len(body), "entries", len(parsed.Entries), "invalid", parsed.Invalid)

	return Result{
		Feed:         parsed,
		ETag:         etag,
		LastModified: lastMod,
		Bytes:        int64(len(body)),
	}, nil
}
