package account

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

const testDigest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestListSaves(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != APIPrefix+"/me/saves" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("authorization = %q", got)
		}
		writeJSON(t, w, http.StatusOK, map[string]any{
			"games":      []Save{{GameID: "g1", SizeBytes: 5, Digest: testDigest, DeviceID: "d", UpdatedAt: time.Unix(1, 0).UTC()}},
			"usedBytes":  5,
			"quotaBytes": 100,
			"maxBytes":   50,
		})
	}))
	defer srv.Close()

	list, err := newTestClient(t, srv.URL, tokenOK("tok")).ListSaves(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Games) != 1 || list.Games[0].GameID != "g1" || list.UsedBytes != 5 || list.QuotaBytes != 100 || list.MaxBytes != 50 {
		t.Fatalf("list = %+v", list)
	}
}

func TestSavesRequireToken(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	defer srv.Close()

	_, err := newTestClient(t, srv.URL, tokenOK("")).ListSaves(context.Background())
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != CodeUnauthenticated {
		t.Fatalf("err = %v, want unauthenticated", err)
	}
	if called {
		t.Fatal("request reached the server without a token")
	}
}

func TestSaveGameIDIsValidated(t *testing.T) {
	c := newTestClient(t, "https://api.example.com", tokenOK("tok"))
	for _, id := range []string{"", "../x", "a/b", "a b", strings.Repeat("a", 65)} {
		if _, err := c.GetSave(context.Background(), id); err == nil {
			t.Errorf("GetSave(%q) accepted a bad id", id)
		}
		if err := c.DeleteSave(context.Background(), id); err == nil {
			t.Errorf("DeleteSave(%q) accepted a bad id", id)
		}
	}
}

func TestDownloadSaveStreamsBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != APIPrefix+"/me/saves/g1/archive" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Header().Set("X-Typhon-Save-Digest", testDigest)
		if _, err := w.Write([]byte("hello")); err != nil {
			t.Errorf("write: %v", err)
		}
	}))
	defer srv.Close()

	var buf bytes.Buffer
	got, err := newTestClient(t, srv.URL, tokenOK("tok")).DownloadSave(context.Background(), "g1", &buf, 5)
	if err != nil {
		t.Fatal(err)
	}
	if buf.String() != "hello" || got.Size != 5 || got.Digest != testDigest {
		t.Fatalf("body %q result %+v", buf.String(), got)
	}
}

func TestDownloadSaveStopsAtLimit(t *testing.T) {
	cases := []struct {
		name    string
		chunked bool
	}{
		{"declared length", false},
		{"chunked", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body := bytes.Repeat([]byte("x"), 1000)
				if tc.chunked {
					w.WriteHeader(http.StatusOK)
					if f, ok := w.(http.Flusher); ok {
						f.Flush()
					}
				}
				if _, err := w.Write(body); err != nil {
					t.Logf("client went away: %v", err)
				}
			}))
			defer srv.Close()

			var buf bytes.Buffer
			_, err := newTestClient(t, srv.URL, tokenOK("tok")).DownloadSave(context.Background(), "g1", &buf, 100)
			var apiErr *Error
			if !errors.As(err, &apiErr) || apiErr.Code != CodeSaveTooLarge {
				t.Fatalf("err = %v, want %s", err, CodeSaveTooLarge)
			}
			if buf.Len() > 101 {
				t.Fatalf("wrote %d bytes past the limit", buf.Len())
			}
		})
	}
}

func TestDownloadSaveNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusNotFound, map[string]any{"error": map[string]any{"code": CodeSaveNotFound}})
	}))
	defer srv.Close()

	_, err := newTestClient(t, srv.URL, tokenOK("tok")).DownloadSave(context.Background(), "g1", io.Discard, 10)
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != CodeSaveNotFound || apiErr.Status != http.StatusNotFound {
		t.Fatalf("err = %v", err)
	}
}

func TestUploadSaveHeadersAndBody(t *testing.T) {
	cases := []struct {
		name string
		up   SaveUpload
	}{
		{"first upload", SaveUpload{Digest: testDigest, DeviceID: "dev-1"}},
		{"guarded upload", SaveUpload{Digest: testDigest, DeviceID: "dev-1", IfMatch: "abc"}},
		{"forced upload", SaveUpload{Digest: testDigest, DeviceID: "dev-1", IfMatch: "abc", Force: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPut || r.URL.Path != APIPrefix+"/me/saves/g1" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				if r.ContentLength != 7 {
					t.Errorf("content length = %d, want 7", r.ContentLength)
				}
				if got := r.Header.Get("Content-Type"); got != "application/zip" {
					t.Errorf("content type = %q", got)
				}
				if got := r.Header.Get("X-Typhon-Save-Digest"); got != tc.up.Digest {
					t.Errorf("digest header = %q", got)
				}
				if got := r.Header.Get("X-Typhon-Device"); got != "dev-1" {
					t.Errorf("device header = %q", got)
				}
				if got := r.Header.Get("If-Match"); got != tc.up.IfMatch {
					t.Errorf("If-Match = %q, want %q", got, tc.up.IfMatch)
				}
				if _, present := r.Header["If-Match"]; present != (tc.up.IfMatch != "") {
					t.Errorf("If-Match present = %v for %q", present, tc.up.IfMatch)
				}
				wantForce := ""
				if tc.up.Force {
					wantForce = "1"
				}
				if got := r.Header.Get("X-Typhon-Force"); got != wantForce {
					t.Errorf("force header = %q, want %q", got, wantForce)
				}
				data, err := io.ReadAll(r.Body)
				if err != nil || string(data) != "zipdata" {
					t.Errorf("body = %q, err %v", data, err)
				}
				writeJSON(t, w, http.StatusOK, Save{GameID: "g1", SizeBytes: 7, Digest: tc.up.Digest, DeviceID: "dev-1"})
			}))
			defer srv.Close()

			saved, err := newTestClient(t, srv.URL, tokenOK("tok")).UploadSave(
				context.Background(), "g1", strings.NewReader("zipdata"), 7, tc.up)
			if err != nil {
				t.Fatal(err)
			}
			if saved.Digest != tc.up.Digest || saved.SizeBytes != 7 {
				t.Fatalf("saved = %+v", saved)
			}
		})
	}
}

func TestUploadSaveConflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusConflict, map[string]any{
			"error":   map[string]any{"code": CodeSaveConflict},
			"current": Save{GameID: "g1", SizeBytes: 9, Digest: "other", DeviceID: "dev-2"},
		})
	}))
	defer srv.Close()

	_, err := newTestClient(t, srv.URL, tokenOK("tok")).UploadSave(
		context.Background(), "g1", strings.NewReader("zipdata"), 7,
		SaveUpload{Digest: testDigest, DeviceID: "dev-1", IfMatch: "abc"})
	var conflict *ErrSaveConflict
	if !errors.As(err, &conflict) {
		t.Fatalf("err = %v, want ErrSaveConflict", err)
	}
	if conflict.Current == nil || conflict.Current.Digest != "other" || conflict.Current.DeviceID != "dev-2" || conflict.Current.SizeBytes != 9 {
		t.Fatalf("current = %+v", conflict.Current)
	}
}

func TestUploadSaveErrorCodes(t *testing.T) {
	cases := []struct {
		name   string
		status int
		code   string
	}{
		{"quota", http.StatusRequestEntityTooLarge, CodeSaveQuota},
		{"too large", http.StatusRequestEntityTooLarge, CodeSaveTooLarge},
		{"invalid", http.StatusUnprocessableEntity, CodeSaveInvalid},
		{"conflict without current", http.StatusConflict, "other_conflict"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeJSON(t, w, tc.status, map[string]any{"error": map[string]any{"code": tc.code}})
			}))
			defer srv.Close()

			_, err := newTestClient(t, srv.URL, tokenOK("tok")).UploadSave(
				context.Background(), "g1", strings.NewReader("zipdata"), 7,
				SaveUpload{Digest: testDigest, DeviceID: "dev-1"})
			var apiErr *Error
			if !errors.As(err, &apiErr) || apiErr.Code != tc.code || apiErr.Status != tc.status {
				t.Fatalf("err = %v, want code %s status %d", err, tc.code, tc.status)
			}
		})
	}
}

func TestUploadSaveRejectsBadInput(t *testing.T) {
	c := newTestClient(t, "https://api.example.com", tokenOK("tok"))
	cases := []struct {
		name string
		size int64
		up   SaveUpload
	}{
		{"empty body", 0, SaveUpload{Digest: testDigest, DeviceID: "d"}},
		{"short digest", 7, SaveUpload{Digest: "abc", DeviceID: "d"}},
		{"upper digest", 7, SaveUpload{Digest: strings.ToUpper(testDigest), DeviceID: "d"}},
		{"no device", 7, SaveUpload{Digest: testDigest}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := c.UploadSave(context.Background(), "g1", strings.NewReader("zipdata"), tc.size, tc.up)
			if err == nil {
				t.Fatal("bad upload accepted")
			}
		})
	}
}

func TestDeleteSave(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	if err := newTestClient(t, srv.URL, tokenOK("tok")).DeleteSave(context.Background(), "g1"); err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodDelete || gotPath != APIPrefix+"/me/saves/g1" {
		t.Fatalf("request = %s %s", gotMethod, gotPath)
	}
}

func TestDeleteSaveNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusNotFound, map[string]any{"error": map[string]any{"code": CodeSaveNotFound}})
	}))
	defer srv.Close()

	err := newTestClient(t, srv.URL, tokenOK("tok")).DeleteSave(context.Background(), "g1")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != CodeSaveNotFound {
		t.Fatalf("err = %v", err)
	}
}

func TestGetSave(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != APIPrefix+"/me/saves/g1" {
			t.Errorf("path = %s", r.URL.Path)
		}
		writeJSON(t, w, http.StatusOK, Save{GameID: "g1", SizeBytes: 3, Digest: testDigest, DeviceID: "d"})
	}))
	defer srv.Close()

	got, err := newTestClient(t, srv.URL, tokenOK("tok")).GetSave(context.Background(), "g1")
	if err != nil || got.Digest != testDigest {
		t.Fatalf("got %+v err %v", got, err)
	}
}

func TestUploadSaveConflictWithoutCurrentMeansNoRecord(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusConflict, map[string]any{"error": map[string]any{"code": CodeSaveConflict}})
	}))
	defer srv.Close()

	_, err := newTestClient(t, srv.URL, tokenOK("tok")).UploadSave(
		context.Background(), "g1", strings.NewReader("zipdata"), 7,
		SaveUpload{Digest: testDigest, DeviceID: "dev-1", IfMatch: "abc"})
	var conflict *ErrSaveConflict
	if !errors.As(err, &conflict) || conflict.Current != nil {
		t.Fatalf("err = %v, want ErrSaveConflict with a nil Current", err)
	}
}

func TestSaveBusyIsTyped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "7")
		writeJSON(t, w, http.StatusServiceUnavailable, map[string]any{"error": map[string]any{"code": CodeSaveBusy}})
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL, tokenOK("tok"))

	calls := map[string]error{}
	_, calls["upload"] = c.UploadSave(context.Background(), "g1", strings.NewReader("zipdata"), 7, SaveUpload{Digest: testDigest, DeviceID: "d"})
	_, calls["download"] = c.DownloadSave(context.Background(), "g1", io.Discard, 10)
	for name, err := range calls {
		var apiErr *Error
		if !errors.As(err, &apiErr) || apiErr.Code != CodeSaveBusy || apiErr.Status != 503 || apiErr.Message != "7" {
			t.Errorf("%s: err = %v, want saves_busy with Retry-After 7", name, err)
		}
	}
}

func TestOnly503WithSavesBusyCodeIsBusy(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		retryAfter string
		wantCode   string
		wantMsg    string
	}{
		{"busy with seconds", `{"error":{"code":"saves_busy"}}`, "12", CodeSaveBusy, "12"},
		{"busy with a date", `{"error":{"code":"saves_busy"}}`, "Wed, 21 Oct 2026 07:28:00 GMT", CodeSaveBusy, ""},
		{"busy with a negative number", `{"error":{"code":"saves_busy"}}`, "-3", CodeSaveBusy, ""},
		{"unavailable from a proxy", `<html>bad gateway</html>`, "5", CodeServer, ""},
		{"unavailable with another code", `{"error":{"code":"maintenance"}}`, "5", "maintenance", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Retry-After", tc.retryAfter)
				w.WriteHeader(http.StatusServiceUnavailable)
				if _, err := w.Write([]byte(tc.body)); err != nil {
					t.Errorf("write: %v", err)
				}
			}))
			defer srv.Close()
			_, err := newTestClient(t, srv.URL, tokenOK("tok")).ListSaves(context.Background())
			var apiErr *Error
			if !errors.As(err, &apiErr) || apiErr.Code != tc.wantCode || apiErr.Message != tc.wantMsg {
				t.Fatalf("err = %v (%+v), want code %s message %q", err, apiErr, tc.wantCode, tc.wantMsg)
			}
		})
	}
}

func TestTransferClientHasNoOverallTimeoutButSameHardening(t *testing.T) {
	c := newTestClient(t, "https://api.example.com", tokenOK("t"))
	if c.transferHTTP.Timeout != 0 {
		t.Fatalf("transfer client timeout = %v, want none (the caller sets a deadline)", c.transferHTTP.Timeout)
	}
	if c.transferHTTP == c.uploadHTTP || c.transferHTTP == c.httpClient {
		t.Fatal("transfers share a client with other requests")
	}
	if c.transferHTTP.CheckRedirect == nil {
		t.Fatal("transfers follow redirects without re-validating them")
	}
	transport, ok := c.transferHTTP.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport is %T", c.transferHTTP.Transport)
	}
	if transport.TLSHandshakeTimeout <= 0 || transport.ResponseHeaderTimeout <= 0 || transport.DialContext == nil {
		t.Fatalf("transport lacks connect/handshake/header timeouts: %+v", transport)
	}
	via := []*http.Request{{URL: mustURL(t, "https://api.example.com/x")}}
	req := &http.Request{URL: mustURL(t, "https://other.example.org/y"), Header: http.Header{"Authorization": {"Bearer t"}}}
	if err := c.transferHTTP.CheckRedirect(req, via); err != nil {
		t.Fatalf("redirect: %v", err)
	}
	if req.Header.Get("Authorization") != "" {
		t.Fatal("Authorization survived a redirect to another host")
	}
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestDownloadSaveTruncatedBodyIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		if _, err := w.Write([]byte("only ten b")); err != nil {
			t.Errorf("write: %v", err)
		}
	}))
	defer srv.Close()

	var buf bytes.Buffer
	_, err := newTestClient(t, srv.URL, tokenOK("tok")).DownloadSave(context.Background(), "g1", &buf, 1000)
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != CodeNetwork {
		t.Fatalf("err = %v, want a network error for a body shorter than Content-Length", err)
	}
}
