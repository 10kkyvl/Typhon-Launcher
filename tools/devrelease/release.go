package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"runtime"
	"time"

	"typhon/internal/selfupdate"
)

func hashFile(path string) (size int64, sum string, err error) {
	//nolint:gosec // G703: path is either our own build output in a temp dir or the developer's own -artifact flag to this local dev tool, not external input
	f, err := os.Open(path)
	if err != nil {
		return 0, "", fmt.Errorf("open artifact %q: %w", path, err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close artifact %q: %w", path, cerr)
		}
	}()

	h := sha256.New()
	size, err = io.Copy(h, f)
	if err != nil {
		return 0, "", fmt.Errorf("hash artifact %q: %w", path, err)
	}
	return size, hex.EncodeToString(h.Sum(nil)), nil
}

func artifactName() string {
	return fmt.Sprintf("typhon-%s-%s", runtime.GOOS, runtime.GOARCH)
}

func buildSignedManifest(version string, artifactPath string, port int, priv ed25519.PrivateKey) ([]byte, selfupdate.Artifact, error) {
	size, sum, err := hashFile(artifactPath)
	if err != nil {
		return nil, selfupdate.Artifact{}, err
	}
	name := artifactName()
	art := selfupdate.Artifact{
		OS:     runtime.GOOS,
		Arch:   runtime.GOARCH,
		Kind:   selfupdate.KindInstaller,
		Name:   name,
		URL:    fmt.Sprintf("http://127.0.0.1:%d/launcher/download/%s/%s", port, version, name),
		Size:   size,
		SHA256: sum,
	}
	if err := art.Validate(); err != nil {
		return nil, selfupdate.Artifact{}, fmt.Errorf("build artifact: %w", err)
	}
	m := selfupdate.Manifest{
		Version:     version,
		PublishedAt: time.Now().UTC(),
		Notes:       "Local devrelease build.",
		Artifacts:   []selfupdate.Artifact{art},
	}
	signed, err := selfupdate.SignManifest(m, priv)
	if err != nil {
		return nil, selfupdate.Artifact{}, fmt.Errorf("sign manifest: %w", err)
	}
	return signed, art, nil
}
