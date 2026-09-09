package online

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"typhon/internal/app"
	"typhon/internal/library"
	"typhon/internal/settings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type request struct {
	method  string
	path    string
	body    []byte
	headers http.Header
}

type harness struct {
	t        *testing.T
	svc      *Service
	settings *settings.Service
	clock    *fakeClock
	tick     chan time.Time
	sent     chan struct{}
	reqs     chan request
	stop     sync.Once
}

type fakeClock struct {
	mu sync.Mutex
	at time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.at
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.at = c.at.Add(d)
}

func staticToken(tok string) func() (string, error) {
	return func() (string, error) { return tok, nil }
}

func newHarness(t *testing.T, token func() (string, error), status int, resolve func(string) string) *harness {
	t.Helper()
	return newSyncedHarness(t, token, status, resolve, true)
}

func newSyncedHarness(t *testing.T, token func() (string, error), status int, resolve func(string) string, accountSync bool) *harness {
	t.Helper()
	h := &harness{
		t:     t,
		clock: &fakeClock{at: time.Date(2025, 3, 10, 12, 0, 0, 0, time.UTC)},
		tick:  make(chan time.Time),
		sent:  make(chan struct{}),
		reqs:  make(chan request, 64),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		select {
		case h.reqs <- request{method: r.Method, path: r.URL.EscapedPath(), body: body, headers: r.Header.Clone()}:
		default:
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)

	set, err := settings.NewServiceAt(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("settings service: %v", err)
	}
	h.settings = set
	h.setSync(accountSync)

	if resolve == nil {
		resolve = func(string) string { return "" }
	}
	svc, err := NewService(srv.URL, token, resolve, set)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	svc.newTicker = func(time.Duration) (<-chan time.Time, func()) { return h.tick, func() {} }
	svc.now = h.clock.now
	svc.sent = h.sent
	h.svc = svc
	return h
}

type fakeIdle struct {
	mu    sync.Mutex
	for_  time.Duration
	known bool
}

func (f *fakeIdle) since() (time.Duration, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.for_, f.known
}

func (f *fakeIdle) set(d time.Duration, known bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.for_ = d
	f.known = known
}

// idleFor подменяет источник простоя: без него тест зависел бы от того, трогал
// ли кто-нибудь мышь на машине, где он идёт.
func (h *harness) idleFor(d time.Duration) *fakeIdle {
	h.t.Helper()
	f := &fakeIdle{for_: d, known: true}
	h.svc.idleSince = f.since
	return f
}

func (h *harness) setAutoAway(on bool) {
	h.t.Helper()
	next := h.settings.GetSettings()
	next.PresenceAutoAway = on
	if err := h.settings.SaveSettings(next); err != nil {
		h.t.Fatalf("SaveSettings: %v", err)
	}
}

func (h *harness) setSync(on bool) {
	h.t.Helper()
	next := h.settings.GetSettings()
	next.AccountSync = on
	if err := h.settings.SaveSettings(next); err != nil {
		h.t.Fatalf("SaveSettings: %v", err)
	}
}

func (h *harness) start() {
	h.t.Helper()
	if err := h.svc.ServiceStartup(h.t.Context(), application.ServiceOptions{}); err != nil {
		h.t.Fatalf("ServiceStartup: %v", err)
	}
	h.t.Cleanup(h.shutdown)
}

func (h *harness) shutdown() {
	h.stop.Do(func() {
		if err := h.svc.ServiceShutdown(); err != nil {
			h.t.Errorf("ServiceShutdown: %v", err)
		}
	})
}

func (h *harness) awaitSend() {
	h.t.Helper()
	select {
	case <-h.sent:
	case <-time.After(5 * time.Second):
		h.t.Fatal("timed out waiting for a presence report")
	}
}

func (h *harness) tickNow() {
	h.t.Helper()
	select {
	case h.tick <- time.Now():
	case <-time.After(5 * time.Second):
		h.t.Fatal("timed out delivering a tick")
	}
}

func (h *harness) nextRequest() request {
	h.t.Helper()
	select {
	case req := <-h.reqs:
		return req
	case <-time.After(5 * time.Second):
		h.t.Fatal("timed out waiting for a request")
		return request{}
	}
}

func (h *harness) noRequest() {
	h.t.Helper()
	select {
	case req := <-h.reqs:
		h.t.Fatalf("unexpected request %s %s", req.method, req.path)
	default:
	}
}

func decodeBody(t *testing.T, req request) payload {
	t.Helper()
	var p payload
	if err := json.Unmarshal(req.body, &p); err != nil {
		t.Fatalf("decode body %q: %v", string(req.body), err)
	}
	return p
}

func TestNewServiceValidates(t *testing.T) {
	set, err := settings.NewServiceAt(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("settings service: %v", err)
	}
	if _, err := NewService("https://api.example.com", staticToken("tok"), nil, set); err == nil {
		t.Fatal("expected an error for a nil resolveIGDBID")
	}
	if _, err := NewService("https://api.example.com", staticToken("tok"), func(string) string { return "" }, nil); err == nil {
		t.Fatal("expected an error for a nil settings service")
	}
	if _, err := NewService("https://api.example.com", nil, func(string) string { return "" }, set); err == nil {
		t.Fatal("expected an error for a nil token resolver")
	}
}

func TestReportsImmediatelyOnStartup(t *testing.T) {
	h := newHarness(t, staticToken("tok"), http.StatusNoContent, nil)
	h.start()
	h.awaitSend()

	req := h.nextRequest()
	if req.method != http.MethodPut || req.path != "/v1/me/presence" {
		t.Fatalf("got %s %s, want PUT /v1/me/presence", req.method, req.path)
	}
	if got := req.headers.Get("Authorization"); got != "Bearer tok" {
		t.Fatalf("Authorization = %q", got)
	}
	if got := req.headers.Get("X-Typhon-Version"); got != app.Version {
		t.Fatalf("X-Typhon-Version = %q, want %q", got, app.Version)
	}
	if req.headers.Get("User-Agent") == "" {
		t.Fatal("User-Agent is empty")
	}
	p := decodeBody(t, req)
	if p.Status != settings.PresenceOnline || p.GameID != "" || p.AppVersion != app.Version {
		t.Fatalf("payload = %+v", p)
	}
}

func TestTickReportsAgain(t *testing.T) {
	h := newHarness(t, staticToken("tok"), http.StatusNoContent, nil)
	h.start()
	h.awaitSend()
	h.nextRequest()

	h.tickNow()
	h.awaitSend()
	req := h.nextRequest()
	if req.method != http.MethodPut {
		t.Fatalf("got %s, want PUT", req.method)
	}
}

func TestSessionStartAndStopReportTheGame(t *testing.T) {
	resolve := func(id string) string {
		switch id {
		case "game-a":
			return "111"
		case "game-b":
			return "222"
		default:
			return ""
		}
	}
	h := newHarness(t, staticToken("tok"), http.StatusNoContent, resolve)
	h.start()
	h.awaitSend()
	h.nextRequest()

	h.svc.SessionStarted(library.Game{ID: "a", CanonicalGameID: "game-a"})
	h.awaitSend()
	if p := decodeBody(t, h.nextRequest()); p.GameID != "111" {
		t.Fatalf("gameId = %q, want 111", p.GameID)
	}

	h.svc.SessionStarted(library.Game{ID: "b", CanonicalGameID: "game-b"})
	h.awaitSend()
	if p := decodeBody(t, h.nextRequest()); p.GameID != "222" {
		t.Fatalf("gameId = %q, want the last started game 222", p.GameID)
	}

	h.svc.SessionStopped("b")
	h.awaitSend()
	if p := decodeBody(t, h.nextRequest()); p.GameID != "111" {
		t.Fatalf("gameId = %q, want the still running game 111", p.GameID)
	}

	h.svc.SessionStopped("a")
	h.awaitSend()
	if p := decodeBody(t, h.nextRequest()); p.GameID != "" {
		t.Fatalf("gameId = %q, want empty after the last session stopped", p.GameID)
	}
}

func TestUnresolvedGameIDIsNotReported(t *testing.T) {
	h := newHarness(t, staticToken("tok"), http.StatusNoContent, func(string) string { return "not-an-id" })
	h.start()
	h.awaitSend()
	h.nextRequest()

	h.svc.SessionStarted(library.Game{ID: "a", CanonicalGameID: "game-a"})
	h.awaitSend()
	if p := decodeBody(t, h.nextRequest()); p.GameID != "" {
		t.Fatalf("gameId = %q, want empty for an unresolvable game", p.GameID)
	}
}

func TestSetStatusReportsTheNewStatus(t *testing.T) {
	h := newHarness(t, staticToken("tok"), http.StatusNoContent, nil)
	h.start()
	h.awaitSend()
	h.nextRequest()

	if err := h.svc.SetStatus(settings.PresenceBusy); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	h.awaitSend()
	if p := decodeBody(t, h.nextRequest()); p.Status != settings.PresenceBusy {
		t.Fatalf("status = %q, want busy", p.Status)
	}
	if got := h.svc.Status(); got != settings.PresenceBusy {
		t.Fatalf("Status() = %q, want busy", got)
	}
	if got := h.settings.GetSettings().PresenceStatus; got != settings.PresenceBusy {
		t.Fatalf("stored status = %q, want busy", got)
	}
}

func TestSetStatusRejectsUnknownStatus(t *testing.T) {
	h := newHarness(t, staticToken("tok"), http.StatusNoContent, nil)
	err := h.svc.SetStatus("offline")
	if !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("SetStatus error = %v, want ErrInvalidStatus", err)
	}
	if got := h.settings.GetSettings().PresenceStatus; got != settings.PresenceOnline {
		t.Fatalf("stored status = %q, want it untouched", got)
	}
}

func TestSettingsChangeReportsTheNewStatus(t *testing.T) {
	h := newHarness(t, staticToken("tok"), http.StatusNoContent, nil)
	h.start()
	h.awaitSend()
	h.nextRequest()

	next := h.settings.GetSettings()
	next.PresenceStatus = settings.PresenceAway
	if err := h.settings.SaveSettings(next); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	h.awaitSend()
	if p := decodeBody(t, h.nextRequest()); p.Status != settings.PresenceAway {
		t.Fatalf("status = %q, want away", p.Status)
	}
	if got := h.svc.Status(); got != settings.PresenceAway {
		t.Fatalf("Status() = %q, want away", got)
	}
}

func TestUnrelatedSettingsChangeDoesNotReport(t *testing.T) {
	h := newHarness(t, staticToken("tok"), http.StatusNoContent, nil)
	h.start()
	h.awaitSend()
	h.nextRequest()

	next := h.settings.GetSettings()
	next.Theme = "light"
	if err := h.settings.SaveSettings(next); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	h.tickNow()
	h.awaitSend()
	h.nextRequest()
	h.noRequest()
}

func TestSignedOutSendsNothing(t *testing.T) {
	h := newHarness(t, staticToken(""), http.StatusNoContent, nil)
	h.start()
	h.awaitSend()
	h.noRequest()

	h.tickNow()
	h.awaitSend()
	h.noRequest()

	h.shutdown()
	h.noRequest()
}

func TestUnauthorizedIsLoggedOnceAndRetried(t *testing.T) {
	var sink logSink
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&sink, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	h := newHarness(t, staticToken("tok"), http.StatusUnauthorized, nil)
	h.start()
	for range 3 {
		h.awaitSend()
		if req := h.nextRequest(); req.method != http.MethodPut {
			t.Fatalf("got %s, want PUT", req.method)
		}
		h.tickNow()
	}
	h.awaitSend()
	h.nextRequest()

	if got := sink.count("presence report failing"); got != 1 {
		t.Fatalf("warned %d times, want exactly once", got)
	}
}

func TestShutdownClearsPresence(t *testing.T) {
	h := newHarness(t, staticToken("tok"), http.StatusNoContent, nil)
	h.start()
	h.awaitSend()
	if req := h.nextRequest(); req.method != http.MethodPut {
		t.Fatalf("got %s, want PUT", req.method)
	}

	h.shutdown()
	req := h.nextRequest()
	if req.method != http.MethodDelete || req.path != "/v1/me/presence" {
		t.Fatalf("got %s %s, want DELETE /v1/me/presence", req.method, req.path)
	}
	if got := req.headers.Get("Authorization"); got != "Bearer tok" {
		t.Fatalf("Authorization = %q", got)
	}
}

func TestUnsupportedIsLoggedOnceAndStopsSending(t *testing.T) {
	var sink logSink
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&sink, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	h := newHarness(t, staticToken("tok"), http.StatusNotFound, nil)
	h.start()
	h.awaitSend()
	h.nextRequest()

	h.tickNow()
	h.awaitSend()
	h.noRequest()

	h.tickNow()
	h.awaitSend()
	h.noRequest()

	if got := sink.count("presence not supported by this server"); got != 1 {
		t.Fatalf("logged %d times, want exactly once", got)
	}
}

func TestKickReenablesAfterUnsupported(t *testing.T) {
	h := newHarness(t, staticToken("tok"), http.StatusNotFound, nil)
	h.start()
	h.awaitSend()
	h.nextRequest()

	h.tickNow()
	h.awaitSend()
	h.noRequest()

	h.svc.Kick()
	h.awaitSend()
	if req := h.nextRequest(); req.method != http.MethodPut {
		t.Fatalf("got %s, want PUT after Kick", req.method)
	}
}

func TestSetStatusDoesNotRetryAfterUnsupported(t *testing.T) {
	h := newHarness(t, staticToken("tok"), http.StatusNotFound, nil)
	h.start()
	h.awaitSend()
	h.nextRequest()

	h.tickNow()
	h.awaitSend()
	h.noRequest()

	if err := h.svc.SetStatus(settings.PresenceBusy); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	h.awaitSend()
	h.noRequest()
}

func TestRestartReenablesAfterUnsupported(t *testing.T) {
	h := newHarness(t, staticToken("tok"), http.StatusNotFound, nil)
	h.start()
	h.awaitSend()
	h.nextRequest()

	h.tickNow()
	h.awaitSend()
	h.noRequest()

	h.shutdown()
	h.nextRequest()

	if err := h.svc.ServiceStartup(h.t.Context(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	h.stop = sync.Once{}
	h.t.Cleanup(h.shutdown)
	h.awaitSend()
	if req := h.nextRequest(); req.method != http.MethodPut {
		t.Fatalf("got %s, want PUT after restart", req.method)
	}
}

func newFlakyHarness(t *testing.T, status *atomic.Int32, resolve func(string) string) *harness {
	t.Helper()
	h := &harness{
		t:     t,
		clock: &fakeClock{at: time.Date(2025, 3, 10, 12, 0, 0, 0, time.UTC)},
		tick:  make(chan time.Time),
		sent:  make(chan struct{}),
		reqs:  make(chan request, 64),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		select {
		case h.reqs <- request{method: r.Method, path: r.URL.EscapedPath(), body: body, headers: r.Header.Clone()}:
		default:
		}
		w.WriteHeader(int(status.Load()))
	}))
	t.Cleanup(srv.Close)

	set, err := settings.NewServiceAt(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("settings service: %v", err)
	}
	h.settings = set
	h.setSync(true)

	if resolve == nil {
		resolve = func(string) string { return "" }
	}
	svc, err := NewService(srv.URL, staticToken("tok"), resolve, set)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	svc.newTicker = func(time.Duration) (<-chan time.Time, func()) { return h.tick, func() {} }
	svc.now = h.clock.now
	svc.sent = h.sent
	h.svc = svc
	return h
}

// Ветка бага: временный 404 (деплой бэкенда, перезапуск прокси) не должен гасить
// presence до перезапуска лаунчера — сервис обязан сам вернуться к обычной работе,
// когда сервер снова отвечает.
func TestUnsupportedRecoversWhenServerRespondsAgain(t *testing.T) {
	var status atomic.Int32
	status.Store(http.StatusNotFound)
	h := newFlakyHarness(t, &status, nil)
	h.start()
	h.awaitSend()
	h.nextRequest()

	h.tickNow()
	h.awaitSend()
	h.noRequest()

	status.Store(http.StatusNoContent)
	h.clock.advance(unsupportedBackoff)
	h.tickNow()
	h.awaitSend()
	if req := h.nextRequest(); req.method != http.MethodPut {
		t.Fatalf("got %s, want PUT once the unsupported backoff elapses and the server recovers", req.method)
	}

	h.tickNow()
	h.awaitSend()
	if req := h.nextRequest(); req.method != http.MethodPut {
		t.Fatalf("got %s, want a regular PUT on the next tick after recovery, not another backoff wait", req.method)
	}
}

// Если сервер и правда не умеет presence (старая версия), лаунчер не должен
// долбить его каждые 30 секунд, и предупреждение не должно сыпаться в лог заново
// при каждой повторной неудаче.
func TestUnsupportedRetriesLessOftenAndLogsOnce(t *testing.T) {
	var sink logSink
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&sink, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	h := newHarness(t, staticToken("tok"), http.StatusNotFound, nil)
	h.start()
	h.awaitSend()
	h.nextRequest()

	for range 5 {
		h.tickNow()
		h.awaitSend()
		h.noRequest()
	}

	h.clock.advance(unsupportedBackoff)
	h.tickNow()
	h.awaitSend()
	if req := h.nextRequest(); req.method != http.MethodPut {
		t.Fatalf("got %s, want PUT once the backoff elapses", req.method)
	}

	if got := sink.count("presence not supported by this server"); got != 1 {
		t.Fatalf("logged %d times, want exactly once", got)
	}
}

func newRateLimitedHarness(t *testing.T, resolve func(string) string) *harness {
	t.Helper()
	h := &harness{
		t:     t,
		clock: &fakeClock{at: time.Date(2025, 3, 10, 12, 0, 0, 0, time.UTC)},
		tick:  make(chan time.Time),
		sent:  make(chan struct{}),
		reqs:  make(chan request, 64),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		select {
		case h.reqs <- request{method: r.Method, path: r.URL.EscapedPath(), body: body, headers: r.Header.Clone()}:
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		if _, err := w.Write([]byte(`{"error":{"code":"rate_limited"}}`)); err != nil {
			t.Errorf("write body: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	set, err := settings.NewServiceAt(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("settings service: %v", err)
	}
	h.settings = set
	h.setSync(true)

	if resolve == nil {
		resolve = func(string) string { return "" }
	}
	svc, err := NewService(srv.URL, staticToken("tok"), resolve, set)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	svc.newTicker = func(time.Duration) (<-chan time.Time, func()) { return h.tick, func() {} }
	svc.now = h.clock.now
	svc.sent = h.sent
	h.svc = svc
	return h
}

func TestRateLimitedDefersUntilTheDeadline(t *testing.T) {
	h := newRateLimitedHarness(t, func(id string) string {
		if id == "game-a" {
			return "111"
		}
		return ""
	})
	h.start()
	h.awaitSend()
	h.nextRequest()

	h.svc.SessionStarted(library.Game{ID: "a", CanonicalGameID: "game-a"})
	h.awaitSend()
	h.noRequest()

	h.tickNow()
	h.awaitSend()
	h.noRequest()

	h.clock.advance(rateLimitBackoff)
	h.tickNow()
	h.awaitSend()
	if p := decodeBody(t, h.nextRequest()); p.GameID != "111" {
		t.Fatalf("gameId = %q, want the state accumulated during the backoff", p.GameID)
	}
}

func TestRateLimitedSendsAfterADeadlinePoke(t *testing.T) {
	h := newRateLimitedHarness(t, nil)
	h.start()
	h.awaitSend()
	h.nextRequest()

	if err := h.svc.SetStatus(settings.PresenceAway); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	h.awaitSend()
	h.noRequest()

	h.clock.advance(rateLimitBackoff)
	if err := h.svc.SetStatus(settings.PresenceBusy); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	h.awaitSend()
	if p := decodeBody(t, h.nextRequest()); p.Status != settings.PresenceBusy {
		t.Fatalf("status = %q, want busy", p.Status)
	}
}

func TestSyncOffSendsNothing(t *testing.T) {
	h := newSyncedHarness(t, staticToken("tok"), http.StatusNoContent, nil, false)
	h.start()
	h.awaitSend()
	h.noRequest()

	h.svc.SessionStarted(library.Game{ID: "a", CanonicalGameID: "game-a"})
	h.awaitSend()
	h.noRequest()

	h.tickNow()
	h.awaitSend()
	h.noRequest()

	h.shutdown()
	h.noRequest()
}

func TestSyncTurnedOnReports(t *testing.T) {
	h := newSyncedHarness(t, staticToken("tok"), http.StatusNoContent, nil, false)
	h.start()
	h.awaitSend()
	h.noRequest()

	h.setSync(true)
	h.awaitSend()
	if req := h.nextRequest(); req.method != http.MethodPut {
		t.Fatalf("got %s, want PUT once sync is on", req.method)
	}
}

func TestSyncTurnedOffStopsReporting(t *testing.T) {
	h := newHarness(t, staticToken("tok"), http.StatusNoContent, nil)
	h.start()
	h.awaitSend()
	h.nextRequest()

	h.setSync(false)
	h.tickNow()
	h.awaitSend()
	h.noRequest()

	h.shutdown()
	h.noRequest()
}

func TestShutdownStopsSettingsUpdates(t *testing.T) {
	h := newHarness(t, staticToken("tok"), http.StatusNoContent, nil)
	h.start()
	h.awaitSend()
	h.nextRequest()

	h.shutdown()
	if req := h.nextRequest(); req.method != http.MethodDelete {
		t.Fatalf("got %s, want DELETE", req.method)
	}

	next := h.settings.GetSettings()
	next.PresenceStatus = settings.PresenceAway
	if err := h.settings.SaveSettings(next); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	if got := h.svc.Status(); got != settings.PresenceOnline {
		t.Fatalf("Status() = %q, want it untouched after shutdown", got)
	}
	h.noRequest()
}

type logSink struct {
	mu    sync.Mutex
	lines []string
}

func (s *logSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lines = append(s.lines, string(p))
	return len(p), nil
}

func (s *logSink) count(substr string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, line := range s.lines {
		if strings.Contains(line, substr) {
			n++
		}
	}
	return n
}

func TestIdleMachineGoesAway(t *testing.T) {
	h := newHarness(t, staticToken("tok"), http.StatusNoContent, nil)
	fake := h.idleFor(time.Minute)
	h.start()
	h.awaitSend()
	if p := decodeBody(t, h.nextRequest()); p.Status != settings.PresenceOnline {
		t.Fatalf("status = %q, want online while the machine is in use", p.Status)
	}

	fake.set(defaultAwayAfter, true)
	h.tickNow()
	h.awaitSend()
	if p := decodeBody(t, h.nextRequest()); p.Status != settings.PresenceAway {
		t.Fatalf("status = %q, want away after the idle threshold", p.Status)
	}
	if got := h.svc.EffectiveStatus(); got != settings.PresenceAway {
		t.Fatalf("EffectiveStatus() = %q, want away", got)
	}
	if got := h.svc.Status(); got != settings.PresenceOnline {
		t.Fatalf("Status() = %q, want the chosen status untouched", got)
	}
	if got := h.settings.GetSettings().PresenceStatus; got != settings.PresenceOnline {
		t.Fatalf("stored status = %q, want the chosen status untouched", got)
	}
}

func TestInputBringsTheStatusBack(t *testing.T) {
	h := newHarness(t, staticToken("tok"), http.StatusNoContent, nil)
	fake := h.idleFor(defaultAwayAfter)
	h.start()
	h.awaitSend()
	if p := decodeBody(t, h.nextRequest()); p.Status != settings.PresenceAway {
		t.Fatalf("status = %q, want away", p.Status)
	}

	fake.set(0, true)
	h.tickNow()
	h.awaitSend()
	if p := decodeBody(t, h.nextRequest()); p.Status != settings.PresenceOnline {
		t.Fatalf("status = %q, want online once the user is back", p.Status)
	}
	if got := h.svc.EffectiveStatus(); got != settings.PresenceOnline {
		t.Fatalf("EffectiveStatus() = %q, want online", got)
	}
}

func TestRunningGameKeepsTheStatusOnline(t *testing.T) {
	h := newHarness(t, staticToken("tok"), http.StatusNoContent, func(string) string { return "1942" })
	h.idleFor(2 * defaultAwayAfter)
	h.start()
	h.awaitSend()
	h.nextRequest()

	h.svc.SessionStarted(library.Game{ID: "g1", CanonicalGameID: "igdb:1942"})
	h.awaitSend()
	p := decodeBody(t, h.nextRequest())
	if p.Status != settings.PresenceOnline {
		t.Fatalf("status = %q, want online while a game is running", p.Status)
	}
	if p.GameID != "1942" {
		t.Fatalf("gameId = %q, want 1942", p.GameID)
	}
}

func TestChosenStatusIsNotOverriddenByIdle(t *testing.T) {
	h := newHarness(t, staticToken("tok"), http.StatusNoContent, nil)
	h.idleFor(2 * defaultAwayAfter)
	if err := h.svc.SetStatus(settings.PresenceBusy); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	h.start()
	h.awaitSend()
	if p := decodeBody(t, h.nextRequest()); p.Status != settings.PresenceBusy {
		t.Fatalf("status = %q, want the chosen busy kept", p.Status)
	}
	if got := h.svc.EffectiveStatus(); got != settings.PresenceBusy {
		t.Fatalf("EffectiveStatus() = %q, want busy", got)
	}
}

func TestAutoAwayOffKeepsTheStatusOnline(t *testing.T) {
	h := newHarness(t, staticToken("tok"), http.StatusNoContent, nil)
	h.setAutoAway(false)
	h.idleFor(2 * defaultAwayAfter)
	h.start()
	h.awaitSend()
	if p := decodeBody(t, h.nextRequest()); p.Status != settings.PresenceOnline {
		t.Fatalf("status = %q, want online with auto away switched off", p.Status)
	}
}

func TestUnknownIdleTimeKeepsTheStatusOnline(t *testing.T) {
	h := newHarness(t, staticToken("tok"), http.StatusNoContent, nil)
	fake := h.idleFor(0)
	fake.set(2*defaultAwayAfter, false)
	h.start()
	h.awaitSend()
	if p := decodeBody(t, h.nextRequest()); p.Status != settings.PresenceOnline {
		t.Fatalf("status = %q, want online when the idle time is unknown", p.Status)
	}
}

func TestAwayAfterFrom(t *testing.T) {
	cases := []struct {
		name   string
		raw    string
		mocked bool
		want   time.Duration
	}{
		{name: "production ignores the variable", raw: "5", mocked: false, want: defaultAwayAfter},
		{name: "devmock shortens the threshold", raw: "5", mocked: true, want: 5 * time.Second},
		{name: "empty keeps the default", raw: "", mocked: true, want: defaultAwayAfter},
		{name: "nonsense keeps the default", raw: "soon", mocked: true, want: defaultAwayAfter},
		{name: "zero keeps the default", raw: "0", mocked: true, want: defaultAwayAfter},
		{name: "negative keeps the default", raw: "-30", mocked: true, want: defaultAwayAfter},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := awayAfterFrom(tc.raw, tc.mocked); got != tc.want {
				t.Fatalf("awayAfterFrom(%q, %v) = %v, want %v", tc.raw, tc.mocked, got, tc.want)
			}
		})
	}
}
