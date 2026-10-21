package messaging

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
	"typhon/internal/uierr"
)

var pngHeader = []byte("\x89PNG\r\n\x1a\n")

func pngBytes(size int) []byte {
	data := make([]byte, size)
	copy(data, pngHeader)
	return data
}

func writeFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

type endlessReader struct{ read int64 }

func (r *endlessReader) Read(p []byte) (int, error) {
	r.read += int64(len(p))
	return len(p), nil
}

func TestReadLimitedStopsOneByteAboveTheLimit(t *testing.T) {
	source := &endlessReader{}
	_, err := readLimited(source)
	if uierr.Code(err) != codeImageTooLarge {
		t.Fatalf("err=%v", err)
	}
	if source.read != maxChatImage+1 {
		t.Fatalf("read %d bytes, limit is %d", source.read, maxChatImage+1)
	}
}

func TestReadChatImage(t *testing.T) {
	ctx := context.Background()
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	dir := t.TempDir()
	tests := []struct {
		name string
		path string
		code string
	}{
		{"too large", writeFile(t, "big.png", pngBytes(maxChatImage+1024)), codeImageTooLarge},
		{"not an image", writeFile(t, "note.png", []byte("just text, not a picture")), codeImageNotImage},
		{"empty file", writeFile(t, "empty.png", nil), codeImageNotImage},
		{"missing", filepath.Join(dir, "missing.png"), codeImageUnreadable},
		{"directory", dir, codeImageUnreadable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := readChatImage(ctx, tt.path)
			if uierr.Code(err) != tt.code {
				t.Fatalf("code=%q err=%v", uierr.Code(err), err)
			}
			if got != (ChatImage{}) {
				t.Fatalf("result must stay empty on error: %+v", got)
			}
		})
	}
	t.Run("cancelled context", func(t *testing.T) {
		_, err := readChatImage(cancelled, writeFile(t, "a.png", pngBytes(32)))
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("exactly the limit", func(t *testing.T) {
		got, err := readChatImage(ctx, writeFile(t, "max.png", pngBytes(maxChatImage)))
		if err != nil || got.Bytes != maxChatImage {
			t.Fatalf("got=%d err=%v", got.Bytes, err)
		}
	})
}

func TestReadChatImageDescribesTheFile(t *testing.T) {
	data := append(append([]byte{}, pngHeader...), 1, 2, 3)
	got, err := readChatImage(context.Background(), writeFile(t, "shot.png", data))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.StdEncoding.DecodeString(got.Encoded)
	if err != nil || !bytes.Equal(decoded, data) {
		t.Fatalf("encoded payload does not round-trip: %v", err)
	}
	if got.Name != "shot.png" || got.MIME != "image/png" || got.Bytes != int64(len(data)) {
		t.Fatalf("got=%+v", got)
	}
}

func TestImageMIMEBySignature(t *testing.T) {
	tests := map[string][]byte{
		"image/png":  pngHeader,
		"image/jpeg": {0xFF, 0xD8, 0xFF, 0xE0},
		"image/webp": []byte("RIFF\x00\x00\x00\x00WEBPVP8 "),
		"image/gif":  []byte("GIF89a"),
	}
	for want, data := range tests {
		if got, ok := imageMIME(data); !ok || got != want {
			t.Errorf("%s: got %q ok=%v", want, got, ok)
		}
	}
	for name, data := range map[string][]byte{"empty": nil, "text": []byte("hello"), "short riff": []byte("RIFF"), "svg": []byte("<svg></svg>")} {
		if _, ok := imageMIME(data); ok {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestDecodeChatImage(t *testing.T) {
	enc := func(data []byte) string { return base64.StdEncoding.EncodeToString(data) }
	tests := []struct {
		name    string
		encoded string
		code    string
	}{
		{"valid", enc(pngBytes(16)), ""},
		{"exactly the limit", enc(pngBytes(maxChatImage)), ""},
		{"one byte over", enc(pngBytes(maxChatImage + 1)), codeImageTooLarge},
		{"far over without decoding", strings.Repeat("A", 2*maxChatImage), codeImageTooLarge},
		{"not base64", "%%%not base64%%%", codeImageNotImage},
		{"not an image", enc([]byte("plain text")), codeImageNotImage},
		{"empty", "", codeImageNotImage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeChatImage(tt.encoded)
			if tt.code == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if uierr.Code(err) != tt.code {
				t.Fatalf("code=%q err=%v", uierr.Code(err), err)
			}
		})
	}
}

const attachmentJSON = `{"id":"a1","url":"https://cdn.example/a1.webp","width":640,"height":480,"bytes":1234,"expiresAt":"2026-11-01T00:00:00Z"}`

func TestUploadChatImageSendsOctetStreamWithBearer(t *testing.T) {
	payload := pngBytes(4096)
	var mu sync.Mutex
	var seen *http.Request
	var body []byte
	h := newHarness(t, func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		mu.Lock()
		seen, body = r, data
		mu.Unlock()
		if _, err := fmt.Fprint(w, attachmentJSON); err != nil {
			t.Error(err)
		}
	})
	got, err := h.svc.UploadChatImage(context.Background(), base64.StdEncoding.EncodeToString(payload))
	if err != nil {
		t.Fatal(err)
	}
	if got != (Attachment{ID: "a1", URL: "https://cdn.example/a1.webp", Width: 640, Height: 480, Bytes: 1234}) {
		t.Fatalf("got=%+v", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if seen.Method != http.MethodPost || seen.URL.Path != prefix+"/attachments" {
		t.Fatalf("request=%s %s", seen.Method, seen.URL.Path)
	}
	if seen.Header.Get("Content-Type") != "application/octet-stream" || seen.Header.Get("Authorization") != "Bearer token-a" || seen.Header.Get("X-Typhon-Version") == "" {
		t.Fatalf("headers=%v", seen.Header)
	}
	if seen.ContentLength != int64(len(payload)) || !bytes.Equal(body, payload) {
		t.Fatalf("content length=%d body=%d bytes", seen.ContentLength, len(body))
	}
}

func TestUploadChatImageMapsServerFailures(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		code   string
	}{
		{"too large code", http.StatusBadRequest, `{"error":{"code":"chat_attachment_too_large"}}`, codeAttachmentTooLarge},
		{"not image", http.StatusUnsupportedMediaType, `{"error":{"code":"chat_attachment_not_image"}}`, codeAttachmentNotImage},
		{"invalid", http.StatusUnprocessableEntity, `{"error":{"code":"chat_attachment_invalid"}}`, codeAttachmentInvalid},
		{"rate limited", http.StatusTooManyRequests, `{"error":{"code":"rate_limited"}}`, codeRateLimited},
		{"muted", http.StatusForbidden, `{"error":{"code":"muted"}}`, codeMuted},
		{"413 without envelope", http.StatusRequestEntityTooLarge, `<html>too big</html>`, codeAttachmentTooLarge},
		{"unknown code stays raw", http.StatusInternalServerError, `{"error":{"code":"boom"}}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				if _, err := fmt.Fprint(w, tt.body); err != nil {
					t.Error(err)
				}
			})
			_, err := h.svc.UploadChatImage(context.Background(), base64.StdEncoding.EncodeToString(pngBytes(32)))
			if err == nil {
				t.Fatal("failure was swallowed")
			}
			if got := uierr.Code(err); got != tt.code {
				t.Fatalf("code=%q err=%v", got, err)
			}
			var failure *responseFailure
			if !errors.As(err, &failure) || failure.status != tt.status {
				t.Fatalf("server status is lost: %v", err)
			}
		})
	}
}

func TestUploadChatImageUnauthorizedIsSignedOut(t *testing.T) {
	h := newHarness(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) })
	_, err := h.svc.UploadChatImage(context.Background(), base64.StdEncoding.EncodeToString(pngBytes(32)))
	if !errors.Is(err, errSignedOut) {
		t.Fatalf("err=%v", err)
	}
}

func TestUploadChatImageRejectsBadResponses(t *testing.T) {
	huge := `{"id":"a1","url":"https://cdn.example/a.webp","pad":"` + strings.Repeat("x", maxUploadResponse) + `"}`
	tests := map[string]string{
		"over 2 MiB":  huge,
		"not json":    `<html>`,
		"empty id":    `{"id":"","url":"https://cdn.example/a.webp"}`,
		"http url":    `{"id":"a1","url":"http://cdn.example/a.webp"}`,
		"missing url": `{"id":"a1"}`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, func(w http.ResponseWriter, r *http.Request) {
				if _, err := fmt.Fprint(w, body); err != nil {
					t.Error(err)
				}
			})
			got, err := h.svc.UploadChatImage(context.Background(), base64.StdEncoding.EncodeToString(pngBytes(32)))
			if err == nil || got != (Attachment{}) {
				t.Fatalf("got=%+v err=%v", got, err)
			}
		})
	}
}

func TestUploadChatImageValidatesBeforeTheNetwork(t *testing.T) {
	var hits int
	var mu sync.Mutex
	h := newHarness(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
	})
	for _, encoded := range []string{"", "!!!", base64.StdEncoding.EncodeToString([]byte("not an image"))} {
		if _, err := h.svc.UploadChatImage(context.Background(), encoded); err == nil {
			t.Fatalf("%q accepted", encoded)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if hits != 0 {
		t.Fatalf("invalid input reached the server %d times", hits)
	}
}

func TestUploadChatImageStopsWithTheCallerContext(t *testing.T) {
	entered := make(chan struct{})
	h := newHarness(t, func(w http.ResponseWriter, r *http.Request) {
		// The server only notices a closed connection once the body is consumed.
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Error(err)
		}
		close(entered)
		<-r.Context().Done()
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := h.svc.UploadChatImage(ctx, base64.StdEncoding.EncodeToString(pngBytes(32)))
		done <- err
	}()
	<-entered
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
}

func TestUploadChatImageStopsWithTheSession(t *testing.T) {
	entered := make(chan struct{})
	h := newHarness(t, func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Error(err)
		}
		close(entered)
		<-r.Context().Done()
	})
	done := make(chan error, 1)
	go func() {
		_, err := h.svc.UploadChatImage(context.Background(), base64.StdEncoding.EncodeToString(pngBytes(32)))
		done <- err
	}()
	<-entered
	h.svc.Stop()
	if err := <-done; err == nil {
		t.Fatal("upload survived the end of the session")
	}
}

func TestUploadChatImageNeedsASession(t *testing.T) {
	s := &Service{}
	if _, err := s.UploadChatImage(context.Background(), base64.StdEncoding.EncodeToString(pngBytes(32))); !errors.Is(err, errSignedOut) {
		t.Fatalf("err=%v", err)
	}
}

func TestMessageAttachmentsFromStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/me" {
			if _, err := fmt.Fprint(w, `{"id":"a"}`); err != nil {
				t.Error(err)
			}
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if _, err := fmt.Fprint(w, "data: {\"kind\":\"message\",\"peerId\":\"b\",\"message\":{\"id\":\"42\",\"text\":\"\",\"attachments\":[{\"id\":\"a1\",\"url\":\"https://cdn.example/a1.webp\",\"width\":10,\"height\":20,\"bytes\":30}]}}\n\n"); err != nil {
			t.Error(err)
			return
		}
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Error(err)
			return
		}
		<-r.Context().Done()
	}))
	defer server.Close()
	s, err := NewService(server.URL, func() (string, error) { return "token", nil }, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Error(err)
		}
	}()
	events := make(chan Event, 32)
	s.emit = func(e Event) { events <- e }
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	e := awaitKind(t, events, "message")
	if e.Message == nil || len(e.Message.Attachments) != 1 || e.Message.Attachments[0] != (Attachment{ID: "a1", URL: "https://cdn.example/a1.webp", Width: 10, Height: 20, Bytes: 30}) {
		t.Fatalf("event=%+v", e)
	}
}

func TestPopupBodyNamesAnImageOnlyMessage(t *testing.T) {
	for _, body := range []string{"", "  \n"} {
		if got := popupBody(body); got != imageNotice {
			t.Errorf("body %q -> %q", body, got)
		}
	}
	if got := popupBody("привет"); got != "привет" {
		t.Fatalf("text replaced: %q", got)
	}
}
