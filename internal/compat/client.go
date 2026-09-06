package compat

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
	"typhon/internal/telemetrylog"
)

const (
	requestTimeout   = 15 * time.Second
	maxErrorBodySize = 8 << 10
	// maxStatsBodySize ограничивает снимок агрегата: он приходит с нашего
	// сервера, но размер ответа всё равно не повод съесть всю память.
	maxStatsBodySize = 8 << 20

	reportPath = "/compat/reports"
	statsPath  = "/compat/stats"
)

type client struct {
	baseURL    string
	httpClient *http.Client
}

func newClient(baseURL string) (*client, error) {
	base, err := account.ValidateBaseURL(baseURL)
	if err != nil {
		return nil, fmt.Errorf("validate compat base url: %w", err)
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

func closeBody(path string, body io.Closer) {
	if err := body.Close(); err != nil {
		slog.Debug("close response body", "path", path, "error", err)
	}
}

func statusError(path string, resp *http.Response) error {
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodySize))
	if err != nil {
		return fmt.Errorf("%s: status %d, read error body: %w", path, resp.StatusCode, err)
	}
	return fmt.Errorf("%s: unexpected status %d: %s", path, resp.StatusCode, string(data))
}

func (c *client) send(ctx context.Context, report Report) error {
	path := account.APIPrefix + reportPath
	body, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("encode %s payload: %w", path, err)
	}
	// Отчёт попадает в журнал отправленного до самой отправки: кнопка «показать
	// отправленные данные» обязана показывать и его, иначе обещание из
	// политики конфиденциальности перестаёт быть правдой.
	telemetrylog.Record(telemetrylog.KindCompat, path, body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request %s: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	defer closeBody(path, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return statusError(path, resp)
	}
	return nil
}

// fetchStats забирает снимок агрегата. Пустой etag означает «отдай целиком»;
// вернувшийся ok == false — что снимок не изменился и трогать кэш не нужно.
func (c *client) fetchStats(ctx context.Context, etag string) (snapshot Snapshot, ok bool, err error) {
	path := account.APIPrefix + statsPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return Snapshot{}, false, fmt.Errorf("build request %s: %w", path, err)
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Snapshot{}, false, fmt.Errorf("%s: %w", path, err)
	}
	defer closeBody(path, resp.Body)

	if resp.StatusCode == http.StatusNotModified {
		return Snapshot{}, false, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Snapshot{}, false, statusError(path, resp)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxStatsBodySize))
	if err != nil {
		return Snapshot{}, false, fmt.Errorf("%s: read body: %w", path, err)
	}
	var payload Snapshot
	if err := json.Unmarshal(data, &payload); err != nil {
		return Snapshot{}, false, fmt.Errorf("%s: decode body: %w", path, err)
	}
	payload.ETag = resp.Header.Get("ETag")
	return payload, true, nil
}
