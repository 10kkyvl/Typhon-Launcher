package typhonapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"typhon/internal/catalog"
)

func writeMatches(t *testing.T, w http.ResponseWriter, titles []string) {
	t.Helper()
	matches := make([]catalog.ReleaseMatch, len(titles))
	for i, title := range titles {
		matches[i].Game = &catalog.Game{ID: title}
	}
	if err := json.NewEncoder(w).Encode(struct {
		Matches []catalog.ReleaseMatch `json:"matches"`
	}{matches}); err != nil {
		t.Error(err)
	}
}

func decodeBatch(t *testing.T, r *http.Request) []string {
	t.Helper()
	var body struct {
		Queries []catalog.ReleaseQuery `json:"queries"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Error(err)
		return nil
	}
	titles := make([]string, len(body.Queries))
	for i, q := range body.Queries {
		titles[i] = q.Titles[0]
	}
	return titles
}

func batchQueries(n int) []catalog.ReleaseQuery {
	queries := make([]catalog.ReleaseQuery, n)
	for i := range queries {
		queries[i].Titles = []string{fmt.Sprint(i)}
	}
	return queries
}

func TestSourceMatcherReturnsPlainErrorWhenAllBatchesFail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	client, err := New(server.URL, func() (string, error) { return "", nil })
	if err != nil {
		t.Fatal(err)
	}

	got, err := client.MatchReleases(context.Background(), batchQueries(250))
	var partial *catalog.PartialMatchError
	if got != nil || err == nil || errors.As(err, &partial) {
		t.Fatalf("results=%v error=%v", got, err)
	}
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("error = %v, want it to wrap ErrUpstream", err)
	}
}

func TestSourceMatcherAbortsOutdatedEvenAfterPartialSuccess(t *testing.T) {
	var succeeded atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		titles := decodeBatch(t, r)
		if titles[0] == "400" {
			http.Error(w, "upgrade required", http.StatusUpgradeRequired)
			return
		}
		succeeded.Add(1)
		matches := make([]catalog.ReleaseMatch, len(titles))
		for i, title := range titles {
			matches[i].Game = &catalog.Game{ID: title}
		}
		if err := json.NewEncoder(w).Encode(struct {
			Matches []catalog.ReleaseMatch `json:"matches"`
		}{matches}); err != nil {
			// The fatal batch aborts the client, so this write may hit a closed connection.
			t.Logf("write matches: %v", err)
		}
	}))
	defer server.Close()
	client, err := New(server.URL, func() (string, error) { return "", nil })
	if err != nil {
		t.Fatal(err)
	}

	got, err := client.MatchReleases(context.Background(), batchQueries(500))
	if got != nil {
		t.Fatalf("results=%v, want nil on a fatal error", got)
	}
	if !errors.Is(err, ErrOutdated) {
		t.Fatalf("error = %v, want ErrOutdated", err)
	}
	if succeeded.Load() == 0 {
		t.Fatal("test did not exercise a successful batch before the fatal one")
	}
}

func TestSourceMatcherStopsDispatchingAfterThreeTransientFailures(t *testing.T) {
	entered := make(chan struct{}, 3)
	release := make(chan struct{})
	var received atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		entered <- struct{}{}
		<-release
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	client, err := New(server.URL, func() (string, error) { return "", nil })
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	var got []catalog.ReleaseMatch
	var matchErr error
	go func() {
		defer close(done)
		got, matchErr = client.MatchReleases(context.Background(), batchQueries(1000))
	}()

	for range 3 {
		<-entered
	}
	close(release)
	<-done

	if got != nil || matchErr == nil {
		t.Fatalf("results=%v error=%v", got, matchErr)
	}
	if n := received.Load(); n < 3 || n > 5 {
		t.Fatalf("server received %d of 10 batches, want dispatch cut short shortly after 3 failures", n)
	}
}

func TestSourceMatcherReturnsPartialWhenParentDeadlineExpiresMidway(t *testing.T) {
	block := make(chan struct{})
	defer close(block)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		titles := decodeBatch(t, r)
		start, err := strconv.Atoi(titles[0])
		if err != nil {
			t.Error(err)
			return
		}
		if start < 300 {
			writeMatches(t, w, titles)
			return
		}
		select {
		case <-block:
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	client, err := New(server.URL, func() (string, error) { return "", nil })
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	got, err := client.MatchReleases(ctx, batchQueries(500))
	var partial *catalog.PartialMatchError
	if !errors.As(err, &partial) {
		t.Fatalf("error = %v, want *catalog.PartialMatchError", err)
	}
	if partial.Total != 500 {
		t.Fatalf("total = %d, want 500", partial.Total)
	}
	if len(partial.Failed) == 0 || len(partial.Failed) >= 500 {
		t.Fatalf("failed = %d, want some but not all of 500", len(partial.Failed))
	}
	for i := range 300 {
		if got[i].Game == nil || got[i].Game.ID != fmt.Sprint(i) {
			t.Fatalf("query %d = %+v, want the pre-deadline match", i, got[i].Game)
		}
	}
}
