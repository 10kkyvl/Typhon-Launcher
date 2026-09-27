package diagnostics

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"typhon/internal/clientid"
)

func TestDeliveryRepairsPersistedFrontendAndContinuesPastRejection(t *testing.T) {
	var received []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b batchPayload
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		rep := b.Reports[0]
		received = append(received, rep.ErrorID)
		if rep.ErrorCode == "" {
			t.Error("empty error_code reached server")
		}
		if rep.ErrorID == "rejected" {
			w.WriteHeader(422)
			return
		}
		w.WriteHeader(204)
	}))
	defer srv.Close()
	s, err := newServiceAt(t.TempDir(), clientid.Identity{InstallationID: "i", SessionID: "s"}, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	s.client, err = newClient(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	for i, id := range []string{"rejected", "legacy", "valid"} {
		code := "frontend_error"
		if id == "legacy" {
			code = ""
		}
		if err := savePending(s.pendingDir, time.Now().Add(time.Duration(i)*time.Second), []reportPayload{{ErrorID: id, Component: "frontend", ErrorCode: code}}); err != nil {
			t.Fatal(err)
		}
	}
	s.drainPendingEpoch(context.Background(), s.pendingDir, 0)
	if strings.Join(received, ",") != "rejected,legacy,valid" {
		t.Fatalf("received=%v", received)
	}
	pending, err := listPendingFiles(s.pendingDir)
	if err != nil {
		t.Fatal(err)
	}
	rejected, err := listPendingFiles(filepath.Join(s.pendingDir, "rejected"))
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 || len(rejected) != 1 {
		t.Fatalf("pending=%v rejected=%v", pending, rejected)
	}
	s.SetEnabled(false)
	rejected, err = listPendingFiles(filepath.Join(s.pendingDir, "rejected"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rejected) != 0 {
		t.Fatal("opt-out left quarantined reports")
	}
}

func TestFrontendCaptureProducesValidCode(t *testing.T) {
	s, err := newServiceAt(t.TempDir(), clientid.Identity{InstallationID: "i", SessionID: "s"}, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ReportClientError("frontend", "window.onerror", "synthetic error", "stack", true); err != nil {
		t.Fatal(err)
	}
	if len(s.queue) != 1 || s.queue[0].ErrorCode != "frontend_error" {
		t.Fatalf("queue=%+v", s.queue)
	}
}

func TestDeliverySplitsBatchesToServerLimits(t *testing.T) {
	var received int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		var batch batchPayload
		if err := json.Unmarshal(data, &batch); err != nil {
			t.Error(err)
		}
		if len(data) > 256<<10 || len(batch.Reports) > 20 {
			t.Errorf("server limits exceeded: bytes=%d reports=%d", len(data), len(batch.Reports))
		}
		received += len(batch.Reports)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	cl, err := newClient(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	reports := make([]reportPayload, 45)
	for i := range reports {
		reports[i] = reportPayload{ErrorCode: "unknown", Message: strings.Repeat("x", 16000)}
	}
	if err := cl.send(context.Background(), clientid.Identity{}, reports); err != nil {
		t.Fatal(err)
	}
	if received != len(reports) {
		t.Fatalf("received %d reports, want %d", received, len(reports))
	}
}

func TestNativeLogCaptureIsBoundedAndDoesNotReadConsentInCaller(t *testing.T) {
	consentReads := 0
	s, err := newServiceAt(t.TempDir(), clientid.Identity{InstallationID: "i", SessionID: "s"}, func() bool { consentReads++; return true })
	if err != nil {
		t.Fatal(err)
	}
	var local bytes.Buffer
	logger := slog.New(NewLogHandler(slog.NewTextHandler(&local, nil), s)).With("component", "install")
	logger.Error("install failed", "error", errors.New("open /Users/example/game.exe: denied"))
	if consentReads != 0 {
		t.Fatal("logging synchronously read settings")
	}
	e := <-s.logEvents
	s.capture(e.component, e.operation, e.message, e.stack, e.code, false)
	if len(s.queue) != 1 || s.queue[0].Component != "install" || strings.Contains(s.queue[0].Message, "example") {
		t.Fatalf("captured=%+v", s.queue)
	}
	if !strings.Contains(local.String(), "install failed") {
		t.Fatal("local logging lost")
	}
	for i := 0; i < 100; i++ {
		logger.Error("another error")
	}
	if len(s.logEvents) != cap(s.logEvents) {
		t.Fatal("capture queue not bounded")
	}
	for len(s.logEvents) > 0 {
		<-s.logEvents
	}
	s.SetEnabled(false)
	logger.Error("after opt-out")
	if len(s.logEvents) != 0 {
		t.Fatal("captured after opt-out")
	}
}

func TestUploadWaitingIsTerminalTransportStage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Error(err)
			return
		}
		if r.Header.Get("X-Typhon-Upload-ID") == "" {
			t.Error("missing upload correlation")
		}
		if _, err := w.Write([]byte(`{"id":"test"}`)); err != nil {
			t.Error(err)
		}
	}))
	defer srv.Close()
	var states []string
	_, err := postLogBundleWithCallbacks(context.Background(), srv.Client(), srv.URL, clientid.Identity{}, []byte("test"), func(int64, int64) { states = append(states, "sending") }, func() { states = append(states, "waiting") })
	if err != nil {
		t.Fatal(err)
	}
	if len(states) < 2 || states[len(states)-1] != "waiting" {
		t.Fatalf("stages=%v", states)
	}
}

func TestUploadAcceptsResponseAfterOldTwentySecondDeadline(t *testing.T) {
	if testing.Short() {
		t.Skip("real 21s HTTP timeout regression")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Error(err)
			return
		}
		select {
		case <-time.After(21 * time.Second):
			if _, err := w.Write([]byte(`{"id":"late"}`)); err != nil {
				t.Error(err)
			}
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	cl, err := newClientWithTimeout(srv.URL, logUploadHTTPTimeout)
	if err != nil {
		t.Fatal(err)
	}
	id, err := postLogBundle(context.Background(), cl.httpClient, srv.URL, clientid.Identity{}, []byte("test"))
	if err != nil || id != "late" {
		t.Fatalf("id=%q error=%v", id, err)
	}
}
