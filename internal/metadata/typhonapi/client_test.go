package typhonapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"typhon/internal/metadata"
)

func newTestClient(t *testing.T, handler http.Handler, token TokenFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	if token == nil {
		token = func() (string, error) { return "", nil }
	}
	client, err := New(srv.URL, token)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return client
}

func writeJSON(t *testing.T, w http.ResponseWriter, status int, body string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write([]byte(body)); err != nil {
		t.Errorf("write body: %v", err)
	}
}

func TestNewValidatesInput(t *testing.T) {
	cases := []struct {
		name    string
		baseURL string
		token   TokenFunc
	}{
		{"empty url", "", func() (string, error) { return "", nil }},
		{"plain http outside loopback", "http://example.com", func() (string, error) { return "", nil }},
		{"unsupported scheme", "ftp://example.com", func() (string, error) { return "", nil }},
		{"no token source", "https://api.example.com", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.baseURL, tc.token); err == nil {
				t.Fatal("client accepted invalid configuration")
			}
		})
	}
}

func TestSearchMapsCandidates(t *testing.T) {
	var gotPath, gotQuery, gotAuth string
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotAuth = r.Header.Get("Authorization")
		writeJSON(t, w, http.StatusOK, `{"candidates":[
			{"providerId":"2657","title":"Prey","releaseYear":2017,"developer":"Arkane Studios","thumbUrl":"https://images.igdb.com/a.jpg"},
			{"providerId":"7","title":" Prey ","releaseYear":2006},
			{"providerId":"","title":"без id"},
			{"providerId":"9","title":"   "}
		]}`)
	}), func() (string, error) { return "session-token", nil })

	got, err := client.Search(context.Background(), "Prey", 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if gotPath != "/metadata/search" {
		t.Fatalf("path = %q", gotPath)
	}
	if !strings.Contains(gotQuery, "q=Prey") || !strings.Contains(gotQuery, "limit=5") {
		t.Fatalf("query = %q", gotQuery)
	}
	if gotAuth != "Bearer session-token" {
		t.Fatalf("authorization = %q", gotAuth)
	}
	if len(got) != 2 {
		t.Fatalf("candidates = %d, want the two valid ones: %+v", len(got), got)
	}
	if got[0].ProviderID != "2657" || got[0].ReleaseYear != 2017 || got[0].Developer != "Arkane Studios" {
		t.Fatalf("candidate = %+v", got[0])
	}
	if got[0].ThumbURL != "https://images.igdb.com/a.jpg" {
		t.Fatalf("thumb = %q", got[0].ThumbURL)
	}
	if got[1].Title != "Prey" {
		t.Fatalf("title not trimmed: %q", got[1].Title)
	}
}

func TestSearchSendsNoAuthorizationForGuest(t *testing.T) {
	var hadAuth bool
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hadAuth = r.Header["Authorization"]
		writeJSON(t, w, http.StatusOK, `{"candidates":[]}`)
	}), func() (string, error) { return "", nil })

	if _, err := client.Search(context.Background(), "Prey", 0); err != nil {
		t.Fatalf("search: %v", err)
	}
	if hadAuth {
		t.Fatal("guest request carried an Authorization header")
	}
}

func TestSearchClampsLimit(t *testing.T) {
	var gotLimit string
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotLimit = r.URL.Query().Get("limit")
		writeJSON(t, w, http.StatusOK, `{"candidates":[]}`)
	}), nil)

	if _, err := client.Search(context.Background(), "Prey", 500); err != nil {
		t.Fatalf("search: %v", err)
	}
	if gotLimit != "25" {
		t.Fatalf("limit = %q, want it clamped to 25", gotLimit)
	}
}

func TestSearchRejectsEmptyQuery(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("empty query reached the server")
	}), nil)

	if _, err := client.Search(context.Background(), "   ", 5); !errors.Is(err, ErrBadRequest) {
		t.Fatalf("err = %v, want ErrBadRequest", err)
	}
}

func TestGetMapsGame(t *testing.T) {
	var gotPath string
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		writeJSON(t, w, http.StatusOK, `{
			"providerId":"2657",
			"title":"Prey",
			"summary":"Первый абзац.\n\nВторой абзац.",
			"releaseDate":"2017-05-05T00:00:00Z",
			"developer":"Arkane Studios",
			"publisher":"Bethesda Softworks",
			"genres":["Shooter","Adventure"],
			"themes":["Science fiction"],
			"platforms":["PC (Microsoft Windows)"],
			"cover":{"url":"https://images.igdb.com/cover.jpg","width":264,"height":374},
			"screenshots":[
				{"url":"https://images.igdb.com/1.jpg","width":1920,"height":1080},
				{"url":"","width":1,"height":1}
			]
		}`)
	}), nil)

	meta, err := client.Get(context.Background(), "2657")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if gotPath != "/metadata/games/2657" {
		t.Fatalf("path = %q", gotPath)
	}
	if meta.ProviderID != "2657" || meta.Title != "Prey" {
		t.Fatalf("meta = %+v", meta)
	}
	if !strings.Contains(meta.Summary, "\n\n") {
		t.Fatalf("summary paragraphs lost: %q", meta.Summary)
	}
	if meta.ReleaseDate == nil || meta.ReleaseDate.Year() != 2017 {
		t.Fatalf("release date = %v", meta.ReleaseDate)
	}
	if meta.Developer != "Arkane Studios" || meta.Publisher != "Bethesda Softworks" {
		t.Fatalf("companies = %q / %q", meta.Developer, meta.Publisher)
	}
	if len(meta.Genres) != 2 || len(meta.Themes) != 1 || len(meta.Platforms) != 1 {
		t.Fatalf("lists = %+v", meta)
	}
	if meta.Cover == nil || meta.Cover.Width != 264 {
		t.Fatalf("cover = %+v", meta.Cover)
	}
	if len(meta.Screenshots) != 1 {
		t.Fatalf("screenshots = %d, want the one with a url", len(meta.Screenshots))
	}
}

func TestGetWithoutDeveloperStaysEmpty(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, `{"providerId":"1","title":"Игра"}`)
	}), nil)

	meta, err := client.Get(context.Background(), "1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if meta.Developer != "" || meta.Publisher != "" {
		t.Fatalf("invented studio: %q / %q", meta.Developer, meta.Publisher)
	}
}

func TestGetRejectsNonNumericID(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("invalid id reached the server")
	}), nil)

	for _, id := range []string{"", "  ", "abc", "12a", "../../secret", "1 2"} {
		if _, err := client.Get(context.Background(), id); !errors.Is(err, ErrBadRequest) {
			t.Fatalf("id %q: err = %v, want ErrBadRequest", id, err)
		}
	}
}

func TestGetRejectsIncompleteResponse(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, `{"providerId":"1","title":"   "}`)
	}), nil)

	if _, err := client.Get(context.Background(), "1"); !errors.Is(err, ErrUpstream) {
		t.Fatalf("err = %v, want ErrUpstream", err)
	}
}

func TestStatusErrorsMapToDomainErrors(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"provider not configured", http.StatusServiceUnavailable, `{"error":{"code":"metadata_unavailable","message":"x"}}`, metadata.ErrNotConfigured},
		{"game not found", http.StatusNotFound, `{"error":{"code":"not_found","message":"x"}}`, metadata.ErrNoMatch},
		{"rate limited", http.StatusTooManyRequests, `{"error":{"code":"rate_limited","message":"x"}}`, metadata.ErrRateLimited},
		{"bad request", http.StatusBadRequest, `{"error":{"code":"bad_request","message":"x"}}`, ErrBadRequest},
		{"server error", http.StatusInternalServerError, `{"error":{"code":"internal","message":"x"}}`, ErrUpstream},
		{"upstream error", http.StatusBadGateway, `{"error":{"code":"upstream_unavailable","message":"x"}}`, ErrUpstream},
		{"unavailable without code", http.StatusServiceUnavailable, `nonsense`, ErrUpstream},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(t, w, tc.status, tc.body)
			}), nil)

			_, err := client.Get(context.Background(), "1")
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestBrokenBodyIsAnError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, `{"providerId":`)
	}), nil)

	if _, err := client.Get(context.Background(), "1"); !errors.Is(err, ErrUpstream) {
		t.Fatalf("err = %v, want ErrUpstream", err)
	}
}

func TestTokenFailurePropagates(t *testing.T) {
	broken := errors.New("нет доступа к хранилищу")
	client := newTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("request sent despite a token failure")
	}), func() (string, error) { return "", broken })

	if _, err := client.Get(context.Background(), "1"); !errors.Is(err, broken) {
		t.Fatalf("err = %v, want the token error", err)
	}
}

func TestCancelledContext(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, `{"candidates":[]}`)
	}), nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.Search(ctx, "Prey", 5); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestNameIsProvider(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), nil)
	if client.Name() != "igdb" {
		t.Fatalf("name = %q", client.Name())
	}
}

func TestRateLimitCarriesRetryAfter(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "30")
		writeJSON(t, w, http.StatusTooManyRequests, `{"error":{"code":"rate_limited","message":"x"}}`)
	}), nil)

	_, err := client.Get(context.Background(), "1")
	var limit *metadata.RateLimitError
	if !errors.As(err, &limit) {
		t.Fatalf("err = %v, want a RateLimitError", err)
	}
	if limit.RetryAfter != 30*time.Second {
		t.Fatalf("retry after = %v, want 30s", limit.RetryAfter)
	}
	if !errors.Is(err, metadata.ErrRateLimited) {
		t.Fatalf("err = %v, does not match the sentinel", err)
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, time.August, 22, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name  string
		value string
		want  time.Duration
	}{
		{"absent", "", 0},
		{"seconds", "30", 30 * time.Second},
		{"padded seconds", "  45  ", 45 * time.Second},
		{"zero", "0", 0},
		{"negative", "-5", 0},
		{"garbage", "soon", 0},
		{"overflowing seconds", "9999999999999999999", maxRetryAfter},
		{"http date", "Sat, 22 Aug 2026 12:02:00 GMT", 2 * time.Minute},
		{"past http date", "Sat, 22 Aug 2026 11:00:00 GMT", 0},
		{"distant http date", "Sun, 30 Aug 2026 12:00:00 GMT", maxRetryAfter},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseRetryAfter(tc.value, now); got != tc.want {
				t.Fatalf("parseRetryAfter(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

func TestResolveSendsOneRequestForTheWholeBatch(t *testing.T) {
	var gotMethod, gotPath, gotBody, gotType string
	var requests int
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		gotMethod, gotPath, gotType = r.Method, r.URL.Path, r.Header.Get("Content-Type")
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		gotBody = string(raw)
		writeJSON(t, w, http.StatusOK, `{"games":[
			{"title":"Prey","game":{"providerId":"2657","title":"Prey","developer":"Arkane Studios",
			 "cover":{"url":"https://images.igdb.com/cover.jpg","width":264,"height":374},
			 "screenshots":[{"url":"https://images.igdb.com/shot.jpg","width":1920,"height":1080}]}},
			{"title":"Celeste","game":{"providerId":"26226","title":"Celeste"}}
		]}`)
	}), nil)

	resolved, err := client.Resolve(context.Background(), []string{"Prey", " Celeste ", "   "})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want one batched request", requests)
	}
	if gotMethod != http.MethodPost || gotPath != "/metadata/resolve" {
		t.Fatalf("request = %s %s, want POST /metadata/resolve", gotMethod, gotPath)
	}
	if gotType != "application/json" {
		t.Fatalf("content type = %q", gotType)
	}
	if !strings.Contains(gotBody, `"titles":["Prey","Celeste"]`) {
		t.Fatalf("body = %s, want the trimmed titles", gotBody)
	}
	if len(resolved) != 2 {
		t.Fatalf("resolved %d games, want 2", len(resolved))
	}
	if resolved[0].Title != "Prey" || resolved[0].Meta.ProviderID != "2657" {
		t.Fatalf("first resolution = %+v", resolved[0])
	}
	if resolved[0].Meta.Cover == nil || resolved[0].Meta.Cover.URL == "" {
		t.Fatal("cover not mapped")
	}
	if len(resolved[0].Meta.Screenshots) != 1 {
		t.Fatalf("screenshots = %d, want 1", len(resolved[0].Meta.Screenshots))
	}
}

func TestResolveSkipsIncompleteEntries(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, `{"games":[
			{"title":"Broken","game":{"providerId":"","title":""}},
			{"title":"Prey","game":{"providerId":"2657","title":"Prey"}}
		]}`)
	}), nil)

	resolved, err := client.Resolve(context.Background(), []string{"Broken", "Prey"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(resolved) != 1 || resolved[0].Title != "Prey" {
		t.Fatalf("resolved = %+v, want only the complete entry", resolved)
	}
}

func TestResolveRejectsEmptyInput(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("client called the server for an empty batch")
	}), nil)

	for _, titles := range [][]string{nil, {}, {"", "   "}} {
		if _, err := client.Resolve(context.Background(), titles); !errors.Is(err, ErrBadRequest) {
			t.Fatalf("resolve(%v) error = %v, want ErrBadRequest", titles, err)
		}
	}
}

func TestResolveCapsTheBatch(t *testing.T) {
	var gotBody string
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		gotBody = string(raw)
		writeJSON(t, w, http.StatusOK, `{"games":[]}`)
	}), nil)

	titles := make([]string, maxResolve+10)
	for i := range titles {
		titles[i] = "Game " + strconv.Itoa(i)
	}
	if _, err := client.Resolve(context.Background(), titles); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got := strings.Count(gotBody, "Game "); got != maxResolve {
		t.Fatalf("sent %d titles, want the cap of %d", got, maxResolve)
	}
}

func TestResolveMapsRateLimit(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "7")
		writeJSON(t, w, http.StatusTooManyRequests, `{"error":{"code":"rate_limited"}}`)
	}), nil)

	_, err := client.Resolve(context.Background(), []string{"Prey"})
	var limit *metadata.RateLimitError
	if !errors.As(err, &limit) {
		t.Fatalf("resolve error = %v, want RateLimitError", err)
	}
	if limit.RetryAfter != 7*time.Second {
		t.Fatalf("retry after = %v, want 7s", limit.RetryAfter)
	}
}
