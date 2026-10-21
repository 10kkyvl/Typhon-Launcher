package typhonapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"typhon/internal/account"
	"typhon/internal/metadata"
)

func achievementsClient(t *testing.T, status int, body string, seen *http.Request) *Client {
	t.Helper()
	return newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if seen != nil {
			*seen = *r
		}
		writeJSON(t, w, status, body)
	}), nil)
}

func TestAchievementsOK(t *testing.T) {
	var seen http.Request
	body := `{"appId":620,"total":3,"items":[
		{"key":"ACH_ONE","name":" First ","description":"d","hidden":true,"icon":"https://cdn.example/a.jpg","iconGray":"https://cdn.example/b.jpg"},
		{"key":"ACH_TWO","name":"Second","description":"","hidden":false,"icon":"http://cdn.example/a.jpg","iconGray":"javascript:alert(1)"},
		{"key":"../bad","name":"x"}]}`
	client := achievementsClient(t, http.StatusOK, body, &seen)

	got, err := client.Achievements(metadata.WithLanguage(context.Background(), "ru"), "620")
	if err != nil {
		t.Fatalf("achievements: %v", err)
	}
	if seen.URL.Path != account.APIPrefix+"/steam/apps/620/achievements" {
		t.Fatalf("path = %q", seen.URL.Path)
	}
	if lang := seen.Header.Get("X-Typhon-Metadata-Language"); lang != "ru" {
		t.Fatalf("language header = %q, want ru", lang)
	}
	if got.AppID != "620" || len(got.Items) != 2 {
		t.Fatalf("got %+v, the key with .. must be dropped", got)
	}
	first := got.Items[0]
	if first.Name != "First" || !first.Hidden || first.Icon != "https://cdn.example/a.jpg" {
		t.Fatalf("first = %+v", first)
	}
	if second := got.Items[1]; second.Icon != "" || second.IconGray != "" {
		t.Fatalf("non-https icons must be dropped: %+v", second)
	}
}

func TestAchievementsStatusesAreDistinct(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   error
		not    error
	}{
		{"no achievements", http.StatusNotFound, `{"error":"not_found"}`, ErrNoAchievements, ErrSteamUnavailable},
		{"no steam key", http.StatusServiceUnavailable, `{"error":"steam_unavailable"}`, ErrSteamUnavailable, ErrNoAchievements},
		{"steam down", http.StatusBadGateway, `{"error":"upstream_unavailable"}`, ErrSteamUnavailable, ErrNoAchievements},
		{"server error", http.StatusInternalServerError, `{}`, ErrUpstream, ErrNoAchievements},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := achievementsClient(t, tc.status, tc.body, nil)
			_, err := client.Achievements(context.Background(), "620")
			if !errors.Is(err, tc.want) || errors.Is(err, tc.not) {
				t.Fatalf("err = %v, want %v and not %v", err, tc.want, tc.not)
			}
			if errors.Is(err, metadata.ErrNoMatch) {
				t.Fatalf("err = %v must not look like a title mismatch", err)
			}
		})
	}
}

func TestAchievementsRejectsBadInput(t *testing.T) {
	huge := `{"appId":620,"items":[` + strings.Repeat(`{"key":"A"},`, maxAchievements) + `{"key":"B"}]}`
	cases := []struct {
		name  string
		appID string
		body  string
	}{
		{"empty id", "", `{}`},
		{"path injection", "620/../x", `{}`},
		{"too long id", strings.Repeat("1", maxAppIDDigits+1), `{}`},
		{"other app", "620", `{"appId":10,"items":[]}`},
		{"broken json", "620", `{"appId":`},
		{"too many items", "620", huge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := achievementsClient(t, http.StatusOK, tc.body, nil)
			if _, err := client.Achievements(context.Background(), tc.appID); err == nil {
				t.Fatal("want an error")
			}
		})
	}
}
