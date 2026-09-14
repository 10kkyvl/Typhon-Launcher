package messaging

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestStreamStopsOnPermanentHTTPFailure(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/me" {
					if _, err := fmt.Fprint(w, `{"id":"a"}`); err != nil {
						t.Errorf("write chat response: %v", err)
					}
					return
				}
				requests.Add(1)
				w.WriteHeader(status)
				if _, err := fmt.Fprint(w, `{"error":{"code":"chat_disabled"}}`); err != nil {
					t.Errorf("write chat response: %v", err)
				}
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
					t.Errorf("shutdown chat service: %v", err)
				}
			}()
			events := make(chan Event, 32)
			s.emit = func(e Event) { events <- e }
			if err = s.Start(); err != nil {
				t.Fatal(err)
			}
			s.mu.Lock()
			ctx := s.run.ctx
			s.mu.Unlock()
			select {
			case <-ctx.Done():
			case <-time.After(3 * time.Second):
				t.Fatal("permanent failure kept retrying")
			}
			s.wg.Wait()
			if requests.Load() != 1 {
				t.Fatalf("requests = %d", requests.Load())
			}
			if e := awaitKind(t, events, "connection"); e.Connected || e.OwnerID != "a" {
				t.Fatalf("missing disconnected status: %+v", e)
			}
		})
	}
}

func TestStreamReconnectsAfterTransientHTTPFailure(t *testing.T) {
	for _, status := range []int{http.StatusRequestTimeout, http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/me" {
					if _, err := fmt.Fprint(w, `{"id":"a"}`); err != nil {
						t.Errorf("write chat response: %v", err)
					}
					return
				}
				if requests.Add(1) == 1 {
					w.WriteHeader(status)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if _, err := fmt.Fprint(w, "data: {\"kind\":\"sync\"}\n\n"); err != nil {
					t.Errorf("write chat response: %v", err)
				}
				if err := http.NewResponseController(w).Flush(); err != nil {
					t.Errorf("flush chat stream: %v", err)
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
					t.Errorf("shutdown chat service: %v", err)
				}
			}()
			events := make(chan Event, 32)
			s.emit = func(e Event) { events <- e }
			if err = s.Start(); err != nil {
				t.Fatal(err)
			}
			awaitKind(t, events, "sync")
			if requests.Load() != 2 {
				t.Fatalf("requests = %d", requests.Load())
			}
		})
	}
}

func TestResponseErrorPreservesCodeAndStatus(t *testing.T) {
	for _, body := range []string{`{"error":{"code":"chat_disabled"}}`, "not JSON"} {
		err := responseError(&http.Response{StatusCode: http.StatusForbidden, Body: io.NopCloser(strings.NewReader(body))})
		want := "chat_server_error_403"
		if strings.HasPrefix(body, "{") {
			want = "chat_disabled"
		}
		if err.Error() != want || !permanentStreamError(err) {
			t.Fatalf("response error = %v", err)
		}
	}
}
