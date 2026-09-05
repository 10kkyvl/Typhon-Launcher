package social

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/iotest"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type emitted struct {
	name string
	data any
}

type fakeSettings struct {
	mu   sync.Mutex
	on   bool
	subs []func(bool)
}

func (f *fakeSettings) AccountSync() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.on
}

func (f *fakeSettings) Subscribe(fn func(bool)) func() {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := len(f.subs)
	f.subs = append(f.subs, fn)
	return func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.subs[id] = nil
	}
}

func (f *fakeSettings) set(on bool) {
	f.mu.Lock()
	f.on = on
	subs := make([]func(bool), len(f.subs))
	copy(subs, f.subs)
	f.mu.Unlock()
	for _, fn := range subs {
		if fn != nil {
			fn(on)
		}
	}
}

type harness struct {
	t        *testing.T
	svc      *Service
	settings *fakeSettings
	tick     chan time.Time
	polled   chan struct{}
	reqs     atomic.Int32
	paths    chan string

	mu    sync.Mutex
	emits []emitted
}

func newHarness(t *testing.T, resolve func(string) string, handler func(h *harness, w http.ResponseWriter, r *http.Request)) *harness {
	t.Helper()
	h := &harness{
		t:        t,
		settings: &fakeSettings{on: true},
		tick:     make(chan time.Time),
		polled:   make(chan struct{}),
		paths:    make(chan string, 64),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.reqs.Add(1)
		select {
		case h.paths <- r.Method + " " + r.URL.EscapedPath():
		default:
		}
		handler(h, w, r)
	}))
	t.Cleanup(srv.Close)

	svc, err := NewService(srv.URL, staticToken("tok"), h.settings, resolve)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	svc.emitFn = h.record
	svc.newTicker = func(time.Duration) (<-chan time.Time, func()) { return h.tick, func() {} }
	svc.polled = h.polled
	h.svc = svc
	return h
}

func (h *harness) record(name string, data any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.emits = append(h.emits, emitted{name: name, data: data})
}

func (h *harness) start() {
	h.t.Helper()
	if err := h.svc.ServiceStartup(h.t.Context(), application.ServiceOptions{}); err != nil {
		h.t.Fatalf("ServiceStartup: %v", err)
	}
	h.t.Cleanup(func() {
		if err := h.svc.ServiceShutdown(); err != nil {
			h.t.Errorf("ServiceShutdown: %v", err)
		}
	})
}

func (h *harness) awaitPoll() {
	h.t.Helper()
	select {
	case <-h.polled:
	case <-time.After(5 * time.Second):
		h.t.Fatal("timed out waiting for a poll")
	}
}

func (h *harness) sendTick() {
	h.t.Helper()
	select {
	case h.tick <- time.Now():
	case <-time.After(5 * time.Second):
		h.t.Fatal("timed out sending a tick")
	}
}

func (h *harness) names() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, 0, len(h.emits))
	for _, e := range h.emits {
		out = append(out, e.name)
	}
	return out
}

func (h *harness) requestSignals() []int {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []int
	for _, e := range h.emits {
		if e.name != EventRequests {
			continue
		}
		signal, ok := e.data.(RequestsSignal)
		if !ok {
			h.t.Fatalf("%s payload = %T, want RequestsSignal", EventRequests, e.data)
		}
		out = append(out, signal.Incoming)
	}
	return out
}

func countName(names []string, want string) int {
	n := 0
	for _, name := range names {
		if name == want {
			n++
		}
	}
	return n
}

func pageWithIncoming(n int) string {
	body := `{"friends":[{"id":"f1","username":"alex"}],"incoming":[`
	for i := range n {
		if i > 0 {
			body += ","
		}
		body += fmt.Sprintf(`{"id":"i%d"}`, i)
	}
	return body + `],"outgoing":[]}`
}

func writeJSON(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if _, err := io.WriteString(w, body); err != nil {
		t.Errorf("write response: %v", err)
	}
}

func TestService_FirstPollIsImmediate(t *testing.T) {
	h := newHarness(t, func(string) string { return "1942" }, func(h *harness, w http.ResponseWriter, _ *http.Request) {
		writeJSON(h.t, w, pageWithIncoming(0))
	})

	h.start()
	h.awaitPoll()

	if got := h.reqs.Load(); got != 1 {
		t.Fatalf("requests after startup = %d, want 1 without any tick", got)
	}
	if got := countName(h.names(), EventFriends); got != 1 {
		t.Fatalf("%s emits = %d, want 1", EventFriends, got)
	}
	select {
	case path := <-h.paths:
		if path != "GET /v1/me/friends" {
			t.Fatalf("polled %q, want GET /v1/me/friends", path)
		}
	default:
		t.Fatal("no request recorded")
	}
}

func TestService_TickTriggersPoll(t *testing.T) {
	h := newHarness(t, func(string) string { return "1942" }, func(h *harness, w http.ResponseWriter, _ *http.Request) {
		writeJSON(h.t, w, pageWithIncoming(0))
	})

	h.start()
	h.awaitPoll()
	h.sendTick()
	h.awaitPoll()

	if got := h.reqs.Load(); got != 2 {
		t.Fatalf("requests = %d, want 2", got)
	}
	if got := countName(h.names(), EventFriends); got != 2 {
		t.Fatalf("%s emits = %d, want 2", EventFriends, got)
	}
}

func TestService_KickTriggersPoll(t *testing.T) {
	h := newHarness(t, func(string) string { return "1942" }, func(h *harness, w http.ResponseWriter, _ *http.Request) {
		writeJSON(h.t, w, pageWithIncoming(0))
	})

	h.start()
	h.awaitPoll()
	h.svc.Kick()
	h.awaitPoll()

	if got := h.reqs.Load(); got != 2 {
		t.Fatalf("requests = %d, want 2", got)
	}
}

func TestService_PausesOnUnauthorizedUntilKick(t *testing.T) {
	h := newHarness(t, func(string) string { return "1942" }, func(h *harness, w http.ResponseWriter, _ *http.Request) {
		if h.reqs.Load() == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			writeJSON(h.t, w, `{"error":{"code":"unauthenticated"}}`)
			return
		}
		writeJSON(h.t, w, pageWithIncoming(1))
	})

	h.start()
	h.awaitPoll()
	if got := h.reqs.Load(); got != 1 {
		t.Fatalf("requests = %d, want 1", got)
	}
	if countName(h.names(), EventFriends) != 0 {
		t.Fatalf("a failed poll must not emit %s: %v", EventFriends, h.names())
	}

	h.sendTick()
	h.awaitPoll()
	if got := h.reqs.Load(); got != 1 {
		t.Fatalf("requests after a tick while paused = %d, want 1", got)
	}

	h.svc.Kick()
	h.awaitPoll()
	if got := h.reqs.Load(); got != 2 {
		t.Fatalf("requests after Kick = %d, want 2", got)
	}
	if got := countName(h.names(), EventFriends); got != 1 {
		t.Fatalf("%s emits = %d, want 1 after the resumed poll", EventFriends, got)
	}

	h.sendTick()
	h.awaitPoll()
	if got := h.reqs.Load(); got != 3 {
		t.Fatalf("requests after resuming = %d, want 3: polling must continue after Kick", got)
	}
}

func TestService_StopsPollingWhenTheServerHasNoSocialAPI(t *testing.T) {
	h := newHarness(t, func(string) string { return "1942" }, func(h *harness, w http.ResponseWriter, _ *http.Request) {
		if h.reqs.Load() == 1 {
			http.NotFound(w, &http.Request{})
			return
		}
		writeJSON(h.t, w, pageWithIncoming(1))
	})

	h.start()
	h.awaitPoll()
	if got := h.reqs.Load(); got != 1 {
		t.Fatalf("requests = %d, want 1", got)
	}

	for i := range 3 {
		h.sendTick()
		h.awaitPoll()
		if got := h.reqs.Load(); got != 1 {
			t.Fatalf("requests after tick %d = %d, want 1: a server without the social api must not be polled again", i+1, got)
		}
	}

	h.svc.Kick()
	h.awaitPoll()
	if got := h.reqs.Load(); got != 2 {
		t.Fatalf("requests after Kick = %d, want 2", got)
	}
}

func TestDecodeErrorSeparatesAMissingRouteFromAMissingRecord(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		check  func(error) bool
	}{
		{"routeless 404", http.StatusNotFound, "404 page not found\n", func(err error) bool { return errors.Is(err, ErrUnsupported) }},
		{"coded 404", http.StatusNotFound, `{"error":{"code":"not_found"}}`, func(err error) bool {
			var api *APIError
			return errors.As(err, &api) && api.Code == "not_found"
		}},
		{"empty 404", http.StatusNotFound, "", func(err error) bool { return errors.Is(err, ErrUnsupported) }},
		{"500", http.StatusInternalServerError, "boom", func(err error) bool {
			var srv *ServerError
			return errors.As(err, &srv) && srv.Status == http.StatusInternalServerError
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := decodeError(tt.status, strings.NewReader(tt.body))
			if !tt.check(err) {
				t.Fatalf("decodeError(%d, %q) = %v", tt.status, tt.body, err)
			}
		})
	}

	t.Run("unreadable body", func(t *testing.T) {
		err := decodeError(http.StatusNotFound, iotest.ErrReader(errors.New("boom")))
		var srv *ServerError
		if !errors.As(err, &srv) || srv.Status != http.StatusNotFound {
			t.Fatalf("decodeError with an unreadable body = %v, want a ServerError: an unread body is not evidence of a missing route", err)
		}
	})
}

func TestService_EmitsRequestsOnlyOnCountChange(t *testing.T) {
	counts := []int{2, 2, 3, 3, 0}
	h := newHarness(t, func(string) string { return "1942" }, func(h *harness, w http.ResponseWriter, _ *http.Request) {
		idx := int(h.reqs.Load()) - 1
		if idx >= len(counts) {
			idx = len(counts) - 1
		}
		writeJSON(h.t, w, pageWithIncoming(counts[idx]))
	})

	h.start()
	h.awaitPoll()
	for range len(counts) - 1 {
		h.sendTick()
		h.awaitPoll()
	}

	if got := countName(h.names(), EventFriends); got != len(counts) {
		t.Fatalf("%s emits = %d, want %d (one per successful poll)", EventFriends, got, len(counts))
	}
	want := []int{2, 3, 0}
	got := h.requestSignals()
	if len(got) != len(want) {
		t.Fatalf("%s payloads = %v, want %v", EventRequests, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s payloads = %v, want %v", EventRequests, got, want)
		}
	}
}

func TestService_NoRequestsSignalWhenCountStaysZero(t *testing.T) {
	h := newHarness(t, func(string) string { return "1942" }, func(h *harness, w http.ResponseWriter, _ *http.Request) {
		writeJSON(h.t, w, pageWithIncoming(0))
	})

	h.start()
	h.awaitPoll()
	h.sendTick()
	h.awaitPoll()

	if got := h.requestSignals(); len(got) != 0 {
		t.Fatalf("%s emits = %v, want none while the count stays 0", EventRequests, got)
	}
}

func TestService_FriendsServesTheCachedPage(t *testing.T) {
	h := newHarness(t, func(string) string { return "1942" }, func(h *harness, w http.ResponseWriter, _ *http.Request) {
		writeJSON(h.t, w, pageWithIncoming(int(h.reqs.Load())))
	})

	h.start()
	h.awaitPoll()

	page, err := h.svc.Friends()
	if err != nil {
		t.Fatalf("Friends: %v", err)
	}
	if len(page.Incoming) != 1 {
		t.Fatalf("incoming = %d, want the first poll's page", len(page.Incoming))
	}
	if got := h.reqs.Load(); got != 1 {
		t.Fatalf("requests = %d, want 1: Friends must serve the cache", got)
	}

	if err := h.svc.Refresh(); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	page, err = h.svc.Friends()
	if err != nil {
		t.Fatalf("Friends after Refresh: %v", err)
	}
	if len(page.Incoming) != 2 {
		t.Fatalf("incoming = %d, want the refreshed page", len(page.Incoming))
	}
}

func TestService_FriendsFetchesWhenCacheIsEmpty(t *testing.T) {
	h := newHarness(t, func(string) string { return "1942" }, func(h *harness, w http.ResponseWriter, _ *http.Request) {
		writeJSON(h.t, w, pageWithIncoming(1))
	})

	if err := h.svc.ServiceStartup(t.Context(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	t.Cleanup(func() {
		if err := h.svc.ServiceShutdown(); err != nil {
			t.Errorf("ServiceShutdown: %v", err)
		}
	})

	page, err := h.svc.Friends()
	if err != nil {
		t.Fatalf("Friends: %v", err)
	}
	if len(page.Incoming) != 1 {
		t.Fatalf("incoming = %d, want a fetched page", len(page.Incoming))
	}
	h.awaitPoll()
}

func TestService_RefreshSurfacesErrors(t *testing.T) {
	h := newHarness(t, func(string) string { return "1942" }, func(h *harness, w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		writeJSON(h.t, w, `{"error":{"code":"internal"}}`)
	})

	h.start()
	h.awaitPoll()

	err := h.svc.Refresh()
	var srvErr *ServerError
	if !errors.As(err, &srvErr) {
		t.Fatalf("Refresh error = %v (%T), want *ServerError", err, err)
	}
	if _, err := h.svc.Friends(); err == nil {
		t.Fatal("Friends must surface the failure instead of an empty page")
	}
}

func TestService_GameFriendsResolvesTheIGDBID(t *testing.T) {
	h := newHarness(t, func(canonical string) string {
		if canonical == "igdb:1942" {
			return "1942"
		}
		return ""
	}, func(h *harness, w http.ResponseWriter, _ *http.Request) {
		writeJSON(h.t, w, `{"played":[{"id":"u1","status":"playing"}],"playingNow":[]}`)
	})

	h.start()
	h.awaitPoll()
	<-h.paths

	res, err := h.svc.GameFriends("igdb:1942")
	if err != nil {
		t.Fatalf("GameFriends: %v", err)
	}
	if len(res.Played) != 1 {
		t.Fatalf("played = %+v", res.Played)
	}
	select {
	case path := <-h.paths:
		if path != "GET /v1/games/1942/friends" {
			t.Fatalf("path = %q, want GET /v1/games/1942/friends", path)
		}
	default:
		t.Fatal("no request recorded")
	}

	before := h.reqs.Load()
	_, err = h.svc.GameFriends("unknown")
	if err == nil {
		t.Fatal("want an error for a game without an IGDB id")
	}
	if err.Error() != "unknown_game" {
		t.Fatalf("GameFriends error = %q, want the unknown_game error code", err)
	}
	if got := h.reqs.Load(); got != before {
		t.Fatalf("requests = %d, want %d: an unresolved id must not hit the API", got, before)
	}
}

func TestService_MutationsTriggerARefresh(t *testing.T) {
	tests := []struct {
		name string
		call func(*Service) error
		path string
	}{
		{name: "accept", call: func(s *Service) error { return s.Accept("u1") }, path: "POST /v1/me/friends/requests/u1/accept"},
		{name: "decline", call: func(s *Service) error { return s.Decline("u1") }, path: "DELETE /v1/me/friends/requests/u1"},
		{name: "unfriend", call: func(s *Service) error { return s.Unfriend("u1") }, path: "DELETE /v1/me/friends/u1"},
		{name: "block", call: func(s *Service) error { return s.Block("u1") }, path: "POST /v1/me/blocks/u1"},
		{name: "unblock", call: func(s *Service) error { return s.Unblock("u1") }, path: "DELETE /v1/me/blocks/u1"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, func(string) string { return "1942" }, func(h *harness, w http.ResponseWriter, r *http.Request) {
				if r.URL.EscapedPath() == "/v1/me/friends" && r.Method == http.MethodGet {
					writeJSON(h.t, w, pageWithIncoming(0))
					return
				}
				w.WriteHeader(http.StatusNoContent)
			})

			h.start()
			h.awaitPoll()
			<-h.paths

			if err := tc.call(h.svc); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			select {
			case path := <-h.paths:
				if path != tc.path {
					t.Fatalf("path = %q, want %q", path, tc.path)
				}
			default:
				t.Fatal("no request recorded")
			}

			h.awaitPoll()
			if got := h.reqs.Load(); got != 3 {
				t.Fatalf("requests = %d, want 3: the mutation must schedule a refresh", got)
			}
		})
	}
}

func TestService_RejectsEmptyIdentifiers(t *testing.T) {
	h := newHarness(t, func(string) string { return "1942" }, func(h *harness, w http.ResponseWriter, _ *http.Request) {
		writeJSON(h.t, w, pageWithIncoming(0))
	})
	h.start()
	h.awaitPoll()
	before := h.reqs.Load()

	calls := map[string]func() error{
		"accept":       func() error { return h.svc.Accept("") },
		"decline":      func() error { return h.svc.Decline("") },
		"unfriend":     func() error { return h.svc.Unfriend("") },
		"block":        func() error { return h.svc.Block("") },
		"unblock":      func() error { return h.svc.Unblock("") },
		"send request": func() error { _, err := h.svc.SendRequest("  "); return err },
		"profile":      func() error { _, err := h.svc.Profile(""); return err },
		"profile by code": func() error {
			_, err := h.svc.ProfileByCode("")
			return err
		},
		"user games":           func() error { _, err := h.svc.UserGames("", ""); return err },
		"game friends":         func() error { _, err := h.svc.GameFriends(""); return err },
		"feed bad cursor":      func() error { _, err := h.svc.Feed("not-a-number"); return err },
		"feed negative cursor": func() error { _, err := h.svc.Feed("-1"); return err },
		"react bad emoji":      func() error { return h.svc.React("1", "banana") },
		"react bad id":         func() error { return h.svc.React("not-a-number", "fire") },
		"unreact bad id":       func() error { return h.svc.Unreact("not-a-number", "fire") },
	}
	for name, call := range calls {
		if err := call(); err == nil {
			t.Errorf("%s with an empty identifier returned no error", name)
		}
	}
	if got := h.reqs.Load(); got != before {
		t.Fatalf("requests = %d, want %d: empty identifiers must not reach the API", got, before)
	}
}

func TestService_MethodsFailBeforeStartup(t *testing.T) {
	h := newHarness(t, func(string) string { return "1942" }, func(h *harness, w http.ResponseWriter, _ *http.Request) {
		writeJSON(h.t, w, pageWithIncoming(0))
	})

	if _, err := h.svc.Friends(); err == nil {
		t.Error("Friends before startup returned no error")
	}
	if err := h.svc.Refresh(); err == nil {
		t.Error("Refresh before startup returned no error")
	}
	if _, err := h.svc.Blocks(); err == nil {
		t.Error("Blocks before startup returned no error")
	}
	if _, err := h.svc.Feed(""); err == nil {
		t.Error("Feed before startup returned no error")
	}
	if err := h.svc.React("1", "fire"); err == nil {
		t.Error("React before startup returned no error")
	}
	if err := h.svc.Unreact("1", "fire"); err == nil {
		t.Error("Unreact before startup returned no error")
	}
	if got := h.reqs.Load(); got != 0 {
		t.Fatalf("requests = %d, want 0 before startup", got)
	}
}

func TestService_ShutdownStopsTheLoop(t *testing.T) {
	h := newHarness(t, func(string) string { return "1942" }, func(h *harness, w http.ResponseWriter, _ *http.Request) {
		writeJSON(h.t, w, pageWithIncoming(0))
	})

	if err := h.svc.ServiceStartup(t.Context(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	h.awaitPoll()
	if err := h.svc.ServiceShutdown(); err != nil {
		t.Fatalf("ServiceShutdown: %v", err)
	}
	if err := h.svc.ServiceShutdown(); err != nil {
		t.Fatalf("second ServiceShutdown: %v", err)
	}

	before := h.reqs.Load()
	h.svc.Kick()
	if got := h.reqs.Load(); got != before {
		t.Fatalf("requests = %d, want %d: a kick after shutdown must not poll", got, before)
	}
}

func TestNewService_Validates(t *testing.T) {
	resolve := func(string) string { return "1942" }
	set := &fakeSettings{on: true}
	if _, err := NewService("", staticToken("tok"), set, resolve); err == nil {
		t.Error("want an error for an empty base url")
	}
	if _, err := NewService("http://example.com", staticToken("tok"), set, resolve); err == nil {
		t.Error("want an error for a plain http non-loopback base url")
	}
	if _, err := NewService("https://api.example.com", nil, set, resolve); err == nil {
		t.Error("want an error for a nil token resolver")
	}
	if _, err := NewService("https://api.example.com", staticToken("tok"), set, nil); err == nil {
		t.Error("want an error for a nil resolveIGDBID callback")
	}
	if _, err := NewService("https://api.example.com", staticToken("tok"), nil, resolve); err == nil {
		t.Error("want an error for a nil settings port")
	}
	svc, err := NewService("https://api.example.com", staticToken("tok"), set, resolve)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if svc.interval != defaultPollInterval {
		t.Fatalf("interval = %v, want %v", svc.interval, defaultPollInterval)
	}
}

func TestService_KickDuringInFlightUnauthorizedDoesNotPause(t *testing.T) {
	release := make(chan struct{})
	h := newHarness(t, func(string) string { return "1942" }, func(h *harness, w http.ResponseWriter, _ *http.Request) {
		if h.reqs.Load() == 1 {
			<-release
			w.WriteHeader(http.StatusUnauthorized)
			writeJSON(h.t, w, `{"error":{"code":"unauthenticated"}}`)
			return
		}
		writeJSON(h.t, w, pageWithIncoming(0))
	})

	h.start()
	for h.reqs.Load() < 1 {
		runtime.Gosched()
	}
	h.svc.Kick()
	close(release)
	h.awaitPoll()
	h.awaitPoll()
	if got := h.reqs.Load(); got != 2 {
		t.Fatalf("requests = %d, want 2: the queued kick must poll after the stale 401", got)
	}
	if h.svc.isPaused() {
		t.Fatal("a 401 from a request that predates Kick must not pause the loop")
	}

	h.sendTick()
	h.awaitPoll()
	if got := h.reqs.Load(); got != 3 {
		t.Fatalf("requests after a tick = %d, want 3", got)
	}
}

func TestService_SyncOffMakesNoRequests(t *testing.T) {
	h := newHarness(t, func(string) string { return "1942" }, func(h *harness, w http.ResponseWriter, _ *http.Request) {
		writeJSON(h.t, w, pageWithIncoming(1))
	})
	h.settings.set(false)

	h.start()
	h.awaitPoll()
	h.sendTick()
	h.awaitPoll()

	if got := h.reqs.Load(); got != 0 {
		t.Fatalf("requests = %d, want 0 while account sync is off", got)
	}
	if got := h.names(); len(got) != 0 {
		t.Fatalf("emits = %v, want none while account sync is off", got)
	}

	page, err := h.svc.Friends()
	if err != nil {
		t.Fatalf("Friends with sync off: %v", err)
	}
	if len(page.Friends) != 0 || len(page.Incoming) != 0 || len(page.Outgoing) != 0 {
		t.Fatalf("Friends = %+v, want an empty page", page)
	}
	if got := h.reqs.Load(); got != 0 {
		t.Fatalf("requests after Friends = %d, want 0", got)
	}
}

func TestService_SyncOffFailsEveryOtherCall(t *testing.T) {
	h := newHarness(t, func(string) string { return "1942" }, func(h *harness, w http.ResponseWriter, _ *http.Request) {
		writeJSON(h.t, w, pageWithIncoming(1))
	})
	h.settings.set(false)

	h.start()
	h.awaitPoll()

	calls := map[string]func() error{
		"refresh":            h.svc.Refresh,
		"accept":             func() error { return h.svc.Accept("u1") },
		"decline":            func() error { return h.svc.Decline("u1") },
		"unfriend":           func() error { return h.svc.Unfriend("u1") },
		"block":              func() error { return h.svc.Block("u1") },
		"unblock":            func() error { return h.svc.Unblock("u1") },
		"send request":       func() error { _, err := h.svc.SendRequest("alex"); return err },
		"blocks":             func() error { _, err := h.svc.Blocks(); return err },
		"friend code":        func() error { _, err := h.svc.FriendCode(); return err },
		"rotate friend code": func() error { _, err := h.svc.RotateFriendCode(); return err },
		"profile":            func() error { _, err := h.svc.Profile("alex"); return err },
		"profile by code":    func() error { _, err := h.svc.ProfileByCode("TY-ABCD-EFGH"); return err },
		"user games":         func() error { _, err := h.svc.UserGames("alex", ""); return err },
		"game friends":       func() error { _, err := h.svc.GameFriends("igdb:1942"); return err },
		"feed":               func() error { _, err := h.svc.Feed(""); return err },
		"react":              func() error { return h.svc.React("1", "fire") },
		"unreact":            func() error { return h.svc.Unreact("1", "fire") },
	}
	for name, call := range calls {
		if err := call(); !errors.Is(err, ErrSyncDisabled) {
			t.Errorf("%s with sync off = %v, want ErrSyncDisabled", name, err)
		}
	}
	if got := h.reqs.Load(); got != 0 {
		t.Fatalf("requests = %d, want 0 while account sync is off", got)
	}
	if ErrSyncDisabled.Error() != "sync_disabled" {
		t.Fatalf("ErrSyncDisabled = %q, want the sync_disabled error code", ErrSyncDisabled)
	}
}

func TestService_EnablingSyncPollsImmediately(t *testing.T) {
	h := newHarness(t, func(string) string { return "1942" }, func(h *harness, w http.ResponseWriter, _ *http.Request) {
		writeJSON(h.t, w, pageWithIncoming(1))
	})
	h.settings.set(false)

	h.start()
	h.awaitPoll()
	if got := h.reqs.Load(); got != 0 {
		t.Fatalf("requests = %d, want 0 before the setting flips", got)
	}

	h.settings.set(true)
	h.awaitPoll()

	if got := h.reqs.Load(); got != 1 {
		t.Fatalf("requests after enabling sync = %d, want 1", got)
	}
	if got := countName(h.names(), EventFriends); got != 1 {
		t.Fatalf("%s emits = %d, want 1", EventFriends, got)
	}
}

func TestService_KickDropsTheCachedPage(t *testing.T) {
	release := make(chan struct{})
	h := newHarness(t, func(string) string { return "1942" }, func(h *harness, w http.ResponseWriter, _ *http.Request) {
		if h.reqs.Load() == 2 {
			<-release
		}
		writeJSON(h.t, w, pageWithIncoming(2))
	})

	h.start()
	h.awaitPoll()

	h.svc.Kick()
	for h.reqs.Load() < 2 {
		runtime.Gosched()
	}

	h.svc.mu.Lock()
	loaded := h.svc.loaded
	page := h.svc.page
	incoming := h.svc.incoming
	h.svc.mu.Unlock()
	if loaded {
		t.Fatal("Kick must drop the cached page so the next account fetches fresh")
	}
	if len(page.Friends) != 0 || len(page.Incoming) != 0 || incoming != 0 {
		t.Fatalf("cached page after Kick = %+v (incoming %d), want empty", page, incoming)
	}

	close(release)
	h.awaitPoll()
}

func TestService_KickDiscardsAStaleSuccessfulPoll(t *testing.T) {
	release := make(chan struct{})
	h := newHarness(t, func(string) string { return "1942" }, func(h *harness, w http.ResponseWriter, _ *http.Request) {
		if h.reqs.Load() == 1 {
			<-release
			writeJSON(h.t, w, pageWithIncoming(7))
			return
		}
		writeJSON(h.t, w, pageWithIncoming(0))
	})

	h.start()
	for h.reqs.Load() < 1 {
		runtime.Gosched()
	}
	h.svc.Kick()
	close(release)
	h.awaitPoll()
	h.awaitPoll()

	page, err := h.svc.Friends()
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Incoming) != 0 {
		t.Fatalf("incoming = %d, want 0: a reply from before Kick must not repopulate the cache", len(page.Incoming))
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, e := range h.emits {
		if e.name == EventFriends {
			if fp, ok := e.data.(FriendsPage); ok && len(fp.Incoming) == 7 {
				t.Fatal("the stale page must not be emitted")
			}
		}
	}
}

func TestService_FeedRequestShape(t *testing.T) {
	h := newHarness(t, func(string) string { return "1942" }, func(h *harness, w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() == "/v1/me/friends" {
			writeJSON(h.t, w, pageWithIncoming(0))
			return
		}
		writeJSON(h.t, w, `{"events":[],"next":0}`)
	})
	h.start()
	h.awaitPoll()
	<-h.paths

	if _, err := h.svc.Feed(""); err != nil {
		t.Fatalf("Feed: %v", err)
	}
	select {
	case path := <-h.paths:
		if path != "GET /v1/me/feed" {
			t.Fatalf("path = %q, want GET /v1/me/feed", path)
		}
	default:
		t.Fatal("no request recorded")
	}

	if _, err := h.svc.Feed("42"); err != nil {
		t.Fatalf("Feed: %v", err)
	}
	select {
	case path := <-h.paths:
		if path != "GET /v1/me/feed" {
			t.Fatalf("path = %q, want GET /v1/me/feed", path)
		}
	default:
		t.Fatal("no request recorded")
	}
}

func TestService_FeedRejectsBadCursor(t *testing.T) {
	h := newHarness(t, func(string) string { return "1942" }, func(h *harness, w http.ResponseWriter, r *http.Request) {
		writeJSON(h.t, w, pageWithIncoming(0))
	})
	h.start()
	h.awaitPoll()
	before := h.reqs.Load()

	for _, cursor := range []string{"not-a-number", "-1", "-9223372036854775808"} {
		if _, err := h.svc.Feed(cursor); !errors.Is(err, errBadCursor) {
			t.Fatalf("Feed(%q) = %v, want errBadCursor", cursor, err)
		}
	}
	if got := h.reqs.Load(); got != before {
		t.Fatalf("requests = %d, want %d: a bad cursor must not reach the API", got, before)
	}
}

func TestService_ReactAndUnreact(t *testing.T) {
	tests := []struct {
		name string
		call func(*Service) error
		path string
	}{
		{name: "react", call: func(s *Service) error { return s.React("7", "fire") }, path: "PUT /v1/activity/7/reactions/fire"},
		{name: "unreact", call: func(s *Service) error { return s.Unreact("7", "fire") }, path: "DELETE /v1/activity/7/reactions/fire"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, func(string) string { return "1942" }, func(h *harness, w http.ResponseWriter, r *http.Request) {
				if r.URL.EscapedPath() == "/v1/me/friends" {
					writeJSON(h.t, w, pageWithIncoming(0))
					return
				}
				w.WriteHeader(http.StatusNoContent)
			})
			h.start()
			h.awaitPoll()
			<-h.paths

			if err := tc.call(h.svc); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			select {
			case path := <-h.paths:
				if path != tc.path {
					t.Fatalf("path = %q, want %q", path, tc.path)
				}
			default:
				t.Fatal("no request recorded")
			}
		})
	}
}

func TestService_ReactRejectsBadEmojiAndID(t *testing.T) {
	h := newHarness(t, func(string) string { return "1942" }, func(h *harness, w http.ResponseWriter, r *http.Request) {
		writeJSON(h.t, w, pageWithIncoming(0))
	})
	h.start()
	h.awaitPoll()
	before := h.reqs.Load()

	if err := h.svc.React("7", "banana"); !errors.Is(err, ErrBadEmoji) {
		t.Fatalf("React with a bad emoji = %v, want ErrBadEmoji", err)
	}
	if err := h.svc.Unreact("7", "banana"); !errors.Is(err, ErrBadEmoji) {
		t.Fatalf("Unreact with a bad emoji = %v, want ErrBadEmoji", err)
	}
	if err := h.svc.React("not-a-number", "fire"); !errors.Is(err, errBadEventID) {
		t.Fatalf("React with a bad id = %v, want errBadEventID", err)
	}
	if ErrBadEmoji.Error() != "reaction_invalid" {
		t.Fatalf("ErrBadEmoji = %q, want the reaction_invalid error code", ErrBadEmoji)
	}
	if got := h.reqs.Load(); got != before {
		t.Fatalf("requests = %d, want %d: an invalid emoji or id must not reach the API", got, before)
	}
}

func TestService_SetNoteRejectsBadID(t *testing.T) {
	h := newHarness(t, func(string) string { return "1942" }, func(h *harness, w http.ResponseWriter, _ *http.Request) {
		writeJSON(h.t, w, pageWithIncoming(0))
	})
	h.start()
	h.awaitPoll()
	before := h.reqs.Load()

	if err := h.svc.SetNote("not-a-number", "gg"); !errors.Is(err, errBadEventID) {
		t.Fatalf("SetNote with a bad id = %v, want errBadEventID", err)
	}
	if got := h.reqs.Load(); got != before {
		t.Fatalf("requests = %d, want %d: a bad id must not reach the API", got, before)
	}
}

func TestService_SetNoteEnforcesRuneLimit(t *testing.T) {
	h := newHarness(t, func(string) string { return "1942" }, func(h *harness, w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() == "/v1/me/friends" {
			writeJSON(h.t, w, pageWithIncoming(0))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	h.start()
	h.awaitPoll()
	<-h.paths

	ok := strings.Repeat("п", MaxNote)
	if err := h.svc.SetNote("7", ok); err != nil {
		t.Fatalf("SetNote with %d cyrillic runes = %v, want nil", MaxNote, err)
	}
	select {
	case path := <-h.paths:
		if path != "PUT /v1/activity/7/note" {
			t.Fatalf("path = %q, want PUT /v1/activity/7/note", path)
		}
	default:
		t.Fatal("no request recorded")
	}

	before := h.reqs.Load()
	tooLong := strings.Repeat("п", MaxNote+1)
	if err := h.svc.SetNote("7", tooLong); !errors.Is(err, ErrNoteTooLong) {
		t.Fatalf("SetNote with %d cyrillic runes = %v, want ErrNoteTooLong", MaxNote+1, err)
	}
	if got := h.reqs.Load(); got != before {
		t.Fatalf("requests = %d, want %d: a too-long note must not reach the API", got, before)
	}
}

func TestService_SetNoteSendsEmptyForWhitespace(t *testing.T) {
	var gotBody string
	h := newHarness(t, func(string) string { return "1942" }, func(h *harness, w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() == "/v1/me/friends" {
			writeJSON(h.t, w, pageWithIncoming(0))
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			h.t.Errorf("read body: %v", err)
		}
		gotBody = strings.TrimSpace(string(body))
		w.WriteHeader(http.StatusNoContent)
	})
	h.start()
	h.awaitPoll()

	if err := h.svc.SetNote("7", "   "); err != nil {
		t.Fatalf("SetNote: %v", err)
	}
	if gotBody != `{"note":""}` {
		t.Fatalf("body = %q, want an empty note", gotBody)
	}
}

func TestService_FeedAndReactSurfaceAPIErrors(t *testing.T) {
	h := newHarness(t, func(string) string { return "1942" }, func(h *harness, w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() == "/v1/me/friends" {
			writeJSON(h.t, w, pageWithIncoming(0))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		if _, err := io.WriteString(w, `{"error":{"code":"activity_not_found"}}`); err != nil {
			t.Errorf("write response: %v", err)
		}
	})
	h.start()
	h.awaitPoll()

	if _, err := h.svc.Feed(""); err == nil {
		t.Fatal("Feed against a failing API returned no error")
	}
	if err := h.svc.React("7", "fire"); err == nil {
		t.Fatal("React against a failing API returned no error")
	}
	var apiErr *APIError
	if err := h.svc.React("7", "fire"); !errors.As(err, &apiErr) || apiErr.Code != "activity_not_found" {
		t.Fatalf("React error = %v, want an APIError with code activity_not_found", err)
	}
}
