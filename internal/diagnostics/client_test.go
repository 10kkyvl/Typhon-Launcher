package diagnostics

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"typhon/internal/clientid"
	"typhon/internal/telemetrylog"
)

func TestNewClientRejectsInvalidBaseURL(t *testing.T) {
	if _, err := newClient(""); err == nil {
		t.Fatal("expected error for empty base url")
	}
	if _, err := newClient("http://example.com"); err == nil {
		t.Fatal("expected error for plain http on a non-loopback host")
	}
}

func TestClientSendPostsExpectedRequest(t *testing.T) {
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

	reports := []reportPayload{{
		ErrorID:    "err-1",
		AppVersion: "0.2.2",
		OS:         "windows",
		Arch:       "amd64",
		Component:  "download",
		Operation:  "start",
		ErrorCode:  "timeout",
		Message:    "boom",
		Stack:      "main.foo",
		Timestamp:  time.Now(),
		Fatal:      false,
	}}
	if err := cl.send(context.Background(), clientid.Identity{InstallationID: "install-1", SessionID: "session-1"}, reports); err != nil {
		t.Fatalf("send: %v", err)
	}

	if gotPath != "/v1/diagnostics/errors" {
		t.Fatalf("unexpected path: %q", gotPath)
	}
	if gotHeader.Get("Authorization") != "" {
		t.Fatalf("unexpected Authorization header: %q", gotHeader.Get("Authorization"))
	}
	if gotHeader.Get("Content-Type") != "application/json" {
		t.Fatalf("unexpected content type: %q", gotHeader.Get("Content-Type"))
	}

	var decoded batchPayload
	if err := json.Unmarshal(gotBody, &decoded); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(decoded.Reports) != 1 {
		t.Fatalf("unexpected reports: %+v", decoded.Reports)
	}
	if decoded.Reports[0].ErrorID != "err-1" {
		t.Fatalf("unexpected payload: %+v", decoded.Reports[0])
	}
	// Identity rides on the envelope, once, and never on a report.
	if decoded.InstallationID != "install-1" || decoded.SessionID != "session-1" {
		t.Fatalf("envelope lost the identity: %+v", decoded)
	}
}

func TestClientSendServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	cl, err := newClient(srv.URL)
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	if err := cl.send(context.Background(), clientid.Identity{}, nil); err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestClientSendUnreachableServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	cl, err := newClient(url)
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	if err := cl.send(context.Background(), clientid.Identity{}, nil); err == nil {
		t.Fatal("expected error for unreachable server")
	}
}

func TestClientSendRespectsCancelledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	cl, err := newClient(srv.URL)
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := cl.send(ctx, clientid.Identity{}, nil); err == nil {
		t.Fatal("expected error for a cancelled context")
	}
}

func TestClientSendRecordsTelemetryLog(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	cl, err := newClient(srv.URL)
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}

	marker := "diag-marker-" + time.Now().Format(time.RFC3339Nano)
	reports := []reportPayload{{ErrorID: marker}}
	if err := cl.send(context.Background(), clientid.Identity{InstallationID: "i", SessionID: "s"}, reports); err != nil {
		t.Fatalf("send: %v", err)
	}

	found := false
	for _, e := range telemetrylog.Entries() {
		if e.Kind == telemetrylog.KindDiagnostics && e.Endpoint == "/v1/diagnostics/errors" && bytes.Contains(e.Payload, []byte(marker)) {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected send to record an entry in the telemetry log")
	}
}
