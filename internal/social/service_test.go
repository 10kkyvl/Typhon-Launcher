package social

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type emitted struct {
	name string
	data any
}

type harness struct {
	t      *testing.T
	svc    *Service
	tick   chan time.Time
	polled chan struct{}
	reqs   atomic.Int32
	paths  chan string

	mu    sync.Mutex
	emits []emitted
}

func newHarness(t *testing.T, resolve func(string) string, handler func(h *harness, w http.ResponseWriter, r *http.Request)) *harness {
	t.Helper()
	h := &harness{
		t:      t,
		tick:   make(chan time.Time),
		polled: make(chan struct{}),
		paths:  make(chan string, 64),
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

	svc, err := NewService(srv.URL, staticToken("tok"), resolve)
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
	if _, err := h.svc.GameFriends("unknown"); err == nil {
		t.Fatal("want an error for a game without an IGDB id")
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
		"user games":   func() error { _, err := h.svc.UserGames("", ""); return err },
		"game friends": func() error { _, err := h.svc.GameFriends(""); return err },
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
	if _, err := NewService("", staticToken("tok"), resolve); err == nil {
		t.Error("want an error for an empty base url")
	}
	if _, err := NewService("http://example.com", staticToken("tok"), resolve); err == nil {
		t.Error("want an error for a plain http non-loopback base url")
	}
	if _, err := NewService("https://api.example.com", nil, resolve); err == nil {
		t.Error("want an error for a nil token resolver")
	}
	if _, err := NewService("https://api.example.com", staticToken("tok"), nil); err == nil {
		t.Error("want an error for a nil resolveIGDBID callback")
	}
	svc, err := NewService("https://api.example.com", staticToken("tok"), resolve)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if svc.interval != defaultPollInterval {
		t.Fatalf("interval = %v, want %v", svc.interval, defaultPollInterval)
	}
}
