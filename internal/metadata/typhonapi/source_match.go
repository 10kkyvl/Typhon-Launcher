package typhonapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"typhon/internal/account"
	"typhon/internal/catalog"
	"typhon/internal/matchprogress"
	"typhon/internal/metadata"
)

const (
	maxTransientBatchFailures = 3

	// The server hands out three match-releases slots per process, shared by
	// every user, so one launcher must not hold more than one of them.
	matchBatchesInFlight = 1
)

func (c *Client) MatchReleases(ctx context.Context, queries []catalog.ReleaseQuery) ([]catalog.ReleaseMatch, error) {
	// Large feeds need hundreds of batches, sent one at a time.
	// A transient batch failure costs only that batch; a fatal one aborts all.
	out := make([]catalog.ReleaseMatch, len(queries))
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	batchCount := (len(queries) + 99) / 100
	var (
		mu               sync.Mutex
		fatalErr         error
		firstTransient   error
		transientCount   int
		succeededBatches int
		processed        int
		failedIdx        = map[int]bool{}
	)
	var stopDispatch atomic.Bool

	markFailed := func(start int) {
		end := min(start+100, len(queries))
		for i := start; i < end; i++ {
			failedIdx[i] = true
		}
	}

	jobs := make(chan int)
	var workers sync.WaitGroup
	for range min(matchBatchesInFlight, batchCount) {
		workers.Go(func() {
			for start := range jobs {
				if ctx.Err() != nil || stopDispatch.Load() {
					mu.Lock()
					markFailed(start)
					mu.Unlock()
					continue
				}
				batch := queries[start:min(start+100, len(queries))]
				matches, err := c.matchReleaseBatch(ctx, batch)
				if err == nil {
					copy(out[start:], matches)
					mu.Lock()
					succeededBatches++
					processed += len(batch)
					matchprogress.Report(ctx, processed, len(queries))
					mu.Unlock()
					continue
				}
				mu.Lock()
				markFailed(start)
				processed += len(batch)
				matchprogress.Report(ctx, processed, len(queries))
				fatal := fatalBatchError(err)
				if fatal {
					if fatalErr == nil {
						fatalErr = err
					}
				} else {
					if firstTransient == nil {
						firstTransient = err
					}
					transientCount++
				}
				if fatal || transientCount >= maxTransientBatchFailures {
					stopDispatch.Store(true)
				}
				mu.Unlock()
				if fatal {
					cancel()
				}
			}
		})
	}

	next := 0
dispatch:
	for next < len(queries) {
		if stopDispatch.Load() {
			break
		}
		select {
		case jobs <- next:
			next += 100
		case <-ctx.Done():
			break dispatch
		}
	}
	close(jobs)
	workers.Wait()

	mu.Lock()
	defer mu.Unlock()
	for start := next; start < len(queries); start += 100 {
		markFailed(start)
	}

	if fatalErr != nil {
		return nil, fatalErr
	}
	if succeededBatches == 0 {
		if firstTransient != nil {
			return nil, firstTransient
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return out, nil
	}
	if len(failedIdx) == 0 {
		return out, nil
	}
	failed := make([]int, 0, len(failedIdx))
	for i := range failedIdx {
		failed = append(failed, i)
	}
	sort.Ints(failed)
	cause := firstTransient
	if cause == nil {
		cause = ctx.Err()
	}
	return out, &catalog.PartialMatchError{Failed: failed, Total: len(queries), Err: cause}
}

func fatalBatchError(err error) bool {
	if errors.Is(err, catalog.ErrBackendOutdated) || errors.Is(err, ErrOutdated) || errors.Is(err, ErrBadRequest) {
		return true
	}
	if errors.Is(err, context.Canceled) {
		return true
	}
	var status *httpStatusError
	if errors.As(err, &status) && (status.status == http.StatusUnauthorized || status.status == http.StatusForbidden) {
		return true
	}
	return false
}

func (c *Client) matchReleaseBatch(ctx context.Context, batch []catalog.ReleaseQuery) ([]catalog.ReleaseMatch, error) {
	body, err := json.Marshal(struct {
		Queries []catalog.ReleaseQuery `json:"queries"`
	}{batch})
	if err != nil {
		return nil, err
	}
	var payload struct {
		RulesVersion int                    `json:"rulesVersion"`
		Matches      []catalog.ReleaseMatch `json:"matches"`
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
	c.noteRulesVersion(payload.RulesVersion)
	return payload.Matches, nil
}

func (c *Client) noteRulesVersion(v int) {
	for {
		cur := c.rulesVersion.Load()
		if int64(v) <= cur || c.rulesVersion.CompareAndSwap(cur, int64(v)) {
			return
		}
	}
}

// MatchRulesVersion is the highest rules version any match answer reported;
// zero while the server has not reported one.
func (c *Client) MatchRulesVersion() int {
	return int(c.rulesVersion.Load())
}
