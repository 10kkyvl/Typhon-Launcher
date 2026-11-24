package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func postControl(t *testing.T, base, query string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, base+"/control?"+query, nil)
	if err != nil {
		t.Fatalf("build control request: %v", err)
	}
	resp, err := testHTTPClient().Do(req)
	if err != nil {
		t.Fatalf("POST /control?%s: %v", query, err)
	}
	defer closeBody(t, resp)
	return resp.StatusCode, string(mustReadAll(t, resp))
}

func getControl(t *testing.T, base string) string {
	t.Helper()
	resp, err := testHTTPClient().Get(base + "/control")
	if err != nil {
		t.Fatalf("GET /control: %v", err)
	}
	defer closeBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /control status = %d", resp.StatusCode)
	}
	return string(mustReadAll(t, resp))
}

// setMode switches the running source the way a developer does from a
// terminal, and fails the test when the source refuses.
func setMode(t *testing.T, base, mode string) {
	t.Helper()
	if code, body := postControl(t, base, "fail="+mode); code != http.StatusOK {
		t.Fatalf("POST /control?fail=%s = %d %s", mode, code, body)
	}
}

func TestControlSwitchesModesAndReportsThem(t *testing.T) {
	srv, _ := chainServer(t)
	ts := httptest.NewServer(newMux(srv))
	defer ts.Close()

	if state := getControl(t, ts.URL); !strings.Contains(state, "fail=none\n") || !strings.Contains(state, "versions=1.4.2,1.4.3,1.4.4\n") {
		t.Fatalf("initial state:\n%s", state)
	}
	for _, mode := range []string{"manifest-500", "bad-signature", "unknown-key", "artifact-500", "stall", "hash", "size"} {
		setMode(t, ts.URL, mode)
		if state := getControl(t, ts.URL); !strings.Contains(state, "fail="+mode+"\n") {
			t.Fatalf("after fail=%s the state says:\n%s", mode, state)
		}
	}
	setMode(t, ts.URL, "")
	if state := getControl(t, ts.URL); !strings.Contains(state, "fail=none\n") {
		t.Fatalf("after fail= the state says:\n%s", state)
	}

	if code, _ := postControl(t, ts.URL, "seconds=5"); code != http.StatusOK {
		t.Fatalf("POST seconds=5 status = %d", code)
	}
	if state := getControl(t, ts.URL); !strings.Contains(state, "seconds=5\n") || !strings.Contains(state, "fail=none\n") {
		t.Fatalf("seconds alone must change the pace and leave the mode:\n%s", state)
	}

	for _, query := range []string{"fail=bogus", "seconds=-1", "seconds=x"} {
		if code, _ := postControl(t, ts.URL, query); code != http.StatusBadRequest {
			t.Fatalf("POST /control?%s status = %d, want 400", query, code)
		}
	}
	if state := getControl(t, ts.URL); !strings.Contains(state, "seconds=5\n") || !strings.Contains(state, "fail=none\n") {
		t.Fatalf("a refused switch changed the state:\n%s", state)
	}
}

func TestControlReportsWhatTheLastLauncherWasOffered(t *testing.T) {
	srv, pub := chainServer(t)
	ts := httptest.NewServer(newMux(srv))
	defer ts.Close()

	fetchOffer(t, ts.URL, pub, "1.4.2")
	if state := getControl(t, ts.URL); !strings.Contains(state, "last-client=1.4.2 offered=1.4.3\n") {
		t.Fatalf("state after a 1.4.2 check:\n%s", state)
	}
}

// A web page the developer has open can send a POST to 127.0.0.1 too; the
// browser marks it with Origin, a terminal does not.
func TestControlRefusesABrowserOrigin(t *testing.T) {
	srv, _ := chainServer(t)
	ts := httptest.NewServer(newMux(srv))
	defer ts.Close()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, ts.URL+"/control?fail=hash", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Origin", "https://example.com")
	resp, err := testHTTPClient().Do(req)
	if err != nil {
		t.Fatalf("POST /control: %v", err)
	}
	closeBody(t, resp)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
	if state := getControl(t, ts.URL); !strings.Contains(state, "fail=none\n") {
		t.Fatalf("a refused request switched the mode:\n%s", state)
	}
}

func TestSourceRefusesANonLoopbackPeer(t *testing.T) {
	srv, _ := chainServer(t)
	handler := newMux(srv)

	for _, tc := range []struct {
		peer string
		want int
	}{
		{peer: "192.168.1.20:5000", want: http.StatusForbidden},
		{peer: "10.0.0.2:5000", want: http.StatusForbidden},
		{peer: "garbage", want: http.StatusForbidden},
		{peer: "127.0.0.1:5000", want: http.StatusOK},
		{peer: "[::1]:5000", want: http.StatusOK},
	} {
		for _, target := range []string{"/launcher/manifest", "/control"} {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)
			req.RemoteAddr = tc.peer
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Errorf("GET %s from %s = %d, want %d", target, tc.peer, rec.Code, tc.want)
			}
		}
	}
}
