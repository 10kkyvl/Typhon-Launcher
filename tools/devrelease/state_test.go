package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"typhon/internal/selfupdate"
)

func TestParseFlagsAcceptsAStateDir(t *testing.T) {
	dir := t.TempDir()
	cfg, err := parseFlags([]string{"-version", "1.2.3", "-state", dir}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if cfg.state != dir {
		t.Fatalf("cfg.state = %q, want %q", cfg.state, dir)
	}
}

func TestStateKeyIsKeptBetweenRuns(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	_, first, err := stateKeyPair(dir)
	if err != nil {
		t.Fatalf("first stateKeyPair: %v", err)
	}
	_, second, err := stateKeyPair(dir)
	if err != nil {
		t.Fatalf("second stateKeyPair: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("the signing key changed between two runs on the same state dir")
	}
	info, err := os.Stat(filepath.Join(dir, keyFileName))
	if err != nil {
		t.Fatalf("stat key file: %v", err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Fatalf("key file mode = %v, want it readable by the owner only", perm)
	}
}

// A key file that cannot be read is not a missing one: replacing it would
// silently invalidate the env file every wrapper already sourced.
func TestStateKeyRefusesAnUnreadableKeyAndKeepsIt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, keyFileName)
	if err := os.WriteFile(path, []byte("not a seed"), 0o600); err != nil {
		t.Fatalf("write key file: %v", err)
	}
	if _, _, err := stateKeyPair(dir); !errors.Is(err, errBadSeed) {
		t.Fatalf("stateKeyPair error = %v, want errBadSeed", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read key file: %v", err)
	}
	if string(got) != "not a seed" {
		t.Fatalf("key file = %q, want it left as it was", got)
	}
}

func TestRunWritesAnEnvFileThatPointsAtTheLiveSource(t *testing.T) {
	state := t.TempDir()
	artifact := writeFakeArtifact(t)

	var pubs []string
	for range 2 {
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() {
			done <- run(ctx, []string{"-version", "1.2.3", "-artifact", artifact, "-port", "0", "-state", state}, &bytes.Buffer{}, &bytes.Buffer{})
		}()

		env := parseEnvFile(t, waitForEnvFile(t, filepath.Join(state, envFileName), done))
		base, pub := env["TYPHON_DEVMOCK_MANIFEST_URL"], env["TYPHON_DEVMOCK_RELEASE_PUBKEY"]
		key, err := base64.StdEncoding.DecodeString(pub)
		if err != nil || len(key) != ed25519.PublicKeySize {
			t.Fatalf("TYPHON_DEVMOCK_RELEASE_PUBKEY = %q: %v", pub, err)
		}

		resp, err := testHTTPClient().Get(base + selfupdate.ManifestPath)
		if err != nil {
			t.Fatalf("GET manifest from the env file url: %v", err)
		}
		body := mustReadAll(t, resp)
		closeBody(t, resp)
		if _, err := selfupdate.VerifyManifest(body, ed25519.PublicKey(key)); err != nil {
			t.Fatalf("manifest does not verify with the env file key: %v", err)
		}
		pubs = append(pubs, pub)

		cancel()
		if err := <-done; err != nil {
			t.Fatalf("run: %v", err)
		}
		if err := os.Remove(filepath.Join(state, envFileName)); err != nil {
			t.Fatalf("remove env file between runs: %v", err)
		}
	}
	if pubs[0] != pubs[1] {
		t.Fatalf("public key changed between runs: %q then %q", pubs[0], pubs[1])
	}
}

func waitForEnvFile(t *testing.T, path string, done <-chan error) []byte {
	t.Helper()
	deadline := time.After(10 * time.Second)
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		data, err := os.ReadFile(path)
		if err == nil {
			return data
		}
		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("read %s: %v", path, err)
		}
		select {
		case err := <-done:
			t.Fatalf("run returned before writing %s: %v", path, err)
		case <-deadline:
			t.Fatalf("timed out waiting for %s", path)
		case <-ticker.C:
		}
	}
}

func parseEnvFile(t *testing.T, data []byte) map[string]string {
	t.Helper()
	env := map[string]string{}
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		rest, ok := strings.CutPrefix(line, "export ")
		if !ok {
			t.Fatalf("env file line %q is not an export", line)
		}
		k, v, ok := strings.Cut(rest, "=")
		if !ok {
			t.Fatalf("env file line %q has no value", line)
		}
		env[k] = strings.Trim(v, "'")
	}
	for _, k := range []string{"TYPHON_DEVMOCK_MANIFEST_URL", "TYPHON_DEVMOCK_RELEASE_PUBKEY"} {
		if env[k] == "" {
			t.Fatalf("env file has no %s:\n%s", k, data)
		}
	}
	return env
}
