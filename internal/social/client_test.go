package social

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"typhon/internal/account"
	"typhon/internal/app"
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
			name: "friends page",
			call: func(ctx context.Context, c *client) error {
				_, err := c.friendsPage(ctx)
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/v1/me/friends",
			response:   `{"friends":[],"incoming":[],"outgoing":[]}`,
		},
		{
			name: "send request",
			call: func(ctx context.Context, c *client) error {
				_, err := c.sendRequest(ctx, "@alex")
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/v1/me/friends/requests",
			wantBody:   `{"query":"@alex"}`,
			response:   `{"request":{"id":"u1"},"accepted":true}`,
		},
		{
			name: "accept",
			call: func(ctx context.Context, c *client) error {
				return c.accept(ctx, "u 1")
			},
			wantMethod: http.MethodPost,
			wantPath:   "/v1/me/friends/requests/u%201/accept",
		},
		{
			name: "decline",
			call: func(ctx context.Context, c *client) error {
				return c.decline(ctx, "u1")
			},
			wantMethod: http.MethodDelete,
			wantPath:   "/v1/me/friends/requests/u1",
		},
		{
			name: "unfriend",
			call: func(ctx context.Context, c *client) error {
				return c.unfriend(ctx, "u1")
			},
			wantMethod: http.MethodDelete,
			wantPath:   "/v1/me/friends/u1",
		},
		{
			name: "block",
			call: func(ctx context.Context, c *client) error {
				return c.block(ctx, "u1")
			},
			wantMethod: http.MethodPost,
			wantPath:   "/v1/me/blocks/u1",
		},
		{
			name: "unblock",
			call: func(ctx context.Context, c *client) error {
				return c.unblock(ctx, "u1")
			},
			wantMethod: http.MethodDelete,
			wantPath:   "/v1/me/blocks/u1",
		},
		{
			name: "blocks",
			call: func(ctx context.Context, c *client) error {
				_, err := c.blocks(ctx)
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/v1/me/blocks",
			response:   `{"blocks":[{"id":"u1"}]}`,
		},
		{
			name: "friend code",
			call: func(ctx context.Context, c *client) error {
				_, err := c.friendCode(ctx)
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/v1/me/friend-code",
			response:   `{"code":"TY-84K2-91FC"}`,
		},
		{
			name: "rotate friend code",
			call: func(ctx context.Context, c *client) error {
				_, err := c.rotateFriendCode(ctx)
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/v1/me/friend-code/rotate",
			response:   `{"code":"TY-0000-0000"}`,
		},
		{
			name: "profile",
			call: func(ctx context.Context, c *client) error {
				_, err := c.profile(ctx, "alex nova")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/v1/users/alex%20nova",
			response:   `{"id":"u1","username":"alex"}`,
		},
		{
			name: "profile by code",
			call: func(ctx context.Context, c *client) error {
				_, err := c.profileByCode(ctx, "TY-84K2-91FC")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/v1/users/by-code/TY-84K2-91FC",
			response:   `{"id":"u1"}`,
		},
		{
			name: "user games without cursor",
			call: func(ctx context.Context, c *client) error {
				_, err := c.userGames(ctx, "alex", "")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/v1/users/alex/games",
			response:   `{"games":[],"next":""}`,
		},
		{
			name: "user games with cursor",
			call: func(ctx context.Context, c *client) error {
				_, err := c.userGames(ctx, "alex", "1942")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/v1/users/alex/games",
			wantQuery:  "cursor=1942",
			response:   `{"games":[],"next":""}`,
		},
		{
			name: "game friends",
			call: func(ctx context.Context, c *client) error {
				_, err := c.gameFriends(ctx, "1942")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/v1/games/1942/friends",
			response:   `{"played":[],"playingNow":[]}`,
		},
		{
			name: "feed",
			call: func(ctx context.Context, c *client) error {
				_, err := c.feed(ctx, 0, 30)
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/v1/me/feed",
			wantQuery:  "cursor=0&limit=30",
			response:   `{"events":[],"next":0}`,
		},
		{
			name: "feed with cursor",
			call: func(ctx context.Context, c *client) error {
				_, err := c.feed(ctx, 1942, 30)
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/v1/me/feed",
			wantQuery:  "cursor=1942&limit=30",
			response:   `{"events":[],"next":0}`,
		},
		{
			name: "react",
			call: func(ctx context.Context, c *client) error {
				return c.react(ctx, 1942, "fire")
			},
			wantMethod: http.MethodPut,
			wantPath:   "/v1/activity/1942/reactions/fire",
		},
		{
			name: "unreact",
			call: func(ctx context.Context, c *client) error {
				return c.unreact(ctx, 1942, "fire")
			},
			wantMethod: http.MethodDelete,
			wantPath:   "/v1/activity/1942/reactions/fire",
		},
		{
			name: "set note",
			call: func(ctx context.Context, c *client) error {
				return c.setNote(ctx, 1942, "gg wp")
			},
			wantMethod: http.MethodPut,
			wantPath:   "/v1/activity/1942/note",
			wantBody:   `{"note":"gg wp"}`,
		},
		{
			name: "set note clears with empty string",
			call: func(ctx context.Context, c *client) error {
				return c.setNote(ctx, 1942, "")
			},
			wantMethod: http.MethodPut,
			wantPath:   "/v1/activity/1942/note",
			wantBody:   `{"note":""}`,
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

func TestClient_SendsHeaders(t *testing.T) {
	var got http.Header
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.WriteHeader(http.StatusNoContent)
	})

	if err := c.accept(t.Context(), "u1"); err != nil {
		t.Fatalf("accept: %v", err)
	}
	if auth := got.Get("Authorization"); auth != "Bearer tok" {
		t.Errorf("Authorization = %q, want %q", auth, "Bearer tok")
	}
	if ua := got.Get("User-Agent"); ua != account.UserAgent {
		t.Errorf("User-Agent = %q, want %q", ua, account.UserAgent)
	}
	if v := got.Get("X-Typhon-Version"); v != app.Version {
		t.Errorf("X-Typhon-Version = %q, want %q", v, app.Version)
	}
	if ct := got.Get("Content-Type"); ct != "" {
		t.Errorf("Content-Type = %q, want empty for a bodyless request", ct)
	}
}

func TestClient_SendsContentTypeWithBody(t *testing.T) {
	var got string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, `{"request":{},"accepted":false}`); err != nil {
			t.Errorf("write response: %v", err)
		}
	})

	if _, err := c.sendRequest(t.Context(), "alex"); err != nil {
		t.Fatalf("sendRequest: %v", err)
	}
	if got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
}

func TestClient_DecodesResponses(t *testing.T) {
	t.Run("friends page", func(t *testing.T) {
		c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			if _, err := io.WriteString(w, `{"friends":[{"id":"u1","username":"alex","displayName":"Alex","avatarUrl":"https://a/1.png","since":"2026-01-02T03:04:05Z"}],"incoming":[{"id":"u2","mutualCount":3,"commonCount":4,"createdAt":"2026-01-02T03:04:05Z"}],"outgoing":[]}`); err != nil {
				t.Errorf("write response: %v", err)
			}
		})
		page, err := c.friendsPage(t.Context())
		if err != nil {
			t.Fatalf("friendsPage: %v", err)
		}
		if len(page.Friends) != 1 || page.Friends[0].Username != "alex" || page.Friends[0].AvatarURL != "https://a/1.png" {
			t.Fatalf("friends = %+v", page.Friends)
		}
		if page.Friends[0].Since.IsZero() {
			t.Error("since not decoded")
		}
		if len(page.Incoming) != 1 || page.Incoming[0].MutualCount != 3 || page.Incoming[0].CommonCount != 4 {
			t.Fatalf("incoming = %+v", page.Incoming)
		}
	})

	t.Run("send result", func(t *testing.T) {
		c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			if _, err := io.WriteString(w, `{"request":{"id":"u2","username":"nova"},"accepted":true}`); err != nil {
				t.Errorf("write response: %v", err)
			}
		})
		res, err := c.sendRequest(t.Context(), "nova")
		if err != nil {
			t.Fatalf("sendRequest: %v", err)
		}
		if !res.Accepted || res.Request.Username != "nova" {
			t.Fatalf("result = %+v", res)
		}
	})

	t.Run("friend code", func(t *testing.T) {
		c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			if _, err := io.WriteString(w, `{"code":"TY-84K2-91FC"}`); err != nil {
				t.Errorf("write response: %v", err)
			}
		})
		code, err := c.friendCode(t.Context())
		if err != nil {
			t.Fatalf("friendCode: %v", err)
		}
		if code != "TY-84K2-91FC" {
			t.Fatalf("code = %q", code)
		}
	})

	t.Run("blocks", func(t *testing.T) {
		c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			if _, err := io.WriteString(w, `{"blocks":[{"id":"u1","username":"alex"}]}`); err != nil {
				t.Errorf("write response: %v", err)
			}
		})
		cards, err := c.blocks(t.Context())
		if err != nil {
			t.Fatalf("blocks: %v", err)
		}
		if len(cards) != 1 || cards[0].Username != "alex" {
			t.Fatalf("blocks = %+v", cards)
		}
	})

	t.Run("game friends playtime", func(t *testing.T) {
		c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			if _, err := io.WriteString(w, `{"played":[{"id":"u1","playtimeSeconds":7200,"status":"playing"}],"playingNow":[]}`); err != nil {
				t.Errorf("write response: %v", err)
			}
		})
		res, err := c.gameFriends(t.Context(), "1942")
		if err != nil {
			t.Fatalf("gameFriends: %v", err)
		}
		if len(res.Played) != 1 || res.Played[0].PlaytimeSeconds == nil || *res.Played[0].PlaytimeSeconds != 7200 {
			t.Fatalf("played = %+v", res.Played)
		}
	})

	t.Run("feed", func(t *testing.T) {
		c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			body := `{"events":[{"id":1,"user":{"id":"u1","username":"alex"},"kind":"completed",` +
				`"game":{"igdbId":1942,"title":"The Witcher 3"},"createdAt":"2026-01-02T03:04:05Z",` +
				`"reactions":[{"emoji":"fire","count":2}]}],"next":7}`
			if _, err := io.WriteString(w, body); err != nil {
				t.Errorf("write response: %v", err)
			}
		})
		page, err := c.feed(t.Context(), 0, 30)
		if err != nil {
			t.Fatalf("feed: %v", err)
		}
		if page.Next != 7 || len(page.Events) != 1 {
			t.Fatalf("page = %+v", page)
		}
		ev := page.Events[0]
		if ev.ID != 1 || ev.User.Username != "alex" || ev.Kind != "completed" || ev.Game.IGDBID != 1942 {
			t.Fatalf("event = %+v", ev)
		}
		if ev.Mine == nil || len(ev.Mine) != 0 {
			t.Fatalf("mine = %+v, want a non-nil empty slice when the field is absent", ev.Mine)
		}
		if len(ev.Reactions) != 1 || ev.Reactions[0].Emoji != "fire" || ev.Reactions[0].Count != 2 {
			t.Fatalf("reactions = %+v", ev.Reactions)
		}
	})

	t.Run("feed reactions absent", func(t *testing.T) {
		c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			body := `{"events":[{"id":1,"user":{"id":"u1","username":"alex"},"kind":"started",` +
				`"game":{"igdbId":1942,"title":"The Witcher 3"},"createdAt":"2026-01-02T03:04:05Z","mine":["fire"]}],"next":0}`
			if _, err := io.WriteString(w, body); err != nil {
				t.Errorf("write response: %v", err)
			}
		})
		page, err := c.feed(t.Context(), 0, 30)
		if err != nil {
			t.Fatalf("feed: %v", err)
		}
		if len(page.Events) != 1 {
			t.Fatalf("page = %+v", page)
		}
		if ev := page.Events[0]; ev.Reactions == nil || len(ev.Reactions) != 0 {
			t.Fatalf("reactions = %+v, want a non-nil empty slice when the field is absent", ev.Reactions)
		}
	})

	t.Run("feed events absent", func(t *testing.T) {
		c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			if _, err := io.WriteString(w, `{"next":0}`); err != nil {
				t.Errorf("write response: %v", err)
			}
		})
		page, err := c.feed(t.Context(), 0, 30)
		if err != nil {
			t.Fatalf("feed: %v", err)
		}
		if page.Events == nil || len(page.Events) != 0 {
			t.Fatalf("events = %+v, want a non-nil empty slice", page.Events)
		}
	})

	t.Run("profile recent activity absent", func(t *testing.T) {
		c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			if _, err := io.WriteString(w, `{"id":"u1","username":"alex"}`); err != nil {
				t.Errorf("write response: %v", err)
			}
		})
		profile, err := c.profile(t.Context(), "alex")
		if err != nil {
			t.Fatalf("profile: %v", err)
		}
		if profile.RecentActivity == nil || len(profile.RecentActivity) != 0 {
			t.Fatalf("recentActivity = %+v, want a non-nil empty slice", profile.RecentActivity)
		}
	})

	t.Run("profile recent activity present", func(t *testing.T) {
		c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			body := `{"id":"u1","recentActivity":[{"id":1,"kind":"started",` +
				`"game":{"igdbId":1942,"title":"The Witcher 3"},"createdAt":"2026-01-02T03:04:05Z"}]}`
			if _, err := io.WriteString(w, body); err != nil {
				t.Errorf("write response: %v", err)
			}
		})
		profile, err := c.profileByCode(t.Context(), "TY-84K2-91FC")
		if err != nil {
			t.Fatalf("profileByCode: %v", err)
		}
		if len(profile.RecentActivity) != 1 || profile.RecentActivity[0].Kind != "started" {
			t.Fatalf("recentActivity = %+v", profile.RecentActivity)
		}
	})
}

func TestClient_DecodesErrorEnvelope(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		code   string
		field  string
	}{
		{name: "already friends", status: http.StatusConflict, body: `{"error":{"code":"already_friends","message":"..."}}`, code: "already_friends"},
		{name: "blocked", status: http.StatusForbidden, body: `{"error":{"code":"friend_blocked"}}`, code: "friend_blocked"},
		{name: "self with field", status: http.StatusUnprocessableEntity, body: `{"error":{"code":"friend_self","field":"query"}}`, code: "friend_self", field: "query"},
		{name: "not found", status: http.StatusNotFound, body: `{"error":{"code":"user_not_found"}}`, code: "user_not_found"},
		{name: "limit", status: http.StatusRequestEntityTooLarge, body: `{"error":{"code":"friend_limit"}}`, code: "friend_limit"},
		{name: "activity not found", status: http.StatusNotFound, body: `{"error":{"code":"activity_not_found"}}`, code: "activity_not_found"},
		{name: "reaction invalid", status: http.StatusUnprocessableEntity, body: `{"error":{"code":"reaction_invalid","field":"emoji"}}`, code: "reaction_invalid", field: "emoji"},
		{name: "note too long", status: http.StatusUnprocessableEntity, body: `{"error":{"code":"note_too_long"}}`, code: "note_too_long"},
		{name: "bad request", status: http.StatusBadRequest, body: `{"error":{"code":"bad_request"}}`, code: "bad_request"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				if _, err := io.WriteString(w, tc.body); err != nil {
					t.Errorf("write response: %v", err)
				}
			})

			err := c.accept(t.Context(), "u1")
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

func TestClient_SetNoteErrorStatus(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		if _, err := io.WriteString(w, `{"error":{"code":"activity_not_found"}}`); err != nil {
			t.Errorf("write response: %v", err)
		}
	})
	err := c.setNote(t.Context(), 1942, "gg")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "activity_not_found" {
		t.Fatalf("setNote error = %v (%T), want APIError activity_not_found", err, err)
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
		if err := c.accept(t.Context(), "u1"); !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("error = %v, want ErrUnauthorized", err)
		}
	})

	t.Run("401 without envelope", func(t *testing.T) {
		c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		})
		if err := c.accept(t.Context(), "u1"); !errors.Is(err, ErrUnauthorized) {
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
		if err := c.accept(t.Context(), "u1"); !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("error = %v, want ErrUnauthorized", err)
		}
		if called {
			t.Error("request sent without a token")
		}
	})

	t.Run("no credential", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
		defer srv.Close()
		c, err := newClient(srv.URL, func() (string, error) { return "", account.ErrNoCredential })
		if err != nil {
			t.Fatalf("newClient: %v", err)
		}
		if err := c.accept(t.Context(), "u1"); !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("error = %v, want ErrUnauthorized", err)
		}
	})

	t.Run("token lookup failure is not unauthorized", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
		defer srv.Close()
		sentinel := errors.New("keyring locked")
		c, err := newClient(srv.URL, func() (string, error) { return "", sentinel })
		if err != nil {
			t.Fatalf("newClient: %v", err)
		}
		err = c.accept(t.Context(), "u1")
		if !errors.Is(err, sentinel) {
			t.Fatalf("error = %v, want the token lookup cause", err)
		}
		if errors.Is(err, ErrUnauthorized) {
			t.Fatal("a token lookup failure must not read as unauthorized")
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
		err := c.accept(t.Context(), "u1")
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
	err := c.accept(t.Context(), "u1")
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

	err = c.accept(t.Context(), "u1")
	var netErr *NetworkError
	if !errors.As(err, &netErr) {
		t.Fatalf("error = %v (%T), want *NetworkError", err, err)
	}
}

func TestClient_LimitsResponseBody(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, `{"bio":"`+strings.Repeat("a", maxResponseBytes+1024)+`"}`); err != nil {
			return
		}
	})

	if _, err := c.profile(t.Context(), "alex"); err == nil {
		t.Fatal("want a decode error once the body is cut at the limit")
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

func TestClient_UserGamesDecodesNext(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		if _, err := io.WriteString(w, `{"games":[{"igdbId":1942,"title":"The Witcher 3","coverUrl":"https://c/1.jpg","playtimeSeconds":3600,"status":"completed","favorite":true,"lastPlayedAt":"2026-01-02T03:04:05Z"}],"next":"1942"}`); err != nil {
			t.Errorf("write response: %v", err)
		}
	})

	page, err := c.userGames(t.Context(), "alex", "")
	if err != nil {
		t.Fatalf("userGames: %v", err)
	}
	if page.Next != "1942" || len(page.Games) != 1 {
		t.Fatalf("page = %+v", page)
	}
	g := page.Games[0]
	if g.IGDBID != 1942 || g.Title != "The Witcher 3" || !g.Favorite || g.Status != "completed" {
		t.Fatalf("game = %+v", g)
	}
	if g.PlaytimeSeconds == nil || *g.PlaytimeSeconds != 3600 {
		t.Fatalf("playtime = %v", g.PlaytimeSeconds)
	}
	if g.LastPlayedAt == nil {
		t.Fatal("lastPlayedAt not decoded")
	}
}

func TestModel_JSONTags(t *testing.T) {
	encoded, err := json.Marshal(FriendView{UserCard: UserCard{ID: "u1", Username: "alex", DisplayName: "Alex", AvatarURL: "https://a/1.png"}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, key := range []string{`"id"`, `"username"`, `"displayName"`, `"avatarUrl"`, `"since"`} {
		if !strings.Contains(string(encoded), key) {
			t.Errorf("FriendView json %s misses %s", encoded, key)
		}
	}
	if strings.Contains(string(encoded), "UserCard") {
		t.Errorf("FriendView json %s must flatten the embedded card", encoded)
	}
	if strings.Contains(string(encoded), "presence") {
		t.Errorf("FriendView json %s must omit a nil presence", encoded)
	}

	bare, err := json.Marshal(PresenceView{Status: "offline"})
	if err != nil {
		t.Fatalf("marshal presence: %v", err)
	}
	if string(bare) != `{"status":"offline"}` {
		t.Errorf("bare PresenceView json = %s, want only the status", bare)
	}

	gameID := int64(1942)
	since := time.Date(2025, 3, 1, 10, 0, 0, 0, time.UTC)
	full, err := json.Marshal(PresenceView{Status: "online", GameID: &gameID, GameTitle: "The Witcher 3", Since: &since, LastSeenAt: &since})
	if err != nil {
		t.Fatalf("marshal presence: %v", err)
	}
	for _, key := range []string{`"status"`, `"gameId":1942`, `"gameTitle"`, `"since"`, `"lastSeenAt"`} {
		if !strings.Contains(string(full), key) {
			t.Errorf("PresenceView json %s misses %s", full, key)
		}
	}
}

func TestPresenceView_Decode(t *testing.T) {
	var view PresenceView
	if err := json.Unmarshal([]byte(`{"status":"online","gameId":1942,"gameTitle":"The Witcher 3","lastSeenAt":"2025-03-01T10:00:00Z"}`), &view); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if view.Status != "online" || view.GameTitle != "The Witcher 3" {
		t.Fatalf("view = %+v", view)
	}
	if view.GameID == nil || *view.GameID != 1942 {
		t.Fatalf("gameId = %v", view.GameID)
	}
	if view.Since != nil {
		t.Fatalf("since = %v, want nil", view.Since)
	}
	if view.LastSeenAt == nil || !view.LastSeenAt.Equal(time.Date(2025, 3, 1, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("lastSeenAt = %v", view.LastSeenAt)
	}
}
