package typhonapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"typhon/internal/catalog"
)

func TestMatchReleasesReportsRulesVersion(t *testing.T) {
	tests := []struct {
		name   string
		bodies []string
		want   int
	}{
		{"field present", []string{`{"rulesVersion":2,"matches":[{}]}`}, 2},
		{"old server without the field", []string{`{"matches":[{}]}`}, 0},
		{"highest of several answers", []string{`{"rulesVersion":3,"matches":[{}]}`, `{"rulesVersion":2,"matches":[{}]}`}, 3},
		{"later answer without the field keeps the known one", []string{`{"rulesVersion":2,"matches":[{}]}`, `{"matches":[{}]}`}, 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			call := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if _, err := w.Write([]byte(tc.bodies[call])); err != nil {
					t.Error(err)
				}
				call++
			}))
			defer server.Close()
			client, err := New(server.URL, func() (string, error) { return "", nil })
			if err != nil {
				t.Fatal(err)
			}
			if got := client.MatchRulesVersion(); got != 0 {
				t.Fatalf("version before any answer = %d, want 0", got)
			}
			for range tc.bodies {
				if _, err := client.MatchReleases(context.Background(), []catalog.ReleaseQuery{{Titles: []string{"A"}}}); err != nil {
					t.Fatal(err)
				}
			}
			if got := client.MatchRulesVersion(); got != tc.want {
				t.Fatalf("MatchRulesVersion = %d, want %d", got, tc.want)
			}
		})
	}
}
