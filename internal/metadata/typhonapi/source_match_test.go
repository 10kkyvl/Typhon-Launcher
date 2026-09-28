package typhonapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"typhon/internal/catalog"
)

func TestSourceMatcherBatchesAndRejectsPartialResponse(t *testing.T) {
	var mu sync.Mutex
	sizes := []int{}
	partial := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/catalog/match-releases" {
			t.Errorf("path=%s", r.URL.Path)
		}
		var body struct {
			Queries []catalog.ReleaseQuery `json:"queries"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		mu.Lock()
		sizes = append(sizes, len(body.Queries))
		mu.Unlock()
		matches := make([]catalog.ReleaseMatch, len(body.Queries))
		if partial {
			matches = nil
		}
		if err := json.NewEncoder(w).Encode(struct {
			Matches []catalog.ReleaseMatch `json:"matches"`
		}{matches}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	client, err := New(server.URL, func() (string, error) { return "", nil })
	if err != nil {
		t.Fatal(err)
	}
	queries := make([]catalog.ReleaseQuery, 205)
	for i := range queries {
		queries[i].Titles = []string{fmt.Sprintf("Game %d", i)}
	}
	got, err := client.MatchReleases(context.Background(), queries)
	slices.Sort(sizes)
	if err != nil || len(got) != 205 || fmt.Sprint(sizes) != "[5 100 100]" {
		t.Fatalf("batch=%v results=%d error=%v", sizes, len(got), err)
	}
	partial = true
	if _, err := client.MatchReleases(context.Background(), queries[:1]); !errors.Is(err, ErrUpstream) {
		t.Fatalf("partial reply=%v", err)
	}
}

func TestSourceMatcherBoundsParallelBatchesAndPreservesOrder(t *testing.T) {
	entered := make(chan struct{}, 3)
	release := make(chan struct{})
	var active, peak atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		defer active.Add(-1)
		for p := peak.Load(); n > p && !peak.CompareAndSwap(p, n); p = peak.Load() {
		}
		var body struct {
			Queries []catalog.ReleaseQuery `json:"queries"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		select {
		case entered <- struct{}{}:
		default:
		}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		matches := make([]catalog.ReleaseMatch, len(body.Queries))
		for i, q := range body.Queries {
			matches[i].Game = &catalog.Game{ID: q.Titles[0]}
		}
		if err := json.NewEncoder(w).Encode(struct {
			Matches []catalog.ReleaseMatch `json:"matches"`
		}{matches}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	client, err := New(server.URL, func() (string, error) { return "", nil })
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	queries := make([]catalog.ReleaseQuery, 507)
	for i := range queries {
		queries[i].Titles = []string{fmt.Sprint(i)}
	}
	done := make(chan struct{})
	var got []catalog.ReleaseMatch
	go func() { defer close(done); got, err = client.MatchReleases(ctx, queries) }()
	for range 3 {
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal("three batches did not run together")
		}
	}
	close(release)
	<-done
	if err != nil || len(got) != len(queries) {
		t.Fatalf("results=%d err=%v", len(got), err)
	}
	if peak.Load() != 3 {
		t.Fatalf("parallel requests=%d", peak.Load())
	}
	for i, match := range got {
		if match.Game == nil || match.Game.ID != fmt.Sprint(i) {
			t.Fatalf("query %d received %+v", i, match.Game)
		}
	}
}

func TestSourceMatcherKeepsOtherBatchesOnTransientFailure(t *testing.T) {
	entered := make(chan struct{}, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Queries []catalog.ReleaseQuery `json:"queries"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		entered <- struct{}{}
		if body.Queries[0].Titles[0] == "0" {
			for range 2 {
				<-entered
			}
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		matches := make([]catalog.ReleaseMatch, len(body.Queries))
		for i, q := range body.Queries {
			matches[i].Game = &catalog.Game{ID: q.Titles[0]}
		}
		if err := json.NewEncoder(w).Encode(struct {
			Matches []catalog.ReleaseMatch `json:"matches"`
		}{matches}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	client, err := New(server.URL, func() (string, error) { return "", nil })
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	queries := make([]catalog.ReleaseQuery, 500)
	for i := range queries {
		queries[i].Titles = []string{fmt.Sprint(i)}
	}
	got, err := client.MatchReleases(ctx, queries)
	var partial *catalog.PartialMatchError
	if !errors.As(err, &partial) {
		t.Fatalf("error = %v, want *catalog.PartialMatchError", err)
	}
	if partial.Total != 500 || len(partial.Failed) != 100 {
		t.Fatalf("partial = %+v", partial)
	}
	for _, idx := range partial.Failed {
		if idx < 0 || idx >= 100 {
			t.Fatalf("failed index %d outside the failing batch [0,100)", idx)
		}
	}
	for i, match := range got {
		if i < 100 {
			continue
		}
		if match.Game == nil || match.Game.ID != fmt.Sprint(i) {
			t.Fatalf("query %d received %+v, want a successful match", i, match.Game)
		}
	}
}
func TestSourceMatcherRequiresUpdatedBackend(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	client, err := New(server.URL, func() (string, error) { return "", nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.MatchReleases(context.Background(), []catalog.ReleaseQuery{{Titles: []string{"Portal"}}}); !errors.Is(err, catalog.ErrBackendOutdated) {
		t.Fatalf("old server: %v", err)
	}
}
