package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"typhon/internal/selfupdate"
)

func generateKey(t *testing.T) (ed25519.PrivateKey, ed25519.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return priv, pub
}

func writeArtifactFile(t *testing.T, dir, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write artifact file: %v", err)
	}
	return path
}

func envFor(env map[string]string) func(string) string {
	return func(key string) string {
		return env[key]
	}
}

func TestRun_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	content := []byte("typhon installer bytes for round trip test")
	path := writeArtifactFile(t, dir, "typhon-amd64-installer.exe", content)
	sum := sha256.Sum256(content)
	wantHash := hex.EncodeToString(sum[:])

	priv, pub := generateKey(t)
	out := filepath.Join(dir, "manifest.json")

	args := []string{
		"-version", "1.2.3",
		"-notes", "first release",
		"-base-url", "https://api.typhon-launcher.com/launcher/download",
		"-out", out,
		"-artifact", "os=windows,arch=amd64,kind=installer,path=" + path,
	}
	env := envFor(map[string]string{privateKeyEnv: base64.StdEncoding.EncodeToString(priv)})

	signed, err := run(args, &bytes.Buffer{}, env)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	onDisk, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !bytes.Equal(onDisk, signed) {
		t.Fatalf("output file does not match returned bytes")
	}

	m, err := selfupdate.VerifyManifest(signed, pub)
	if err != nil {
		t.Fatalf("VerifyManifest: %v", err)
	}
	if m.Version != "1.2.3" {
		t.Errorf("version = %q, want 1.2.3", m.Version)
	}
	if len(m.Artifacts) != 1 {
		t.Fatalf("artifacts = %d, want 1", len(m.Artifacts))
	}
	a := m.Artifacts[0]
	if a.Size != int64(len(content)) {
		t.Errorf("size = %d, want %d", a.Size, len(content))
	}
	if a.SHA256 != wantHash {
		t.Errorf("sha256 = %q, want %q", a.SHA256, wantHash)
	}
	if a.URL != "https://api.typhon-launcher.com/launcher/download/1.2.3/typhon-amd64-installer.exe" {
		t.Errorf("url = %q", a.URL)
	}
}

func TestRun_TamperedManifestByteFailsVerification(t *testing.T) {
	dir := t.TempDir()
	content := []byte("payload")
	path := writeArtifactFile(t, dir, "app-installer.exe", content)
	sum := sha256.Sum256(content)
	hash := hex.EncodeToString(sum[:])
	priv, pub := generateKey(t)
	out := filepath.Join(dir, "manifest.json")

	args := []string{
		"-version", "1.0.0",
		"-base-url", "https://api.typhon-launcher.com/launcher/download",
		"-out", out,
		"-artifact", "os=windows,arch=amd64,kind=installer,path=" + path,
	}
	env := envFor(map[string]string{privateKeyEnv: base64.StdEncoding.EncodeToString(priv)})

	signed, err := run(args, &bytes.Buffer{}, env)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	// Flip one hex digit inside the artifact's sha256 value: this keeps the
	// document valid JSON, so only the signature check can catch the change.
	tampered := append([]byte(nil), signed...)
	idx := bytes.Index(tampered, []byte(hash))
	if idx < 0 {
		t.Fatalf("sha256 value not found in output")
	}
	if tampered[idx] == 'a' {
		tampered[idx] = 'b'
	} else {
		tampered[idx] = 'a'
	}

	if _, err := selfupdate.VerifyManifest(tampered, pub); !errors.Is(err, selfupdate.ErrBadSignature) {
		t.Fatalf("VerifyManifest on tampered bytes = %v, want ErrBadSignature", err)
	}
}

func TestRun_VerifyWithWrongKeyFails(t *testing.T) {
	dir := t.TempDir()
	path := writeArtifactFile(t, dir, "app-installer.exe", []byte("payload"))
	priv, _ := generateKey(t)
	_, otherPub := generateKey(t)
	out := filepath.Join(dir, "manifest.json")

	args := []string{
		"-version", "1.0.0",
		"-base-url", "https://api.typhon-launcher.com/launcher/download",
		"-out", out,
		"-artifact", "os=windows,arch=amd64,kind=installer,path=" + path,
	}
	env := envFor(map[string]string{privateKeyEnv: base64.StdEncoding.EncodeToString(priv)})

	signed, err := run(args, &bytes.Buffer{}, env)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if _, err := selfupdate.VerifyManifest(signed, otherPub); !errors.Is(err, selfupdate.ErrBadSignature) {
		t.Fatalf("VerifyManifest with wrong key = %v, want ErrBadSignature", err)
	}
}

func TestDecodePrivateKey(t *testing.T) {
	seedKey, _ := generateKey(t)
	seed := seedKey.Seed()

	cases := []struct {
		name    string
		encoded string
		wantErr error
	}{
		{"empty", "", errEmptyPrivateKey},
		{"not base64", "not-base64!!", errBadPrivateKey},
		{"wrong length", base64.StdEncoding.EncodeToString([]byte("too short")), errBadPrivateKey},
		{"valid seed", base64.StdEncoding.EncodeToString(seed), nil},
		{"valid full key", base64.StdEncoding.EncodeToString(seedKey), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key, err := decodePrivateKey(tc.encoded)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(key) != ed25519.PrivateKeySize {
				t.Fatalf("key size = %d, want %d", len(key), ed25519.PrivateKeySize)
			}
		})
	}
}

func TestRun_EmptyPrivateKeyEnv(t *testing.T) {
	dir := t.TempDir()
	path := writeArtifactFile(t, dir, "app-installer.exe", []byte("payload"))
	out := filepath.Join(dir, "manifest.json")

	args := []string{
		"-version", "1.0.0",
		"-base-url", "https://api.typhon-launcher.com/launcher/download",
		"-out", out,
		"-artifact", "os=windows,arch=amd64,kind=installer,path=" + path,
	}
	env := envFor(nil)

	if _, err := run(args, &bytes.Buffer{}, env); !errors.Is(err, errEmptyPrivateKey) {
		t.Fatalf("run() err = %v, want errEmptyPrivateKey", err)
	}
	if _, statErr := os.Stat(out); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("output file was written despite the failure: %v", statErr)
	}
}

func TestBuildArtifact_HashAndSizeFromFile(t *testing.T) {
	dir := t.TempDir()
	content := bytes.Repeat([]byte{0x42}, 4096+17)
	path := writeArtifactFile(t, dir, "typhon-arm64-installer.exe", content)
	sum := sha256.Sum256(content)

	spec := artifactSpec{os: "windows", arch: "arm64", kind: "installer", path: path, name: "typhon-arm64-installer.exe"}
	a, err := buildArtifact(spec, "https://api.typhon-launcher.com/launcher/download", "2.0.0")
	if err != nil {
		t.Fatalf("buildArtifact: %v", err)
	}
	if a.Size != int64(len(content)) {
		t.Errorf("size = %d, want %d", a.Size, len(content))
	}
	if a.SHA256 != hex.EncodeToString(sum[:]) {
		t.Errorf("sha256 = %q, want %q", a.SHA256, hex.EncodeToString(sum[:]))
	}
}

func TestBuildArtifact_MissingFile(t *testing.T) {
	spec := artifactSpec{os: "windows", arch: "amd64", kind: "installer", path: filepath.Join(t.TempDir(), "missing.exe"), name: "missing.exe"}
	_, err := buildArtifact(spec, "https://api.typhon-launcher.com/launcher/download", "1.0.0")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("buildArtifact err = %v, want wrapped fs.ErrNotExist", err)
	}
}

func TestBuildArtifact_RejectedByValidate(t *testing.T) {
	dir := t.TempDir()
	path := writeArtifactFile(t, dir, "installer.exe", []byte("payload"))

	cases := []struct {
		name    string
		spec    artifactSpec
		baseURL string
		wantErr error
	}{
		{
			name:    "unsafe artifact name",
			spec:    artifactSpec{os: "windows", arch: "amd64", kind: "installer", path: path, name: "../evil.exe"},
			baseURL: "https://api.typhon-launcher.com/launcher/download",
			wantErr: selfupdate.ErrInvalidArtifactName,
		},
		{
			name:    "unsupported kind",
			spec:    artifactSpec{os: "windows", arch: "amd64", kind: "zip", path: path, name: "installer.exe"},
			baseURL: "https://api.typhon-launcher.com/launcher/download",
			wantErr: selfupdate.ErrUnsupportedKind,
		},
		{
			name:    "plain http base url",
			spec:    artifactSpec{os: "windows", arch: "amd64", kind: "installer", path: path, name: "installer.exe"},
			baseURL: "http://api.typhon-launcher.com/launcher/download",
			wantErr: selfupdate.ErrInvalidArtifactURL,
		},
		{
			name:    "missing os",
			spec:    artifactSpec{os: "", arch: "amd64", kind: "installer", path: path, name: "installer.exe"},
			baseURL: "https://api.typhon-launcher.com/launcher/download",
			wantErr: selfupdate.ErrInvalidArtifact,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := buildArtifact(tc.spec, tc.baseURL, "1.0.0")
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want wrapped %v", err, tc.wantErr)
			}
		})
	}
}

func TestParseArtifactSpec(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    artifactSpec
		wantErr bool
	}{
		{
			name:  "full spec",
			input: "os=windows,arch=amd64,kind=installer,path=bin/typhon-amd64-installer.exe,name=custom.exe",
			want:  artifactSpec{os: "windows", arch: "amd64", kind: "installer", path: "bin/typhon-amd64-installer.exe", name: "custom.exe"},
		},
		{
			name:  "name defaults to file base name",
			input: "os=windows,arch=arm64,kind=installer,path=bin/typhon-arm64-installer.exe",
			want:  artifactSpec{os: "windows", arch: "arm64", kind: "installer", path: "bin/typhon-arm64-installer.exe", name: "typhon-arm64-installer.exe"},
		},
		{name: "missing field", input: "os=windows,arch=amd64,kind=installer", wantErr: true},
		{name: "unknown key", input: "os=windows,arch=amd64,kind=installer,path=x,bogus=1", wantErr: true},
		{name: "malformed pair", input: "os=windows,arch", wantErr: true},
		{name: "empty string", input: "", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseArtifactSpec(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got spec %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestSignManifest_TooLargeIsRejected(t *testing.T) {
	priv, _ := generateKey(t)
	m := selfupdate.Manifest{
		Version:     "1.0.0",
		PublishedAt: time.Now().UTC(),
		Notes:       strings.Repeat("x", selfupdate.MaxManifestSize),
		Artifacts: []selfupdate.Artifact{{
			OS:     "windows",
			Arch:   "amd64",
			Kind:   selfupdate.KindInstaller,
			Name:   "installer.exe",
			URL:    "https://api.typhon-launcher.com/launcher/download/1.0.0/installer.exe",
			Size:   1,
			SHA256: strings.Repeat("a", 64),
		}},
	}
	if _, err := signManifest(m, priv); !errors.Is(err, errManifestTooBig) {
		t.Fatalf("signManifest err = %v, want errManifestTooBig", err)
	}
}

func TestResolvePublishedAt(t *testing.T) {
	if _, err := resolvePublishedAt(""); err != nil {
		t.Fatalf("empty input should default to now: %v", err)
	}
	got, err := resolvePublishedAt("2026-01-02T03:04:05Z")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Equal(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)) {
		t.Fatalf("got %v", got)
	}
	if _, err := resolvePublishedAt("not-a-time"); err == nil {
		t.Fatalf("expected error for malformed timestamp")
	}
}

func TestRun_MissingRequiredFlags(t *testing.T) {
	dir := t.TempDir()
	path := writeArtifactFile(t, dir, "installer.exe", []byte("payload"))
	base := []string{
		"-version", "1.0.0",
		"-base-url", "https://api.typhon-launcher.com/launcher/download",
		"-out", filepath.Join(dir, "manifest.json"),
		"-artifact", "os=windows,arch=amd64,kind=installer,path=" + path,
	}
	env := envFor(map[string]string{privateKeyEnv: "irrelevant"})

	cases := []struct {
		name    string
		drop    string
		wantErr error
	}{
		{"missing version", "-version", errMissingVersion},
		{"missing base-url", "-base-url", errMissingBaseURL},
		{"missing out", "-out", errMissingOut},
		{"missing artifact", "-artifact", errNoArtifacts},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := dropFlag(base, tc.drop)
			if _, err := run(args, &bytes.Buffer{}, env); !errors.Is(err, tc.wantErr) {
				t.Fatalf("run() err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func dropFlag(args []string, flagName string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == flagName {
			i++
			continue
		}
		out = append(out, args[i])
	}
	return out
}
