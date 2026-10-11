package diagnostics

import (
	"os"
	"testing"
)

// newServiceAt points the client at account.BaseURL(), which is production by
// default: services built without a test server flushed their fake reports to
// the live API from every local and CI run.
func TestMain(m *testing.M) {
	if err := os.Setenv("TYPHON_API_URL", "http://127.0.0.1:1"); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func TestDefaultClientStaysOnLoopback(t *testing.T) {
	s, err := newServiceAt(t.TempDir(), testIdentity(), func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if s.client.baseURL != "http://127.0.0.1:1" {
		t.Fatalf("tests would deliver to %q", s.client.baseURL)
	}
}
