package account

import (
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func quietServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestSessionTokenTellsAMissingSessionFromABrokenVault(t *testing.T) {
	vaultErr := errors.New("vault locked")
	cases := []struct {
		name    string
		store   *fakeStore
		want    string
		wantErr error
	}{
		{"nothing stored", &fakeStore{}, "", nil},
		{"stored token", &fakeStore{cred: Credential{Token: "abc"}, present: true}, "abc", nil},
		{"vault failure", &fakeStore{loadErr: vaultErr}, "", vaultErr},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := quietServer(t)
			s := startedService(t, tc.store, srv.URL)
			got, err := s.SessionToken()
			if got != tc.want {
				t.Fatalf("token = %q, want %q", got, tc.want)
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestBootstrapVaultFailureIsAnErrorNotASignOut(t *testing.T) {
	srv, hits := quietServer(t)
	store := &fakeStore{cred: Credential{Token: "kept"}, present: true, loadErr: errors.New("vault locked")}
	s := startedService(t, store, srv.URL)

	state, err := s.Bootstrap()
	if err == nil {
		t.Fatalf("a vault that cannot be read reported state %+v", state)
	}
	if state.Status != "" {
		t.Fatalf("state = %+v alongside the error", state)
	}
	if store.deletes != 0 {
		t.Fatalf("credential deletes = %d, a read failure must not discard the session", store.deletes)
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("server saw %d requests without a readable token", n)
	}
}

func TestBootstrapEmptyStoredTokenIsSignedOut(t *testing.T) {
	srv, hits := quietServer(t)
	store := &fakeStore{cred: Credential{Username: "playerone"}, present: true}
	state, err := startedService(t, store, srv.URL).Bootstrap()
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if state.Status != StatusUnauthenticated {
		t.Fatalf("status = %q, want %q", state.Status, StatusUnauthenticated)
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("server saw %d requests for a session with no token", n)
	}
}

func TestBootstrapReportsAFailureToDiscardARejectedCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusUnauthorized, map[string]any{
			"error": map[string]string{"code": "unauthenticated"},
		})
	}))
	defer srv.Close()

	store := &fakeStore{cred: Credential{Token: "revoked"}, present: true, delErr: errors.New("access denied")}
	state, err := startedService(t, store, srv.URL).Bootstrap()
	if err == nil {
		t.Fatalf("a credential that could not be discarded reported state %+v", state)
	}
	if state.Status == StatusUnauthenticated {
		t.Fatal("reported signed out while the rejected token is still stored")
	}
}

func TestGetCurrentUserSeparatesNoSessionFromABrokenVault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer live" {
			t.Errorf("authorization = %q", got)
		}
		writeJSON(t, w, http.StatusOK, sampleUser())
	}))
	defer srv.Close()

	t.Run("signed in", func(t *testing.T) {
		store := &fakeStore{cred: Credential{Token: "live"}, present: true}
		user, err := startedService(t, store, srv.URL).GetCurrentUser()
		if err != nil || user.Username != "playerone" {
			t.Fatalf("user = %+v, err = %v", user, err)
		}
	})
	t.Run("no session", func(t *testing.T) {
		_, err := startedService(t, &fakeStore{}, srv.URL).GetCurrentUser()
		if got := codeOf(t, err); got != CodeUnauthenticated {
			t.Fatalf("code = %q, want %q", got, CodeUnauthenticated)
		}
	})
	t.Run("vault failure", func(t *testing.T) {
		store := &fakeStore{loadErr: errors.New("vault locked")}
		_, err := startedService(t, store, srv.URL).GetCurrentUser()
		if got := codeOf(t, err); got != CodeServer {
			t.Fatalf("code = %q, want %q", got, CodeServer)
		}
	})
}

func TestLogoutClearsTheLocalSessionEvenWhenTheServerCannotBeReached(t *testing.T) {
	cases := []struct {
		name   string
		status int
		code   string
	}{
		{"server error", http.StatusInternalServerError, CodeServer},
		{"rate limited", http.StatusTooManyRequests, CodeRateLimited},
		{"server gone", 0, CodeNetwork},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
			}))
			url := srv.URL
			if tc.status == 0 {
				srv.Close()
			} else {
				defer srv.Close()
			}

			store := &fakeStore{cred: Credential{Token: "live"}, present: true}
			s := startedService(t, store, url)
			s.profile = cachedProfile{User: sampleUser()}

			err := s.Logout()
			if err == nil {
				t.Fatal("a session that could not be revoked on the server was reported as closed cleanly")
			}
			if got := codeOf(t, err); got != tc.code {
				t.Fatalf("code = %q, want %q", got, tc.code)
			}
			if _, present := store.snapshot(); present {
				t.Fatal("the credential survived a failed revoke, so the user stays signed in")
			}
			if got := s.currentProfile(); got.User.ID != "" {
				t.Fatalf("cached profile survived logout: %+v", got)
			}
		})
	}
}

func TestLogoutReportsEveryFailureAtOnce(t *testing.T) {
	srv, _ := quietServer(t)
	deleteErr := errors.New("access denied")
	store := &fakeStore{cred: Credential{Token: "live"}, present: true, delErr: deleteErr}
	s := startedService(t, store, srv.URL)

	err := s.Logout()
	if err == nil {
		t.Fatal("expected an error")
	}
	if !errors.Is(err, deleteErr) {
		t.Fatalf("error = %v, want the credential delete failure", err)
	}
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != CodeServer {
		t.Fatalf("error = %v, want the failed revoke as well", err)
	}
}

func TestLogoutWithoutAStoredSessionNeedsNoServer(t *testing.T) {
	srv, hits := quietServer(t)
	store := &fakeStore{}
	if err := startedService(t, store, srv.URL).Logout(); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("server saw %d requests with nothing to revoke", n)
	}
}

func TestLogoutRefusesWhenTheVaultCannotBeRead(t *testing.T) {
	srv, hits := quietServer(t)
	store := &fakeStore{cred: Credential{Token: "live"}, present: true, loadErr: errors.New("vault locked")}
	if err := startedService(t, store, srv.URL).Logout(); err == nil {
		t.Fatal("Logout reported success without being able to read the session")
	}
	if store.deletes != 0 || hits.Load() != 0 {
		t.Fatalf("deletes = %d, requests = %d, nothing should happen on a read failure", store.deletes, hits.Load())
	}
}

func TestContinueAsGuestFailureLeavesNoGuestBehind(t *testing.T) {
	srv, _ := quietServer(t)
	dir := filepath.Join(t.TempDir(), "state")
	s, err := newService(&fakeStore{}, srv.URL, filepath.Join(dir, "account.json"))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	// A file where the state folder should be fails the write on Windows as
	// well as on Unix, whoever runs the tests.
	if err := os.WriteFile(dir, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := s.ContinueAsGuest(); err == nil {
		t.Fatal("the guest marker could not be written, yet the call succeeded")
	}
	if s.isGuest() {
		t.Fatal("the service stayed a guest in memory after the write failed")
	}
	if got := s.signedOutState().Status; got != StatusUnauthenticated {
		t.Fatalf("signed-out status = %q, want %q", got, StatusUnauthenticated)
	}
}

func TestRemoveAvatarRefreshesTheCachedProfile(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		wantErr   string
		wantCache string
	}{
		{"removed", http.StatusOK, "", ""},
		{"session expired", http.StatusUnauthorized, CodeUnauthenticated, "old-name"},
		{"server failure", http.StatusInternalServerError, CodeServer, "old-name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodDelete || r.URL.Path != APIPrefix+"/me/avatar" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				if tc.status != http.StatusOK {
					w.WriteHeader(tc.status)
					return
				}
				user := sampleUser()
				user.DisplayName = ""
				writeJSON(t, w, http.StatusOK, user)
			}))
			defer srv.Close()

			store := &fakeStore{cred: Credential{Token: "live"}, present: true}
			s := startedService(t, store, srv.URL)
			cached := sampleUser()
			cached.DisplayName = "old-name"
			s.profile = cachedProfile{User: cached}

			user, err := s.RemoveAvatar()
			if tc.wantErr != "" {
				if got := codeOf(t, err); got != tc.wantErr {
					t.Fatalf("code = %q, want %q", got, tc.wantErr)
				}
			} else if err != nil {
				t.Fatalf("RemoveAvatar: %v", err)
			}
			if tc.wantErr == "" && user.AvatarURL != "" {
				t.Fatalf("avatar still set: %+v", user)
			}
			if got := s.currentProfile().User.DisplayName; got != tc.wantCache {
				t.Fatalf("cached display name = %q, want %q", got, tc.wantCache)
			}
		})
	}
}

func TestUpdateProfileKeepsTheCacheWhenTheServerRefuses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusConflict, map[string]any{
			"error": map[string]string{"code": "username_taken", "field": "username"},
		})
	}))
	defer srv.Close()

	s := startedService(t, &fakeStore{cred: Credential{Token: "live"}, present: true}, srv.URL)
	s.profile = cachedProfile{User: sampleUser()}

	name := "taken"
	_, err := s.UpdateProfile(Patch{Username: &name})
	if got := codeOf(t, err); got != CodeUsernameTaken {
		t.Fatalf("code = %q, want %q", got, CodeUsernameTaken)
	}
	if got := s.currentProfile().User.Username; got != "playerone" {
		t.Fatalf("cached username = %q, a refused change leaked into the cache", got)
	}
}

func TestUploadCoverThroughTheService(t *testing.T) {
	webp := base64.StdEncoding.EncodeToString(webpBytes)

	t.Run("sends the decoded bytes and returns the owned url", func(t *testing.T) {
		received := make(chan []byte, 1)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPut || r.URL.Path != APIPrefix+"/me/cover" {
				t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read body: %v", err)
			}
			received <- body
			writeJSON(t, w, http.StatusOK, CoverUpload{CoverURL: "https://cdn.example.com/c.webp"})
		}))
		defer srv.Close()

		s := startedService(t, &fakeStore{cred: Credential{Token: "live"}, present: true}, srv.URL)
		cover, err := s.UploadCover(webp)
		if err != nil {
			t.Fatalf("UploadCover: %v", err)
		}
		if cover.CoverURL != "https://cdn.example.com/c.webp" {
			t.Fatalf("cover = %+v", cover)
		}
		if got := <-received; string(got) != string(webpBytes) {
			t.Fatalf("server received %v", got)
		}
	})

	t.Run("rejects bad payloads before any request", func(t *testing.T) {
		srv, hits := quietServer(t)
		s := startedService(t, &fakeStore{cred: Credential{Token: "live"}, present: true}, srv.URL)
		for _, encoded := range []string{"", "!!!", base64.StdEncoding.EncodeToString([]byte("plain text"))} {
			if _, err := s.UploadCover(encoded); err == nil {
				t.Fatalf("payload %q was accepted", encoded)
			}
		}
		if n := hits.Load(); n != 0 {
			t.Fatalf("server saw %d requests for payloads that never validated", n)
		}
	})

	t.Run("passes the server refusal through", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(t, w, http.StatusUnprocessableEntity, map[string]any{
				"error": map[string]string{"code": "unsupported_cover"},
			})
		}))
		defer srv.Close()

		s := startedService(t, &fakeStore{cred: Credential{Token: "live"}, present: true}, srv.URL)
		_, err := s.UploadCover(webp)
		if got := codeOf(t, err); got != CodeUnsupportedCover {
			t.Fatalf("code = %q, want %q", got, CodeUnsupportedCover)
		}
	})
}

func TestConcurrentSessionCallsAreRaceFree(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeJSON(t, w, http.StatusOK, sampleUser())
	}))
	defer srv.Close()

	store := &fakeStore{cred: Credential{Token: "live"}, present: true}
	s := startedService(t, store, srv.URL)

	var wg sync.WaitGroup
	for range 6 {
		wg.Add(3)
		go func() {
			defer wg.Done()
			if _, err := s.Bootstrap(); err != nil {
				t.Errorf("Bootstrap: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			if got := s.CurrentProfileSettings(); got.Showcase == nil {
				t.Errorf("settings lost their showcase: %+v", got)
			}
			if _, err := s.SessionToken(); err != nil {
				t.Errorf("SessionToken: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := s.ContinueAsGuest(); err != nil {
				t.Errorf("ContinueAsGuest: %v", err)
			}
		}()
	}
	wg.Wait()
}
