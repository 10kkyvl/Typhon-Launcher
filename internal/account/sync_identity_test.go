package account

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSyncIdentityDoesNotWaitForAvatarDuringLogin(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	store := &fakeStore{present: true, cred: Credential{Token: "token-A", Username: "alice"}}
	s := startedService(t, store, server.URL)
	s.profile = cachedProfile{User: CurrentUser{ID: "A", Username: "alice"}}
	s.bindSyncIdentity("token-A", "A")
	result := make(chan error, 1)
	go func() {
		_, err := s.adopt(context.Background(), Session{Token: "token-B", User: CurrentUser{ID: "B", Username: "bob", AvatarURL: server.URL + "/avatar"}})
		result <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("avatar request not started")
	}
	got := s.SyncAccountID("token-B")
	old := s.SyncAccountID("token-A")
	close(release)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if got != "B" || old != "" {
		t.Fatalf("wrong identity while avatar cache is stale: new=%q old=%q", got, old)
	}
}
