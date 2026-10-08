package selfupdate

import (
	"runtime"
	"testing"
)

func TestNewServiceRefusesToStartWithoutAConfigDir(t *testing.T) {
	switch runtime.GOOS {
	case "windows":
		t.Setenv("APPDATA", "")
	case "darwin":
		t.Setenv("HOME", "")
	default:
		t.Setenv("XDG_CONFIG_HOME", "")
		t.Setenv("HOME", "")
	}

	s, err := NewService()
	if err == nil {
		t.Fatal("NewService() error = nil without a config dir: every path would resolve against the working directory")
	}
	if s != nil {
		t.Fatalf("NewService() = %+v next to an error", s)
	}
}

func TestNewServiceRefusesAnInsecureUpdateServer(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("TYPHON_DEVMOCK_MANIFEST_URL", "")

	for _, base := range []string{"http://updates.example.com", "ftp://updates.example.com", "updates.example.com"} {
		t.Run(base, func(t *testing.T) {
			t.Setenv("TYPHON_API_URL", base)
			s, err := NewService()
			if err == nil {
				t.Fatalf("NewService() error = nil for %q: the manifest and its installer would travel in the clear", base)
			}
			if s != nil {
				t.Fatalf("NewService() = %+v next to an error", s)
			}
		})
	}

	t.Run("loopback over http is allowed", func(t *testing.T) {
		t.Setenv("TYPHON_API_URL", "http://127.0.0.1:8080")
		if _, err := NewService(); err != nil {
			t.Fatalf("NewService() error = %v", err)
		}
	})
}
