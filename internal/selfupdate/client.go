package selfupdate

import (
	"context"
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

var ErrManifestStatus = errors.New("selfupdate: manifest endpoint returned an error status")

// The manifest is a few kilobytes: on a blocked network the user should
// learn within seconds that the check failed, not after half a minute.
var httpTimeout = 15 * time.Second

type Client struct {
	baseURL        string
	httpClient     *http.Client
	downloadClient *http.Client
}

func NewClient(baseURL string) (*Client, error) {
	base, err := account.ValidateBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 10 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
		ExpectContinueTimeout: 5 * time.Second,
	}
	return &Client{
		baseURL: base,
		httpClient: &http.Client{
			Timeout:       httpTimeout,
			Transport:     transport,
			CheckRedirect: account.CheckRedirect,
		},
		// Артефакт весит до MaxArtifactSize: дедлайн на весь запрос убивает
		// исправную медленную загрузку, поэтому здесь его нет, а зависшую
		// передачу обрывает stallTimeout в Download.
		downloadClient: &http.Client{
			Transport:     transport,
			CheckRedirect: account.CheckRedirect,
		},
	}, nil
}

func (c *Client) FetchManifest(ctx context.Context) (Manifest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+ManifestPath, nil)
	if err != nil {
		return Manifest{}, fmt.Errorf("build manifest request: %w", err)
	}
	req.Header.Set("User-Agent", account.UserAgent)
	req.Header.Set("X-Typhon-Version", app.Version)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Manifest{}, fmt.Errorf("fetch manifest: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			slog.Debug("close manifest response body", "error", cerr)
		}
	}()

	if resp.StatusCode == http.StatusUpgradeRequired {
		return Manifest{}, ErrManifestOutdated
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Manifest{}, fmt.Errorf("%w: status %d", ErrManifestStatus, resp.StatusCode)
	}

	limited := io.LimitReader(resp.Body, MaxManifestSize+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest body: %w", err)
	}
	if int64(len(data)) > MaxManifestSize {
		return Manifest{}, ErrManifestTooLarge
	}

	key, err := PublicKey()
	if err != nil {
		return Manifest{}, err
	}
	return VerifyManifest(data, key)
}
