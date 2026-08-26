package account

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func serviceAt(t *testing.T, store CredentialStore, baseURL, path string) *Service {
	t.Helper()
	s, err := newService(store, baseURL, path)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if err := s.ServiceStartup(t.Context(), application.ServiceOptions{}); err != nil {
		t.Fatalf("service startup: %v", err)
	}
	t.Cleanup(func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Errorf("service shutdown: %v", err)
		}
	})
	return s
}

func TestContinueAsGuestSurvivesRestart(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Errorf("guest mode must not call the backend, got %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "account.json")

	first := serviceAt(t, &fakeStore{}, srv.URL, path)
	state, err := first.ContinueAsGuest()
	if err != nil {
		t.Fatalf("ContinueAsGuest() error = %v", err)
	}
	if state.Status != StatusGuest {
		t.Fatalf("status = %q, want %q", state.Status, StatusGuest)
	}

	restarted := serviceAt(t, &fakeStore{}, srv.URL, path)
	state, err = restarted.Bootstrap()
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if state.Status != StatusGuest {
		t.Fatalf("status after restart = %q, want %q", state.Status, StatusGuest)
	}
	if state.User.ID != "" {
		t.Errorf("guest state carries a user: %+v", state.User)
	}
}

func TestBootstrapWithoutGuestMarker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	state, err := serviceAt(t, &fakeStore{}, srv.URL, filepath.Join(t.TempDir(), "account.json")).Bootstrap()
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if state.Status != StatusUnauthenticated {
		t.Fatalf("status = %q, want %q", state.Status, StatusUnauthenticated)
	}
}

func TestStoredCredentialWinsOverGuestMarker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, sampleUser())
	}))
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "account.json")
	guest := serviceAt(t, &fakeStore{}, srv.URL, path)
	if _, err := guest.ContinueAsGuest(); err != nil {
		t.Fatalf("ContinueAsGuest() error = %v", err)
	}

	signedIn := serviceAt(t, &fakeStore{cred: Credential{Token: "tok"}, present: true}, srv.URL, path)
	state, err := signedIn.Bootstrap()
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if state.Status != StatusAuthenticated {
		t.Fatalf("status = %q, want %q", state.Status, StatusAuthenticated)
	}
}

func TestLeavingGuestModeClearsTheMarker(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, s *Service) error
	}{
		{
			name: "logout",
			run:  func(_ *testing.T, s *Service) error { return s.Logout() },
		},
		{
			name: "login",
			run: func(_ *testing.T, s *Service) error {
				_, err := s.Login(LoginInput{Identifier: "playerone", Password: "password"})
				return err
			},
		},
		{
			name: "register",
			run: func(_ *testing.T, s *Service) error {
				_, err := s.Register(RegisterInput{
					Email:       "player@example.com",
					Username:    "playerone",
					DisplayName: "Player One",
					Password:    "password",
				})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case APIPrefix + "/auth/logout":
					w.WriteHeader(http.StatusNoContent)
				case APIPrefix + "/auth/register":
					writeSession(t, w, http.StatusCreated, "fresh-token")
				default:
					writeSession(t, w, http.StatusOK, "fresh-token")
				}
			}))
			defer srv.Close()

			path := filepath.Join(t.TempDir(), "account.json")
			store := &fakeStore{}
			s := serviceAt(t, store, srv.URL, path)
			if _, err := s.ContinueAsGuest(); err != nil {
				t.Fatalf("ContinueAsGuest() error = %v", err)
			}

			if err := tt.run(t, s); err != nil {
				t.Fatalf("%s error = %v", tt.name, err)
			}
			if s.isGuest() {
				t.Error("guest marker survived in memory")
			}

			restarted := serviceAt(t, &fakeStore{}, srv.URL, path)
			if restarted.isGuest() {
				t.Error("guest marker survived on disk")
			}
		})
	}
}

func TestLoginRefusesWhenGuestMarkerCannotBeCleared(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Errorf("login must not reach the backend, got %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write blocker file: %v", err)
	}

	s := serviceAt(t, &fakeStore{}, srv.URL, filepath.Join(blocker, "account.json"))
	s.mu.Lock()
	s.guest = true
	s.mu.Unlock()

	if _, err := s.Login(LoginInput{Identifier: "playerone", Password: "password"}); err == nil {
		t.Fatal("Login() error = nil, want the state write failure")
	}
	if !s.isGuest() {
		t.Error("guest marker was cleared in memory despite the failed write")
	}
}

func TestLoadStateRejectsBrokenFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "account.json")
	if err := os.WriteFile(path, []byte("{ not json"), 0o600); err != nil {
		t.Fatalf("write broken state: %v", err)
	}

	if _, err := loadState(path); err == nil {
		t.Fatal("loadState() error = nil, want a parse error")
	}

	if _, err := newService(&fakeStore{}, "http://127.0.0.1:1", path); err == nil {
		t.Fatal("newService() error = nil, want it to refuse to start on a broken state file")
	}
}

func TestLoadStateTreatsMissingFileAsEmpty(t *testing.T) {
	loaded, err := loadState(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Fatalf("loadState() error = %v, want nil", err)
	}
	if loaded.Guest {
		t.Error("missing state file reported guest mode")
	}
}

func TestNewServiceRejectsEmptyStatePath(t *testing.T) {
	if _, err := newService(&fakeStore{}, "http://127.0.0.1:1", ""); err == nil {
		t.Fatal("newService() error = nil, want an error for an empty state path")
	}
}
