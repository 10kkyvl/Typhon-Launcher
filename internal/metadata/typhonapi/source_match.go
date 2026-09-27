package typhonapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"typhon/internal/account"
	"typhon/internal/catalog"
	"typhon/internal/metadata"
)

func (c *Client) MatchReleases(ctx context.Context, queries []catalog.ReleaseQuery) ([]catalog.ReleaseMatch, error) {
	// Large feeds need hundreds of batches. Keep a small bounded window in
	// flight so network latency does not consume the source refresh deadline.
	// Results retain query order, and no partial match set escapes on failure.
	out := make([]catalog.ReleaseMatch, len(queries))
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var workers sync.WaitGroup
	var once sync.Once
	var firstErr error
	jobs := make(chan int)
	for range min(3, (len(queries)+99)/100) {
		workers.Go(func() {
			for start := range jobs {
				if ctx.Err() != nil {
					return
				}
				batch := queries[start:min(start+100, len(queries))]
				matches, err := c.matchReleaseBatch(ctx, batch)
				if err != nil {
					once.Do(func() { firstErr = err; cancel() })
					return
				}
				copy(out[start:], matches)
			}
		})
	}
dispatch:
	for start := 0; start < len(queries); start += 100 {
		select {
		case jobs <- start:
		case <-ctx.Done():
			break dispatch
		}
	}
	close(jobs)
	workers.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) matchReleaseBatch(ctx context.Context, batch []catalog.ReleaseQuery) ([]catalog.ReleaseMatch, error) {
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
	return payload.Matches, nil
}
