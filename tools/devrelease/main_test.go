package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"typhon/internal/selfupdate"
)

func testHTTPClient() *http.Client {
	return &http.Client{Timeout: 5 * time.Second}
}

func closeBody(t *testing.T, resp *http.Response) {
	t.Helper()
	if err := resp.Body.Close(); err != nil {
		t.Errorf("close response body: %v", err)
	}
}

func TestParseFlagsRequiresVersion(t *testing.T) {
	if _, err := parseFlags(nil, &bytes.Buffer{}); !errors.Is(err, errMissingVersion) {
		t.Fatalf("parseFlags() error = %v, want errMissingVersion", err)
	}
}

func TestParseFlagsDefaults(t *testing.T) {
	cfg, err := parseFlags([]string{"-version", "1.2.3"}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if cfg.version != "1.2.3" || cfg.port != 8099 || cfg.artifact != "" || cfg.seed != "" {
		t.Fatalf("parseFlags() = %+v", cfg)
	}
}

func TestResolveKeyPairGeneratesWhenEmpty(t *testing.T) {
	priv, pub, err := resolveKeyPair("")
	if err != nil {
		t.Fatalf("resolveKeyPair: %v", err)
	}
	if len(priv) != ed25519.PrivateKeySize || len(pub) != ed25519.PublicKeySize {
		t.Fatalf("resolveKeyPair() gave key sizes %d/%d", len(priv), len(pub))
	}
}

func TestResolveKeyPairUsesGivenSeed(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i)
	}
	encoded := base64.StdEncoding.EncodeToString(seed)

	priv, pub, err := resolveKeyPair(encoded)
	if err != nil {
		t.Fatalf("resolveKeyPair: %v", err)
	}
	wantPriv := ed25519.NewKeyFromSeed(seed)
	if !bytes.Equal(priv, wantPriv) {
		t.Fatal("resolveKeyPair() private key does not match the seed")
	}
	wantPub, ok := wantPriv.Public().(ed25519.PublicKey)
	if !ok || !bytes.Equal(pub, wantPub) {
		t.Fatal("resolveKeyPair() public key does not match the seed")
	}
}

func TestResolveKeyPairRejectsBadSeed(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "not base64", value: "not*base64"},
		{name: "wrong length", value: base64.StdEncoding.EncodeToString([]byte("short"))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := resolveKeyPair(tt.value); !errors.Is(err, errBadSeed) {
				t.Fatalf("resolveKeyPair(%q) error = %v, want errBadSeed", tt.value, err)
			}
		})
	}
}

func TestResolveArtifactUsesGivenFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "typhon")
	if err := os.WriteFile(path, []byte("binary"), 0o755); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	got, cleanup, err := resolveArtifact(t.Context(), config{artifact: path}, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("resolveArtifact: %v", err)
	}
	defer cleanup()
	if got != path {
		t.Fatalf("resolveArtifact() = %q, want %q", got, path)
	}
}

func TestResolveArtifactMissingFile(t *testing.T) {
	if _, _, err := resolveArtifact(t.Context(), config{artifact: filepath.Join(t.TempDir(), "missing")}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("resolveArtifact() error = nil, want an error for a missing artifact")
	}
}

func TestResolveArtifactRequiresRepoRootWhenBuilding(t *testing.T) {
	t.Chdir(t.TempDir())
	if _, _, err := resolveArtifact(t.Context(), config{}, &bytes.Buffer{}, &bytes.Buffer{}); !errors.Is(err, errNotRepoRoot) {
		t.Fatalf("resolveArtifact() error = %v, want errNotRepoRoot", err)
	}
}

func TestBuildSignedManifestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	artifactPath := filepath.Join(dir, "typhon")
	content := []byte("fake launcher binary")
	if err := os.WriteFile(artifactPath, content, 0o755); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	priv, pub, err := resolveKeyPair("")
	if err != nil {
		t.Fatalf("resolveKeyPair: %v", err)
	}

	signed, art, err := buildSignedManifest("1.2.3", artifactPath, 8099, priv)
	if err != nil {
		t.Fatalf("buildSignedManifest: %v", err)
	}
	if art.Name != artifactName() || art.OS != runtime.GOOS || art.Arch != runtime.GOARCH {
		t.Fatalf("artifact = %+v", art)
	}
	if art.Size != int64(len(content)) {
		t.Fatalf("artifact.Size = %d, want %d", art.Size, len(content))
	}

	m, err := selfupdate.VerifyManifest(signed, pub)
	if err != nil {
		t.Fatalf("VerifyManifest: %v", err)
	}
	if m.Version != "1.2.3" || len(m.Artifacts) != 1 || m.Artifacts[0].Name != art.Name {
		t.Fatalf("verified manifest = %+v", m)
	}

	wantURL := "http://127.0.0.1:8099/launcher/download/1.2.3/" + art.Name
	if m.Artifacts[0].URL != wantURL {
		t.Fatalf("artifact url = %q, want %q", m.Artifacts[0].URL, wantURL)
	}
}

func TestHandleManifestServesSignedBytesWithNoStore(t *testing.T) {
	priv, pub, err := resolveKeyPair("")
	if err != nil {
		t.Fatalf("resolveKeyPair: %v", err)
	}
	signed, _, err := buildSignedManifest("1.2.3", writeFakeArtifact(t), 8099, priv)
	if err != nil {
		t.Fatalf("buildSignedManifest: %v", err)
	}

	srv := &releaseServer{signed: signed, version: "1.2.3"}
	ts := httptest.NewServer(newMux(srv))
	defer ts.Close()

	resp, err := testHTTPClient().Get(ts.URL + "/launcher/manifest")
	if err != nil {
		t.Fatalf("GET /launcher/manifest: %v", err)
	}
	defer closeBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}

	m, err := selfupdate.VerifyManifest(mustReadAll(t, resp), pub)
	if err != nil {
		t.Fatalf("VerifyManifest on served body: %v", err)
	}
	if m.Version != "1.2.3" {
		t.Fatalf("served manifest version = %q", m.Version)
	}
}

func TestHandleDownloadServesArtifactBytes(t *testing.T) {
	content := []byte("fake launcher binary bytes")
	path := filepath.Join(t.TempDir(), "typhon")
	if err := os.WriteFile(path, content, 0o755); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	srv := &releaseServer{version: "1.2.3", artifactName: "typhon-devmock", artifactPath: path}
	ts := httptest.NewServer(newMux(srv))
	defer ts.Close()

	resp, err := testHTTPClient().Get(ts.URL + "/launcher/download/1.2.3/typhon-devmock")
	if err != nil {
		t.Fatalf("GET download: %v", err)
	}
	defer closeBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Accept-Ranges"); got != "bytes" {
		t.Fatalf("Accept-Ranges = %q, want bytes", got)
	}
	got := mustReadAll(t, resp)
	if !bytes.Equal(got, content) {
		t.Fatalf("body = %q, want %q", got, content)
	}
}

func TestHandleDownloadUnknownVersionOrNameIs404(t *testing.T) {
	path := filepath.Join(t.TempDir(), "typhon")
	if err := os.WriteFile(path, []byte("x"), 0o755); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	srv := &releaseServer{version: "1.2.3", artifactName: "typhon-devmock", artifactPath: path}
	ts := httptest.NewServer(newMux(srv))
	defer ts.Close()

	tests := []string{
		"/launcher/download/9.9.9/typhon-devmock",
		"/launcher/download/1.2.3/bogus",
		"/launcher/unknown",
	}
	for _, target := range tests {
		t.Run(target, func(t *testing.T) {
			resp, err := testHTTPClient().Get(ts.URL + target)
			if err != nil {
				t.Fatalf("GET %s: %v", target, err)
			}
			defer closeBody(t, resp)
			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("GET %s status = %d, want 404", target, resp.StatusCode)
			}
		})
	}
}

func writeFakeArtifact(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "typhon")
	if err := os.WriteFile(path, []byte("fake launcher"), 0o755); err != nil {
		t.Fatalf("write fake artifact: %v", err)
	}
	return path
}

func TestHandleDownloadServesRange(t *testing.T) {
	content := []byte("fake launcher binary bytes")
	path := filepath.Join(t.TempDir(), "typhon")
	if err := os.WriteFile(path, content, 0o755); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	srv := &releaseServer{version: "1.2.3", artifactName: "typhon-devmock", artifactPath: path}
	ts := httptest.NewServer(newMux(srv))
	defer ts.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/launcher/download/1.2.3/typhon-devmock", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Range", "bytes=5-")
	resp, err := testHTTPClient().Do(req)
	if err != nil {
		t.Fatalf("GET download range: %v", err)
	}
	defer closeBody(t, resp)

	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", resp.StatusCode)
	}
	wantRange := fmt.Sprintf("bytes 5-%d/%d", len(content)-1, len(content))
	if got := resp.Header.Get("Content-Range"); got != wantRange {
		t.Fatalf("Content-Range = %q, want %q", got, wantRange)
	}
	got := mustReadAll(t, resp)
	if !bytes.Equal(got, content[5:]) {
		t.Fatalf("body = %q, want %q", got, content[5:])
	}
}

func mustReadAll(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return buf.Bytes()
}
