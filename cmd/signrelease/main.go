// Command signrelease builds and signs the launcher's self-update manifest
// from a set of built release artifacts. It is run by CI right before those
// artifacts are uploaded, never interactively with a key on the flag line.
package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"typhon/internal/selfupdate"
	"typhon/internal/storage"
)

const privateKeyEnv = "TYPHON_RELEASE_PRIVATE_KEY"

var (
	errNoArtifacts     = errors.New("signrelease: at least one -artifact is required")
	errMissingVersion  = errors.New("signrelease: -version is required")
	errMissingBaseURL  = errors.New("signrelease: -base-url is required")
	errMissingOut      = errors.New("signrelease: -out is required")
	errEmptyPrivateKey = fmt.Errorf("signrelease: %s is not set", privateKeyEnv)
	errBadPrivateKey   = errors.New("signrelease: private key must be a base64-encoded ed25519 seed (32 bytes) or private key (64 bytes)")
	errManifestTooBig  = errors.New("signrelease: signed manifest exceeds the size limit")
	errBadPublicKey    = errors.New("signrelease: private key did not produce an ed25519 public key")
)

type artifactSpec struct {
	os   string
	arch string
	kind string
	path string
	name string
}

func parseArtifactSpec(s string) (artifactSpec, error) {
	var spec artifactSpec
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			return artifactSpec{}, fmt.Errorf("signrelease: invalid artifact field %q, want key=value", part)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "os":
			spec.os = value
		case "arch":
			spec.arch = value
		case "kind":
			spec.kind = value
		case "path":
			spec.path = value
		case "name":
			spec.name = value
		default:
			return artifactSpec{}, fmt.Errorf("signrelease: unknown artifact field %q", key)
		}
	}
	if spec.os == "" || spec.arch == "" || spec.kind == "" || spec.path == "" {
		return artifactSpec{}, fmt.Errorf("signrelease: artifact spec %q missing one of os/arch/kind/path", s)
	}
	if spec.name == "" {
		spec.name = filepath.Base(spec.path)
	}
	return spec, nil
}

type artifactFlags []artifactSpec

func (a *artifactFlags) String() string {
	if a == nil {
		return ""
	}
	parts := make([]string, len(*a))
	for i, s := range *a {
		parts[i] = s.path
	}
	return strings.Join(parts, ",")
}

func (a *artifactFlags) Set(s string) error {
	spec, err := parseArtifactSpec(s)
	if err != nil {
		return err
	}
	*a = append(*a, spec)
	return nil
}

type config struct {
	version     string
	publishedAt string
	notes       string
	baseURL     string
	out         string
	artifacts   artifactFlags
}

func parseFlags(args []string, stderr io.Writer) (config, error) {
	fs := flag.NewFlagSet("signrelease", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var cfg config
	fs.StringVar(&cfg.version, "version", "", "release version, must match the VERSION file / git tag")
	fs.StringVar(&cfg.publishedAt, "published-at", "", "RFC3339 publish timestamp (default: now, UTC)")
	fs.StringVar(&cfg.notes, "notes", "", "release notes")
	fs.StringVar(&cfg.baseURL, "base-url", "", "base URL artifacts are served from, e.g. https://api.typhon-launcher.com/launcher/download")
	fs.StringVar(&cfg.out, "out", "", "path to write the signed manifest to")
	fs.Var(&cfg.artifacts, "artifact", "artifact spec os=...,arch=...,kind=...,path=...[,name=...] (repeatable)")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if cfg.version == "" {
		return config{}, errMissingVersion
	}
	if cfg.baseURL == "" {
		return config{}, errMissingBaseURL
	}
	if cfg.out == "" {
		return config{}, errMissingOut
	}
	if len(cfg.artifacts) == 0 {
		return config{}, errNoArtifacts
	}
	return cfg, nil
}

func resolvePublishedAt(s string) (time.Time, error) {
	if s == "" {
		return time.Now().UTC(), nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("signrelease: -published-at: %w", err)
	}
	return t.UTC(), nil
}

func hashFile(path string) (size int64, sum string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, "", fmt.Errorf("signrelease: open artifact %q: %w", path, err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("signrelease: close artifact %q: %w", path, cerr)
		}
	}()

	h := sha256.New()
	size, err = io.Copy(h, f)
	if err != nil {
		return 0, "", fmt.Errorf("signrelease: hash artifact %q: %w", path, err)
	}
	return size, hex.EncodeToString(h.Sum(nil)), nil
}

func buildArtifact(spec artifactSpec, baseURL, version string) (selfupdate.Artifact, error) {
	size, sum, err := hashFile(spec.path)
	if err != nil {
		return selfupdate.Artifact{}, err
	}
	url := strings.TrimRight(baseURL, "/") + "/" + version + "/" + spec.name
	a := selfupdate.Artifact{
		OS:     spec.os,
		Arch:   spec.arch,
		Kind:   selfupdate.Kind(spec.kind),
		Name:   spec.name,
		URL:    url,
		Size:   size,
		SHA256: sum,
	}
	if err := a.Validate(); err != nil {
		return selfupdate.Artifact{}, fmt.Errorf("signrelease: artifact %q: %w", spec.path, err)
	}
	return a, nil
}

func buildManifest(version, notes string, publishedAt time.Time, artifacts []selfupdate.Artifact) (selfupdate.Manifest, error) {
	m := selfupdate.Manifest{
		Version:     version,
		PublishedAt: publishedAt,
		Notes:       notes,
		Artifacts:   artifacts,
	}
	if err := m.Validate(); err != nil {
		return selfupdate.Manifest{}, fmt.Errorf("signrelease: manifest: %w", err)
	}
	return m, nil
}

func decodePrivateKey(encoded string) (ed25519.PrivateKey, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil, errEmptyPrivateKey
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errBadPrivateKey, err)
	}
	switch len(raw) {
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(raw), nil
	case ed25519.PrivateKeySize:
		return ed25519.PrivateKey(raw), nil
	default:
		return nil, fmt.Errorf("%w: got %d bytes", errBadPrivateKey, len(raw))
	}
}

// signManifest marshals m exactly once, signs those raw bytes, and embeds
// them verbatim in the SignedManifest — re-encoding the manifest after
// signing would change its bytes and invalidate the signature.
func signManifest(m selfupdate.Manifest, priv ed25519.PrivateKey) ([]byte, error) {
	raw, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("signrelease: marshal manifest: %w", err)
	}
	sig := ed25519.Sign(priv, raw)
	sm := selfupdate.SignedManifest{
		KeyID:     selfupdate.KeyID,
		Signature: base64.StdEncoding.EncodeToString(sig),
		Manifest:  json.RawMessage(raw),
	}
	out, err := json.Marshal(sm)
	if err != nil {
		return nil, fmt.Errorf("signrelease: marshal signed manifest: %w", err)
	}
	if len(out) > selfupdate.MaxManifestSize {
		return nil, fmt.Errorf("%w: %d > %d bytes", errManifestTooBig, len(out), selfupdate.MaxManifestSize)
	}

	pub, ok := priv.Public().(ed25519.PublicKey)
	if !ok {
		return nil, errBadPublicKey
	}
	if _, err := selfupdate.VerifyManifest(out, pub); err != nil {
		return nil, fmt.Errorf("signrelease: self-verification failed: %w", err)
	}
	return out, nil
}

func run(args []string, stderr io.Writer, getenv func(string) string) ([]byte, error) {
	cfg, err := parseFlags(args, stderr)
	if err != nil {
		return nil, err
	}

	publishedAt, err := resolvePublishedAt(cfg.publishedAt)
	if err != nil {
		return nil, err
	}

	artifacts := make([]selfupdate.Artifact, 0, len(cfg.artifacts))
	for _, spec := range cfg.artifacts {
		a, err := buildArtifact(spec, cfg.baseURL, cfg.version)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, a)
	}

	manifest, err := buildManifest(cfg.version, cfg.notes, publishedAt, artifacts)
	if err != nil {
		return nil, err
	}

	priv, err := decodePrivateKey(getenv(privateKeyEnv))
	if err != nil {
		return nil, err
	}

	signed, err := signManifest(manifest, priv)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(cfg.out), 0o755); err != nil {
		return nil, fmt.Errorf("signrelease: create output directory for %q: %w", cfg.out, err)
	}
	if err := storage.WriteAtomic(cfg.out, signed); err != nil {
		return nil, fmt.Errorf("signrelease: write %q: %w", cfg.out, err)
	}
	return signed, nil
}

func main() {
	if _, err := run(os.Args[1:], os.Stderr, os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1) //nolint:forbidigo // forbidigo's own rule is "exit only from main"; this is func main of a standalone cmd, the same place root main.go is exempted for
	}
}
