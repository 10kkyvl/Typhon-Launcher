package accountsync

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func testToken() (string, error) { return "tok", nil }

func TestHTTPClientGet(t *testing.T) {
	t.Run("decodes a successful snapshot", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer tok" {
				t.Errorf("missing bearer token, got %q", r.Header.Get("Authorization"))
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"settings":         map[string]any{"theme": "dark"},
				"settingsRevision": 3,
				"games":            []any{map[string]any{"igdbId": 5, "owned": true, "playtimeSeconds": 10}},
			})
		}))
		defer srv.Close()

		c, err := newHTTPClient(srv.URL, testToken)
		if err != nil {
			t.Fatalf("newHTTPClient: %v", err)
		}
		body, err := c.get(context.Background())
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if body.SettingsRevision != 3 || len(body.Games) != 1 || body.Games[0].IGDBID != 5 {
			t.Fatalf("unexpected body: %+v", body)
		}
	})

	t.Run("401 maps to ErrUnauthorized", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeSyncError(w, http.StatusUnauthorized, "unauthenticated", "")
		}))
		defer srv.Close()

		c, err := newHTTPClient(srv.URL, testToken)
		if err != nil {
			t.Fatalf("newHTTPClient: %v", err)
		}
		_, err = c.get(context.Background())
		if !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("expected ErrUnauthorized, got %v", err)
		}
	})

	t.Run("network error is distinct from 401 and validation errors", func(t *testing.T) {
		c, err := newHTTPClient("https://127.0.0.1:1", testToken)
		if err != nil {
			t.Fatalf("newHTTPClient: %v", err)
		}
		_, err = c.get(context.Background())
		var netErr *NetworkError
		if !errors.As(err, &netErr) {
			t.Fatalf("expected *NetworkError, got %T: %v", err, err)
		}
		if errors.Is(err, ErrUnauthorized) {
			t.Fatal("network error must not compare equal to ErrUnauthorized")
		}
	})

	t.Run("credential store failure surfaces as ErrUnauthorized", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Fatal("request should not reach the server without a token")
		}))
		defer srv.Close()

		errNoCred := errors.New("no stored credential")
		c, err := newHTTPClient(srv.URL, func() (string, error) { return "", errNoCred })
		if err != nil {
			t.Fatalf("newHTTPClient: %v", err)
		}
		_, err = c.get(context.Background())
		if err == nil {
			t.Fatal("expected an error")
		}
	})
}

func TestHTTPClientPut(t *testing.T) {
	t.Run("409 sync_conflict maps to ErrConflict", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeSyncError(w, http.StatusConflict, "sync_conflict", "")
		}))
		defer srv.Close()

		c, err := newHTTPClient(srv.URL, testToken)
		if err != nil {
			t.Fatalf("newHTTPClient: %v", err)
		}
		_, err = c.put(context.Background(), putRequest{DeviceID: "d", Games: []wireGame{}})
		if !errors.Is(err, ErrConflict) {
			t.Fatalf("expected ErrConflict, got %v", err)
		}
	})

	t.Run("400 sync_too_many_games maps to ErrTooManyGames", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeSyncError(w, http.StatusBadRequest, "sync_too_many_games", "games")
		}))
		defer srv.Close()

		c, err := newHTTPClient(srv.URL, testToken)
		if err != nil {
			t.Fatalf("newHTTPClient: %v", err)
		}
		_, err = c.put(context.Background(), putRequest{DeviceID: "d", Games: []wireGame{}})
		if !errors.Is(err, ErrTooManyGames) {
			t.Fatalf("expected ErrTooManyGames, got %v", err)
		}
	})

	t.Run("400 sync_device_limit maps to ErrDeviceLimit and is distinct from too-many-games", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeSyncError(w, http.StatusBadRequest, "sync_device_limit", "deviceId")
		}))
		defer srv.Close()

		c, err := newHTTPClient(srv.URL, testToken)
		if err != nil {
			t.Fatalf("newHTTPClient: %v", err)
		}
		_, err = c.put(context.Background(), putRequest{DeviceID: "d", Games: []wireGame{}})
		if !errors.Is(err, ErrDeviceLimit) {
			t.Fatalf("expected ErrDeviceLimit, got %v", err)
		}
		if errors.Is(err, ErrTooManyGames) {
			t.Fatal("device limit must not compare equal to too-many-games")
		}
	})

	t.Run("5xx maps to ServerError, not swallowed as validation", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		c, err := newHTTPClient(srv.URL, testToken)
		if err != nil {
			t.Fatalf("newHTTPClient: %v", err)
		}
		_, err = c.put(context.Background(), putRequest{DeviceID: "d", Games: []wireGame{}})
		var serverErr *ServerError
		if !errors.As(err, &serverErr) {
			t.Fatalf("expected *ServerError, got %T: %v", err, err)
		}
		if serverErr.Status != http.StatusInternalServerError {
			t.Fatalf("unexpected status: %d", serverErr.Status)
		}
	})

	t.Run("successful put returns skipped ids", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req putRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"settings":         map[string]any{},
				"settingsRevision": 1,
				"games":            []any{},
				"skipped":          []int64{99},
			})
		}))
		defer srv.Close()

		c, err := newHTTPClient(srv.URL, testToken)
		if err != nil {
			t.Fatalf("newHTTPClient: %v", err)
		}
		resp, err := c.put(context.Background(), putRequest{DeviceID: "d", Games: []wireGame{{IGDBID: 1}}})
		if err != nil {
			t.Fatalf("put: %v", err)
		}
		if len(resp.Skipped) != 1 || resp.Skipped[0] != 99 {
			t.Fatalf("unexpected skipped list: %+v", resp.Skipped)
		}
	})
}
