package account

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"typhon/internal/legal"
)

type termsBackend struct {
	t *testing.T

	mu         sync.Mutex
	meTerms    *TermsAcceptance
	postStatus int
	registered map[string]any
	posts      []map[string]string
	postAuth   []string
	posted     chan struct{}
	holdPost   chan struct{}
	server     *httptest.Server
}

func newTermsBackend(t *testing.T, meTerms *TermsAcceptance) *termsBackend {
	t.Helper()
	b := &termsBackend{t: t, meTerms: meTerms, postStatus: http.StatusOK, posted: make(chan struct{}, 16)}
	b.server = httptest.NewServer(http.HandlerFunc(b.serve))
	t.Cleanup(b.server.Close)
	return b
}

func (b *termsBackend) user() CurrentUser {
	b.mu.Lock()
	defer b.mu.Unlock()
	user := sampleUser()
	user.TermsAccepted = b.meTerms
	return user
}

func (b *termsBackend) serve(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && r.URL.Path == APIPrefix+"/auth/register":
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			b.t.Errorf("decode register body: %v", err)
		}
		b.mu.Lock()
		b.registered = body
		b.mu.Unlock()
		writeJSON(b.t, w, http.StatusCreated, Session{User: b.user(), Token: "fresh-token", ExpiresAt: time.Now().Add(time.Hour)})
	case r.Method == http.MethodPost && r.URL.Path == APIPrefix+"/auth/login":
		writeJSON(b.t, w, http.StatusOK, Session{User: b.user(), Token: "fresh-token", ExpiresAt: time.Now().Add(time.Hour)})
	case r.Method == http.MethodGet && r.URL.Path == APIPrefix+"/me":
		writeJSON(b.t, w, http.StatusOK, b.user())
	case r.Method == http.MethodPost && r.URL.Path == APIPrefix+"/me/terms":
		b.acceptTerms(w, r)
	default:
		b.t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}
}

func (b *termsBackend) acceptTerms(w http.ResponseWriter, r *http.Request) {
	var body map[string]string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		b.t.Errorf("decode terms body: %v", err)
	}
	b.mu.Lock()
	b.posts = append(b.posts, body)
	b.postAuth = append(b.postAuth, r.Header.Get("Authorization"))
	status := b.postStatus
	hold := b.holdPost
	b.mu.Unlock()
	b.posted <- struct{}{}
	if hold != nil {
		select {
		case <-hold:
		case <-r.Context().Done():
			return
		}
	}
	if status != http.StatusOK {
		w.WriteHeader(status)
		return
	}
	b.mu.Lock()
	b.meTerms = &TermsAcceptance{Version: body["termsVersion"], AcceptedAt: time.Now().UTC()}
	b.mu.Unlock()
	writeJSON(b.t, w, http.StatusOK, b.user())
}

func (b *termsBackend) postCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.posts)
}

type acceptedTerms struct {
	mu      sync.Mutex
	version string
}

func (a *acceptedTerms) get() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.version
}

func (a *acceptedTerms) set(version string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.version = version
}

func termsService(t *testing.T, b *termsBackend, store *fakeStore, accepted *acceptedTerms) *Service {
	t.Helper()
	s := startedService(t, store, b.server.URL)
	s.SetAcceptedTerms(accepted.get)
	return s
}

func settled(s *Service) {
	s.wg.Wait()
}

func TestRegisterSendsTermsVersionOnlyForTheAcceptedCurrentRevision(t *testing.T) {
	tests := []struct {
		name     string
		accepted string
		forged   string
		want     string
		wantSent bool
	}{
		{name: "current revision accepted", accepted: legal.Version, want: legal.Version, wantSent: true},
		{name: "nothing accepted", accepted: ""},
		{name: "older revision accepted", accepted: "2020-01-01"},
		{name: "newer revision accepted", accepted: "2999-01-01"},
		{name: "ui value is overwritten when nothing is accepted", accepted: "", forged: legal.Version},
		{name: "ui value is overwritten by the accepted one", accepted: legal.Version, forged: "forged", want: legal.Version, wantSent: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTermsBackend(t, &TermsAcceptance{Version: legal.Version, AcceptedAt: time.Now().UTC()})
			accepted := &acceptedTerms{version: tt.accepted}
			s := termsService(t, b, &fakeStore{}, accepted)

			_, err := s.Register(RegisterInput{
				Email: "player@example.com", Username: "playerone", DisplayName: "Player One",
				Password: "secret pass", TermsVersion: tt.forged,
			})
			if err != nil {
				t.Fatalf("Register() error = %v", err)
			}
			settled(s)

			b.mu.Lock()
			got, sent := b.registered["termsVersion"]
			b.mu.Unlock()
			if sent != tt.wantSent {
				t.Fatalf("termsVersion sent = %v (%v), want %v", sent, got, tt.wantSent)
			}
			if tt.wantSent && got != tt.want {
				t.Errorf("termsVersion = %v, want %q", got, tt.want)
			}
			if n := b.postCount(); n != 0 {
				t.Errorf("POST /me/terms calls = %d, want 0: the register reply already carries the acceptance", n)
			}
		})
	}
}

func TestRegisterWithoutAcceptedProviderOmitsTermsVersion(t *testing.T) {
	b := newTermsBackend(t, nil)
	s := startedService(t, &fakeStore{}, b.server.URL)
	if _, err := s.Register(RegisterInput{Email: "player@example.com", Username: "playerone", DisplayName: "P", Password: "secret pass"}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, sent := b.registered["termsVersion"]; sent {
		t.Errorf("termsVersion was sent without a way to know what the user accepted: %v", b.registered)
	}
}

func TestCurrentUserDecodesTermsAccepted(t *testing.T) {
	at := time.Date(2026, 10, 11, 12, 30, 0, 0, time.UTC)
	tests := []struct {
		name string
		body string
		want *TermsAcceptance
	}{
		{name: "object", body: `{"id":"u1","termsAccepted":{"version":"2026-10-11","acceptedAt":"2026-10-11T12:30:00Z"}}`, want: &TermsAcceptance{Version: "2026-10-11", AcceptedAt: at}},
		{name: "null", body: `{"id":"u1","termsAccepted":null}`},
		{name: "absent", body: `{"id":"u1"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if _, err := io.WriteString(w, tt.body); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer srv.Close()
			client, err := NewClient(srv.URL, func() (string, error) { return "tok", nil })
			if err != nil {
				t.Fatal(err)
			}
			user, err := client.Me(t.Context())
			if err != nil {
				t.Fatalf("Me() error = %v", err)
			}
			switch {
			case tt.want == nil && user.TermsAccepted != nil:
				t.Fatalf("TermsAccepted = %+v, want nil", *user.TermsAccepted)
			case tt.want != nil && (user.TermsAccepted == nil || *user.TermsAccepted != *tt.want):
				t.Fatalf("TermsAccepted = %+v, want %+v", user.TermsAccepted, *tt.want)
			}
		})
	}
}

func TestClientAcceptTerms(t *testing.T) {
	t.Run("sends the version and returns the user", func(t *testing.T) {
		var gotBody map[string]string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || r.URL.Path != APIPrefix+"/me/terms" {
				t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer tok" {
				t.Errorf("authorization = %q, want Bearer tok", got)
			}
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Errorf("decode body: %v", err)
			}
			user := sampleUser()
			user.TermsAccepted = &TermsAcceptance{Version: gotBody["termsVersion"], AcceptedAt: time.Now().UTC()}
			writeJSON(t, w, http.StatusOK, user)
		}))
		defer srv.Close()
		client, err := NewClient(srv.URL, func() (string, error) { return "tok", nil })
		if err != nil {
			t.Fatal(err)
		}
		user, err := client.AcceptTerms(t.Context(), "2026-10-11")
		if err != nil {
			t.Fatalf("AcceptTerms() error = %v", err)
		}
		if gotBody["termsVersion"] != "2026-10-11" || len(gotBody) != 1 {
			t.Errorf("body = %v, want only termsVersion", gotBody)
		}
		if user.TermsAccepted == nil || user.TermsAccepted.Version != "2026-10-11" {
			t.Errorf("TermsAccepted = %+v, want 2026-10-11", user.TermsAccepted)
		}
	})

	t.Run("terms_required keeps its code and field", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(t, w, http.StatusBadRequest, map[string]any{"error": map[string]string{"code": "terms_required", "field": "termsVersion"}})
		}))
		defer srv.Close()
		client, err := NewClient(srv.URL, func() (string, error) { return "tok", nil })
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.AcceptTerms(t.Context(), "1999-01-01")
		var apiErr *Error
		if !errors.As(err, &apiErr) || apiErr.Code != CodeTermsRequired || apiErr.Field != "termsVersion" {
			t.Fatalf("error = %v, want terms_required on termsVersion", err)
		}
	})
}

func TestSignInRecordsTheAcceptanceWhenTheServerLacksIt(t *testing.T) {
	stale := &TermsAcceptance{Version: "2020-01-01", AcceptedAt: time.Now().UTC()}
	current := &TermsAcceptance{Version: legal.Version, AcceptedAt: time.Now().UTC()}
	tests := []struct {
		name      string
		accepted  string
		server    *TermsAcceptance
		wantPosts int
	}{
		{name: "server has nothing", accepted: legal.Version, server: nil, wantPosts: 1},
		{name: "server has an older revision", accepted: legal.Version, server: stale, wantPosts: 1},
		{name: "server already has the current revision", accepted: legal.Version, server: current, wantPosts: 0},
		{name: "user accepted nothing locally", accepted: "", server: nil, wantPosts: 0},
		{name: "user accepted an older revision locally", accepted: "2020-01-01", server: nil, wantPosts: 0},
	}
	flows := []struct {
		name string
		run  func(t *testing.T, s *Service)
	}{
		{"login", func(t *testing.T, s *Service) {
			if _, err := s.Login(LoginInput{Identifier: "playerone", Password: "secret pass"}); err != nil {
				t.Fatalf("Login() error = %v", err)
			}
		}},
		{"restore", func(t *testing.T, s *Service) {
			state, err := s.Bootstrap()
			if err != nil || state.Status != StatusAuthenticated {
				t.Fatalf("Bootstrap() = %+v, %v", state, err)
			}
		}},
	}
	for _, flow := range flows {
		for _, tt := range tests {
			t.Run(flow.name+"/"+tt.name, func(t *testing.T) {
				b := newTermsBackend(t, tt.server)
				store := &fakeStore{}
				if flow.name == "restore" {
					store = &fakeStore{present: true, cred: Credential{Token: "stored-token", Username: "playerone"}}
				}
				s := termsService(t, b, store, &acceptedTerms{version: tt.accepted})
				flow.run(t, s)
				settled(s)

				if n := b.postCount(); n != tt.wantPosts {
					t.Fatalf("POST /me/terms calls = %d, want %d", n, tt.wantPosts)
				}
				if tt.wantPosts == 1 {
					b.mu.Lock()
					defer b.mu.Unlock()
					if b.posts[0]["termsVersion"] != legal.Version {
						t.Errorf("termsVersion = %q, want %q", b.posts[0]["termsVersion"], legal.Version)
					}
					wantAuth := "Bearer fresh-token"
					if flow.name == "restore" {
						wantAuth = "Bearer stored-token"
					}
					if b.postAuth[0] != wantAuth {
						t.Errorf("authorization = %q, want %q", b.postAuth[0], wantAuth)
					}
				}
			})
		}
	}
}

func TestTermsSyncFailureNeitherBreaksSignInNorTouchesTheLocalAcceptance(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusInternalServerError, http.StatusBadRequest} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			b := newTermsBackend(t, nil)
			b.postStatus = status
			accepted := &acceptedTerms{version: legal.Version}
			store := &fakeStore{present: true, cred: Credential{Token: "stored-token", Username: "playerone"}}
			s := termsService(t, b, store, accepted)

			state, err := s.Bootstrap()
			if err != nil {
				t.Fatalf("Bootstrap() error = %v", err)
			}
			if state.Status != StatusAuthenticated {
				t.Fatalf("status = %q, want %q", state.Status, StatusAuthenticated)
			}
			user, err := s.Login(LoginInput{Identifier: "playerone", Password: "secret pass"})
			if err != nil {
				t.Fatalf("Login() error = %v", err)
			}
			if user.Username != "playerone" {
				t.Errorf("user = %+v", user)
			}
			settled(s)

			if n := b.postCount(); n != 2 {
				t.Errorf("POST /me/terms calls = %d, want 2 (restore and login each try once)", n)
			}
			if got := accepted.get(); got != legal.Version {
				t.Errorf("local acceptance = %q, want it untouched (%q)", got, legal.Version)
			}
			if cred, present := store.snapshot(); !present || cred.Token != "fresh-token" {
				t.Errorf("credential = %+v present=%v, want the session kept", cred, present)
			}
		})
	}
}

func TestSyncTermsAfterTheLocalAcceptanceChanges(t *testing.T) {
	b := newTermsBackend(t, nil)
	accepted := &acceptedTerms{}
	s := termsService(t, b, &fakeStore{}, accepted)

	s.SyncTerms()
	settled(s)
	if n := b.postCount(); n != 0 {
		t.Fatalf("POST before sign-in = %d, want 0", n)
	}

	if _, err := s.Login(LoginInput{Identifier: "playerone", Password: "secret pass"}); err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	settled(s)
	if n := b.postCount(); n != 0 {
		t.Fatalf("POST before acceptance = %d, want 0", n)
	}

	accepted.set(legal.Version)
	s.SyncTerms()
	settled(s)
	if n := b.postCount(); n != 1 {
		t.Fatalf("POST after acceptance = %d, want 1", n)
	}

	s.SyncTerms()
	settled(s)
	if n := b.postCount(); n != 1 {
		t.Fatalf("POST after the server confirmed = %d, want still 1", n)
	}
}

func TestSyncTermsDoesNotRetryAFailureOnEverySettingsChange(t *testing.T) {
	b := newTermsBackend(t, nil)
	b.postStatus = http.StatusInternalServerError
	accepted := &acceptedTerms{}
	s := termsService(t, b, &fakeStore{}, accepted)
	if _, err := s.Login(LoginInput{Identifier: "playerone", Password: "secret pass"}); err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	accepted.set(legal.Version)

	for range 3 {
		s.SyncTerms()
		settled(s)
	}
	if n := b.postCount(); n != 1 {
		t.Fatalf("POST calls = %d, want 1: a failure is retried on the next sign-in or launch", n)
	}

	if _, err := s.Login(LoginInput{Identifier: "playerone", Password: "secret pass"}); err != nil {
		t.Fatalf("second Login() error = %v", err)
	}
	settled(s)
	if n := b.postCount(); n != 2 {
		t.Fatalf("POST calls after the next sign-in = %d, want 2", n)
	}
}

func TestSyncTermsAfterSignOutSendsNothing(t *testing.T) {
	b := newTermsBackend(t, nil)
	accepted := &acceptedTerms{}
	store := &fakeStore{}
	s := termsService(t, b, store, accepted)
	if _, err := s.Login(LoginInput{Identifier: "playerone", Password: "secret pass"}); err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if err := store.Delete(); err != nil {
		t.Fatal(err)
	}

	accepted.set(legal.Version)
	s.SyncTerms()
	settled(s)
	if n := b.postCount(); n != 0 {
		t.Fatalf("POST after the credential is gone = %d, want 0", n)
	}
}

func TestTermsSyncStopsWithTheService(t *testing.T) {
	t.Run("shutdown cancels an in-flight request and waits for it", func(t *testing.T) {
		b := newTermsBackend(t, nil)
		b.holdPost = make(chan struct{})
		defer close(b.holdPost)
		s := termsService(t, b, &fakeStore{}, &acceptedTerms{version: legal.Version})
		if _, err := s.Login(LoginInput{Identifier: "playerone", Password: "secret pass"}); err != nil {
			t.Fatalf("Login() error = %v", err)
		}
		select {
		case <-b.posted:
		case <-time.After(5 * time.Second):
			t.Fatal("terms request was not sent")
		}

		done := make(chan error, 1)
		go func() { done <- s.ServiceShutdown() }()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("ServiceShutdown() error = %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("ServiceShutdown() did not return: the sync goroutine ignores ctx")
		}
	})

	t.Run("nothing starts after shutdown", func(t *testing.T) {
		b := newTermsBackend(t, nil)
		accepted := &acceptedTerms{}
		s := termsService(t, b, &fakeStore{}, accepted)
		if _, err := s.Login(LoginInput{Identifier: "playerone", Password: "secret pass"}); err != nil {
			t.Fatalf("Login() error = %v", err)
		}
		if err := s.ServiceShutdown(); err != nil {
			t.Fatal(err)
		}
		accepted.set(legal.Version)
		s.SyncTerms()
		settled(s)
		if n := b.postCount(); n != 0 {
			t.Fatalf("POST after shutdown = %d, want 0", n)
		}
	})

	t.Run("a cancelled parent context sends nothing", func(t *testing.T) {
		b := newTermsBackend(t, nil)
		s, err := newService(&fakeStore{}, b.server.URL, statePathFor(t))
		if err != nil {
			t.Fatal(err)
		}
		parent, cancel := context.WithCancel(t.Context())
		cancel()
		if err := s.ServiceStartup(parent, application.ServiceOptions{}); err != nil {
			t.Fatal(err)
		}
		s.SetAcceptedTerms(func() string { return legal.Version })
		s.bindSyncIdentity("tok", "u1")
		s.SyncTerms()
		settled(s)
		if n := b.postCount(); n != 0 {
			t.Fatalf("POST with a cancelled context = %d, want 0", n)
		}
	})
}
