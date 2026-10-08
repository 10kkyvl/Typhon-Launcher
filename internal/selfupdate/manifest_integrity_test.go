package selfupdate

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func envelopeJSON(t *testing.T, priv ed25519.PrivateKey, payload []byte) []byte {
	t.Helper()
	out, err := json.Marshal(struct {
		KeyID     string          `json:"keyId"`
		Signature string          `json:"signature"`
		Manifest  json.RawMessage `json:"manifest"`
	}{
		KeyID:     KeyID,
		Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(priv, payload)),
		Manifest:  payload,
	})
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	return out
}

func TestVerifyManifestRejectsAnyEditAfterSigning(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	signed := signManifest(t, priv, validManifest())
	if _, err := VerifyManifest(signed, pub); err != nil {
		t.Fatalf("untouched manifest must verify: %v", err)
	}

	tests := []struct {
		name     string
		old, new string
	}{
		{"artifact hash", "0123456789abcdef", "0123456789abcdee"},
		{"artifact size", `"size":1024`, `"size":1025`},
		{"artifact host", "cdn.example.com", "evil.example.com"},
		{"artifact name", `"name":"typhon-setup.exe"`, `"name":"typhon-setup.exf"`},
		{"artifact kind", `"kind":"installer"`, `"kind":"bundle"`},
		{"version", `"version":"1.2.3"`, `"version":"1.2.4"`},
		{"whitespace inside the signed bytes", `{"version"`, `{ "version"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			edited := strings.Replace(string(signed), tt.old, tt.new, 1)
			if edited == string(signed) {
				t.Fatalf("%q not found in the signed document, the row tests nothing", tt.old)
			}
			if _, err := VerifyManifest([]byte(edited), pub); !errors.Is(err, ErrBadSignature) {
				t.Fatalf("VerifyManifest() error = %v, want ErrBadSignature", err)
			}
		})
	}
}

func TestVerifyManifestEnvelopeShapes(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	good := signManifest(t, priv, validManifest())
	var env SignedManifest
	if err := json.Unmarshal(good, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	withSignature := func(sig string) []byte {
		out, err := json.Marshal(SignedManifest{KeyID: KeyID, Signature: sig, Manifest: env.Manifest})
		if err != nil {
			t.Fatalf("marshal envelope: %v", err)
		}
		return out
	}
	realSig, err := base64.StdEncoding.DecodeString(env.Signature)
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}

	tests := []struct {
		name string
		in   []byte
		want error
	}{
		{"empty body", nil, ErrInvalidManifest},
		{"truncated envelope", good[:len(good)/2], ErrInvalidManifest},
		{"json array", []byte(`[]`), ErrInvalidManifest},
		{"trailing garbage after the envelope", append(append([]byte(nil), good...), []byte("garbage")...), ErrInvalidManifest},
		{"no key id", []byte(`{}`), ErrUnknownKey},
		{"json null", []byte(`null`), ErrUnknownKey},
		{"missing manifest", []byte(`{"keyId":"` + KeyID + `"}`), ErrInvalidManifest},
		{"null manifest without signature", []byte(`{"keyId":"` + KeyID + `","manifest":null}`), ErrBadSignature},
		{"empty signature", withSignature(""), ErrBadSignature},
		{"signature not base64", withSignature("###"), ErrBadSignature},
		{"signature without padding", withSignature(strings.TrimRight(base64.StdEncoding.EncodeToString(realSig), "=")), ErrBadSignature},
		{"signature one byte short", withSignature(base64.StdEncoding.EncodeToString(realSig[:len(realSig)-1])), ErrBadSignature},
		{"signature one byte long", withSignature(base64.StdEncoding.EncodeToString(append(append([]byte(nil), realSig...), 0))), ErrBadSignature},
		{"all zero signature", withSignature(base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))), ErrBadSignature},
		{"signed payload is a string", envelopeJSON(t, priv, []byte(`"abc"`)), ErrInvalidManifest},
		{"signed payload is an array", envelopeJSON(t, priv, []byte(`[1]`)), ErrInvalidManifest},
		{"signed empty object has no version", envelopeJSON(t, priv, []byte(`{}`)), ErrInvalidVersion},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := VerifyManifest(tt.in, pub)
			if !errors.Is(err, tt.want) {
				t.Fatalf("VerifyManifest() error = %v, want %v", err, tt.want)
			}
			if m.Version != "" || len(m.Artifacts) != 0 {
				t.Fatalf("VerifyManifest() returned %+v alongside an error, a rejected manifest must come back empty", m)
			}
		})
	}
}

func TestVerifyManifestSignedButUnusableContent(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	mutate := func(f func(m *Manifest)) Manifest {
		m := validManifest()
		f(&m)
		return m
	}
	tests := []struct {
		name string
		m    Manifest
		want error
	}{
		{"no artifacts", mutate(func(m *Manifest) { m.Artifacts = nil }), ErrInvalidManifest},
		{"empty artifact list", mutate(func(m *Manifest) { m.Artifacts = []Artifact{} }), ErrInvalidManifest},
		{"empty version", mutate(func(m *Manifest) { m.Version = "" }), ErrInvalidVersion},
		{"unsupported kind", mutate(func(m *Manifest) { m.Artifacts[0].Kind = "dmg" }), ErrUnsupportedKind},
		{"missing os", mutate(func(m *Manifest) { m.Artifacts[0].OS = "" }), ErrInvalidArtifact},
		{"missing arch", mutate(func(m *Manifest) { m.Artifacts[0].Arch = "" }), ErrInvalidArtifact},
		{"zero size", mutate(func(m *Manifest) { m.Artifacts[0].Size = 0 }), ErrInvalidArtifactSize},
		{"negative size", mutate(func(m *Manifest) { m.Artifacts[0].Size = -1 }), ErrInvalidArtifactSize},
		{"size above the limit", mutate(func(m *Manifest) { m.Artifacts[0].Size = MaxArtifactSize + 1 }), ErrInvalidArtifactSize},
		{"uppercase hash", mutate(func(m *Manifest) { m.Artifacts[0].SHA256 = strings.ToUpper(m.Artifacts[0].SHA256) }), ErrInvalidHash},
		{"short hash", mutate(func(m *Manifest) { m.Artifacts[0].SHA256 = m.Artifacts[0].SHA256[:63] }), ErrInvalidHash},
		{"empty hash", mutate(func(m *Manifest) { m.Artifacts[0].SHA256 = "" }), ErrInvalidHash},
		{"url without host", mutate(func(m *Manifest) { m.Artifacts[0].URL = "https:///typhon-setup.exe" }), ErrInvalidArtifactURL},
		{"file url", mutate(func(m *Manifest) { m.Artifacts[0].URL = "file:///C:/typhon-setup.exe" }), ErrInvalidArtifactURL},
		{"empty url", mutate(func(m *Manifest) { m.Artifacts[0].URL = "" }), ErrInvalidArtifactURL},
		{"name with a stream separator", mutate(func(m *Manifest) { m.Artifacts[0].Name = "setup.exe:evil" }), ErrInvalidArtifactName},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := VerifyManifest(signManifest(t, priv, tt.m), pub); !errors.Is(err, tt.want) {
				t.Fatalf("VerifyManifest() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestVerifyManifestIgnoresFieldsAddedLater(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	payload := []byte(`{"version":"1.2.3","publishedAt":"2026-09-02T12:00:00Z","channel":"stable","artifacts":[` +
		`{"os":"windows","arch":"amd64","kind":"installer","name":"setup.exe","url":"https://cdn.example.com/setup.exe",` +
		`"size":1024,"sha256":"` + strings.Repeat("ab", 32) + `","signatureHint":"x"}]}`)
	m, err := VerifyManifest(envelopeJSON(t, priv, payload), pub)
	if err != nil {
		t.Fatalf("VerifyManifest() error = %v, an older launcher must keep reading manifests that gained fields", err)
	}
	if m.Version != "1.2.3" || len(m.Artifacts) != 1 || m.Artifacts[0].Name != "setup.exe" {
		t.Fatalf("VerifyManifest() = %+v", m)
	}
}

func TestPublicKeyIsAUsableEd25519Key(t *testing.T) {
	t.Setenv("TYPHON_DEVMOCK_RELEASE_PUBKEY", "")

	raw, err := base64.StdEncoding.DecodeString(publicKeyBase64)
	if err != nil {
		t.Fatalf("embedded release key is not base64: %v", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		t.Fatalf("embedded release key is %d bytes, want %d: no signed manifest would ever verify", len(raw), ed25519.PublicKeySize)
	}
	if bytes.Equal(raw, make([]byte, ed25519.PublicKeySize)) {
		t.Fatal("embedded release key is all zeros")
	}

	key, err := PublicKey()
	if err != nil {
		t.Fatalf("PublicKey() error = %v", err)
	}
	if !bytes.Equal(key, raw) {
		t.Fatal("PublicKey() does not return the embedded release key")
	}
}

func TestIsNewerComparesNumerically(t *testing.T) {
	tests := []struct {
		name      string
		available string
		current   string
		want      bool
	}{
		{"two digit minor beats one digit", "0.10.0", "0.9.0", true},
		{"one digit minor does not beat two digit", "0.9.0", "0.10.0", false},
		{"two digit patch beats one digit", "0.4.10", "0.4.9", true},
		{"two digit patch is not older than one digit", "0.4.9", "0.4.10", false},
		{"major rollover", "10.0.0", "9.9.9", true},
		{"major downgrade", "1.0.0", "2.0.0", false},
		{"equal is not an update", "0.4.10", "0.4.10", false},
		{"v prefix on the manifest side", "v1.2.4", "1.2.3", true},
		{"v prefix on both sides, equal", "v1.2.3", "1.2.3", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IsNewer(tt.available, tt.current)
			if err != nil {
				t.Fatalf("IsNewer(%q, %q) error = %v", tt.available, tt.current, err)
			}
			if got != tt.want {
				t.Fatalf("IsNewer(%q, %q) = %v, want %v", tt.available, tt.current, got, tt.want)
			}
		})
	}
}
