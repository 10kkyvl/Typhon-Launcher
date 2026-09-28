package reviews

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"typhon/internal/account"
)

func staticToken(tok string) func() (string, error) {
	return func() (string, error) { return tok, nil }
}

func newTestClient(t *testing.T, handler http.HandlerFunc) *client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := newClient(srv.URL, staticToken("tok"))
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	return c
}

func TestClient_RequestShape(t *testing.T) {
	tests := []struct {
		name       string
		call       func(context.Context, *client) error
		wantMethod string
		wantPath   string
		wantQuery  string
		wantBody   string
		response   string
	}{
		{
			name: "list",
			call: func(ctx context.Context, c *client) error {
				_, err := c.list(ctx, "1942", "helpful", "all", "")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/v1/games/1942/reviews",
			wantQuery:  "filter=all&sort=helpful",
			response:   `{"summary":{"total":0,"positive":0},"reviews":[],"next":""}`,
		},
		{
			name: "list with cursor",
			call: func(ctx context.Context, c *client) error {
				_, err := c.list(ctx, "1942", "recent", "positive", "abc")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/v1/games/1942/reviews",
			wantQuery:  "cursor=abc&filter=positive&sort=recent",
			response:   `{"summary":{"total":0,"positive":0},"reviews":[],"next":""}`,
		},
		{
			name: "mine",
			call: func(ctx context.Context, c *client) error {
				_, err := c.mine(ctx, "1942")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/v1/games/1942/reviews/mine",
			response:   `{"review":null,"eligibility":{"canPost":true,"reason":"","retryAt":null,"playtimeSeconds":0,"requiredPlaytimeSeconds":1800}}`,
		},
		{
			name: "save",
			call: func(ctx context.Context, c *client) error {
				_, err := c.save(ctx, "1942", true, "great game")
				return err
			},
			wantMethod: http.MethodPut,
			wantPath:   "/v1/games/1942/reviews/mine",
			wantBody:   `{"recommended":true,"body":"great game"}`,
			response:   `{"id":1,"gameId":1942}`,
		},
		{
			name: "delete",
			call: func(ctx context.Context, c *client) error {
				return c.delete(ctx, "1942")
			},
			wantMethod: http.MethodDelete,
			wantPath:   "/v1/games/1942/reviews/mine",
		},
		{
			name: "vote",
			call: func(ctx context.Context, c *client) error {
				_, err := c.vote(ctx, 42, "helpful")
				return err
			},
			wantMethod: http.MethodPut,
			wantPath:   "/v1/reviews/42/vote",
			wantBody:   `{"vote":"helpful"}`,
			response:   `{"helpful":1,"unhelpful":0,"myVote":"helpful"}`,
		},
		{
			name: "vote clear",
			call: func(ctx context.Context, c *client) error {
				_, err := c.vote(ctx, 42, "")
				return err
			},
			wantMethod: http.MethodPut,
			wantPath:   "/v1/reviews/42/vote",
			wantBody:   `{"vote":""}`,
			response:   `{"helpful":0,"unhelpful":0,"myVote":""}`,
		},
		{
			name: "report",
			call: func(ctx context.Context, c *client) error {
				return c.report(ctx, 42, "spam")
			},
			wantMethod: http.MethodPost,
			wantPath:   "/v1/reviews/42/report",
			wantBody:   `{"reason":"spam"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var (
				gotMethod string
				gotPath   string
				gotQuery  string
				gotBody   string
			)
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
				gotPath = r.URL.EscapedPath()
				gotQuery = r.URL.RawQuery
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("read body: %v", err)
				}
				gotBody = strings.TrimSpace(string(body))
				if tc.response == "" {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if _, err := io.WriteString(w, tc.response); err != nil {
					t.Errorf("write response: %v", err)
				}
			})

			if err := tc.call(t.Context(), c); err != nil {
				t.Fatalf("call: %v", err)
			}
			if gotMethod != tc.wantMethod {
				t.Errorf("method = %q, want %q", gotMethod, tc.wantMethod)
			}
			if gotPath != tc.wantPath {
				t.Errorf("path = %q, want %q", gotPath, tc.wantPath)
			}
			if gotQuery != tc.wantQuery {
				t.Errorf("query = %q, want %q", gotQuery, tc.wantQuery)
			}
			if gotBody != tc.wantBody {
				t.Errorf("body = %q, want %q", gotBody, tc.wantBody)
			}
		})
	}
}

func TestClient_SendsAuthorizationWhenAuthenticated(t *testing.T) {
	var got http.Header
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.WriteHeader(http.StatusNoContent)
	})
	if err := c.delete(t.Context(), "1942"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if auth := got.Get("Authorization"); auth != "Bearer tok" {
		t.Errorf("Authorization = %q, want %q", auth, "Bearer tok")
	}
	if ua := got.Get("User-Agent"); ua != account.UserAgent {
		t.Errorf("User-Agent = %q, want %q", ua, account.UserAgent)
	}
	if got.Get("Content-Type") != "" {
		t.Error("Content-Type set for a bodyless request")
	}
}

func TestClient_SendsContentTypeWithBody(t *testing.T) {
	var got string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, `{"id":1}`); err != nil {
			t.Errorf("write response: %v", err)
		}
	})
	if _, err := c.save(t.Context(), "1942", true, "gg"); err != nil {
		t.Fatalf("save: %v", err)
	}
	if got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
}

func TestClient_ListGuest(t *testing.T) {
	var gotHeader http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, `{"summary":{"total":0,"positive":0},"reviews":[],"next":""}`); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer srv.Close()
	c, err := newClient(srv.URL, func() (string, error) { return "", account.ErrNoCredential })
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	if _, err := c.list(t.Context(), "1942", "helpful", "all", ""); err != nil {
		t.Fatalf("list: %v", err)
	}
	if _, ok := gotHeader["Authorization"]; ok {
		t.Errorf("Authorization present for a guest request: %q", gotHeader.Get("Authorization"))
	}
}

func TestClient_ListWithOtherTokenErrorFails(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	}))
	defer srv.Close()
	sentinel := errors.New("keyring locked")
	c, err := newClient(srv.URL, func() (string, error) { return "", sentinel })
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	_, err = c.list(t.Context(), "1942", "helpful", "all", "")
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want the token lookup cause", err)
	}
	if errors.Is(err, ErrUnauthorized) {
		t.Fatal("a token lookup failure must not be turned into anonymous or unauthorized")
	}
	if called {
		t.Error("request sent despite a token lookup failure")
	}
}

func TestClient_NonListRequiresAuthorization(t *testing.T) {
	tests := []struct {
		name string
		call func(context.Context, *client) error
	}{
		{"mine", func(ctx context.Context, c *client) error { _, err := c.mine(ctx, "1942"); return err }},
		{"save", func(ctx context.Context, c *client) error { _, err := c.save(ctx, "1942", true, "text"); return err }},
		{"delete", func(ctx context.Context, c *client) error { return c.delete(ctx, "1942") }},
		{"vote", func(ctx context.Context, c *client) error { _, err := c.vote(ctx, 1, "helpful"); return err }},
		{"report", func(ctx context.Context, c *client) error { return c.report(ctx, 1, "spam") }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
				called = true
			}))
			defer srv.Close()
			c, err := newClient(srv.URL, func() (string, error) { return "", account.ErrNoCredential })
			if err != nil {
				t.Fatalf("newClient: %v", err)
			}
			if err := tc.call(t.Context(), c); !errors.Is(err, ErrUnauthorized) {
				t.Fatalf("error = %v, want ErrUnauthorized", err)
			}
			if called {
				t.Errorf("%s: request sent for a guest caller", tc.name)
			}
		})
	}
}

func TestClient_DecodesContractErrorCodes(t *testing.T) {
	tests := []struct {
		name   string
		status int
		code   string
		field  string
	}{
		{"not found", http.StatusNotFound, "review_not_found", ""},
		{"too short", http.StatusBadRequest, "review_too_short", "body"},
		{"too long", http.StatusBadRequest, "review_too_long", "body"},
		{"low effort", http.StatusBadRequest, "review_low_effort", "body"},
		{"links", http.StatusBadRequest, "review_links", "body"},
		{"duplicate", http.StatusConflict, "review_duplicate", "body"},
		{"not played", http.StatusForbidden, "review_not_played", ""},
		{"account too new", http.StatusForbidden, "review_account_too_new", ""},
		{"own review", http.StatusForbidden, "review_own", ""},
		{"post cooldown", http.StatusTooManyRequests, "review_post_cooldown", ""},
		{"daily limit", http.StatusTooManyRequests, "review_daily_limit", ""},
		{"repost cooldown", http.StatusTooManyRequests, "review_repost_cooldown", ""},
		{"edit cooldown", http.StatusTooManyRequests, "review_edit_cooldown", ""},
		{"report limit", http.StatusTooManyRequests, "review_report_limit", ""},
		{"bad reason", http.StatusBadRequest, "review_bad_reason", "reason"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				body := `{"error":{"code":"` + tc.code + `"`
				if tc.field != "" {
					body += `,"field":"` + tc.field + `"`
				}
				body += `}}`
				if _, err := io.WriteString(w, body); err != nil {
					t.Errorf("write response: %v", err)
				}
			})

			err := c.delete(t.Context(), "1942")
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("error = %v (%T), want *APIError", err, err)
			}
			if apiErr.Code != tc.code || apiErr.Field != tc.field || apiErr.Status != tc.status {
				t.Fatalf("APIError = %+v, want code %q field %q status %d", apiErr, tc.code, tc.field, tc.status)
			}
			if apiErr.Error() != tc.code {
				t.Fatalf("Error() = %q, want the bare code %q", apiErr.Error(), tc.code)
			}
		})
	}
}

func TestClient_RetryAfterHeaderDoesNotHideCode(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "42")
		w.WriteHeader(http.StatusTooManyRequests)
		if _, err := io.WriteString(w, `{"error":{"code":"review_post_cooldown"}}`); err != nil {
			t.Errorf("write response: %v", err)
		}
	})
	err := c.delete(t.Context(), "1942")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "review_post_cooldown" || apiErr.Status != http.StatusTooManyRequests {
		t.Fatalf("error = %v (%T), want APIError review_post_cooldown/429", err, err)
	}
}

func TestClient_Unauthorized(t *testing.T) {
	t.Run("status 401", func(t *testing.T) {
		c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			if _, err := io.WriteString(w, `{"error":{"code":"unauthenticated"}}`); err != nil {
				t.Errorf("write response: %v", err)
			}
		})
		if err := c.delete(t.Context(), "1942"); !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("error = %v, want ErrUnauthorized", err)
		}
	})

	t.Run("401 without envelope", func(t *testing.T) {
		c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		})
		if err := c.delete(t.Context(), "1942"); !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("error = %v, want ErrUnauthorized", err)
		}
	})

	t.Run("empty token", func(t *testing.T) {
		called := false
		srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			called = true
		}))
		defer srv.Close()
		c, err := newClient(srv.URL, staticToken(""))
		if err != nil {
			t.Fatalf("newClient: %v", err)
		}
		if err := c.delete(t.Context(), "1942"); !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("error = %v, want ErrUnauthorized", err)
		}
		if called {
			t.Error("request sent without a token")
		}
	})
}

func TestClient_ServerError(t *testing.T) {
	for _, status := range []int{http.StatusInternalServerError, http.StatusBadGateway} {
		c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			if _, err := io.WriteString(w, `{"error":{"code":"internal"}}`); err != nil {
				t.Errorf("write response: %v", err)
			}
		})
		err := c.delete(t.Context(), "1942")
		var srvErr *ServerError
		if !errors.As(err, &srvErr) {
			t.Fatalf("status %d: error = %v (%T), want *ServerError", status, err, err)
		}
		if srvErr.Status != status {
			t.Fatalf("ServerError.Status = %d, want %d", srvErr.Status, status)
		}
	}
}

func TestClient_UnparsableErrorBody(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		if _, err := io.WriteString(w, "not json"); err != nil {
			t.Errorf("write response: %v", err)
		}
	})
	err := c.delete(t.Context(), "1942")
	if err == nil {
		t.Fatal("want an error for an unparsable non-2xx body")
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		t.Fatalf("unparsable body must not become an APIError, got %+v", apiErr)
	}
}

func TestClient_NetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	c, err := newClient(srv.URL, staticToken("tok"))
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	srv.Close()

	err = c.delete(t.Context(), "1942")
	var netErr *NetworkError
	if !errors.As(err, &netErr) {
		t.Fatalf("error = %v (%T), want *NetworkError", err, err)
	}
}

func TestClient_LimitsResponseBody(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, `{"review":{"body":"`+strings.Repeat("a", maxResponseBytes+1024)+`"}}`); err != nil {
			return
		}
	})
	if _, err := c.mine(t.Context(), "1942"); err == nil {
		t.Fatal("want a decode error once the body is cut at the limit")
	}
}

func TestClient_BrokenJSONYieldsError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, `{"review":`); err != nil {
			t.Errorf("write response: %v", err)
		}
	})
	mine, err := c.mine(t.Context(), "1942")
	if err == nil {
		t.Fatal("want a decode error for a truncated body")
	}
	if mine != (Mine{}) {
		t.Fatalf("mine = %+v, want the zero value on error", mine)
	}
}

func TestClient_MineNullReview(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		body := `{"review":null,"eligibility":{"canPost":true,"reason":"","retryAt":null,"playtimeSeconds":0,"requiredPlaytimeSeconds":1800}}`
		if _, err := io.WriteString(w, body); err != nil {
			t.Errorf("write response: %v", err)
		}
	})
	mine, err := c.mine(t.Context(), "1942")
	if err != nil {
		t.Fatalf("mine: %v", err)
	}
	if mine.Review != nil {
		t.Fatalf("review = %+v, want nil", mine.Review)
	}
	if !mine.Eligibility.CanPost {
		t.Error("canPost lost in decode")
	}
	if mine.Eligibility.RetryAt != "" {
		t.Errorf("retryAt = %q, want empty for a null retryAt", mine.Eligibility.RetryAt)
	}
}

func TestClient_MineReviewPresent(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		body := `{"review":{"id":1,"gameId":1942,"recommended":true,"body":"gg","mine":true},` +
			`"eligibility":{"canPost":false,"reason":"review_edit_cooldown","retryAt":"2026-01-02T03:04:05Z"}}`
		if _, err := io.WriteString(w, body); err != nil {
			t.Errorf("write response: %v", err)
		}
	})
	mine, err := c.mine(t.Context(), "1942")
	if err != nil {
		t.Fatalf("mine: %v", err)
	}
	if mine.Review == nil || mine.Review.ID != 1 || !mine.Review.Mine {
		t.Fatalf("review = %+v", mine.Review)
	}
	if mine.Eligibility.RetryAt != "2026-01-02T03:04:05Z" {
		t.Fatalf("retryAt = %q", mine.Eligibility.RetryAt)
	}
}

func TestClient_ListEmptyReviewsIsNonNilSlice(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		if _, err := io.WriteString(w, `{"summary":{"total":0,"positive":0},"next":""}`); err != nil {
			t.Errorf("write response: %v", err)
		}
	})
	page, err := c.list(t.Context(), "1942", "helpful", "all", "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if page.Reviews == nil || len(page.Reviews) != 0 {
		t.Fatalf("reviews = %+v, want a non-nil empty slice", page.Reviews)
	}
}

func TestClient_ListReviewsSurviveDecode(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		body := `{"summary":{"total":1,"positive":1},"reviews":[{"id":1,"gameId":1942,"author":{"id":"u1","username":"alex"},"recommended":true,"body":"gg","helpful":2,"myVote":"helpful","mine":true}],"next":"cursor2"}`
		if _, err := io.WriteString(w, body); err != nil {
			t.Errorf("write response: %v", err)
		}
	})
	page, err := c.list(t.Context(), "1942", "recent", "positive", "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if page.Next != "cursor2" || len(page.Reviews) != 1 {
		t.Fatalf("page = %+v", page)
	}
	r := page.Reviews[0]
	if r.ID != 1 || r.Author.Username != "alex" || r.Helpful != 2 || r.MyVote != "helpful" || !r.Mine {
		t.Fatalf("review = %+v", r)
	}
}

func TestClient_RedirectToAnotherHostDropsAuthorization(t *testing.T) {
	var sawAuth bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization") != ""
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer origin.Close()

	c, err := newClient(origin.URL, staticToken("tok"))
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	if err := c.delete(t.Context(), "1942"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if sawAuth {
		t.Error("Authorization leaked across a redirect to another host")
	}
}

func TestClient_RejectsInsecureBaseURL(t *testing.T) {
	if _, err := newClient("http://example.com", staticToken("tok")); err == nil {
		t.Fatal("want an error for a plain http non-loopback base url")
	}
	if _, err := newClient("", staticToken("tok")); err == nil {
		t.Fatal("want an error for an empty base url")
	}
}

func TestClient_RejectsNilToken(t *testing.T) {
	if _, err := newClient("https://api.example.com", nil); err == nil {
		t.Fatal("want an error for a nil token resolver")
	}
}

func TestModel_ReviewJSONTags(t *testing.T) {
	r := Review{ID: 1, GameID: 1942, Recommended: true, Body: "gg", PlaytimeSeconds: 10, Helpful: 1, Unhelpful: 2, MyVote: "helpful", Mine: true, Hidden: false}
	encoded, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, key := range []string{`"id"`, `"gameId"`, `"recommended"`, `"body"`, `"playtimeSeconds"`, `"helpful"`, `"unhelpful"`, `"myVote"`, `"mine"`, `"hidden"`, `"createdAt"`, `"updatedAt"`} {
		if !strings.Contains(string(encoded), key) {
			t.Errorf("Review json %s misses %s", encoded, key)
		}
	}
}
