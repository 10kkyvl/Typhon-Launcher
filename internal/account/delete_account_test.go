package account

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

func seedProfile(t *testing.T, s *Service) {
	t.Helper()
	epoch := s.profileEpoch
	if err := s.setProfile(epoch, cachedProfile{User: sampleUser()}); err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	if _, err := os.Stat(s.profilePath); err != nil {
		t.Fatalf("profile cache not written: %v", err)
	}
}

func TestDeleteAccountSuccessSignsOutAndForgets(t *testing.T) {
	var logoutCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == APIPrefix+"/auth/logout" {
			logoutCalls.Add(1)
		}
		if r.Method != http.MethodDelete || r.URL.Path != APIPrefix+"/me" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("authorization = %q", got)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["password"] != "hunter22" {
			t.Errorf("body = %v, err = %v", body, err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	store := &fakeStore{cred: Credential{Token: "tok"}, present: true}
	s := startedService(t, store, srv.URL)
	seedProfile(t, s)

	if err := s.DeleteAccount("hunter22"); err != nil {
		t.Fatalf("DeleteAccount() error = %v", err)
	}
	if _, present := store.snapshot(); present {
		t.Error("credential kept after account deletion")
	}
	if _, err := os.Stat(s.profilePath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("profile cache survived: %v", err)
	}
	if s.currentProfile().User.ID != "" {
		t.Error("profile kept in memory")
	}
	if s.isGuest() {
		t.Error("service became guest")
	}
	if logoutCalls.Load() != 0 {
		t.Error("deletion called /auth/logout")
	}
}

func TestDeleteAccountFailureKeepsState(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		body     any
		wantCode string
	}{
		{"wrong password", http.StatusUnauthorized, map[string]any{"error": map[string]string{"code": "invalid_credentials"}}, CodeInvalidLogin},
		{"server error", http.StatusInternalServerError, map[string]any{"error": map[string]string{"code": "internal"}}, CodeInternal},
		{"rate limited", http.StatusTooManyRequests, map[string]any{"error": map[string]string{"code": "rate_limited"}}, CodeRateLimited},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(t, w, tt.status, tt.body)
			}))
			defer srv.Close()

			store := &fakeStore{cred: Credential{Token: "tok"}, present: true}
			s := startedService(t, store, srv.URL)
			seedProfile(t, s)

			err := s.DeleteAccount("hunter22")
			var apiErr *Error
			if !errors.As(err, &apiErr) || apiErr.Code != tt.wantCode {
				t.Fatalf("error = %v, want code %q", err, tt.wantCode)
			}
			if strings.Contains(err.Error(), "hunter22") {
				t.Error("password leaked into the error")
			}
			if _, present := store.snapshot(); !present || store.deletes != 0 {
				t.Error("credential touched")
			}
			if _, statErr := os.Stat(s.profilePath); statErr != nil {
				t.Errorf("profile cache removed: %v", statErr)
			}
			if s.currentProfile().User.ID == "" {
				t.Error("profile dropped from memory")
			}
		})
	}
}

func TestDeleteAccountNetworkErrorKeepsState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()

	store := &fakeStore{cred: Credential{Token: "tok"}, present: true}
	s := startedService(t, store, url)
	seedProfile(t, s)

	err := s.DeleteAccount("hunter22")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != CodeNetwork {
		t.Fatalf("error = %v, want %q", err, CodeNetwork)
	}
	if _, present := store.snapshot(); !present {
		t.Error("credential removed")
	}
	if s.currentProfile().User.ID == "" {
		t.Error("profile dropped")
	}
}

func TestDeleteAccountEmptyPasswordSkipsRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	store := &fakeStore{cred: Credential{Token: "tok"}, present: true}
	err := startedService(t, store, srv.URL).DeleteAccount("")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != CodeBadRequest || apiErr.Field != "password" {
		t.Fatalf("error = %v, want bad_request on password", err)
	}
	if _, present := store.snapshot(); !present {
		t.Error("credential removed")
	}
}

func TestDeleteAccountWithoutSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	err := startedService(t, &fakeStore{}, srv.URL).DeleteAccount("hunter22")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != CodeUnauthenticated {
		t.Fatalf("error = %v, want %q", err, CodeUnauthenticated)
	}
}
