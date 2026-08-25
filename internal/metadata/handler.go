package metadata

import (
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const mediaURLPrefix = "/" + mediaDirName + "/"

//wails:ignore
func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, mediaURLPrefix) {
			next.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.serveMedia(w, r, strings.TrimPrefix(r.URL.Path, mediaURLPrefix))
	})
}

func (s *Service) serveMedia(w http.ResponseWriter, r *http.Request, rel string) {
	full, err := assetPath(s.store.mediaRoot(), rel)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	f, err := os.Open(full)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("open media asset", "path", full, "error", err)
		}
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	defer func() {
		if err := f.Close(); err != nil {
			slog.Debug("close media asset", "path", full, "error", err)
		}
	}()

	info, err := f.Stat()
	if err != nil || info.IsDir() {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", contentType(full))
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}

func contentType(full string) string {
	switch strings.ToLower(filepath.Ext(full)) {
	case ".png":
		return "image/png"
	default:
		return "image/jpeg"
	}
}
