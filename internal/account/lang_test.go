package account

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func registerBody(t *testing.T, language func() string, input RegisterInput) map[string]any {
	t.Helper()
	var received map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		writeSession(t, w, http.StatusCreated, "fresh-token")
	}))
	defer srv.Close()

	s := startedService(t, &fakeStore{}, srv.URL)
	if language != nil {
		s.SetLanguage(language)
	}
	if _, err := s.Register(input); err != nil {
		t.Fatalf("Register: %v", err)
	}
	return received
}

func TestRegisterSendsTheUILanguage(t *testing.T) {
	cases := []struct {
		name     string
		language func() string
		input    RegisterInput
		want     any
	}{
		{"russian", func() string { return "ru" }, RegisterInput{}, "ru"},
		{"english", func() string { return "en" }, RegisterInput{}, "en"},
		{"the UI cannot override it", func() string { return "en" }, RegisterInput{Lang: "ru"}, "en"},
		{"no language source sends none", nil, RegisterInput{}, nil},
		{"empty language sends none", func() string { return "" }, RegisterInput{}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.input.Email, tc.input.Username, tc.input.DisplayName, tc.input.Password = "p@example.com", "playerone", "P", "secret pass"
			got := registerBody(t, tc.language, tc.input)
			if got["lang"] != tc.want {
				t.Fatalf("lang = %v, want %v (body %v)", got["lang"], tc.want, got)
			}
		})
	}
}

func TestLoginSendsNoLanguage(t *testing.T) {
	var received map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		writeSession(t, w, http.StatusOK, "fresh-token")
	}))
	defer srv.Close()
	s := startedService(t, &fakeStore{}, srv.URL)
	s.SetLanguage(func() string { return "en" })

	if _, err := s.Login(LoginInput{Identifier: "playerone", Password: "secret pass"}); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if _, has := received["lang"]; has {
		t.Fatalf("login body = %v, want no lang", received)
	}
}

func TestForgotPasswordURL(t *testing.T) {
	cases := map[string]string{
		"ru":      "https://typhon-launcher.com/account/forgot",
		"en":      "https://typhon-launcher.com/en/account/forgot",
		"":        "https://typhon-launcher.com/account/forgot",
		"de":      "https://typhon-launcher.com/account/forgot",
		"EN":      "https://typhon-launcher.com/account/forgot",
		"system":  "https://typhon-launcher.com/account/forgot",
		"en/evil": "https://typhon-launcher.com/account/forgot",
	}
	for language, want := range cases {
		if got := ForgotPasswordURL(language); got != want {
			t.Errorf("ForgotPasswordURL(%q) = %q, want %q", language, got, want)
		}
	}
}

func TestOpenForgotPassword(t *testing.T) {
	cases := []struct {
		name     string
		language func() string
		want     string
	}{
		{"russian", func() string { return "ru" }, "https://typhon-launcher.com/account/forgot"},
		{"english", func() string { return "en" }, "https://typhon-launcher.com/en/account/forgot"},
		{"no language source", nil, "https://typhon-launcher.com/account/forgot"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := startedService(t, &fakeStore{}, "https://api.example.com")
			var opened []string
			s.openURL = func(link string) error {
				opened = append(opened, link)
				return nil
			}
			if tc.language != nil {
				s.SetLanguage(tc.language)
			}
			if err := s.OpenForgotPassword(); err != nil {
				t.Fatalf("OpenForgotPassword: %v", err)
			}
			if len(opened) != 1 || opened[0] != tc.want {
				t.Fatalf("opened = %v, want [%s]", opened, tc.want)
			}
		})
	}
}

func TestOpenForgotPasswordReportsBrowserFailure(t *testing.T) {
	s := startedService(t, &fakeStore{}, "https://api.example.com")
	refused := errors.New("no default browser")
	s.openURL = func(string) error { return refused }

	if err := s.OpenForgotPassword(); !errors.Is(err, refused) {
		t.Fatalf("error = %v, want it to wrap the browser failure", err)
	}
}

func TestOpenForgotPasswordWithoutApplication(t *testing.T) {
	s := startedService(t, &fakeStore{}, "https://api.example.com")

	if err := s.OpenForgotPassword(); !errors.Is(err, errNoApplication) {
		t.Fatalf("error = %v, want errNoApplication", err)
	}
}

func TestOpenStatusPage(t *testing.T) {
	s := startedService(t, &fakeStore{}, "https://api.example.com")
	var opened []string
	s.openURL = func(link string) error {
		opened = append(opened, link)
		return nil
	}
	s.SetLanguage(func() string { return "en" })

	if err := s.OpenStatusPage(); err != nil {
		t.Fatalf("OpenStatusPage: %v", err)
	}
	if len(opened) != 1 || opened[0] != "https://typhon.betteruptime.com" {
		t.Fatalf("opened = %v, want the status page", opened)
	}
}

func TestOpenStatusPageReportsBrowserFailure(t *testing.T) {
	s := startedService(t, &fakeStore{}, "https://api.example.com")
	refused := errors.New("no default browser")
	s.openURL = func(string) error { return refused }

	if err := s.OpenStatusPage(); !errors.Is(err, refused) {
		t.Fatalf("error = %v, want it to wrap the browser failure", err)
	}
}

func TestOpenSupportSite(t *testing.T) {
	cases := []struct {
		name     string
		language func() string
		want     string
	}{
		{"russian", func() string { return "ru" }, "https://typhon-launcher.com/support"},
		{"english", func() string { return "en" }, "https://typhon-launcher.com/en/support"},
		{"no language source", nil, "https://typhon-launcher.com/support"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := startedService(t, &fakeStore{}, "https://api.example.com")
			var opened []string
			s.openURL = func(link string) error {
				opened = append(opened, link)
				return nil
			}
			if tc.language != nil {
				s.SetLanguage(tc.language)
			}
			if err := s.OpenSupportSite(); err != nil {
				t.Fatalf("OpenSupportSite: %v", err)
			}
			if len(opened) != 1 || opened[0] != tc.want {
				t.Fatalf("opened = %v, want [%s]", opened, tc.want)
			}
		})
	}
}

func TestOpenSupportSiteReportsBrowserFailure(t *testing.T) {
	s := startedService(t, &fakeStore{}, "https://api.example.com")
	refused := errors.New("no default browser")
	s.openURL = func(string) error { return refused }

	if err := s.OpenSupportSite(); !errors.Is(err, refused) {
		t.Fatalf("error = %v, want it to wrap the browser failure", err)
	}
}
