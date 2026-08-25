package account

import (
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

var (
	pngBytes  = []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 1, 2, 3, 4}
	jpegBytes = []byte{0xFF, 0xD8, 0xFF, 0xE0, 1, 2, 3, 4}
	webpBytes = []byte{'R', 'I', 'F', 'F', 0, 0, 0, 0, 'W', 'E', 'B', 'P', 1, 2}
)

func writeTempFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func codeOf(t *testing.T, err error) string {
	t.Helper()
	var accErr *Error
	if !errors.As(err, &accErr) {
		t.Fatalf("expected *Error, got %v", err)
	}
	return accErr.Code
}

func TestReadAvatarImage(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent.png")
	oversized := writeTempFile(t, "big.png", append(append([]byte{}, pngBytes...), make([]byte, maxAvatarSize)...))

	tests := []struct {
		name string
		path string
		mime string
		code string
	}{
		{name: "empty path", path: "", code: CodeInvalidAvatar},
		{name: "missing file", path: missing, code: CodeInvalidAvatar},
		{name: "directory", path: t.TempDir(), code: CodeInvalidAvatar},
		{name: "empty file", path: writeTempFile(t, "empty.png", nil), code: CodeInvalidAvatar},
		{name: "not an image", path: writeTempFile(t, "notes.txt", []byte("plain text")), code: CodeUnsupportedAvatar},
		{name: "oversized", path: oversized, code: CodeAvatarTooLarge},
		{name: "png", path: writeTempFile(t, "a.png", pngBytes), mime: "image/png"},
		{name: "jpeg", path: writeTempFile(t, "a.jpg", jpegBytes), mime: "image/jpeg"},
		{name: "webp", path: writeTempFile(t, "a.webp", webpBytes), mime: "image/webp"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			image, err := readAvatarImage(tt.path)
			if tt.code != "" {
				if code := codeOf(t, err); code != tt.code {
					t.Fatalf("expected code %q, got %q", tt.code, code)
				}
				if image.Data != "" || image.MIME != "" {
					t.Fatalf("expected empty image on error, got %+v", image)
				}
				return
			}
			if err != nil {
				t.Fatalf("readAvatarImage() error = %v", err)
			}
			if image.MIME != tt.mime {
				t.Fatalf("expected mime %q, got %q", tt.mime, image.MIME)
			}
			decoded, err := base64.StdEncoding.DecodeString(image.Data)
			if err != nil {
				t.Fatalf("decode payload: %v", err)
			}
			original, err := os.ReadFile(tt.path)
			if err != nil {
				t.Fatalf("read original: %v", err)
			}
			if string(decoded) != string(original) {
				t.Fatalf("payload does not match the file on disk")
			}
		})
	}
}

func TestDecodeAvatar(t *testing.T) {
	tests := []struct {
		name    string
		encoded string
		code    string
	}{
		{name: "empty", encoded: "", code: CodeInvalidAvatar},
		{name: "not base64", encoded: "!!!not base64!!!", code: CodeInvalidAvatar},
		{name: "empty payload", encoded: base64.StdEncoding.EncodeToString(nil), code: CodeInvalidAvatar},
		{name: "not an image", encoded: base64.StdEncoding.EncodeToString([]byte("plain text")), code: CodeUnsupportedAvatar},
		{name: "oversized", encoded: base64.StdEncoding.EncodeToString(make([]byte, maxAvatarSize+1)), code: CodeAvatarTooLarge},
		{name: "png", encoded: base64.StdEncoding.EncodeToString(pngBytes)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := decodeAvatar(tt.encoded)
			if tt.code != "" {
				if code := codeOf(t, err); code != tt.code {
					t.Fatalf("expected code %q, got %q", tt.code, code)
				}
				if data != nil {
					t.Fatalf("expected no payload on error, got %d bytes", len(data))
				}
				return
			}
			if err != nil {
				t.Fatalf("decodeAvatar() error = %v", err)
			}
			if string(data) != string(pngBytes) {
				t.Fatalf("unexpected payload %v", data)
			}
		})
	}
}

func TestUploadAvatarSendsDecodedBytes(t *testing.T) {
	var received []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/me/avatar" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		received = body
		writeJSON(t, w, http.StatusOK, CurrentUser{ID: "u1", AvatarURL: "https://cdn/a.webp"})
	}))
	defer srv.Close()

	s := startedService(t, &fakeStore{cred: Credential{Token: "t"}, present: true}, srv.URL)
	user, err := s.UploadAvatar(base64.StdEncoding.EncodeToString(pngBytes))
	if err != nil {
		t.Fatalf("UploadAvatar() error = %v", err)
	}
	if user.AvatarURL != "https://cdn/a.webp" {
		t.Fatalf("unexpected user: %+v", user)
	}
	if string(received) != string(pngBytes) {
		t.Fatalf("expected decoded payload %v, got %v", pngBytes, received)
	}
}

func TestUploadAvatarRejectsBadPayloadWithoutRequest(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := startedService(t, &fakeStore{cred: Credential{Token: "t"}, present: true}, srv.URL)
	if _, err := s.UploadAvatar(base64.StdEncoding.EncodeToString([]byte("plain text"))); codeOf(t, err) != CodeUnsupportedAvatar {
		t.Fatalf("expected %q", CodeUnsupportedAvatar)
	}
	if _, err := s.UploadAvatar(""); codeOf(t, err) != CodeInvalidAvatar {
		t.Fatalf("expected %q", CodeInvalidAvatar)
	}
	if requests != 0 {
		t.Fatalf("expected zero requests, got %d", requests)
	}
}
