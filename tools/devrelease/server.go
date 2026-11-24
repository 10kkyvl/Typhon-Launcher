package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"typhon/internal/selfupdate"
)

// versionHeader is what the launcher's manifest request says about the
// build that is asking (selfupdate/client.go sets it from app.Version).
const versionHeader = "X-Typhon-Version"

type releaseServer struct {
	releases []release
	control  *control
	wait     waitFunc
}

func newReleaseServer(releases []release, seconds int, wait waitFunc) *releaseServer {
	if wait == nil {
		wait = sleepCtx
	}
	return &releaseServer{releases: releases, control: newControl(seconds), wait: wait}
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
	mode := s.control.current()
	s.control.offered(client, rel.version)
	slog.Info("devrelease: manifest", "client", client, "offered", rel.version, "fail", string(mode))

	body := rel.signed
	switch mode {
	case failManifest500:
		http.Error(w, "devrelease: fail=manifest-500", http.StatusInternalServerError)
		return
	case failBadSignature, failUnknownKey:
		forged, err := forgeManifest(rel, mode)
		if err != nil {
			slog.Error("devrelease: forge manifest", "fail", string(mode), "error", err)
			http.Error(w, "devrelease: cannot forge the manifest", http.StatusInternalServerError)
			return
		}
		body = forged
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if _, err := w.Write(body); err != nil {
		slog.Warn("devrelease: write manifest response", "error", err)
	}
}

func (s *releaseServer) handleDownload(w http.ResponseWriter, r *http.Request) {
	rel, ok := s.find(r.PathValue("version"), r.PathValue("name"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	mode := s.control.current()
	slog.Info("devrelease: download", "version", rel.version, "range", r.Header.Get("Range"), "fail", string(mode))
	if mode == failArtifact500 {
		http.Error(w, "devrelease: fail=artifact-500", http.StatusInternalServerError)
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

	// A paced or stalled download outlives the server's WriteTimeout by
	// design; the client's own stall guard is what ends a hung transfer.
	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		slog.Warn("devrelease: lift the write deadline for the download", "error", err)
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	paced := newPacedWriter(r.Context(), w, info.Size(), s.control.pace(), s.wait)
	content := newArtifactReader(r.Context(), f, info.Size(), mode, s.control)
	http.ServeContent(paced, r, "", info.ModTime(), content)
}

// forgeManifest is the release's manifest as a broken server would send it:
// signed by a key the launcher does not hold, or naming a key id it does not
// know.
func forgeManifest(rel release, mode failMode) ([]byte, error) {
	if mode == failBadSignature {
		_, wrong, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("generate the wrong key: %w", err)
		}
		return selfupdate.SignManifest(rel.m, wrong)
	}
	var sm selfupdate.SignedManifest
	if err := json.Unmarshal(rel.signed, &sm); err != nil {
		return nil, fmt.Errorf("decode signed manifest: %w", err)
	}
	sm.KeyID = "typhon-devrelease-unknown"
	out, err := json.Marshal(sm)
	if err != nil {
		return nil, fmt.Errorf("encode signed manifest: %w", err)
	}
	return out, nil
}

func newMux(s *releaseServer) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /launcher/manifest", s.handleManifest)
	mux.HandleFunc("GET /launcher/download/{version}/{name}", s.handleDownload)
	mux.HandleFunc("GET /control", s.handleControlGet)
	mux.HandleFunc("POST /control", s.handleControlPost)
	return loopbackOnly(mux)
}
