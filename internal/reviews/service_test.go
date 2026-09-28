package reviews

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	"typhon/internal/account"
)

func testResolver(known map[string]string) func(string) string {
	return func(id string) string { return known[id] }
}

func newTestService(t *testing.T, handler http.HandlerFunc, resolve func(string) string) *Service {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	s, err := NewService(srv.URL, staticToken("tok"), resolve)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if err := s.ServiceStartup(t.Context(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	t.Cleanup(func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Errorf("ServiceShutdown: %v", err)
		}
	})
	return s
}

func TestNewService_RejectsNilResolver(t *testing.T) {
	if _, err := NewService("https://api.example.com", staticToken("tok"), nil); err == nil {
		t.Fatal("want an error for a nil resolveIGDBID callback")
	}
}

func TestNewService_RejectsBadBaseURL(t *testing.T) {
	if _, err := NewService("http://example.com", staticToken("tok"), testResolver(nil)); err == nil {
		t.Fatal("want an error for an insecure base url")
	}
}

func TestService_Limits(t *testing.T) {
	s, err := NewService("https://api.example.com", staticToken("tok"), testResolver(nil))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	limits := s.Limits()
	if limits.MinBodyRunes != 20 || limits.MaxBodyRunes != 5000 || limits.MinPlaytimeSeconds != 1800 {
		t.Fatalf("limits = %+v", limits)
	}
}

func TestService_UnknownGameRejectsWithoutRequest(t *testing.T) {
	called := false
	s := newTestService(t, func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}, testResolver(map[string]string{"known": "1942", "junk": "0"}))

	tests := []struct {
		name string
		call func() error
	}{
		{"list empty id", func() error { _, err := s.List("", "helpful", "all", ""); return err }},
		{"list unresolved id", func() error { _, err := s.List("missing", "helpful", "all", ""); return err }},
		{"list junk id", func() error { _, err := s.List("junk", "helpful", "all", ""); return err }},
		{"mine unresolved id", func() error { _, err := s.Mine("missing"); return err }},
		{"save unresolved id", func() error { _, err := s.Save("missing", true, "x"); return err }},
		{"delete unresolved id", func() error { return s.Delete("missing") }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			called = false
			if err := tc.call(); !errors.Is(err, errUnknownGame) {
				t.Fatalf("error = %v, want unknown_game", err)
			}
			if called {
				t.Error("request sent for an unresolvable game id")
			}
		})
	}
}

func TestService_ListValidatesEnums(t *testing.T) {
	called := false
	s := newTestService(t, func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, `{"summary":{"total":0,"positive":0},"reviews":[],"next":""}`); err != nil {
			t.Errorf("write response: %v", err)
		}
	}, testResolver(map[string]string{"g": "1942"}))

	tests := []struct {
		name   string
		sort   string
		filter string
	}{
		{"bad sort", "newest", "all"},
		{"bad filter", "helpful", "everything"},
		{"empty sort", "", "all"},
		{"empty filter", "helpful", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			called = false
			if _, err := s.List("g", tc.sort, tc.filter, ""); !errors.Is(err, errBadRequest) {
				t.Fatalf("error = %v, want bad_request", err)
			}
			if called {
				t.Error("request sent for an invalid sort/filter")
			}
		})
	}

	called = false
	if _, err := s.List("g", "helpful", "all", ""); err != nil {
		t.Fatalf("valid list: %v", err)
	}
	if !called {
		t.Error("valid sort/filter never reached the server")
	}
}

func TestService_VoteValidatesInput(t *testing.T) {
	called := false
	s := newTestService(t, func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, `{"helpful":1,"unhelpful":0,"myVote":"helpful"}`); err != nil {
			t.Errorf("write response: %v", err)
		}
	}, testResolver(nil))

	tests := []struct {
		name     string
		reviewID int64
		vote     string
	}{
		{"zero id", 0, "helpful"},
		{"negative id", -1, "helpful"},
		{"bad vote", 1, "maybe"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			called = false
			if _, err := s.Vote(tc.reviewID, tc.vote); !errors.Is(err, errBadRequest) {
				t.Fatalf("error = %v, want bad_request", err)
			}
			if called {
				t.Error("request sent for invalid vote input")
			}
		})
	}

	called = false
	if _, err := s.Vote(1, ""); err != nil {
		t.Fatalf("clear vote: %v", err)
	}
	if !called {
		t.Error("valid clear-vote never reached the server")
	}
}

func TestService_ReportValidatesInput(t *testing.T) {
	called := false
	s := newTestService(t, func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}, testResolver(nil))

	tests := []struct {
		name     string
		reviewID int64
		reason   string
	}{
		{"zero id", 0, "spam"},
		{"negative id", -3, "spam"},
		{"bad reason", 1, "nonsense"},
		{"empty reason", 1, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			called = false
			if err := s.Report(tc.reviewID, tc.reason); !errors.Is(err, errBadRequest) {
				t.Fatalf("error = %v, want bad_request", err)
			}
			if called {
				t.Error("request sent for invalid report input")
			}
		})
	}

	called = false
	if err := s.Report(1, "spam"); err != nil {
		t.Fatalf("valid report: %v", err)
	}
	if !called {
		t.Error("valid report never reached the server")
	}
}

func TestService_NotStartedRejectsRequests(t *testing.T) {
	s, err := NewService("https://api.example.com", staticToken("tok"), testResolver(map[string]string{"g": "1942"}))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if _, err := s.List("g", "helpful", "all", ""); !errors.Is(err, errNotStarted) {
		t.Fatalf("error = %v, want errNotStarted", err)
	}
}

func TestService_ListWorksForGuestEndToEnd(t *testing.T) {
	var sawAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization") != ""
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, `{"summary":{"total":0,"positive":0},"reviews":[],"next":""}`); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer srv.Close()

	s, err := NewService(srv.URL, func() (string, error) { return "", account.ErrNoCredential }, testResolver(map[string]string{"g": "1942"}))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if err := s.ServiceStartup(t.Context(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	defer func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Errorf("ServiceShutdown: %v", err)
		}
	}()

	if _, err := s.List("g", "helpful", "all", ""); err != nil {
		t.Fatalf("List: %v", err)
	}
	if sawAuth {
		t.Error("guest List sent Authorization")
	}
}

func TestService_MineRequiresAuthorizationEndToEnd(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	}))
	defer srv.Close()

	s, err := NewService(srv.URL, func() (string, error) { return "", account.ErrNoCredential }, testResolver(map[string]string{"g": "1942"}))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if err := s.ServiceStartup(t.Context(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	defer func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Errorf("ServiceShutdown: %v", err)
		}
	}()

	if _, err := s.Mine("g"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("error = %v, want ErrUnauthorized", err)
	}
	if called {
		t.Error("request sent for a guest caller on an auth-required endpoint")
	}
}

func TestService_ConcurrentRequestsAndShutdownRace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, `{"summary":{"total":0,"positive":0},"reviews":[],"next":""}`); err != nil {
			return
		}
	}))
	defer srv.Close()

	s, err := NewService(srv.URL, staticToken("tok"), testResolver(map[string]string{"g": "1942"}))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if err := s.ServiceStartup(t.Context(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.List("g", "helpful", "all", ""); err != nil {
				t.Errorf("List: %v", err)
			}
		}()
	}
	wg.Wait()

	if err := s.ServiceShutdown(); err != nil {
		t.Fatalf("ServiceShutdown: %v", err)
	}
}

func TestService_ErrorsReachTheUIAsCodes(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		want    string
		wantErr func(error) bool
	}{
		{
			name:    "unauthorized",
			status:  http.StatusUnauthorized,
			body:    `{"error":{"code":"unauthenticated"}}`,
			want:    "unauthenticated",
			wantErr: func(err error) bool { return errors.Is(err, ErrUnauthorized) },
		},
		{
			name:   "server error",
			status: http.StatusBadGateway,
			body:   `{"error":{"code":"internal"}}`,
			want:   "server_error",
			wantErr: func(err error) bool {
				var s *ServerError
				return errors.As(err, &s) && s.Status == http.StatusBadGateway
			},
		},
		{
			name:   "api error keeps the backend code",
			status: http.StatusTooManyRequests,
			body:   `{"error":{"code":"review_post_cooldown"}}`,
			want:   "review_post_cooldown",
			wantErr: func(err error) bool {
				var a *APIError
				return errors.As(err, &a) && a.Status == http.StatusTooManyRequests
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestService(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				if _, err := io.WriteString(w, tc.body); err != nil {
					t.Errorf("write response: %v", err)
				}
			}, testResolver(map[string]string{"g": "1942"}))
			_, err := s.Save("g", true, "text")
			if err == nil || err.Error() != tc.want {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
			if !tc.wantErr(err) {
				t.Fatalf("error %v lost its cause", err)
			}
		})
	}
}

func TestService_NetworkFailureReachesTheUIAsCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()
	s, err := NewService(url, staticToken("tok"), testResolver(map[string]string{"g": "1942"}))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if err := s.ServiceStartup(t.Context(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	t.Cleanup(func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Errorf("ServiceShutdown: %v", err)
		}
	})
	_, err = s.List("g", "helpful", "all", "")
	var network *NetworkError
	if err == nil || err.Error() != "network_error" || !errors.As(err, &network) {
		t.Fatalf("error = %v, want network_error wrapping NetworkError", err)
	}
}
