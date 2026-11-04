package account

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func discordErr(t *testing.T, err error, code string) *Error {
	t.Helper()
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != code {
		t.Fatalf("error = %v, want code %q", err, code)
	}
	return apiErr
}

func TestCreateDiscordLinkCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/me/discord/link-code" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("authorization = %q", got)
		}
		writeJSON(t, w, http.StatusOK, map[string]any{"code": "ABCD2345", "expiresAt": "2026-11-05T12:10:00Z"})
	}))
	defer srv.Close()

	got, err := newTestClient(t, srv.URL, tokenOK("tok")).CreateDiscordLinkCode(context.Background())
	if err != nil {
		t.Fatalf("create link code: %v", err)
	}
	if got.Code != "ABCD2345" || !got.ExpiresAt.Equal(time.Date(2026, 11, 5, 12, 10, 0, 0, time.UTC)) {
		t.Fatalf("code = %+v", got)
	}
}

func TestDiscordRoutesMapStatuses(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   any
		code   string
	}{
		{"missing route", http.StatusNotFound, nil, CodeDiscordUnavailable},
		{"missing route with envelope", http.StatusNotFound, map[string]any{"error": map[string]any{"code": "not_found"}}, CodeDiscordUnavailable},
		{"rate limited", http.StatusTooManyRequests, map[string]any{"error": map[string]any{"code": "rate_limited"}}, CodeRateLimited},
		{"rate limited without envelope", http.StatusTooManyRequests, nil, CodeRateLimited},
		{"unauthenticated", http.StatusUnauthorized, nil, CodeUnauthenticated},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tc.body == nil {
					w.WriteHeader(tc.status)
					return
				}
				writeJSON(t, w, tc.status, tc.body)
			}))
			defer srv.Close()
			c := newTestClient(t, srv.URL, tokenOK("tok"))
			ctx := context.Background()

			_, err := c.CreateDiscordLinkCode(ctx)
			discordErr(t, err, tc.code)
			_, err = c.GetDiscordLink(ctx)
			discordErr(t, err, tc.code)
			discordErr(t, c.UnlinkDiscord(ctx), tc.code)
		})
	}
}

func TestDiscordRoutesWithoutSessionAreUnauthenticated(t *testing.T) {
	c := newTestClient(t, "http://127.0.0.1:1", func() (string, error) { return "", ErrNoCredential })
	_, err := c.GetDiscordLink(context.Background())
	discordErr(t, err, CodeUnauthenticated)
}

func TestGetDiscordLinkParsesBothStates(t *testing.T) {
	var reply map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/me/discord" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		writeJSON(t, w, http.StatusOK, reply)
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL, tokenOK("tok"))

	reply = map[string]any{"linked": false}
	got, err := c.GetDiscordLink(context.Background())
	if err != nil || got != (DiscordLink{}) {
		t.Fatalf("unlinked = %+v, %v", got, err)
	}

	reply = map[string]any{"linked": true, "discordId": "123", "discordUsername": "typhon_fan", "linkedAt": "2026-11-05T10:00:00Z"}
	got, err = c.GetDiscordLink(context.Background())
	if err != nil {
		t.Fatalf("linked: %v", err)
	}
	if !got.Linked || got.DiscordID != "123" || got.DiscordUsername != "typhon_fan" || got.LinkedAt.IsZero() {
		t.Fatalf("linked = %+v", got)
	}
}

func TestGetDiscordLinkRejectsGarbage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html>"))
	}))
	defer srv.Close()
	_, err := newTestClient(t, srv.URL, tokenOK("tok")).GetDiscordLink(context.Background())
	discordErr(t, err, CodeServer)
}

func TestUnlinkDiscordSendsDelete(t *testing.T) {
	var method, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	if err := newTestClient(t, srv.URL, tokenOK("tok")).UnlinkDiscord(context.Background()); err != nil {
		t.Fatalf("unlink: %v", err)
	}
	if method != http.MethodDelete || path != "/v1/me/discord" {
		t.Fatalf("request = %s %s", method, path)
	}
}

func TestOpenDiscordInviteUsesSystemBrowser(t *testing.T) {
	srv, _ := quietServer(t)
	s := startedService(t, &fakeStore{}, srv.URL)
	var opened string
	s.openURL = func(u string) error { opened = u; return nil }
	if err := s.OpenDiscordInvite(); err != nil {
		t.Fatalf("open invite: %v", err)
	}
	if opened != "https://discord.gg/xDDFYy6Nj2" {
		t.Fatalf("opened %q", opened)
	}
}
