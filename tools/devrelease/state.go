package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"typhon/internal/storage"
)

const (
	keyFileName  = "release-key"
	envFileName  = "update.env"
	psFileName   = "update.ps1"
	stateDirMode = 0o700
)

// stateKeyPair keeps the signing seed in the state dir, so the public key a
// wrapper script sourced from update.env stays valid across restarts of the
// source. A key file that exists but does not decode is an error, never a
// reason to mint a new key over it.
func stateKeyPair(dir string) (ed25519.PrivateKey, ed25519.PublicKey, error) {
	path := filepath.Join(dir, keyFileName)
	//nolint:gosec // G304: the path is the developer's own -state flag to this local dev tool, not external input
	raw, err := os.ReadFile(path)
	switch {
	case err == nil:
		priv, pub, kerr := resolveKeyPair(strings.TrimSpace(string(raw)))
		if kerr != nil {
			return nil, nil, fmt.Errorf("%s: %w", path, kerr)
		}
		return priv, pub, nil
	case !errors.Is(err, fs.ErrNotExist):
		return nil, nil, fmt.Errorf("read signing key: %w", err)
	}

	priv, pub, err := resolveKeyPair("")
	if err != nil {
		return nil, nil, err
	}
	if err := os.MkdirAll(dir, stateDirMode); err != nil {
		return nil, nil, fmt.Errorf("create state dir: %w", err)
	}
	seed := base64.StdEncoding.EncodeToString(priv.Seed()) + "\n"
	if err := storage.WriteAtomic(path, []byte(seed)); err != nil {
		return nil, nil, fmt.Errorf("save signing key: %w", err)
	}
	return priv, pub, nil
}

// envLines names the source for the build that reads it: the devmock
// launcher, or the qa Windows launcher (internal/selfupdate/qa_source.go).
func envLines(windows bool, baseURL string, pub ed25519.PublicKey) []string {
	urlVar, keyVar := "TYPHON_DEVMOCK_MANIFEST_URL", "TYPHON_DEVMOCK_RELEASE_PUBKEY"
	if windows {
		urlVar, keyVar = "TYPHON_QA_MANIFEST_URL", "TYPHON_QA_RELEASE_PUBKEY"
	}
	return []string{
		urlVar + "=" + baseURL,
		keyVar + "=" + base64.StdEncoding.EncodeToString(pub),
	}
}

// writePowerShellEnv is update.env for a Windows machine, where there is no
// shell to source an export from. The values are a loopback url and base64,
// so double quotes hold them without escaping.
func writePowerShellEnv(dir string, lines []string) error {
	var b strings.Builder
	for _, line := range lines {
		name, value, _ := strings.Cut(line, "=")
		b.WriteString("$env:" + name + " = \"" + value + "\"\r\n")
	}
	if err := storage.WriteAtomic(filepath.Join(dir, psFileName), []byte(b.String())); err != nil {
		return fmt.Errorf("write PowerShell env file: %w", err)
	}
	return nil
}

func writeEnvFile(dir string, lines []string) error {
	var b strings.Builder
	for _, line := range lines {
		b.WriteString("export " + line + "\n")
	}
	if err := storage.WriteAtomic(filepath.Join(dir, envFileName), []byte(b.String())); err != nil {
		return fmt.Errorf("write env file: %w", err)
	}
	return nil
}
