package messaging

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestCredentialReadFailureKeepsStreamAndRecoversRequests(t *testing.T) {
	h := newHarness(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"conversations":[]}`)
	})
	awaitKind(t, h.events, "sync")
	h.svc.mu.Lock()
	r := h.svc.run
	h.svc.mu.Unlock()
	storeErr := errors.New("credential file temporarily unavailable")
	h.mu.Lock()
	h.tokenErr = storeErr
	h.mu.Unlock()
	if _, err := h.svc.Conversations(); !errors.Is(err, storeErr) || errors.Is(err, errSignedOut) {
		t.Fatalf("credential error lost: %v", err)
	}
	// Let the actual idle watcher observe a failed credential read.
	select {
	case <-r.ctx.Done():
		t.Fatal("credential read error cancelled stream")
	case <-time.After(1200 * time.Millisecond):
	}
	h.svc.publish(r, Event{Kind: "connection", Connected: false})
	if e := awaitKind(t, h.events, "connection"); e.Connected || e.OwnerID != "a" {
		t.Fatalf("connection status suppressed during storage failure: %+v", e)
	}
	h.mu.Lock()
	h.tokenErr = nil
	h.mu.Unlock()
	if _, err := h.svc.Conversations(); err != nil {
		t.Fatalf("request did not recover without Start: %v", err)
	}
}

func TestAcceptedSendSurvivesCredentialReadFailure(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	h := newHarness(t, func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		select {
		case <-release:
		case <-r.Context().Done():
		}
		fmt.Fprint(w, `{"id":"42","clientId":"sent-once","senderId":"a","recipientId":"b","text":"delivered"}`)
	})
	result := make(chan error, 1)
	go func() {
		message, err := h.svc.Send("b", "sent-once", "delivered")
		if err == nil && message.ID != "42" {
			err = fmt.Errorf("accepted response lost: %+v", message)
		}
		result <- err
	}()
	<-entered
	h.mu.Lock()
	h.tokenErr = errors.New("credential file temporarily unavailable")
	h.mu.Unlock()
	close(release)
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("accepted send reported as failed: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("send did not complete")
	}
}
