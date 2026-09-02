//go:build devmock && !windows

package selfupdate

import (
	"errors"
	"testing"

	"typhon/internal/account"
)

func TestManifestBaseURLDefaultsToAccount(t *testing.T) {
	t.Setenv(manifestURLEnv, "")
	got, err := manifestBaseURL()
	if err != nil {
		t.Fatalf("manifestBaseURL: %v", err)
	}
	if want := account.BaseURL(); got != want {
		t.Fatalf("manifestBaseURL() = %q, want %q", got, want)
	}
}

func TestManifestBaseURLOverride(t *testing.T) {
	t.Setenv(manifestURLEnv, "http://127.0.0.1:8099/")
	got, err := manifestBaseURL()
	if err != nil {
		t.Fatalf("manifestBaseURL: %v", err)
	}
	if got != "http://127.0.0.1:8099" {
		t.Fatalf("manifestBaseURL() = %q", got)
	}
}

func TestManifestBaseURLOverrideRejected(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "plain http off loopback", value: "http://example.com"},
		{name: "unsupported scheme", value: "ftp://127.0.0.1"},
		{name: "no host", value: "http://"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(manifestURLEnv, tt.value)
			if _, err := manifestBaseURL(); !errors.Is(err, account.ErrInsecureBaseURL) {
				t.Fatalf("manifestBaseURL() = %v, want ErrInsecureBaseURL", err)
			}
		})
	}
}

func TestNewServiceRejectsBadManifestURL(t *testing.T) {
	testConfigDir(t)
	t.Setenv(manifestURLEnv, "http://example.com")
	if _, err := NewService(); !errors.Is(err, account.ErrInsecureBaseURL) {
		t.Fatalf("NewService() = %v, want ErrInsecureBaseURL", err)
	}
}
