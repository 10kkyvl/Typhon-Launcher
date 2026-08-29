package account

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"typhon/internal/storage"
)

type fakeStore struct {
	mu      sync.Mutex
	cred    Credential
	present bool
	saveErr error
	loadErr error
	delErr  error
	deletes int
	saves   int
}

func (f *fakeStore) Load() (Credential, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.loadErr != nil {
		return Credential{}, f.loadErr
	}
	if !f.present {
		return Credential{}, ErrNoCredential
	}
	return f.cred, nil
}

func (f *fakeStore) Save(cred Credential) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saves++
	if f.saveErr != nil {
		return f.saveErr
	}
	f.cred = cred
	f.present = true
	return nil
}

func (f *fakeStore) Delete() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deletes++
	if f.delErr != nil {
		return f.delErr
	}
	f.cred = Credential{}
	f.present = false
	return nil
}

func (f *fakeStore) snapshot() (Credential, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cred, f.present
}

func statePathFor(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "account.json")
}

func startedService(t *testing.T, store CredentialStore, baseURL string) *Service {
	t.Helper()
	s, err := newService(store, baseURL, statePathFor(t))
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

func sampleUser() CurrentUser {
	return CurrentUser{
		ID:          "u1",
		Username:    "playerone",
		DisplayName: "Player One",
		Email:       "player@example.com",
		CreatedAt:   time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
	}
}

func writeSession(t *testing.T, w http.ResponseWriter, status int, token string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(Session{
		User:      sampleUser(),
		Token:     token,
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("encode session: %v", err)
	}
}

func TestBootstrapWithoutCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	store := &fakeStore{}
	state, err := startedService(t, store, srv.URL).Bootstrap()
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if state.Status != StatusUnauthenticated {
		t.Fatalf("status = %q, want %q", state.Status, StatusUnauthenticated)
	}
	if store.deletes != 0 {
		t.Errorf("credential deletes = %d, want 0", store.deletes)
	}
}

func TestBootstrapWithValidCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer stored-token" {
			t.Errorf("authorization = %q, want the stored token", got)
		}
		writeJSON(t, w, http.StatusOK, sampleUser())
	}))
	defer srv.Close()

	store := &fakeStore{cred: Credential{Token: "stored-token"}, present: true}
	state, err := startedService(t, store, srv.URL).Bootstrap()
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if state.Status != StatusAuthenticated {
		t.Fatalf("status = %q, want %q", state.Status, StatusAuthenticated)
	}
	if state.User.Username != "playerone" {
		t.Errorf("user = %+v, want playerone", state.User)
	}
	if _, present := store.snapshot(); !present {
		t.Error("credential was removed on a successful bootstrap")
	}
}

func TestBootstrapDiscardsRejectedCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusUnauthorized, map[string]any{
			"error": map[string]string{"code": "unauthenticated"},
		})
	}))
	defer srv.Close()

	store := &fakeStore{cred: Credential{Token: "revoked"}, present: true}
	state, err := startedService(t, store, srv.URL).Bootstrap()
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if state.Status != StatusUnauthenticated {
		t.Fatalf("status = %q, want %q", state.Status, StatusUnauthenticated)
	}
	if _, present := store.snapshot(); present {
		t.Error("rejected credential was kept")
	}
}

func TestBootstrapKeepsCredentialWhenBackendIsUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	unreachable := srv.URL
	srv.Close()

	store := &fakeStore{cred: Credential{Token: "still-valid"}, present: true}
	state, err := startedService(t, store, unreachable).Bootstrap()
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if state.Status != StatusUnavailable {
		t.Fatalf("status = %q, want %q", state.Status, StatusUnavailable)
	}
	if state.Reason != CodeNetwork {
		t.Errorf("reason = %q, want %q", state.Reason, CodeNetwork)
	}
	if cred, present := store.snapshot(); !present || cred.Token != "still-valid" {
		t.Error("credential was discarded because of a network failure")
	}
	if store.deletes != 0 {
		t.Errorf("credential deletes = %d, want 0", store.deletes)
	}
}

func TestBootstrapKeepsCredentialOnServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusInternalServerError, map[string]any{
			"error": map[string]string{"code": "internal"},
		})
	}))
	defer srv.Close()

	store := &fakeStore{cred: Credential{Token: "still-valid"}, present: true}
	state, err := startedService(t, store, srv.URL).Bootstrap()
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if state.Status != StatusUnavailable {
		t.Fatalf("status = %q, want %q", state.Status, StatusUnavailable)
	}
	if _, present := store.snapshot(); !present {
		t.Error("credential was discarded because of a server error")
	}
}

func TestBootstrapOfflineOnNetworkFailureWithCachedProfile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	unreachable := srv.URL
	srv.Close()

	store := &fakeStore{cred: Credential{Token: "still-valid"}, present: true}
	s := startedService(t, store, unreachable)
	cached := sampleUser()
	cached.ID = "cached-user"
	s.mu.Lock()
	s.profile = cachedProfile{
		User:      cached,
		Avatar:    AvatarImage{Data: "YWJj", MIME: "image/png"},
		AvatarURL: "https://cdn.example/a.png",
	}
	s.mu.Unlock()

	state, err := s.Bootstrap()
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if state.Status != StatusOffline {
		t.Fatalf("status = %q, want %q", state.Status, StatusOffline)
	}
	if state.User.ID != "cached-user" {
		t.Fatalf("user = %+v, want the cached user", state.User)
	}
	if want := "data:image/png;base64,YWJj"; state.User.AvatarURL != want {
		t.Fatalf("avatar url = %q, want %q", state.User.AvatarURL, want)
	}
	if state.Reason != CodeNetwork {
		t.Fatalf("reason = %q, want %q", state.Reason, CodeNetwork)
	}
	if _, present := store.snapshot(); !present {
		t.Error("credential was discarded while offline")
	}
}

func TestBootstrapOfflineOnServerErrorWithCachedProfile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	store := &fakeStore{cred: Credential{Token: "still-valid"}, present: true}
	s := startedService(t, store, srv.URL)
	s.mu.Lock()
	s.profile = cachedProfile{User: sampleUser()}
	s.mu.Unlock()

	state, err := s.Bootstrap()
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if state.Status != StatusOffline {
		t.Fatalf("status = %q, want %q", state.Status, StatusOffline)
	}
	if state.User.ID != sampleUser().ID {
		t.Fatalf("user = %+v, want the cached user", state.User)
	}
}

func TestBootstrapStaysUnavailableWithCacheOnCodedErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		code   string
	}{
		{"rate limited", http.StatusTooManyRequests, CodeRateLimited},
		{"launcher outdated", http.StatusUpgradeRequired, CodeOutdated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			}))
			defer srv.Close()

			store := &fakeStore{cred: Credential{Token: "still-valid"}, present: true}
			s := startedService(t, store, srv.URL)
			s.mu.Lock()
			s.profile = cachedProfile{User: sampleUser()}
			s.mu.Unlock()

			state, err := s.Bootstrap()
			if err != nil {
				t.Fatalf("Bootstrap() error = %v", err)
			}
			if state.Status != StatusUnavailable {
				t.Fatalf("status = %q, want %q even though a profile is cached", state.Status, StatusUnavailable)
			}
			if state.Reason != tt.code {
				t.Fatalf("reason = %q, want %q", state.Reason, tt.code)
			}
		})
	}
}

func TestBootstrapUnauthenticatedDeletesCredentialAndProfileCache(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusUnauthorized, map[string]any{
			"error": map[string]string{"code": "unauthenticated"},
		})
	}))
	defer srv.Close()

	store := &fakeStore{cred: Credential{Token: "revoked"}, present: true}
	s := startedService(t, store, srv.URL)
	if err := s.setProfile(s.profileEpoch, cachedProfile{User: sampleUser()}); err != nil {
		t.Fatalf("seed profile cache: %v", err)
	}
	if _, err := os.Stat(s.profilePath); err != nil {
		t.Fatalf("profile cache file missing before bootstrap: %v", err)
	}

	state, err := s.Bootstrap()
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if state.Status != StatusUnauthenticated {
		t.Fatalf("status = %q, want %q", state.Status, StatusUnauthenticated)
	}
	if store.deletes != 1 {
		t.Fatalf("credential deletes = %d, want 1", store.deletes)
	}
	if _, err := os.Stat(s.profilePath); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("profile cache file still exists after an unauthenticated bootstrap")
	}
	if got := s.currentProfile(); got.User.ID != "" {
		t.Fatalf("profile cache in memory = %+v, want cleared", got)
	}
}

func TestBootstrapCachesProfileOnSuccessAndAvoidsRefetch(t *testing.T) {
	avatarRequests := 0
	avatarSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		avatarRequests++
		if _, err := w.Write([]byte("\x89PNG\r\n\x1a\n")); err != nil {
			t.Error(err)
		}
	}))
	defer avatarSrv.Close()

	user := sampleUser()
	user.AvatarURL = avatarSrv.URL + "/a.png"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, user)
	}))
	defer srv.Close()

	store := &fakeStore{cred: Credential{Token: "tok"}, present: true}
	s := startedService(t, store, srv.URL)

	if _, err := s.Bootstrap(); err != nil {
		t.Fatalf("Bootstrap() #1 error = %v", err)
	}
	if _, err := s.Bootstrap(); err != nil {
		t.Fatalf("Bootstrap() #2 error = %v", err)
	}

	if avatarRequests != 1 {
		t.Fatalf("avatar requests = %d, want 1 (a repeat bootstrap must reuse the cache)", avatarRequests)
	}
	cached := s.currentProfile()
	if cached.User.ID != user.ID || cached.Avatar.Data == "" {
		t.Fatalf("cached profile = %+v, want the user with a cached avatar", cached)
	}

	var onDisk cachedProfile
	if err := storage.Load(s.profilePath, profileVersion, nil, &onDisk); err != nil {
		t.Fatalf("read cached profile from disk: %v", err)
	}
	if onDisk.User.ID != user.ID {
		t.Fatalf("cache on disk = %+v, want the user persisted", onDisk)
	}
}

func TestBootstrapAvatarURLChangeWithFailedDownloadClearsAvatar(t *testing.T) {
	failingAvatarSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer failingAvatarSrv.Close()

	user := sampleUser()
	user.AvatarURL = failingAvatarSrv.URL + "/new.png"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, user)
	}))
	defer srv.Close()

	store := &fakeStore{cred: Credential{Token: "tok"}, present: true}
	s := startedService(t, store, srv.URL)
	s.mu.Lock()
	s.profile = cachedProfile{
		User:      sampleUser(),
		Avatar:    AvatarImage{Data: "b2xk", MIME: "image/png"},
		AvatarURL: "https://cdn.example/old.png",
	}
	s.mu.Unlock()

	state, err := s.Bootstrap()
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if state.Status != StatusAuthenticated {
		t.Fatalf("status = %q, want %q", state.Status, StatusAuthenticated)
	}

	cached := s.currentProfile()
	if cached.User.ID != user.ID {
		t.Fatalf("cached user = %+v, want %+v", cached.User, user)
	}
	if cached.Avatar.Data != "" || cached.AvatarURL != "" {
		t.Fatalf("cached avatar = %+v url=%q, want cleared after a failed download for the new url", cached.Avatar, cached.AvatarURL)
	}
}

func TestLogoutRemovesProfileCacheFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	store := &fakeStore{cred: Credential{Token: "live"}, present: true}
	s := startedService(t, store, srv.URL)
	if err := s.setProfile(s.profileEpoch, cachedProfile{User: sampleUser()}); err != nil {
		t.Fatalf("seed profile cache: %v", err)
	}

	if err := s.Logout(); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if _, err := os.Stat(s.profilePath); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("profile cache file still exists after logout")
	}
}

func TestRegisterAndLoginStoreTheSession(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		status int
		call   func(s *Service) (CurrentUser, error)
	}{
		{
			name:   "register",
			path:   APIPrefix + "/auth/register",
			status: http.StatusCreated,
			call: func(s *Service) (CurrentUser, error) {
				return s.Register(RegisterInput{
					Email:       "player@example.com",
					Username:    "playerone",
					DisplayName: "Player One",
					Password:    "  secret pass  ",
				})
			},
		},
		{
			name:   "login",
			path:   APIPrefix + "/auth/login",
			status: http.StatusOK,
			call: func(s *Service) (CurrentUser, error) {
				return s.Login(LoginInput{Identifier: "PLAYERONE", Password: "  secret pass  "})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var received map[string]string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tt.path || r.Method != http.MethodPost {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				if got := r.Header.Get("Authorization"); got != "" {
					t.Errorf("authorization = %q, want no header on %s", got, tt.path)
				}
				if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
					t.Errorf("decode request body: %v", err)
				}
				writeSession(t, w, tt.status, "fresh-token")
			}))
			defer srv.Close()

			store := &fakeStore{}
			user, err := tt.call(startedService(t, store, srv.URL))
			if err != nil {
				t.Fatalf("%s error = %v", tt.name, err)
			}
			if user.Username != "playerone" {
				t.Errorf("user = %+v, want playerone", user)
			}
			if received["password"] != "  secret pass  " {
				t.Errorf("password = %q, want it sent untrimmed", received["password"])
			}

			cred, present := store.snapshot()
			if !present {
				t.Fatal("session was not stored")
			}
			if cred.Token != "fresh-token" {
				t.Errorf("stored token = %q, want fresh-token", cred.Token)
			}
			if cred.Username != "playerone" {
				t.Errorf("stored username = %q, want playerone", cred.Username)
			}
		})
	}
}

func TestLoginRevokesSessionWhenCredentialWriteFails(t *testing.T) {
	var logouts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case APIPrefix + "/auth/login":
			writeSession(t, w, http.StatusOK, "orphan-token")
		case APIPrefix + "/auth/logout":
			logouts = append(logouts, r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	store := &fakeStore{saveErr: errors.New("keyring is locked")}
	_, err := startedService(t, store, srv.URL).Login(LoginInput{Identifier: "playerone", Password: "password"})
	if err == nil {
		t.Fatal("Login() error = nil, want the credential write failure")
	}
	if !strings.Contains(err.Error(), "keyring is locked") {
		t.Errorf("error = %v, want it to mention the storage failure", err)
	}
	if len(logouts) != 1 || logouts[0] != "Bearer orphan-token" {
		t.Fatalf("logout calls = %v, want one revoke of the orphan token", logouts)
	}
	if _, present := store.snapshot(); present {
		t.Error("a credential was stored despite the write failure")
	}
}

func TestLoginKeepsTheSessionWhenTheProfileCacheCannotBeWritten(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != APIPrefix+"/auth/login" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		writeSession(t, w, http.StatusOK, "fresh-token")
	}))
	defer srv.Close()

	store := &fakeStore{}
	s := startedService(t, store, srv.URL)

	blocked := filepath.Join(t.TempDir(), "profile.json")
	if err := os.Mkdir(blocked, 0o755); err != nil {
		t.Fatalf("mkdir profile path: %v", err)
	}
	if err := os.WriteFile(filepath.Join(blocked, "keep.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("seed non-empty dir: %v", err)
	}
	s.profilePath = blocked

	user, err := s.Login(LoginInput{Identifier: "playerone", Password: "password"})
	if err != nil {
		t.Fatalf("Login() error = %v, want a stored session to survive a profile cache failure", err)
	}
	if user.Username != "playerone" {
		t.Errorf("user = %+v, want playerone", user)
	}

	cred, present := store.snapshot()
	if !present || cred.Token != "fresh-token" {
		t.Fatalf("stored credential = %+v present = %v, want the session kept", cred, present)
	}
	if got := s.currentProfile(); got.User.ID != "" {
		t.Errorf("cached profile = %+v, want it dropped rather than left stale", got)
	}
}

func TestLogout(t *testing.T) {
	t.Run("revokes the session and clears the credential", func(t *testing.T) {
		var authorization string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != APIPrefix+"/auth/logout" || r.Method != http.MethodPost {
				t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			}
			authorization = r.Header.Get("Authorization")
			w.WriteHeader(http.StatusNoContent)
		}))
		defer srv.Close()

		store := &fakeStore{cred: Credential{Token: "live-token"}, present: true}
		if err := startedService(t, store, srv.URL).Logout(); err != nil {
			t.Fatalf("Logout() error = %v", err)
		}
		if authorization != "Bearer live-token" {
			t.Errorf("authorization = %q, want the stored token", authorization)
		}
		if _, present := store.snapshot(); present {
			t.Error("credential survived logout")
		}
	})

	t.Run("clears the credential when the session is already gone", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(t, w, http.StatusUnauthorized, map[string]any{
				"error": map[string]string{"code": "unauthenticated"},
			})
		}))
		defer srv.Close()

		store := &fakeStore{cred: Credential{Token: "stale"}, present: true}
		if err := startedService(t, store, srv.URL).Logout(); err != nil {
			t.Fatalf("Logout() error = %v", err)
		}
		if _, present := store.snapshot(); present {
			t.Error("credential survived logout")
		}
	})

	t.Run("reports a delete failure", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
		defer srv.Close()

		store := &fakeStore{cred: Credential{Token: "live"}, present: true, delErr: errors.New("access denied")}
		err := startedService(t, store, srv.URL).Logout()
		if err == nil {
			t.Fatal("Logout() error = nil, want the delete failure")
		}
		if !strings.Contains(err.Error(), "access denied") {
			t.Errorf("error = %v, want it to mention the delete failure", err)
		}
	})
}

func TestServiceRejectsCallsBeforeStartup(t *testing.T) {
	s, err := newService(&fakeStore{}, "http://127.0.0.1:1", statePathFor(t))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	if _, err := s.GetCurrentUser(); !errors.Is(err, errNotStarted) {
		t.Errorf("GetCurrentUser() error = %v, want errNotStarted", err)
	}
	if _, err := s.Login(LoginInput{Identifier: "a", Password: "b"}); !errors.Is(err, errNotStarted) {
		t.Errorf("Login() error = %v, want errNotStarted", err)
	}
}

func TestBootstrapAbortsOnCancelledContext(t *testing.T) {
	blocked := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		<-blocked
	}))
	defer srv.Close()
	defer close(blocked)

	store := &fakeStore{cred: Credential{Token: "token"}, present: true}
	s, err := newService(store, srv.URL, statePathFor(t))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	if err := s.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
		t.Fatalf("service startup: %v", err)
	}
	cancel()

	state, err := s.Bootstrap()
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if state.Status != StatusUnavailable {
		t.Fatalf("status = %q, want %q", state.Status, StatusUnavailable)
	}
	if _, present := store.snapshot(); !present {
		t.Error("credential was discarded because the context was cancelled")
	}
}
