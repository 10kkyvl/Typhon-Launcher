package main

import (
	"log/slog"
	"net/http"
	"os"
)

type releaseServer struct {
	signed       []byte
	version      string
	artifactName string
	artifactPath string
}

func (s *releaseServer) handleManifest(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if _, err := w.Write(s.signed); err != nil {
		slog.Warn("devrelease: write manifest response", "error", err)
	}
}

func (s *releaseServer) handleDownload(w http.ResponseWriter, r *http.Request) {
	version := r.PathValue("version")
	name := r.PathValue("name")
	if version != s.version || name != s.artifactName {
		http.NotFound(w, r)
		return
	}

	f, err := os.Open(s.artifactPath)
	if err != nil {
		slog.Error("devrelease: open artifact for download", "error", err)
		http.Error(w, "artifact unavailable", http.StatusInternalServerError)
		return
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			slog.Warn("devrelease: close artifact file", "error", cerr)
		}
	}()

	info, err := f.Stat()
	if err != nil {
		slog.Error("devrelease: stat artifact for download", "error", err)
		http.Error(w, "artifact unavailable", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeContent(w, r, "", info.ModTime(), f)
}

func newMux(s *releaseServer) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /launcher/manifest", s.handleManifest)
	mux.HandleFunc("GET /launcher/download/{version}/{name}", s.handleDownload)
	return mux
}
