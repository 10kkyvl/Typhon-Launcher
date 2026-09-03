package account

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"typhon/internal/app"
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
		if r.Method != http.MethodGet || r.URL.Path != APIPrefix+"/me" {
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
	if !reflect.DeepEqual(user, CurrentUser{}) {
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
		if r.Method != http.MethodPut || r.URL.Path != APIPrefix+"/me/avatar" {
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
		if r.Method != http.MethodDelete || r.URL.Path != APIPrefix+"/me/avatar" {
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

func TestClientEndpointsUseVersionedPathAndHeaders(t *testing.T) {
	tests := []struct {
		name       string
		wantMethod string
		call       func(c *Client) (CurrentUser, error)
	}{
		{
			name:       "Me",
			wantMethod: http.MethodGet,
			call: func(c *Client) (CurrentUser, error) {
				return c.Me(context.Background())
			},
		},
		{
			name:       "UpdateProfile",
			wantMethod: http.MethodPatch,
			call: func(c *Client) (CurrentUser, error) {
				name := "newname"
				return c.UpdateProfile(context.Background(), Patch{Username: &name})
			},
		},
		{
			name:       "UploadAvatar",
			wantMethod: http.MethodPut,
			call: func(c *Client) (CurrentUser, error) {
				return c.UploadAvatar(context.Background(), []byte{1, 2, 3})
			},
		},
		{
			name:       "RemoveAvatar",
			wantMethod: http.MethodDelete,
			call: func(c *Client) (CurrentUser, error) {
				return c.RemoveAvatar(context.Background())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotPath, gotMethod, gotUA, gotVersion string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				gotMethod = r.Method
				gotUA = r.Header.Get("User-Agent")
				gotVersion = r.Header.Get("X-Typhon-Version")
				writeJSON(t, w, http.StatusOK, CurrentUser{ID: "u1"})
			}))
			defer srv.Close()

			c := newTestClient(t, srv.URL, tokenOK("t"))
			if _, err := tt.call(c); err != nil {
				t.Fatalf("%s() error = %v", tt.name, err)
			}
			if gotMethod != tt.wantMethod {
				t.Fatalf("method = %q, want %q", gotMethod, tt.wantMethod)
			}
			if !strings.HasPrefix(gotPath, APIPrefix) {
				t.Fatalf("path = %q, want prefix %q", gotPath, APIPrefix)
			}
			if gotUA != UserAgent {
				t.Fatalf("User-Agent = %q, want %q", gotUA, UserAgent)
			}
			if gotVersion != app.Version {
				t.Fatalf("X-Typhon-Version = %q, want %q", gotVersion, app.Version)
			}
		})
	}
}

func TestClientOutdatedLauncher(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUpgradeRequired)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL, tokenOK("t"))
	_, err := c.Me(context.Background())
	var accErr *Error
	if !errors.As(err, &accErr) {
		t.Fatalf("expected *Error, got %v (%T)", err, err)
	}
	if accErr.Code != CodeOutdated {
		t.Fatalf("expected code %q, got %q", CodeOutdated, accErr.Code)
	}
	if accErr.Status != http.StatusUpgradeRequired {
		t.Fatalf("expected status 426, got %d", accErr.Status)
	}
}

type closeTrackingBody struct {
	io.ReadCloser
	closed *bool
}

func (b closeTrackingBody) Close() error {
	*b.closed = true
	return b.ReadCloser.Close()
}

type closeTrackingTransport struct {
	base   http.RoundTripper
	closed *bool
}

func (t closeTrackingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return resp, err
	}
	resp.Body = closeTrackingBody{resp.Body, t.closed}
	return resp, nil
}

func TestClientFetchAvatarRejectsSchemesBeforeAnyRequest(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"empty url", ""},
		{"unsupported scheme", "ftp://example.com/a.png"},
		{"plain http on a non-loopback host", "http://example.com/a.png"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, "https://account-api.invalid", tokenOK("t"))
			_, err := c.FetchAvatar(context.Background(), tt.url)
			var accErr *Error
			if !errors.As(err, &accErr) {
				t.Fatalf("expected *Error, got %v (%T)", err, err)
			}
			if accErr.Code != CodeBadRequest {
				t.Fatalf("code = %q, want %q", accErr.Code, CodeBadRequest)
			}
		})
	}
}

func TestClientFetchAvatarSuccessSendsNoAuthorization(t *testing.T) {
	payload := append([]byte("\x89PNG\r\n\x1a\n"), []byte{0, 0, 0, 0}...)
	var authorization string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		if _, err := w.Write(payload); err != nil {
			t.Error(err)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, "https://account-api.invalid", tokenOK("t"))
	img, err := c.FetchAvatar(context.Background(), srv.URL+"/a.png")
	if err != nil {
		t.Fatalf("FetchAvatar() error = %v", err)
	}
	if img.MIME != "image/png" {
		t.Fatalf("mime = %q, want image/png", img.MIME)
	}
	if authorization != "" {
		t.Fatalf("authorization = %q, want none for a public avatar request", authorization)
	}
}

func TestClientFetchAvatarNotFoundClosesBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTestClient(t, "https://account-api.invalid", tokenOK("t"))
	closed := false
	c.httpClient.Transport = closeTrackingTransport{base: c.httpClient.Transport, closed: &closed}

	_, err := c.FetchAvatar(context.Background(), srv.URL+"/missing.png")
	var accErr *Error
	if !errors.As(err, &accErr) {
		t.Fatalf("expected *Error, got %v (%T)", err, err)
	}
	if accErr.Status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", accErr.Status)
	}
	if !closed {
		t.Error("response body was not closed")
	}
}

func TestClientFetchAvatarOversizedBodyRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write(bytes.Repeat([]byte{'a'}, maxAvatarSize+1024)); err != nil {
			t.Error(err)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, "https://account-api.invalid", tokenOK("t"))
	_, err := c.FetchAvatar(context.Background(), srv.URL+"/big.png")
	var accErr *Error
	if !errors.As(err, &accErr) {
		t.Fatalf("expected *Error, got %v (%T)", err, err)
	}
	if accErr.Code != CodeAvatarTooLarge {
		t.Fatalf("code = %q, want %q", accErr.Code, CodeAvatarTooLarge)
	}
}

func TestClientFetchAvatarUnsupportedContentRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write([]byte("not an image")); err != nil {
			t.Error(err)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, "https://account-api.invalid", tokenOK("t"))
	_, err := c.FetchAvatar(context.Background(), srv.URL+"/a.png")
	var accErr *Error
	if !errors.As(err, &accErr) {
		t.Fatalf("expected *Error, got %v (%T)", err, err)
	}
	if accErr.Code != CodeUnsupportedAvatar {
		t.Fatalf("code = %q, want %q", accErr.Code, CodeUnsupportedAvatar)
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
