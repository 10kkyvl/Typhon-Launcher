package support

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"typhon/internal/account"
	"typhon/internal/app"
)

type seen struct {
	method string
	path   string
	query  string
	header http.Header
	body   []byte
}

type fakeAPI struct {
	t      *testing.T
	server *httptest.Server

	mu    sync.Mutex
	calls []seen
}

func newFakeAPI(t *testing.T, handler http.HandlerFunc) *fakeAPI {
	t.Helper()
	api := &fakeAPI{t: t}
	api.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		api.mu.Lock()
		api.calls = append(api.calls, seen{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Clone(), body})
		api.mu.Unlock()
		handler(w, r)
	}))
	t.Cleanup(api.server.Close)
	return api
}

func (a *fakeAPI) requests() []seen {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]seen(nil), a.calls...)
}

func (a *fakeAPI) only() seen {
	a.t.Helper()
	calls := a.requests()
	if len(calls) != 1 {
		a.t.Fatalf("got %d requests, want 1: %+v", len(calls), calls)
	}
	return calls[0]
}

func reply(t *testing.T, w http.ResponseWriter, status int, body string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := io.WriteString(w, body); err != nil {
		t.Errorf("write response: %v", err)
	}
}

func staticToken(tok string) func() (string, error) {
	return func() (string, error) { return tok, nil }
}

func newTestClient(t *testing.T, base string, token func() (string, error)) *Client {
	t.Helper()
	c, err := NewClient(base, token)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return c
}

func apiCode(t *testing.T, err error) *account.Error {
	t.Helper()
	var apiErr *account.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error %v is not an *account.Error", err)
	}
	return apiErr
}

const ticketJSON = `{"number":1042,"ref":"T-1042","topic":"install","subject":"Won't install","status":"open","gameIgdbId":1942,` +
	`"client":{"appVersion":"0.9.0","os":"windows","arch":"amd64"},"unread":true,` +
	`"createdAt":"2026-10-01T10:00:00Z","updatedAt":"2026-10-02T11:00:00Z","resolvedAt":null}`

const messageJSON = `{"id":7,"author":"staff","body":"Try again","logBundleId":"ABCD1234","createdAt":"2026-10-02T11:00:00Z"}`

func TestClientCalls(t *testing.T) {
	cases := []struct {
		name       string
		call       func(c *Client) (any, error)
		reply      string
		status     int
		method     string
		path       string
		query      string
		wantBody   string
		check      func(t *testing.T, got any)
		wantNoBody bool
	}{
		{
			name:   "create",
			status: http.StatusCreated,
			reply:  `{"ticket":` + ticketJSON + `}`,
			method: http.MethodPost,
			path:   "/v1/support/tickets",
			call: func(c *Client) (any, error) {
				game := int64(1942)
				return c.Create(context.Background(), createRequest{
					Topic: "install", Subject: "Won't install", Body: "details", GameIgdbID: &game,
					LogBundleID: "ABCD1234", Client: &ClientInfo{AppVersion: "0.9.0", OS: "windows", Arch: "amd64"}, Lang: "en",
				})
			},
			wantBody: `{"topic":"install","subject":"Won't install","body":"details","gameIgdbId":1942,"logBundleId":"ABCD1234",` +
				`"client":{"appVersion":"0.9.0","os":"windows","arch":"amd64"},"lang":"en"}`,
			check: func(t *testing.T, got any) {
				ticket := as[Ticket](t, got)
				if ticket.Number != 1042 || ticket.Ref != "T-1042" || !ticket.Unread || ticket.GameIgdbID == nil || *ticket.GameIgdbID != 1942 {
					t.Fatalf("ticket = %+v", ticket)
				}
				if ticket.Client == nil || ticket.Client.OS != "windows" || ticket.ResolvedAt != nil {
					t.Fatalf("ticket client/resolved = %+v", ticket)
				}
			},
		},
		{
			name:   "create omits what was not given",
			status: http.StatusCreated,
			reply:  `{"ticket":` + ticketJSON + `}`,
			method: http.MethodPost,
			path:   "/v1/support/tickets",
			call: func(c *Client) (any, error) {
				return c.Create(context.Background(), createRequest{Topic: "other", Subject: "s", Body: "b"})
			},
			wantBody: `{"topic":"other","subject":"s","body":"b"}`,
		},
		{
			name:       "list first page",
			status:     http.StatusOK,
			reply:      `{"tickets":[` + ticketJSON + `],"next":"cur2"}`,
			method:     http.MethodGet,
			path:       "/v1/support/tickets",
			wantNoBody: true,
			call:       func(c *Client) (any, error) { return c.List(context.Background(), "") },
			check: func(t *testing.T, got any) {
				page := as[Page](t, got)
				if len(page.Tickets) != 1 || page.Next != "cur2" {
					t.Fatalf("page = %+v", page)
				}
			},
		},
		{
			name:       "list with cursor is escaped",
			status:     http.StatusOK,
			reply:      `{"tickets":null,"next":""}`,
			method:     http.MethodGet,
			path:       "/v1/support/tickets",
			query:      "cursor=a%2Bb%26c%3Dd",
			wantNoBody: true,
			call:       func(c *Client) (any, error) { return c.List(context.Background(), "a+b&c=d") },
			check: func(t *testing.T, got any) {
				page := as[Page](t, got)
				if page.Tickets == nil || len(page.Tickets) != 0 {
					t.Fatalf("tickets = %#v, want an empty non-nil slice", page.Tickets)
				}
			},
		},
		{
			name:       "get",
			status:     http.StatusOK,
			reply:      `{"ticket":` + ticketJSON + `,"messages":[` + messageJSON + `]}`,
			method:     http.MethodGet,
			path:       "/v1/support/tickets/1042",
			wantNoBody: true,
			call:       func(c *Client) (any, error) { return c.Get(context.Background(), 1042) },
			check: func(t *testing.T, got any) {
				detail := as[Detail](t, got)
				if detail.Ticket.Number != 1042 || len(detail.Messages) != 1 {
					t.Fatalf("detail = %+v", detail)
				}
				msg := detail.Messages[0]
				if msg.ID != 7 || msg.Author != "staff" || msg.LogBundleID == nil || *msg.LogBundleID != "ABCD1234" {
					t.Fatalf("message = %+v", msg)
				}
			},
		},
		{
			name:       "get without messages",
			status:     http.StatusOK,
			reply:      `{"ticket":` + ticketJSON + `,"messages":null}`,
			method:     http.MethodGet,
			path:       "/v1/support/tickets/1042",
			wantNoBody: true,
			call:       func(c *Client) (any, error) { return c.Get(context.Background(), 1042) },
			check: func(t *testing.T, got any) {
				if detail := as[Detail](t, got); detail.Messages == nil {
					t.Fatal("messages are nil, want an empty slice")
				}
			},
		},
		{
			name:     "reply",
			status:   http.StatusCreated,
			reply:    `{"message":` + messageJSON + `}`,
			method:   http.MethodPost,
			path:     "/v1/support/tickets/1042/messages",
			wantBody: `{"body":"thanks","logBundleId":"ABCD1234"}`,
			call:     func(c *Client) (any, error) { return c.Reply(context.Background(), 1042, "thanks", "ABCD1234") },
			check: func(t *testing.T, got any) {
				if msg := as[Message](t, got); msg.ID != 7 {
					t.Fatalf("message = %+v", msg)
				}
			},
		},
		{
			name:     "reply without logs",
			status:   http.StatusCreated,
			reply:    `{"message":` + messageJSON + `}`,
			method:   http.MethodPost,
			path:     "/v1/support/tickets/1042/messages",
			wantBody: `{"body":"thanks"}`,
			call:     func(c *Client) (any, error) { return c.Reply(context.Background(), 1042, "thanks", "") },
		},
		{
			name:       "resolve",
			status:     http.StatusOK,
			reply:      `{"ticket":` + ticketJSON + `}`,
			method:     http.MethodPost,
			path:       "/v1/support/tickets/1042/resolve",
			wantNoBody: true,
			call:       func(c *Client) (any, error) { return c.Resolve(context.Background(), 1042) },
			check: func(t *testing.T, got any) {
				if ticket := as[Ticket](t, got); ticket.Number != 1042 {
					t.Fatalf("ticket = %+v", ticket)
				}
			},
		},
		{
			name:       "unread",
			status:     http.StatusOK,
			reply:      `{"count":3}`,
			method:     http.MethodGet,
			path:       "/v1/support/unread",
			wantNoBody: true,
			call:       func(c *Client) (any, error) { return c.Unread(context.Background()) },
			check: func(t *testing.T, got any) {
				if as[int](t, got) != 3 {
					t.Fatalf("count = %v", got)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) { reply(t, w, tc.status, tc.reply) })
			c := newTestClient(t, api.server.URL, staticToken("session-token"))

			got, err := tc.call(c)
			if err != nil {
				t.Fatalf("call: %v", err)
			}
			req := api.only()
			if req.method != tc.method || req.path != tc.path || req.query != tc.query {
				t.Fatalf("request = %s %s?%s, want %s %s?%s", req.method, req.path, req.query, tc.method, tc.path, tc.query)
			}
			if auth := req.header.Get("Authorization"); auth != "Bearer session-token" {
				t.Fatalf("Authorization = %q", auth)
			}
			if v := req.header.Get("X-Typhon-Version"); v != app.Version {
				t.Fatalf("X-Typhon-Version = %q, want %q", v, app.Version)
			}
			if ua := req.header.Get("User-Agent"); ua != account.UserAgent {
				t.Fatalf("User-Agent = %q, want %q", ua, account.UserAgent)
			}
			if tc.wantNoBody {
				if len(req.body) != 0 || req.header.Get("Content-Type") != "" {
					t.Fatalf("body = %q, content type %q, want none", req.body, req.header.Get("Content-Type"))
				}
			} else {
				if ct := req.header.Get("Content-Type"); ct != "application/json" {
					t.Fatalf("Content-Type = %q", ct)
				}
				if !jsonEqual(t, req.body, tc.wantBody) {
					t.Fatalf("body = %s, want %s", req.body, tc.wantBody)
				}
			}
			if tc.check != nil {
				tc.check(t, got)
			}
		})
	}
}

func jsonEqual(t *testing.T, got []byte, want string) bool {
	t.Helper()
	var a, b any
	if err := json.Unmarshal(got, &a); err != nil {
		t.Fatalf("request body is not JSON: %v: %s", err, got)
	}
	if err := json.Unmarshal([]byte(want), &b); err != nil {
		t.Fatalf("want is not JSON: %v", err)
	}
	return reflect.DeepEqual(a, b)
}

func TestClientErrorDecoding(t *testing.T) {
	envelope := func(code, field string) string {
		return `{"error":{"code":"` + code + `","field":"` + field + `","message":"detail"}}`
	}
	cases := []struct {
		name      string
		status    int
		body      string
		wantCode  string
		wantField string
	}{
		{"ticket not found", 404, envelope("ticket_not_found", ""), CodeTicketNotFound, ""},
		{"invalid topic", 422, envelope("ticket_invalid_topic", "topic"), CodeTicketInvalidTopic, "topic"},
		{"invalid subject", 422, envelope("ticket_invalid_subject", "subject"), CodeTicketInvalidSubject, "subject"},
		{"invalid body", 422, envelope("ticket_invalid_body", "body"), CodeTicketInvalidBody, "body"},
		{"invalid game", 422, envelope("ticket_invalid_game", "gameIgdbId"), CodeTicketInvalidGame, "gameIgdbId"},
		{"invalid client", 422, envelope("ticket_invalid_client", "client"), CodeTicketInvalidClient, "client"},
		{"bundle unavailable", 422, envelope("ticket_bundle_unavailable", "logBundleId"), CodeTicketBundleUnavailable, "logBundleId"},
		{"ticket limit", 409, envelope("ticket_limit", ""), CodeTicketLimit, ""},
		{"bundle limit", 409, envelope("ticket_bundle_limit", "logBundleId"), CodeTicketBundleLimit, "logBundleId"},
		{"rate limited", 429, envelope("rate_limited", ""), account.CodeRateLimited, ""},
		{"unauthenticated", 401, envelope("unauthenticated", ""), account.CodeUnauthenticated, ""},
		{"outdated launcher", 426, envelope("launcher_outdated", ""), account.CodeOutdated, ""},
		{"unknown code passes through", 418, envelope("a_code_from_the_future", ""), "a_code_from_the_future", ""},
		{"html from a proxy", 502, "<html>bad gateway</html>", account.CodeServer, ""},
		{"empty 500", 500, "", account.CodeServer, ""},
		{"429 outside the envelope", 429, "slow down", account.CodeRateLimited, ""},
		{"401 outside the envelope", 401, "nope", account.CodeUnauthenticated, ""},
		{"403 outside the envelope", 403, "blocked", account.CodeBlocked, ""},
		{"json without a code", 500, `{"error":{}}`, account.CodeServer, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) { reply(t, w, tc.status, tc.body) })
			c := newTestClient(t, api.server.URL, staticToken("tok"))

			_, err := c.Get(context.Background(), 5)
			apiErr := apiCode(t, err)
			if apiErr.Code != tc.wantCode || apiErr.Field != tc.wantField || apiErr.Status != tc.status {
				t.Fatalf("error = {%q %q %d}, want {%q %q %d}", apiErr.Code, apiErr.Field, apiErr.Status, tc.wantCode, tc.wantField, tc.status)
			}
		})
	}
}

func TestClientEveryCallDecodesErrors(t *testing.T) {
	calls := map[string]func(c *Client) error{
		"create": func(c *Client) error {
			_, err := c.Create(context.Background(), createRequest{Topic: "other", Subject: "s", Body: "b"})
			return err
		},
		"list":    func(c *Client) error { _, err := c.List(context.Background(), ""); return err },
		"get":     func(c *Client) error { _, err := c.Get(context.Background(), 1); return err },
		"reply":   func(c *Client) error { _, err := c.Reply(context.Background(), 1, "b", ""); return err },
		"resolve": func(c *Client) error { _, err := c.Resolve(context.Background(), 1); return err },
		"unread":  func(c *Client) error { _, err := c.Unread(context.Background()); return err },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			api := newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
				reply(t, w, 409, `{"error":{"code":"ticket_limit"}}`)
			})
			if code := apiCode(t, call(newTestClient(t, api.server.URL, staticToken("tok")))).Code; code != CodeTicketLimit {
				t.Fatalf("code = %q", code)
			}
		})
	}
}

func TestClientMalformedSuccessBody(t *testing.T) {
	cases := map[string]string{
		"not json":    "<html>",
		"truncated":   `{"ticket":{"number":1`,
		"wrong shape": `{"ticket":"nope"}`,
		"empty":       "",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			api := newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) { reply(t, w, http.StatusOK, body) })
			_, err := newTestClient(t, api.server.URL, staticToken("tok")).Resolve(context.Background(), 1)
			if apiErr := apiCode(t, err); apiErr.Code != account.CodeServer {
				t.Fatalf("code = %q, want %q", apiErr.Code, account.CodeServer)
			}
		})
	}
}

func TestClientResponseSizeIsLimited(t *testing.T) {
	pad := strings.Repeat("a", 2<<20)
	api := newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := io.WriteString(w, `{"ticket":{"number":1,"subject":"`+pad+`"}}`); err != nil {
			return // the client hangs up once it has read as much as it allows
		}
	})
	_, err := newTestClient(t, api.server.URL, staticToken("tok")).Resolve(context.Background(), 1)
	if apiErr := apiCode(t, err); apiErr.Code != account.CodeServer {
		t.Fatalf("code = %q, want %q for a reply cut at the limit", apiErr.Code, account.CodeServer)
	}
}

func TestClientWithoutSessionSendsNothing(t *testing.T) {
	tokens := map[string]func() (string, error){
		"empty token":   staticToken(""),
		"no credential": func() (string, error) { return "", account.ErrNoCredential },
	}
	for name, token := range tokens {
		t.Run(name, func(t *testing.T) {
			api := newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) { reply(t, w, http.StatusOK, `{"count":1}`) })
			_, err := newTestClient(t, api.server.URL, token).Unread(context.Background())
			if apiErr := apiCode(t, err); apiErr.Code != account.CodeUnauthenticated {
				t.Fatalf("code = %q", apiErr.Code)
			}
			if n := len(api.requests()); n != 0 {
				t.Fatalf("%d requests went out without a session", n)
			}
		})
	}
}

func TestClientTokenFailureIsNotUnauthenticated(t *testing.T) {
	api := newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) { reply(t, w, http.StatusOK, `{"count":1}`) })
	broken := errors.New("keychain locked")
	c := newTestClient(t, api.server.URL, func() (string, error) { return "", broken })

	_, err := c.Unread(context.Background())
	if apiErr := apiCode(t, err); apiErr.Code != account.CodeVaultError || !errors.Is(err, broken) {
		t.Fatalf("error = %+v, want a vault error wrapping the keychain failure", err)
	}
	if n := len(api.requests()); n != 0 {
		t.Fatalf("%d requests went out", n)
	}
}

func TestClientTicketNumberMustBePositive(t *testing.T) {
	api := newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) { reply(t, w, http.StatusOK, `{}`) })
	c := newTestClient(t, api.server.URL, staticToken("tok"))
	for _, number := range []int64{0, -1} {
		calls := map[string]func() error{
			"get":     func() error { _, err := c.Get(context.Background(), number); return err },
			"reply":   func() error { _, err := c.Reply(context.Background(), number, "b", ""); return err },
			"resolve": func() error { _, err := c.Resolve(context.Background(), number); return err },
		}
		for name, call := range calls {
			if apiErr := apiCode(t, call()); apiErr.Code != CodeTicketNotFound {
				t.Fatalf("%s(%d) code = %q", name, number, apiErr.Code)
			}
		}
	}
	if n := len(api.requests()); n != 0 {
		t.Fatalf("%d requests went out for a number that cannot exist", n)
	}
}

func TestClientCancelledContext(t *testing.T) {
	api := newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) { reply(t, w, http.StatusOK, `{"count":1}`) })
	c := newTestClient(t, api.server.URL, staticToken("tok"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.Unread(ctx)
	if apiErr := apiCode(t, err); apiErr.Code != account.CodeNetwork || !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want a network error caused by cancellation", err)
	}
	if n := len(api.requests()); n != 0 {
		t.Fatalf("%d requests went out on a cancelled context", n)
	}
}

func TestClientCancelledWhileWaiting(t *testing.T) {
	entered := make(chan struct{})
	api := newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
	})
	c := newTestClient(t, api.server.URL, staticToken("tok"))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := c.Unread(ctx)
		done <- err
	}()

	<-entered
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the call did not return after cancellation")
	}
}

func TestNewClientRejectsBadSetup(t *testing.T) {
	cases := map[string]struct {
		base  string
		token func() (string, error)
	}{
		"empty base":     {"", staticToken("t")},
		"plain http":     {"http://api.example.com", staticToken("t")},
		"nil token":      {"https://api.example.com", nil},
		"unsupported":    {"ftp://api.example.com", staticToken("t")},
		"no host":        {"https://", staticToken("t")},
		"whitespace url": {"   ", staticToken("t")},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := NewClient(tc.base, tc.token); err == nil {
				t.Fatal("NewClient accepted it")
			}
		})
	}
}

func as[T any](t *testing.T, got any) T {
	t.Helper()
	value, ok := got.(T)
	if !ok {
		t.Fatalf("result is %T, want %T", got, value)
	}
	return value
}
