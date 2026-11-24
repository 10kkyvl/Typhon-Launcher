package main

import (
	"log/slog"
	"net/http"
	"os"

	"typhon/internal/selfupdate"
)

// versionHeader is what the launcher's manifest request says about the
// build that is asking (selfupdate/client.go sets it from app.Version).
const versionHeader = "X-Typhon-Version"

type releaseServer struct {
	releases []release
}

// offerFor picks what to answer a launcher running clientVersion: the oldest
// release newer than it, so a launcher that has just moved to one version of
// the chain is offered the next. With nothing newer it gets the newest one,
// which the launcher reads as "up to date"; an unknown or missing version
// gets the start of the chain.
func (s *releaseServer) offerFor(clientVersion string) release {
	if clientVersion == "" {
		return s.releases[0]
	}
	for _, rel := range s.releases {
		newer, err := selfupdate.IsNewer(rel.version, clientVersion)
		if err != nil {
			return s.releases[0]
		}
		if newer {
			return rel
		}
	}
	return s.releases[len(s.releases)-1]
}

func (s *releaseServer) find(version, name string) (release, bool) {
	for _, rel := range s.releases {
		if rel.version == version && rel.art.Name == name {
			return rel, true
		}
	}
	return release{}, false
}

func (s *releaseServer) handleManifest(w http.ResponseWriter, r *http.Request) {
	client := r.Header.Get(versionHeader)
	rel := s.offerFor(client)
	slog.Info("devrelease: manifest", "client", client, "offered", rel.version)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if _, err := w.Write(rel.signed); err != nil {
		slog.Warn("devrelease: write manifest response", "error", err)
	}
}

func (s *releaseServer) handleDownload(w http.ResponseWriter, r *http.Request) {
	rel, ok := s.find(r.PathValue("version"), r.PathValue("name"))
	if !ok {
		http.NotFound(w, r)
		return
	}

	f, err := os.Open(rel.path)
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
