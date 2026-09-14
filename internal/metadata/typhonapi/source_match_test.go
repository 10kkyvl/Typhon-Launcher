package typhonapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"typhon/internal/catalog"
)

func TestSourceMatcherBatchesAndRejectsPartialResponse(t *testing.T) {
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
		sizes = append(sizes, len(body.Queries))
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
	if err != nil || len(got) != 205 || fmt.Sprint(sizes) != "[100 100 5]" {
		t.Fatalf("batch=%v results=%d error=%v", sizes, len(got), err)
	}
	partial = true
	if _, err := client.MatchReleases(context.Background(), queries[:1]); !errors.Is(err, ErrUpstream) {
		t.Fatalf("partial reply=%v", err)
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
