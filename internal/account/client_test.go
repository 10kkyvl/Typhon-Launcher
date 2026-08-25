package account

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestClient(t *testing.T, baseURL string, token func() (string, error)) *Client {
	t.Helper()
	c, err := NewClient(baseURL, token)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return c
}

func tokenOK(tok string) func() (string, error) {
	return func() (string, error) { return tok, nil }
}

func writeJSON(t *testing.T, w http.ResponseWriter, status int, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}

func TestClientMe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/me" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("unexpected authorization header %q", got)
		}
		writeJSON(t, w, http.StatusOK, CurrentUser{
			ID:          "u1",
			Username:    "egor",
			DisplayName: "Egor",
			Email:       "egor@example.com",
			AvatarURL:   "",
			CreatedAt:   time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL, tokenOK("test-token"))
	user, err := c.Me(context.Background())
	if err != nil {
		t.Fatalf("Me() error = %v", err)
	}
	if user.Username != "egor" || user.ID != "u1" {
		t.Fatalf("unexpected user: %+v", user)
	}
}

func TestClientMeUnauthenticated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusUnauthorized, map[string]any{
			"error": map[string]string{"code": "unauthenticated", "message": "no session"},
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL, tokenOK("bad-token"))
	_, err := c.Me(context.Background())
	var accErr *Error
	if !errors.As(err, &accErr) {
		t.Fatalf("expected *Error, got %v (%T)", err, err)
	}
	if accErr.Code != CodeUnauthenticated {
		t.Fatalf("expected code %q, got %q", CodeUnauthenticated, accErr.Code)
	}
	if accErr.Status != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", accErr.Status)
	}
}

func TestClientUpdateProfileUsernameTaken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusConflict, map[string]any{
			"error": map[string]string{"code": "username_taken", "field": "username", "message": "taken"},
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL, tokenOK("t"))
	username := "egor"
	_, err := c.UpdateProfile(context.Background(), Patch{Username: &username})
	var accErr *Error
	if !errors.As(err, &accErr) {
		t.Fatalf("expected *Error, got %v", err)
	}
	if accErr.Code != CodeUsernameTaken {
		t.Fatalf("expected code %q, got %q", CodeUsernameTaken, accErr.Code)
	}
	if accErr.Field != "username" {
		t.Fatalf("expected field %q, got %q", "username", accErr.Field)
	}
}

func TestClientUpdateProfileInvalidUsername(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusUnprocessableEntity, map[string]any{
			"error": map[string]string{"code": "invalid_username", "field": "username"},
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL, tokenOK("t"))
	username := "!"
	_, err := c.UpdateProfile(context.Background(), Patch{Username: &username})
	var accErr *Error
	if !errors.As(err, &accErr) {
		t.Fatalf("expected *Error, got %v", err)
	}
	if accErr.Code != CodeInvalidUsername {
		t.Fatalf("expected code %q, got %q", CodeInvalidUsername, accErr.Code)
	}
}

func TestClientMalformedErrorEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		if _, err := w.Write([]byte("not json")); err != nil {
			t.Error(err)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL, tokenOK("t"))
	_, err := c.Me(context.Background())
	var accErr *Error
	if !errors.As(err, &accErr) {
		t.Fatalf("expected *Error, got %v", err)
	}
	if accErr.Code != CodeServer {
		t.Fatalf("expected code %q, got %q", CodeServer, accErr.Code)
	}
}

func TestClientErrorWithoutEnvelope(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"proxy rejects the method", http.StatusForbidden, "", CodeBlocked},
		{"proxy rejects with a page", http.StatusForbidden, "<html>403</html>", CodeBlocked},
		{"gateway is down", http.StatusBadGateway, "", CodeServer},
		{"api is broken", http.StatusInternalServerError, "not json", CodeServer},
		{"token is stale", http.StatusUnauthorized, "", CodeUnauthenticated},
		{"budget is spent", http.StatusTooManyRequests, "", CodeRateLimited},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				if _, err := io.WriteString(w, tt.body); err != nil {
					t.Error(err)
				}
			}))
			defer srv.Close()

			name := "newname"
			c := newTestClient(t, srv.URL, tokenOK("t"))
			_, err := c.UpdateProfile(context.Background(), Patch{Username: &name})

			var accErr *Error
			if !errors.As(err, &accErr) {
				t.Fatalf("expected *Error, got %v", err)
			}
			if accErr.Code != tt.want {
				t.Fatalf("code = %q, want %q", accErr.Code, tt.want)
			}
			if accErr.Status != tt.status {
				t.Fatalf("status = %d, want %d", accErr.Status, tt.status)
			}
		})
	}
}

func TestClientUndecodableSuccessBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("not json")); err != nil {
			t.Error(err)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL, tokenOK("t"))
	user, err := c.Me(context.Background())
	if err == nil {
		t.Fatalf("expected error for undecodable body, got user %+v", user)
	}
	if user != (CurrentUser{}) {
		t.Fatalf("expected zero user on error, got %+v", user)
	}
}

func TestClientMissingToken(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL, func() (string, error) { return "", ErrNoCredential })
	_, err := c.Me(context.Background())
	var accErr *Error
	if !errors.As(err, &accErr) {
		t.Fatalf("expected *Error, got %v", err)
	}
	if accErr.Code != CodeUnauthenticated {
		t.Fatalf("expected code %q, got %q", CodeUnauthenticated, accErr.Code)
	}
	if requests != 0 {
		t.Fatalf("expected zero requests, got %d", requests)
	}
}

func TestClientUpdateProfileSendsOnlyNonNilFields(t *testing.T) {
	var received map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(body, &received); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		writeJSON(t, w, http.StatusOK, CurrentUser{ID: "u1"})
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL, tokenOK("t"))
	displayName := "New Name"
	_, err := c.UpdateProfile(context.Background(), Patch{DisplayName: &displayName})
	if err != nil {
		t.Fatalf("UpdateProfile() error = %v", err)
	}
	if _, ok := received["username"]; ok {
		t.Fatalf("expected no username field in body, got %v", received)
	}
	if received["displayName"] != displayName {
		t.Fatalf("expected displayName %q, got %v", displayName, received["displayName"])
	}
}

func TestClientUploadAvatarSendsRawBytes(t *testing.T) {
	payload := []byte{0x89, 'P', 'N', 'G', 1, 2, 3, 4}
	var receivedBody []byte
	var receivedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/me/avatar" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		receivedAuth = r.Header.Get("Authorization")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		receivedBody = body
		writeJSON(t, w, http.StatusOK, CurrentUser{ID: "u1", AvatarURL: "https://cdn/avatar.png"})
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL, tokenOK("upload-token"))
	user, err := c.UploadAvatar(context.Background(), payload)
	if err != nil {
		t.Fatalf("UploadAvatar() error = %v", err)
	}
	if user.AvatarURL != "https://cdn/avatar.png" {
		t.Fatalf("unexpected user: %+v", user)
	}
	if string(receivedBody) != string(payload) {
		t.Fatalf("expected raw payload %v, got %v", payload, receivedBody)
	}
	if receivedAuth != "Bearer upload-token" {
		t.Fatalf("unexpected authorization header %q", receivedAuth)
	}
}

func TestClientUploadAvatarOversizedRejectedLocally(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL, tokenOK("t"))
	oversized := make([]byte, maxAvatarSize+1)
	_, err := c.UploadAvatar(context.Background(), oversized)
	var accErr *Error
	if !errors.As(err, &accErr) {
		t.Fatalf("expected *Error, got %v", err)
	}
	if accErr.Code != CodeAvatarTooLarge {
		t.Fatalf("expected code %q, got %q", CodeAvatarTooLarge, accErr.Code)
	}
	if requests != 0 {
		t.Fatalf("expected zero requests, got %d", requests)
	}
}

func TestClientUploadAvatarEmptyRejectedLocally(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL, tokenOK("t"))
	_, err := c.UploadAvatar(context.Background(), nil)
	var accErr *Error
	if !errors.As(err, &accErr) {
		t.Fatalf("expected *Error, got %v", err)
	}
	if accErr.Code != CodeInvalidAvatar {
		t.Fatalf("expected code %q, got %q", CodeInvalidAvatar, accErr.Code)
	}
	if requests != 0 {
		t.Fatalf("expected zero requests, got %d", requests)
	}
}

func TestClientRemoveAvatarIssuesDelete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/me/avatar" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		writeJSON(t, w, http.StatusOK, CurrentUser{ID: "u1", AvatarURL: ""})
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL, tokenOK("t"))
	user, err := c.RemoveAvatar(context.Background())
	if err != nil {
		t.Fatalf("RemoveAvatar() error = %v", err)
	}
	if user.AvatarURL != "" {
		t.Fatalf("expected empty avatar url, got %q", user.AvatarURL)
	}
}

func TestBaseURL(t *testing.T) {
	orig := apiBaseURL
	t.Cleanup(func() { apiBaseURL = orig })

	apiBaseURL = "http://default.example/"
	t.Setenv("TYPHON_API_URL", "")
	if got := BaseURL(); got != "http://default.example" {
		t.Fatalf("expected trailing slash trimmed, got %q", got)
	}

	t.Setenv("TYPHON_API_URL", "https://override.example/api/")
	if got := BaseURL(); got != "https://override.example/api" {
		t.Fatalf("expected env override, got %q", got)
	}
}

func TestClientMeRateLimited(t *testing.T) {
	cases := []struct {
		name string
		body func(t *testing.T, w http.ResponseWriter)
	}{
		{
			name: "coded envelope",
			body: func(t *testing.T, w http.ResponseWriter) {
				writeJSON(t, w, http.StatusTooManyRequests, map[string]any{
					"error": map[string]string{"code": "rate_limited", "message": "too many requests"},
				})
			},
		},
		{
			name: "bare body from a proxy",
			body: func(_ *testing.T, w http.ResponseWriter) {
				w.WriteHeader(http.StatusTooManyRequests)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				tc.body(t, w)
			}))
			defer srv.Close()

			c := newTestClient(t, srv.URL, tokenOK("t"))
			_, err := c.Me(context.Background())
			var accErr *Error
			if !errors.As(err, &accErr) {
				t.Fatalf("expected *Error, got %v (%T)", err, err)
			}
			if accErr.Code != CodeRateLimited {
				t.Fatalf("expected code %q, got %q", CodeRateLimited, accErr.Code)
			}
			if accErr.Status != http.StatusTooManyRequests {
				t.Fatalf("expected status 429, got %d", accErr.Status)
			}
		})
	}
}
