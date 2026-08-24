package metadata

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func encodePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func encodeJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

func TestFetchImageSuccess(t *testing.T) {
	pngData := encodePNG(t, 4, 3)
	jpegData := encodeJPEG(t, 5, 6)

	tests := []struct {
		name       string
		body       []byte
		wantFormat string
		wantW      int
		wantH      int
	}{
		{"png", pngData, "png", 4, 3},
		{"jpeg", jpegData, "jpeg", 5, 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if _, err := w.Write(tt.body); err != nil {
					return
				}
			}))
			defer srv.Close()

			img, err := fetchImage(context.Background(), newImageClient(), srv.URL+"/img")
			if err != nil {
				t.Fatalf("fetchImage: %v", err)
			}
			if img.Format != tt.wantFormat {
				t.Fatalf("format = %q, want %q", img.Format, tt.wantFormat)
			}
			if img.Width != tt.wantW || img.Height != tt.wantH {
				t.Fatalf("size = %dx%d, want %dx%d", img.Width, img.Height, tt.wantW, tt.wantH)
			}
			if !bytes.Equal(img.Data, tt.body) {
				t.Fatal("data mismatch")
			}
		})
	}
}

func TestFetchImageNotImage(t *testing.T) {
	tests := []struct {
		name string
		body []byte
	}{
		{"html", []byte("<html><body>not an image</body></html>")},
		{"random bytes", []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if _, err := w.Write(tt.body); err != nil {
					return
				}
			}))
			defer srv.Close()

			_, err := fetchImage(context.Background(), newImageClient(), srv.URL+"/img")
			if !errors.Is(err, errImageFormat) {
				t.Fatalf("err = %v, want errImageFormat", err)
			}
		})
	}
}

func TestFetchImageEmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, err := fetchImage(context.Background(), newImageClient(), srv.URL+"/img")
	if !errors.Is(err, errImageFormat) {
		t.Fatalf("err = %v, want errImageFormat", err)
	}
}

func TestFetchImageStatus(t *testing.T) {
	tests := []int{http.StatusNotFound, http.StatusInternalServerError}
	for _, status := range tests {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
			}))
			defer srv.Close()

			_, err := fetchImage(context.Background(), newImageClient(), srv.URL+"/img")
			if !errors.Is(err, errImageStatus) {
				t.Fatalf("err = %v, want errImageStatus", err)
			}
		})
	}
}

func TestFetchImageTooLarge(t *testing.T) {
	t.Run("declared content length", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", strconv.Itoa(maxImageBytes+4096))
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte("short")); err != nil {
				return
			}
		}))
		defer srv.Close()

		_, err := fetchImage(context.Background(), newImageClient(), srv.URL+"/img")
		if !errors.Is(err, errImageTooLarge) {
			t.Fatalf("err = %v, want errImageTooLarge", err)
		}
	})

	t.Run("actual size without declared content length", func(t *testing.T) {
		body := make([]byte, maxImageBytes+4096)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, err := w.Write(body); err != nil {
				return
			}
		}))
		defer srv.Close()

		_, err := fetchImage(context.Background(), newImageClient(), srv.URL+"/img")
		if !errors.Is(err, errImageTooLarge) {
			t.Fatalf("err = %v, want errImageTooLarge", err)
		}
	})
}

func TestFetchImageInvalidURL(t *testing.T) {
	tests := []string{"", "ftp://host/x", "http://example.com/x", "no-scheme-at-all"}
	for _, raw := range tests {
		t.Run(raw, func(t *testing.T) {
			_, err := fetchImage(context.Background(), newImageClient(), raw)
			if !errors.Is(err, errImageURL) {
				t.Fatalf("err = %v, want errImageURL", err)
			}
		})
	}
}

func TestFetchImageContextCancelled(t *testing.T) {
	body := encodePNG(t, 2, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write(body); err != nil {
			return
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := fetchImage(ctx, newImageClient(), srv.URL+"/img")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestFetchImageRedirectRejectsUnsafeScheme(t *testing.T) {
	tests := []struct {
		name     string
		location string
	}{
		{"ftp", "ftp://evil.example.com/x"},
		{"non-loopback http", "http://evil.example.com/x"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, tt.location, http.StatusFound)
			}))
			defer srv.Close()

			_, err := fetchImage(context.Background(), newImageClient(), srv.URL+"/img")
			if !errors.Is(err, errImageURL) {
				t.Fatalf("err = %v, want errImageURL", err)
			}
		})
	}
}

func TestFetchImageRedirectFollowsLoopback(t *testing.T) {
	body := encodePNG(t, 3, 3)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write(body); err != nil {
			return
		}
	}))
	defer target.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/img", http.StatusFound)
	}))
	defer origin.Close()

	img, err := fetchImage(context.Background(), newImageClient(), origin.URL+"/img")
	if err != nil {
		t.Fatalf("fetchImage: %v", err)
	}
	if img.Format != "png" {
		t.Fatalf("format = %q, want png", img.Format)
	}
	if !bytes.Equal(img.Data, body) {
		t.Fatal("data mismatch after redirect")
	}
}

func TestWriteAssetSuccess(t *testing.T) {
	root := t.TempDir()
	data := []byte("jpeg-bytes")
	img := fetchedImage{Data: data, Format: "jpeg", Width: 1, Height: 1}

	rel, err := writeAsset(root, "game1", "cover", img)
	if err != nil {
		t.Fatalf("writeAsset: %v", err)
	}
	if rel != "games/game1/cover.jpg" {
		t.Fatalf("rel = %q, want games/game1/cover.jpg", rel)
	}

	full := filepath.Join(root, "games", "game1", "cover.jpg")
	got, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("read written asset: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("written content mismatch")
	}
}

func TestWriteAssetPNGExtension(t *testing.T) {
	root := t.TempDir()
	img := fetchedImage{Data: []byte("png-bytes"), Format: "png", Width: 1, Height: 1}

	rel, err := writeAsset(root, "game1", "screenshot1", img)
	if err != nil {
		t.Fatalf("writeAsset: %v", err)
	}
	if rel != "games/game1/screenshot1.png" {
		t.Fatalf("rel = %q, want games/game1/screenshot1.png", rel)
	}
}

func TestWriteAssetInvalidIDs(t *testing.T) {
	tests := []struct {
		name    string
		gameID  string
		assetID string
	}{
		{"empty gameID", "", "cover"},
		{"empty assetID", "game1", ""},
		{"space", "game 1", "cover"},
		{"dotdot", "..", "cover"},
		{"slash", "game1", "a/b"},
		{"windows path", "game1", `C:\x`},
		{"cyrillic", "игра1", "cover"},
		{"too long", strings.Repeat("a", 65), "cover"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			img := fetchedImage{Data: []byte("data"), Format: "jpeg", Width: 1, Height: 1}

			_, err := writeAsset(root, tt.gameID, tt.assetID, img)
			if !errors.Is(err, errAssetPath) {
				t.Fatalf("err = %v, want errAssetPath", err)
			}

			entries, err := os.ReadDir(filepath.Join(root, "games"))
			if err != nil {
				if !os.IsNotExist(err) {
					t.Fatalf("read games dir: %v", err)
				}
			} else if len(entries) != 0 {
				t.Fatal("expected no files created")
			}
		})
	}
}

func TestWriteAssetUnknownFormat(t *testing.T) {
	root := t.TempDir()
	img := fetchedImage{Data: []byte("data"), Format: "gif", Width: 1, Height: 1}

	_, err := writeAsset(root, "game1", "cover", img)
	if !errors.Is(err, errAssetPath) {
		t.Fatalf("err = %v, want errAssetPath", err)
	}
}

func TestWriteAssetEmptyData(t *testing.T) {
	root := t.TempDir()
	img := fetchedImage{Data: nil, Format: "jpeg", Width: 1, Height: 1}

	_, err := writeAsset(root, "game1", "cover", img)
	if !errors.Is(err, errAssetPath) {
		t.Fatalf("err = %v, want errAssetPath", err)
	}
}

func TestAssetPathValid(t *testing.T) {
	root := t.TempDir()
	got, err := assetPath(root, "games/game1/cover.jpg")
	if err != nil {
		t.Fatalf("assetPath: %v", err)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("abs root: %v", err)
	}
	want := filepath.Join(absRoot, "games", "game1", "cover.jpg")
	if got != want {
		t.Fatalf("got = %q, want %q", got, want)
	}
}

func TestAssetPathInvalid(t *testing.T) {
	tests := []struct {
		name string
		rel  string
	}{
		{"empty", ""},
		{"parent traversal", "../../secret"},
		{"unix absolute", "/etc/passwd"},
		{"windows absolute", `C:\Windows\x`},
	}

	root := t.TempDir()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := assetPath(root, tt.rel)
			if !errors.Is(err, errAssetPath) {
				t.Fatalf("err = %v, want errAssetPath", err)
			}
		})
	}
}
