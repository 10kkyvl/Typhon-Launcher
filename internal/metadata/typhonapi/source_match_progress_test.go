package typhonapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"typhon/internal/matchprogress"
)

type matchReports struct {
	mu    sync.Mutex
	done  []int
	total []int
}

func (r *matchReports) record(done, total int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.done = append(r.done, done)
	r.total = append(r.total, total)
}

func TestSourceMatcherReportsProgressAfterEveryBatch(t *testing.T) {
	cases := []struct {
		name     string
		failFrom string
		want     []int
	}{
		{"all batches answered", "", []int{100, 200, 250}},
		{"a failed batch still counts as processed", "100", []int{100, 200, 250}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				titles := decodeBatch(t, r)
				if tc.failFrom != "" && titles[0] == tc.failFrom {
					http.Error(w, "unavailable", http.StatusServiceUnavailable)
					return
				}
				writeMatches(t, w, titles)
			}))
			defer server.Close()
			client, err := New(server.URL, func() (string, error) { return "", nil })
			if err != nil {
				t.Fatal(err)
			}

			reports := &matchReports{}
			ctx := matchprogress.With(context.Background(), reports.record)
			if _, err := client.MatchReleases(ctx, batchQueries(250)); err != nil && tc.failFrom == "" {
				t.Fatal(err)
			}
			if !slices.Equal(reports.done, tc.want) {
				t.Fatalf("progress = %v, want %v", reports.done, tc.want)
			}
			for _, total := range reports.total {
				if total != 250 {
					t.Fatalf("totals = %v, want 250 each", reports.total)
				}
			}
		})
	}
}
