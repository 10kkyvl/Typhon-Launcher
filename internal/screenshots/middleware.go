package screenshots

import (
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

//wails:ignore
func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, URLPrefix) {
			next.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		s.serve(w, r, strings.TrimPrefix(r.URL.Path, URLPrefix))
	})
}

func (s *Service) serve(w http.ResponseWriter, r *http.Request, rel string) {
	gameID, name, ok := strings.Cut(rel, "/")
	if !ok || !validSegment(gameID) || !validShotName(name) {
		http.NotFound(w, r)
		return
	}
	root := s.config().ScreenshotsPath
	if root == "" {
		http.NotFound(w, r)
		return
	}
	full := filepath.Join(root, gameID, name)
	info, err := os.Lstat(full) //nolint:gosec // G703: gameID and name passed validSegment/validShotName above (invariant 32): no separators, no "..", fixed .png extension.
	if err != nil || !info.Mode().IsRegular() {
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("stat screenshot", "path", full, "error", err)
		}
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	http.ServeFile(w, r, full) //nolint:gosec // G703: same validated gameID and name as the Lstat above (invariant 32).
}
