package typhonapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"typhon/internal/account"
	"typhon/internal/catalog"
	"typhon/internal/metadata"
)

func (c *Client) MatchReleases(ctx context.Context, queries []catalog.ReleaseQuery) ([]catalog.ReleaseMatch, error) {
	out := make([]catalog.ReleaseMatch, 0, len(queries))
	for start := 0; start < len(queries); start += 100 {
		batch := queries[start:min(start+100, len(queries))]
		body, err := json.Marshal(struct {
			Queries []catalog.ReleaseQuery `json:"queries"`
		}{batch})
		if err != nil {
			return nil, err
		}
		var payload struct {
			Matches []catalog.ReleaseMatch `json:"matches"`
		}
		for attempt := 0; ; attempt++ {
			err = c.post(ctx, account.APIPrefix+"/catalog/match-releases", body, &payload)
			var limit *metadata.RateLimitError
			if !errors.As(err, &limit) || attempt >= 2 {
				break
			}
			timer := time.NewTimer(max(time.Second, limit.RetryAfter))
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
		if err != nil {
			var status *httpStatusError
			if errors.As(err, &status) && (status.status == http.StatusNotFound || status.status == http.StatusMethodNotAllowed) {
				return nil, catalog.ErrBackendOutdated
			}
			return nil, err
		}
		if len(payload.Matches) != len(batch) {
			return nil, ErrUpstream
		}
		out = append(out, payload.Matches...)
	}
	return out, nil
}
