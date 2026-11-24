package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"typhon/internal/selfupdate"
)

func TestParseFlagsChainDefaults(t *testing.T) {
	cfg, err := parseFlags([]string{"-version", "1.4.2"}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if cfg.count != 3 || cfg.notes != "" {
		t.Fatalf("parseFlags() = %+v, want count 3 and the built-in notes", cfg)
	}

	cfg, err = parseFlags([]string{"-version", "1.4.2", "-artifact", "x"}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parseFlags with -artifact: %v", err)
	}
	if cfg.count != 1 {
		t.Fatalf("count with -artifact = %d, want 1: one build carries one version", cfg.count)
	}

	if _, err := parseFlags([]string{"-version", "1.4.2", "-artifact", "x", "-count", "3"}, &bytes.Buffer{}); !errors.Is(err, errArtifactWithCount) {
		t.Fatalf("parseFlags(-artifact, -count 3) error = %v, want errArtifactWithCount", err)
	}
}

func TestVersionChainCountsPatchVersionsUp(t *testing.T) {
	got, err := versionChain("1.4.2", 3)
	if err != nil {
		t.Fatalf("versionChain: %v", err)
	}
	if want := []string{"1.4.2", "1.4.3", "1.4.4"}; !slices.Equal(got, want) {
		t.Fatalf("versionChain() = %q, want %q", got, want)
	}
	if got, err := versionChain("1.4.2-beta.1", 1); err != nil || !slices.Equal(got, []string{"1.4.2-beta.1"}) {
		t.Fatalf("versionChain(prerelease, 1) = %q, %v; a single version is taken as given", got, err)
	}
	if _, err := versionChain("1.4.2-beta.1", 2); !errors.Is(err, errChainNeedsPlainSemver) {
		t.Fatalf("versionChain(prerelease, 2) error = %v, want errChainNeedsPlainSemver", err)
	}
	for _, n := range []int{0, maxCount + 1} {
		if _, err := versionChain("1.4.2", n); !errors.Is(err, errBadCount) {
			t.Fatalf("versionChain(_, %d) error = %v, want errBadCount", n, err)
		}
	}
}

func TestResolveArtifactsBuildsEveryVersionOfTheChain(t *testing.T) {
	var built []string
	build := func(_ context.Context, version, out string) error {
		built = append(built, version)
		return os.WriteFile(out, []byte("launcher "+version), 0o755)
	}
	paths, cleanup, err := resolveArtifacts(t.Context(), config{}, []string{"1.4.2", "1.4.3"}, build)
	if err != nil {
		t.Fatalf("resolveArtifacts: %v", err)
	}
	defer cleanup()
	if !slices.Equal(built, []string{"1.4.2", "1.4.3"}) || len(paths) != 2 {
		t.Fatalf("built %q into %q, want both versions", built, paths)
	}
	for i, v := range built {
		got, err := os.ReadFile(paths[i])
		if err != nil {
			t.Fatalf("read build of %s: %v", v, err)
		}
		if string(got) != "launcher "+v {
			t.Fatalf("artifact %d = %q, want the build of %s", i, got, v)
		}
	}
}

// chainServer serves the default three-version chain from fake artifacts
// whose bytes name their version.
func chainServer(t *testing.T) (*releaseServer, ed25519.PublicKey) {
	t.Helper()
	priv, pub, err := resolveKeyPair("")
	if err != nil {
		t.Fatalf("resolveKeyPair: %v", err)
	}
	versions := []string{"1.4.2", "1.4.3", "1.4.4"}
	dir := t.TempDir()
	paths := make([]string, len(versions))
	for i, v := range versions {
		paths[i] = filepath.Join(dir, v)
		if err := os.WriteFile(paths[i], []byte("launcher "+v), 0o755); err != nil {
			t.Fatalf("write artifact: %v", err)
		}
	}
	releases, err := prepareReleases(versions, paths, 8099, sampleNotes, time.Now().UTC(), priv)
	if err != nil {
		t.Fatalf("prepareReleases: %v", err)
	}
	return &releaseServer{releases: releases}, pub
}

func fetchOffer(t *testing.T, url string, pub ed25519.PublicKey, clientVersion string) selfupdate.Manifest {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url+selfupdate.ManifestPath, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if clientVersion != "" {
		req.Header.Set(versionHeader, clientVersion)
	}
	resp, err := testHTTPClient().Do(req)
	if err != nil {
		t.Fatalf("GET manifest: %v", err)
	}
	defer closeBody(t, resp)
	m, err := selfupdate.VerifyManifest(mustReadAll(t, resp), pub)
	if err != nil {
		t.Fatalf("VerifyManifest: %v", err)
	}
	return m
}

func TestManifestOffersTheVersionAfterTheClients(t *testing.T) {
	srv, pub := chainServer(t)
	ts := httptest.NewServer(newMux(srv))
	defer ts.Close()

	for _, tc := range []struct{ client, want string }{
		{client: "", want: "1.4.2"},
		{client: "1.4.1", want: "1.4.2"},
		{client: "1.4.2", want: "1.4.3"},
		{client: "1.4.3", want: "1.4.4"},
		{client: "1.4.4", want: "1.4.4"},
		{client: "not-a-version", want: "1.4.2"},
	} {
		if got := fetchOffer(t, ts.URL, pub, tc.client).Version; got != tc.want {
			t.Errorf("client %q was offered %s, want %s", tc.client, got, tc.want)
		}
	}
}

func TestManifestCarriesTheReleaseNotesOfTheChain(t *testing.T) {
	srv, pub := chainServer(t)
	ts := httptest.NewServer(newMux(srv))
	defer ts.Close()

	m := fetchOffer(t, ts.URL, pub, "1.4.2")
	if m.Notes != sampleNotes[1].Summary {
		t.Fatalf("Notes = %q, want the summary of 1.4.3", m.Notes)
	}
	var versions []string
	for _, n := range m.Releases {
		versions = append(versions, n.Version)
	}
	if want := []string{"1.4.3", "1.4.2"}; !slices.Equal(versions, want) {
		t.Fatalf("Releases = %q, want %q newest first", versions, want)
	}
	if len(m.Releases[0].Changes) != len(sampleNotes[1].Changes) {
		t.Fatalf("1.4.3 changes = %+v, want the sample entry", m.Releases[0].Changes)
	}
}

func TestDownloadServesTheBuildOfTheRequestedVersion(t *testing.T) {
	srv, _ := chainServer(t)
	ts := httptest.NewServer(newMux(srv))
	defer ts.Close()

	resp, err := testHTTPClient().Get(ts.URL + "/launcher/download/1.4.3/" + artifactName())
	if err != nil {
		t.Fatalf("GET download: %v", err)
	}
	defer closeBody(t, resp)
	if got := mustReadAll(t, resp); string(got) != "launcher 1.4.3" {
		t.Fatalf("body = %q, want the 1.4.3 build", got)
	}
}

func TestNotesFromAFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.json")
	if err := os.WriteFile(path, []byte(`[{"summary":"Own notes","changes":[{"kind":"fixed","text":"Something"}]}]`), 0o600); err != nil {
		t.Fatalf("write notes: %v", err)
	}
	templates, err := loadNotes(path)
	if err != nil {
		t.Fatalf("loadNotes: %v", err)
	}
	if len(templates) != 1 || templates[0].Summary != "Own notes" || templateFor(templates, 2).Summary != "Own notes" {
		t.Fatalf("templates = %+v, want the file's entry for every version", templates)
	}

	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, []byte(`[{"summary":"No changes","changes":[]}]`), 0o600); err != nil {
		t.Fatalf("write notes: %v", err)
	}
	if _, err := loadNotes(bad); !errors.Is(err, selfupdate.ErrInvalidReleaseNote) {
		t.Fatalf("loadNotes(no changes) error = %v, want ErrInvalidReleaseNote: the launcher would refuse the manifest", err)
	}
}
