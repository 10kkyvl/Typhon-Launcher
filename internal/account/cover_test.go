package account

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestReadCoverImage(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent.png")
	oversized := writeTempFile(t, "big.png", append(append([]byte{}, pngBytes...), make([]byte, maxCoverSize)...))

	tests := []struct {
		name string
		path string
		mime string
		code string
	}{
		{name: "empty path", path: "", code: CodeInvalidCover},
		{name: "missing file", path: missing, code: CodeInvalidCover},
		{name: "directory", path: t.TempDir(), code: CodeInvalidCover},
		{name: "empty file", path: writeTempFile(t, "empty.png", nil), code: CodeInvalidCover},
		{name: "not an image", path: writeTempFile(t, "notes.txt", []byte("plain text")), code: CodeUnsupportedCover},
		{name: "gif goes to the server, which decides on animation", path: writeTempFile(t, "a.gif", gifBytes), mime: "image/gif"},
		{name: "oversized", path: oversized, code: CodeCoverTooLarge},
		{name: "png", path: writeTempFile(t, "a.png", pngBytes), mime: "image/png"},
		{name: "jpeg", path: writeTempFile(t, "a.jpg", jpegBytes), mime: "image/jpeg"},
		{name: "webp", path: writeTempFile(t, "a.webp", webpBytes), mime: "image/webp"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			image, err := readCoverImage(tt.path)
			if tt.code != "" {
				if code := codeOf(t, err); code != tt.code {
					t.Fatalf("code = %q, want %q", code, tt.code)
				}
				if image.Data != "" || image.MIME != "" {
					t.Fatalf("an error came with an image: %+v", image)
				}
				return
			}
			if err != nil {
				t.Fatalf("readCoverImage: %v", err)
			}
			if image.MIME != tt.mime {
				t.Fatalf("mime = %q, want %q", image.MIME, tt.mime)
			}
			decoded, err := base64.StdEncoding.DecodeString(image.Data)
			if err != nil {
				t.Fatalf("decode payload: %v", err)
			}
			original, err := os.ReadFile(filepath.Clean(tt.path))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(decoded, original) {
				t.Fatal("payload does not match the file on disk")
			}
		})
	}
}

func TestDecodeCover(t *testing.T) {
	tests := []struct {
		name    string
		encoded string
		code    string
		payload []byte
	}{
		{name: "empty", encoded: "", code: CodeInvalidCover},
		{name: "not base64", encoded: "!!!not base64!!!", code: CodeInvalidCover},
		{name: "decodes to nothing", encoded: "====", code: CodeInvalidCover},
		{name: "not an image", encoded: base64.StdEncoding.EncodeToString([]byte("plain text")), code: CodeUnsupportedCover},
		{name: "gif", encoded: base64.StdEncoding.EncodeToString(gifBytes), payload: gifBytes},
		{name: "oversized", encoded: base64.StdEncoding.EncodeToString(make([]byte, maxCoverSize+1)), code: CodeCoverTooLarge},
		{name: "png", encoded: base64.StdEncoding.EncodeToString(pngBytes), payload: pngBytes},
		{name: "jpeg", encoded: base64.StdEncoding.EncodeToString(jpegBytes), payload: jpegBytes},
		{name: "webp", encoded: base64.StdEncoding.EncodeToString(webpBytes), payload: webpBytes},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := decodeCover(tt.encoded)
			if tt.code != "" {
				if code := codeOf(t, err); code != tt.code {
					t.Fatalf("code = %q, want %q", code, tt.code)
				}
				if data != nil {
					t.Fatalf("an error came with %d bytes", len(data))
				}
				return
			}
			if err != nil {
				t.Fatalf("decodeCover: %v", err)
			}
			if !bytes.Equal(data, tt.payload) {
				t.Fatalf("payload = %v", data)
			}
		})
	}
}
