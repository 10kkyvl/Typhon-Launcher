package messaging

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/wailsapp/wails/v3/pkg/application"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type chatHarness struct {
	svc      *Service
	mu       sync.Mutex
	token    string
	tokenErr error
	enabled  atomic.Bool
	events   chan Event
}

func newHarness(t *testing.T, handle http.HandlerFunc) *chatHarness {
	t.Helper()
	h := &chatHarness{token: "token-a", events: make(chan Event, 32)}
	h.enabled.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/me" {
			if _, err := fmt.Fprint(w, `{"id":"a"}`); err != nil {
				t.Error(err)
				return
			}
			return
		}
		if r.URL.Path == prefix+"/events" {
			w.Header().Set("Content-Type", "text/event-stream")
			if _, err := fmt.Fprint(w, "data: {\"kind\":\"sync\"}\n\n"); err != nil {
				t.Error(err)
				return
			}
			if err := http.NewResponseController(w).Flush(); err != nil {
				t.Error(err)
				return
			}
			<-r.Context().Done()
			return
		}
		handle(w, r)
	}))
	t.Cleanup(server.Close)
	svc, err := NewService(server.URL, func() (string, error) { h.mu.Lock(); defer h.mu.Unlock(); return h.token, h.tokenErr }, h.enabled.Load)
	if err != nil {
		t.Fatal(err)
	}
	h.svc = svc
	svc.emit = func(e Event) { h.events <- e }
	if err := svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := svc.ServiceShutdown(); err != nil {
			t.Error(err)
		}
	})
	if err = svc.Start(); err != nil {
		t.Fatal(err)
	}
	return h
}
func awaitKind(t *testing.T, ch <-chan Event, kind string) Event {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case e := <-ch:
			if e.Kind == kind {
				return e
			}
		case <-timer.C:
			t.Fatalf("no event %s", kind)
			return Event{}
		}
	}
}
func TestMessagingContractsAndRetryIdentity(t *testing.T) {
	const encoded = `{"id":"42","clientId":"retry-id","senderId":"a","recipientId":"b","text":"го играть","createdAt":"2026-09-12T12:00:00Z","expiresAt":"2026-09-19T12:00:00Z","editedAt":null,"reactions":[{"emoji":"heart","userIds":["b"]}]}`
	var mu sync.Mutex
	var calls, clientIDs []string
	h := newHarness(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token-a" || r.Header.Get("X-Typhon-Version") == "" {
			t.Error("missing auth/version headers")
		}
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, r.Method+" "+r.URL.RequestURI())
		switch {
		case r.URL.Path == prefix+"/conversations":
			if _, err := fmt.Fprintf(w, `{"conversations":[{"peer":{"id":"b","username":"bob"},"lastMessage":%s,"unread":1,"canSend":true}]}`, encoded); err != nil {
				t.Error(err)
				return
			}
		case r.Method == http.MethodGet:
			if _, err := fmt.Fprintf(w, `{"messages":[%s],"next":"41","canSend":true}`, encoded); err != nil {
				t.Error(err)
				return
			}
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/messages"):
			var body struct {
				ClientID string `json:"clientId"`
				Text     string `json:"text"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
				return
			}
			if body.Text != "го играть" {
				t.Errorf("text=%q", body.Text)
			}
			clientIDs = append(clientIDs, body.ClientID)
			if _, err := fmt.Fprint(w, encoded); err != nil {
				t.Error(err)
				return
			}
		case r.Method == http.MethodPatch:
			if _, err := fmt.Fprint(w, encoded); err != nil {
				t.Error(err)
				return
			}
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	})
	if e := awaitKind(t, h.events, "sync"); e.OwnerID != "a" {
		t.Fatalf("owner=%q", e.OwnerID)
	}
	cs, err := h.svc.Conversations()
	if err != nil || len(cs) != 1 || cs[0].Unread != 1 || !cs[0].CanSend {
		t.Fatalf("conversations=%+v err=%v", cs, err)
	}
	p, err := h.svc.Messages("b", "41")
	if err != nil || len(p.Messages) != 1 || p.Messages[0].Reactions[0].UserIDs[0] != "b" || p.Next != "41" {
		t.Fatalf("page=%+v err=%v", p, err)
	}
	for i := 0; i < 2; i++ {
		m, err := h.svc.Send("b", "retry-id", "го играть")
		if err != nil || m.ID != "42" || m.ExpiresAt == "" {
			t.Fatalf("send=%+v err=%v", m, err)
		}
	}
	if _, err := h.svc.Edit("b", "42", "edited"); err != nil {
		t.Fatal(err)
	}
	for _, err := range []error{h.svc.React("b", "42", "heart"), h.svc.Unreact("b", "42", "heart"), h.svc.Read("b", "42"), h.svc.Typing("b", true)} {
		if err != nil {
			t.Fatal(err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(clientIDs) != 2 || clientIDs[0] != clientIDs[1] {
		t.Fatalf("retry identity=%v", clientIDs)
	}
	for _, want := range []string{"GET " + prefix + "/b/messages?before=41", "PATCH " + prefix + "/b/messages/42", "PUT " + prefix + "/b/messages/42/reactions/heart", "DELETE " + prefix + "/b/messages/42/reactions/heart", "POST " + prefix + "/b/read", "POST " + prefix + "/b/typing"} {
		found := false
		for _, c := range calls {
			found = found || c == want
		}
		if !found {
			t.Errorf("missing %s", want)
		}
	}
}
func TestLogoutDiscardsInFlightHistory(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	h := newHarness(t, func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		select {
		case <-release:
		case <-r.Context().Done():
		}
		if _, err := fmt.Fprint(w, `{"messages":[{"text":"private"}]}`); err != nil {
			t.Error(err)
			return
		}
	})
	result := make(chan error, 1)
	go func() {
		p, err := h.svc.Messages("b", "")
		if len(p.Messages) > 0 {
			result <- fmt.Errorf("stale history escaped")
			return
		}
		result <- err
	}()
	<-entered
	h.svc.Stop()
	close(release)
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("logout did not cancel read")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("request not cancelled")
	}
	if _, err := h.svc.Conversations(); err == nil {
		t.Fatal("signed-out query allowed")
	}
}
func TestTokenChangeAndConsentStopIdleConnection(t *testing.T) {
	for _, withdraw := range []bool{false, true} {
		t.Run(fmt.Sprint(withdraw), func(t *testing.T) {
			h := newHarness(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected request") })
			awaitKind(t, h.events, "sync")
			h.svc.mu.Lock()
			ctx := h.svc.run.ctx
			h.svc.mu.Unlock()
			if withdraw {
				h.enabled.Store(false)
			} else {
				h.mu.Lock()
				h.token = "token-b"
				h.mu.Unlock()
			}
			select {
			case <-ctx.Done():
			case <-time.After(3 * time.Second):
				t.Fatal("old stream survived account/consent change")
			}
			if h.svc.Notify("b", "old", "private") {
				t.Fatal("stale notification allowed")
			}
			if e := awaitKind(t, h.events, "connection"); e.Connected || e.OwnerID != "a" {
				t.Fatalf("missing disconnect status: %+v", e)
			}
		})
	}
}
func TestConcurrentLogoutCancelsStart(t *testing.T) {
	entered := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-r.Context().Done() }))
	defer server.Close()
	s, err := NewService(server.URL, func() (string, error) { return "token", nil }, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Error(err)
		}
	}()
	result := make(chan error, 1)
	go func() { result <- s.Start() }()
	<-entered
	s.Stop()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("cancelled start succeeded")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("start ignored logout")
	}
}
func TestStreamReconnectSynchronisesWithoutReplayingNotifications(t *testing.T) {
	var connections atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/me" {
			if _, err := fmt.Fprint(w, `{"id":"a"}`); err != nil {
				t.Error(err)
				return
			}
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if _, err := fmt.Fprint(w, "data: {\"kind\":\"sync\"}\n\n"); err != nil {
			t.Error(err)
			return
		}
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Error(err)
			return
		}
		if connections.Add(1) == 1 {
			if _, err := fmt.Fprint(w, "data: {\"kind\":\"message\",\"peerId\":\"b\",\"message\":{\"id\":\"42\",\"senderId\":\"b\"}}\n\n"); err != nil {
				t.Error(err)
				return
			}
			return
		}
		<-r.Context().Done()
	}))
	defer server.Close()
	s, err := NewService(server.URL, func() (string, error) { return "token", nil }, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Error(err)
		}
	}()
	events := make(chan Event, 32)
	s.emit = func(e Event) { events <- e }
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	awaitKind(t, events, "sync")
	e := awaitKind(t, events, "message")
	if e.OwnerID != "a" || e.PeerID != "b" || e.Message.ID != "42" {
		t.Fatalf("event=%+v", e)
	}
	awaitKind(t, events, "sync")
	if connections.Load() != 2 {
		t.Fatalf("connections=%d", connections.Load())
	}
}
func TestPopupEscapesMessageAndToneIsBoundedPCM(t *testing.T) {
	page := popupHTML(`<img src=x onerror=alert(1)>`, `</p><script>steal()</script>`)
	if strings.Contains(page, "<img") || strings.Contains(page, "<script>steal") || !strings.Contains(page, "&lt;script&gt;") {
		t.Fatal("unsafe HTML")
	}
	wav := messageTone()
	if string(wav[:4]) != "RIFF" || string(wav[8:12]) != "WAVE" || int64(binary.LittleEndian.Uint32(wav[40:])) != int64(len(wav))-44 {
		t.Fatal("invalid PCM")
	}
	peak := int32(0)
	for i := 44; i < len(wav); i += 2 {
		v := int32(binary.LittleEndian.Uint16(wav[i:]))
		if v >= 1<<15 {
			v -= 1 << 16
		}
		if v < 0 {
			v = -v
		}
		if v > peak {
			peak = v
		}
	}
	if peak <= 0 || peak > 12000 {
		t.Fatalf("peak=%d", peak)
	}
}
