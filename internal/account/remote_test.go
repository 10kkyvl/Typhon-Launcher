package account

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPostRemoteNotification(t *testing.T) {
	var got map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != APIPrefix+"/me/remote/notifications" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("authorization %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL, tokenOK("tok"))
	err := c.PostRemoteNotification(context.Background(), RemoteNotification{InstallationID: "inst-aaaaaaaa", Kind: "add_failed", Title: "Celeste", Code: "download.no_metadata"})
	if err != nil {
		t.Fatal(err)
	}
	if got["installationId"] != "inst-aaaaaaaa" || got["kind"] != "add_failed" || got["title"] != "Celeste" || got["code"] != "download.no_metadata" {
		t.Fatalf("body %v", got)
	}
}

func TestPostRemoteNotificationCarriesTheBackendCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusNotFound, map[string]any{"error": map[string]string{"code": "not_linked", "message": "x"}})
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL, tokenOK("tok"))
	err := c.PostRemoteNotification(context.Background(), RemoteNotification{InstallationID: "inst-aaaaaaaa", Kind: "add_failed", Title: "x"})
	if err == nil {
		t.Fatal("a refused notification reported success")
	}
}
