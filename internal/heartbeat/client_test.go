package heartbeat

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewClientRejectsInvalidBaseURL(t *testing.T) {
	if _, err := newClient(""); err == nil {
		t.Fatal("expected error for empty base url")
	}
	if _, err := newClient("http://example.com"); err == nil {
		t.Fatal("expected error for plain http on a non-loopback host")
	}
}

func TestClientHeartbeatSendsExpectedRequest(t *testing.T) {
	var gotPath string
	var gotHeader http.Header
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotHeader = r.Header.Clone()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		gotBody = body
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	cl, err := newClient(srv.URL)
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	gameID := "42"
	err = cl.heartbeat(context.Background(), heartbeatPayload{
		SessionID:      "session-1",
		InstallationID: "install-1",
		State:          statePlaying,
		GameID:         &gameID,
		AppVersion:     "0.2.2",
		OS:             "windows",
		Arch:           "amd64",
	})
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}

	if gotPath != "/v1/presence/heartbeat" {
		t.Fatalf("unexpected path: %q", gotPath)
	}
	if gotHeader.Get("Authorization") != "" {
		t.Fatalf("unexpected Authorization header: %q", gotHeader.Get("Authorization"))
	}
	if gotHeader.Get("Content-Type") != "application/json" {
		t.Fatalf("unexpected content type: %q", gotHeader.Get("Content-Type"))
	}

	var decoded heartbeatPayload
	if err := json.Unmarshal(gotBody, &decoded); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if decoded.SessionID != "session-1" || decoded.InstallationID != "install-1" {
		t.Fatalf("unexpected payload: %+v", decoded)
	}
	if decoded.GameID == nil || *decoded.GameID != "42" {
		t.Fatalf("unexpected game id: %+v", decoded.GameID)
	}
}

func TestClientHeartbeatNullGameID(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		gotBody = body
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	cl, err := newClient(srv.URL)
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	if err := cl.heartbeat(context.Background(), heartbeatPayload{
		SessionID:      "session-1",
		InstallationID: "install-1",
		State:          stateIdle,
	}); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}

	if !strings.Contains(string(gotBody), `"game_id":null`) {
		t.Fatalf("expected game_id to be present and null, got %s", gotBody)
	}
}

func TestClientDisconnectSendsExpectedRequest(t *testing.T) {
	var gotPath string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		gotBody = body
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	cl, err := newClient(srv.URL)
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	if err := cl.disconnect(context.Background(), disconnectPayload{SessionID: "s1", InstallationID: "i1"}); err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	if gotPath != "/v1/presence/disconnect" {
		t.Fatalf("unexpected path: %q", gotPath)
	}
	var decoded disconnectPayload
	if err := json.Unmarshal(gotBody, &decoded); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if decoded.SessionID != "s1" || decoded.InstallationID != "i1" {
		t.Fatalf("unexpected payload: %+v", decoded)
	}
}

func TestClientHeartbeatServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	cl, err := newClient(srv.URL)
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	err = cl.heartbeat(context.Background(), heartbeatPayload{SessionID: "s", InstallationID: "i", State: stateIdle})
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestClientHeartbeatUnreachableServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	cl, err := newClient(url)
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	if err := cl.heartbeat(context.Background(), heartbeatPayload{SessionID: "s", InstallationID: "i", State: stateIdle}); err == nil {
		t.Fatal("expected error for unreachable server")
	}
}
