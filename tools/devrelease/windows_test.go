package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"typhon/internal/selfupdate"
)

func TestParseFlagsWindowsMode(t *testing.T) {
	cfg, err := parseFlags([]string{"-version", "1.4.2"}, &bytes.Buffer{})
	if err != nil || cfg.windows {
		t.Fatalf("parseFlags() = %+v, %v; want the macOS mode by default", cfg, err)
	}
	cfg, err = parseFlags([]string{"-version", "1.4.2", "-windows"}, &bytes.Buffer{})
	if err != nil || !cfg.windows {
		t.Fatalf("parseFlags(-windows) = %+v, %v; want the Windows mode", cfg, err)
	}
}

// fakeExes writes one stand-in typhon.exe per version whose bytes name it.
func fakeExes(t *testing.T, versions []string) []string {
	t.Helper()
	dir := t.TempDir()
	paths := make([]string, len(versions))
	for i, v := range versions {
		paths[i] = filepath.Join(dir, v, "typhon.exe")
		if err := os.MkdirAll(filepath.Dir(paths[i]), 0o755); err != nil {
			t.Fatalf("create exe dir: %v", err)
		}
		if err := os.WriteFile(paths[i], []byte("qa launcher "+v), 0o755); err != nil {
			t.Fatalf("write exe: %v", err)
		}
	}
	return paths
}

func windowsChain(t *testing.T, versions []string, port int) ([]release, ed25519.PublicKey) {
	t.Helper()
	priv, pub, err := resolveKeyPair("")
	if err != nil {
		t.Fatalf("resolveKeyPair: %v", err)
	}
	releases, err := prepareWindowsReleases(versions, fakeExes(t, versions), t.TempDir(), port, sampleNotes, time.Now().UTC(), priv)
	if err != nil {
		t.Fatalf("prepareWindowsReleases: %v", err)
	}
	return releases, pub
}

func fileDigest(t *testing.T, data []byte) (int64, string) {
	t.Helper()
	sum := sha256.Sum256(data)
	return int64(len(data)), hex.EncodeToString(sum[:])
}

// The zip is what the portable launcher's own check accepts, and its one
// entry is the launcher built for that version.
func TestWindowsZipIsWhatThePortableLauncherApplies(t *testing.T) {
	releases, _ := windowsChain(t, []string{"1.4.2", "1.4.3"}, 8099)
	for _, rel := range releases {
		if err := selfupdate.CheckPortableZip(rel.portablePath); err != nil {
			t.Fatalf("CheckPortableZip(%s): %v", rel.version, err)
		}
		zr, err := zip.OpenReader(rel.portablePath)
		if err != nil {
			t.Fatalf("open zip: %v", err)
		}
		if len(zr.File) != 1 || zr.File[0].Name != "typhon.exe" {
			t.Fatalf("zip of %s holds %d entries; want exactly typhon.exe", rel.version, len(zr.File))
		}
		rc, err := zr.File[0].Open()
		if err != nil {
			t.Fatalf("open typhon.exe in the zip: %v", err)
		}
		body, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("read typhon.exe in the zip: %v", err)
		}
		if err := rc.Close(); err != nil {
			t.Fatalf("close zip entry: %v", err)
		}
		if err := zr.Close(); err != nil {
			t.Fatalf("close zip: %v", err)
		}
		if string(body) != "qa launcher "+rel.version {
			t.Fatalf("typhon.exe in the zip of %s = %q", rel.version, body)
		}
	}
}

func TestWindowsManifestOffersThePortableZip(t *testing.T) {
	releases, pub := windowsChain(t, []string{"1.4.2", "1.4.3"}, 8099)
	for _, rel := range releases {
		m, err := selfupdate.VerifyManifest(rel.signed, pub)
		if err != nil {
			t.Fatalf("VerifyManifest(%s): %v", rel.version, err)
		}
		if m.Version != rel.version || len(m.Artifacts) != 1 {
			t.Fatalf("manifest = %+v", m)
		}
		inst := m.Artifacts[0]
		if inst.OS != "windows" || inst.Arch != "amd64" || inst.Kind != selfupdate.KindInstaller {
			t.Fatalf("installer entry = %+v, want windows/amd64 installer", inst)
		}
		if !strings.Contains(inst.Name, "not-an-installer") || strings.HasSuffix(strings.ToLower(inst.Name), ".exe") {
			t.Fatalf("installer entry name %q could pass for an installer", inst.Name)
		}

		art, err := m.PortableArtifactFor("windows", "amd64")
		if err != nil {
			t.Fatalf("PortableArtifactFor: %v", err)
		}
		zipBytes, err := os.ReadFile(rel.portablePath)
		if err != nil {
			t.Fatalf("read zip: %v", err)
		}
		size, sum := fileDigest(t, zipBytes)
		wantName := "typhon-" + rel.version + "-portable.zip"
		wantURL := "http://127.0.0.1:8099/launcher/download/" + rel.version + "/" + wantName
		if art.Name != wantName || art.URL != wantURL || art.Size != size || art.SHA256 != sum {
			t.Fatalf("portable artifact = %+v, want %s at %s, %d bytes, %s", art, wantName, wantURL, size, sum)
		}
	}
}

// Both names the manifest announces are served, each with the size and hash
// the signed manifest gives for it.
func TestWindowsChainServesBothFiles(t *testing.T) {
	releases, pub := windowsChain(t, []string{"1.4.2"}, 8099)
	ts := httptest.NewServer(newMux(newReleaseServer(releases, 0, nil)))
	defer ts.Close()

	m, err := selfupdate.VerifyManifest(releases[0].signed, pub)
	if err != nil {
		t.Fatalf("VerifyManifest: %v", err)
	}
	inst := m.Artifacts[0]
	for _, want := range []struct {
		url, sha string
		size     int64
	}{
		{url: inst.URL, sha: inst.SHA256, size: inst.Size},
		{url: inst.PortableURL, sha: inst.PortableSHA256, size: inst.PortableSize},
	} {
		u, err := url.Parse(want.url)
		if err != nil {
			t.Fatalf("parse %q: %v", want.url, err)
		}
		resp, err := testHTTPClient().Get(ts.URL + u.Path)
		if err != nil {
			t.Fatalf("GET %s: %v", u.Path, err)
		}
		body := mustReadAll(t, resp)
		closeBody(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s status = %d", u.Path, resp.StatusCode)
		}
		if size, sum := fileDigest(t, body); size != want.size || sum != want.sha {
			t.Fatalf("GET %s = %d bytes %s, want %d bytes %s", u.Path, size, sum, want.size, want.sha)
		}
	}
}

var psLine = regexp.MustCompile(`^\$env:(TYPHON_QA_[A-Z_]+) = "([^"$` + "`" + `]*)"$`)

func parsePowerShellEnv(t *testing.T, data []byte) map[string]string {
	t.Helper()
	env := map[string]string{}
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		m := psLine.FindStringSubmatch(strings.TrimSuffix(line, "\r"))
		if m == nil {
			t.Fatalf("update.ps1 line %q is not a plain $env: assignment", line)
		}
		env[m[1]] = m[2]
	}
	return env
}

// In the Windows mode the state folder carries the qa variables for both
// shells, and they point at the running source with the key it signs with.
func TestRunWindowsWritesUpdatePS1ForTheLiveSource(t *testing.T) {
	state := t.TempDir()
	exe := fakeExes(t, []string{"1.4.2"})[0]
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var stdout bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, []string{"-windows", "-version", "1.4.2", "-artifact", exe, "-port", "0", "-state", state}, &stdout, &bytes.Buffer{})
	}()

	ps := parsePowerShellEnv(t, waitForEnvFile(t, filepath.Join(state, psFileName), done))
	sh := map[string]string{}
	shData, err := os.ReadFile(filepath.Join(state, envFileName))
	if err != nil {
		t.Fatalf("read update.env: %v", err)
	}
	for line := range strings.SplitSeq(strings.TrimSpace(string(shData)), "\n") {
		k, v, ok := strings.Cut(strings.TrimPrefix(line, "export "), "=")
		if !ok {
			t.Fatalf("update.env line %q", line)
		}
		sh[k] = v
	}
	base, pub := ps["TYPHON_QA_MANIFEST_URL"], ps["TYPHON_QA_RELEASE_PUBKEY"]
	if len(ps) != 2 || base == "" || pub == "" {
		t.Fatalf("update.ps1 = %q, want the two qa variables", ps)
	}
	if sh["TYPHON_QA_MANIFEST_URL"] != base || sh["TYPHON_QA_RELEASE_PUBKEY"] != pub || len(sh) != 2 {
		t.Fatalf("update.env = %q, want the same two variables as update.ps1 %q", sh, ps)
	}
	key, err := base64.StdEncoding.DecodeString(pub)
	if err != nil {
		t.Fatalf("decode key: %v", err)
	}

	m := fetchOffer(t, base, ed25519.PublicKey(key), "1.4.1")
	if _, err := m.PortableArtifactFor("windows", "amd64"); err != nil {
		t.Fatalf("served manifest has no portable zip: %v", err)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout.String(), "only the portable zip is served") {
		t.Fatalf("output does not say that only the portable path is served:\n%s", stdout.String())
	}
}

func TestMacOSModeWritesNoPowerShellFile(t *testing.T) {
	state := t.TempDir()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, []string{"-version", "1.2.3", "-artifact", writeFakeArtifact(t), "-port", "0", "-state", state}, &bytes.Buffer{}, &bytes.Buffer{})
	}()
	waitForEnvFile(t, filepath.Join(state, envFileName), done)
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(state, psFileName)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("stat update.ps1 = %v, want none in the macOS mode", err)
	}
}

func repoLikeDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module typhon\n"), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	t.Chdir(dir)
	return dir
}

func TestWindowsBuildStopsWithoutABuiltFrontend(t *testing.T) {
	dir := repoLikeDir(t)
	for _, prepare := range []func() error{
		func() error { return nil },
		func() error { return os.MkdirAll(filepath.Join(dir, "frontend", "dist"), 0o755) },
		func() error { return os.WriteFile(filepath.Join(dir, "frontend", "dist", ".gitkeep"), nil, 0o600) },
	} {
		if err := prepare(); err != nil {
			t.Fatalf("prepare: %v", err)
		}
		err := goBuildWindows(&bytes.Buffer{}, &bytes.Buffer{})(t.Context(), "1.4.2", filepath.Join(t.TempDir(), "typhon.exe"))
		if !errors.Is(err, errNoFrontend) || !strings.Contains(err.Error(), "wails3 task common:build:frontend") {
			t.Fatalf("goBuildWindows() error = %v, want errNoFrontend naming the build command", err)
		}
	}
}

func TestWindowsResourceWithoutWails3BuildsWithoutAndSaysSo(t *testing.T) {
	dir := repoLikeDir(t)
	t.Setenv("PATH", t.TempDir())
	var out bytes.Buffer
	cleanup, err := windowsResource(t.Context(), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("windowsResource: %v", err)
	}
	if err := cleanup(); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if !strings.Contains(out.String(), "without the Windows resource") {
		t.Fatalf("output = %q, want it to say the builds carry no resource", out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, sysoName)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("stat %s = %v, want none", sysoName, err)
	}
}

// fakeWails3 puts a wails3 on PATH that writes the -out file it is given,
// the way `wails3 generate syso` does.
func fakeWails3(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake wails3 is a shell script")
	}
	bin := t.TempDir()
	script := "#!/bin/sh\nwhile [ $# -gt 0 ]; do if [ \"$1\" = -out ]; then printf syso > \"$2\"; fi; shift; done\n"
	if err := os.WriteFile(filepath.Join(bin, "wails3"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake wails3: %v", err)
	}
	t.Setenv("PATH", bin)
}

func TestWindowsResourceGeneratedForTheBuildsAndRemovedAfter(t *testing.T) {
	dir := repoLikeDir(t)
	if err := os.Mkdir(filepath.Join(dir, "build"), 0o755); err != nil {
		t.Fatalf("create build dir: %v", err)
	}
	fakeWails3(t)
	cleanup, err := windowsResource(t.Context(), &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("windowsResource: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, sysoName)); err != nil || string(got) != "syso" {
		t.Fatalf("%s during the builds = %q, %v; want the generated file", sysoName, got, err)
	}
	if err := cleanup(); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, sysoName)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("stat %s after the builds = %v, want it removed", sysoName, err)
	}
}

func TestWindowsResourceLeavesAnExistingFileAlone(t *testing.T) {
	dir := repoLikeDir(t)
	fakeWails3(t)
	if err := os.WriteFile(filepath.Join(dir, sysoName), []byte("mine"), 0o600); err != nil {
		t.Fatalf("write existing syso: %v", err)
	}
	cleanup, err := windowsResource(t.Context(), &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("windowsResource: %v", err)
	}
	if err := cleanup(); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, sysoName)); err != nil || string(got) != "mine" {
		t.Fatalf("existing %s = %q, %v; want it kept as it was", sysoName, got, err)
	}
}
