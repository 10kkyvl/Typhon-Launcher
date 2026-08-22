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

func TestClientRegisterAndLogin(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		status int
		call   func(c *Client) (Session, error)
		want   map[string]any
	}{
		{
			name:   "register",
			path:   "/auth/register",
			status: http.StatusCreated,
			call: func(c *Client) (Session, error) {
				return c.Register(context.Background(), RegisterInput{
					Email:       "  User@Example.COM ",
					Username:    "PlayerOne",
					DisplayName: "Player One",
					Password:    "  secret pass  ",
				})
			},
			want: map[string]any{
				"email":       "  User@Example.COM ",
				"username":    "PlayerOne",
				"displayName": "Player One",
				"password":    "  secret pass  ",
			},
		},
		{
			name:   "login",
			path:   "/auth/login",
			status: http.StatusOK,
			call: func(c *Client) (Session, error) {
				return c.Login(context.Background(), LoginInput{Identifier: "PLAYERONE", Password: "  secret pass  "})
			},
			want: map[string]any{
				"emailOrUsername": "PLAYERONE",
				"password":        "  secret pass  ",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var received map[string]any
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != tt.path {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				if got := r.Header.Get("Authorization"); got != "" {
					t.Errorf("authorization = %q, want no header", got)
				}
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatalf("read body: %v", err)
				}
				if err := json.Unmarshal(body, &received); err != nil {
					t.Fatalf("unmarshal body: %v", err)
				}
				writeJSON(t, w, tt.status, Session{
					User:      CurrentUser{ID: "u1", Username: "playerone"},
					Token:     "fresh-token",
					ExpiresAt: time.Now().Add(time.Hour),
				})
			}))
			defer srv.Close()

			session, err := tt.call(newTestClient(t, srv.URL, tokenOK("unused")))
			if err != nil {
				t.Fatalf("%s error = %v", tt.name, err)
			}
			if session.Token != "fresh-token" {
				t.Errorf("token = %q, want fresh-token", session.Token)
			}
			if session.User.Username != "playerone" {
				t.Errorf("user = %+v, want playerone", session.User)
			}
			for key, want := range tt.want {
				if received[key] != want {
					t.Errorf("body[%q] = %v, want %v", key, received[key], want)
				}
			}
		})
	}
}

func TestClientLoginInvalidCredentialsKeepsItsCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusUnauthorized, map[string]any{
			"error": map[string]string{"code": "invalid_credentials", "message": "nope"},
		})
	}))
	defer srv.Close()

	_, err := newTestClient(t, srv.URL, tokenOK("unused")).
		Login(context.Background(), LoginInput{Identifier: "playerone", Password: "wrong"})

	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *Error, got %v (%T)", err, err)
	}
	if apiErr.Code != CodeInvalidLogin {
		t.Fatalf("code = %q, want %q", apiErr.Code, CodeInvalidLogin)
	}
}

func TestClientUnauthorizedWithoutCodeFallsBackToUnauthenticated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := newTestClient(t, srv.URL, tokenOK("tok")).Me(context.Background())

	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *Error, got %v (%T)", err, err)
	}
	if apiErr.Code != CodeUnauthenticated {
		t.Fatalf("code = %q, want %q", apiErr.Code, CodeUnauthenticated)
	}
}

func TestClientRegisterRejectsEmptyToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusCreated, map[string]any{"user": CurrentUser{ID: "u1"}, "token": ""})
	}))
	defer srv.Close()

	_, err := newTestClient(t, srv.URL, tokenOK("unused")).
		Register(context.Background(), RegisterInput{Email: "a@b.c", Username: "player", DisplayName: "P", Password: "password"})
	if err == nil {
		t.Fatal("Register() error = nil, want an error for an empty token")
	}
}

func TestClientLogoutSendsTheGivenToken(t *testing.T) {
	var authorization string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/auth/logout" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		authorization = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	if err := newTestClient(t, srv.URL, tokenOK("stored")).Logout(context.Background(), "explicit-token"); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if authorization != "Bearer explicit-token" {
		t.Errorf("authorization = %q, want the explicitly passed token", authorization)
	}
}
