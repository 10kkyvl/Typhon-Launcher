package selfupdate

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func signManifest(t *testing.T, priv ed25519.PrivateKey, m Manifest) []byte {
	t.Helper()
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	sig := ed25519.Sign(priv, raw)
	sm := SignedManifest{
		KeyID:     KeyID,
		Signature: base64.StdEncoding.EncodeToString(sig),
		Manifest:  raw,
	}
	out, err := json.Marshal(sm)
	if err != nil {
		t.Fatalf("marshal signed manifest: %v", err)
	}
	return out
}

func validManifest() Manifest {
	return Manifest{
		Version:     "1.2.3",
		PublishedAt: time.Now(),
		Artifacts: []Artifact{
			{
				OS:     "windows",
				Arch:   "amd64",
				Kind:   KindInstaller,
				Name:   "typhon-setup.exe",
				URL:    "https://cdn.example.com/typhon-setup.exe",
				Size:   1024,
				SHA256: strings.Repeat("0123456789abcdef", 4),
			},
		},
	}
}

func TestVerifyManifest(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	otherPub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate other key: %v", err)
	}

	t.Run("valid", func(t *testing.T) {
		signed := signManifest(t, priv, validManifest())
		m, err := VerifyManifest(signed, pub)
		if err != nil {
			t.Fatalf("VerifyManifest() error = %v, want nil", err)
		}
		if m.Version != "1.2.3" {
			t.Fatalf("Version = %q, want 1.2.3", m.Version)
		}
	})

	t.Run("unknown key id", func(t *testing.T) {
		raw, err := json.Marshal(validManifest())
		if err != nil {
			t.Fatalf("marshal manifest: %v", err)
		}
		sig := ed25519.Sign(priv, raw)
		sm := SignedManifest{KeyID: "some-other-key", Signature: base64.StdEncoding.EncodeToString(sig), Manifest: raw}
		signed, err := json.Marshal(sm)
		if err != nil {
			t.Fatalf("marshal signed: %v", err)
		}
		_, err = VerifyManifest(signed, pub)
		if !errors.Is(err, ErrUnknownKey) {
			t.Fatalf("VerifyManifest() error = %v, want ErrUnknownKey", err)
		}
	})

	t.Run("signature verified with wrong public key", func(t *testing.T) {
		signed := signManifest(t, priv, validManifest())
		_, err := VerifyManifest(signed, otherPub)
		if !errors.Is(err, ErrBadSignature) {
			t.Fatalf("VerifyManifest() error = %v, want ErrBadSignature", err)
		}
	})

	t.Run("corrupted signature", func(t *testing.T) {
		raw, err := json.Marshal(validManifest())
		if err != nil {
			t.Fatalf("marshal manifest: %v", err)
		}
		sm := SignedManifest{KeyID: KeyID, Signature: "not-base64!!", Manifest: raw}
		signed, err := json.Marshal(sm)
		if err != nil {
			t.Fatalf("marshal signed: %v", err)
		}
		_, err = VerifyManifest(signed, pub)
		if !errors.Is(err, ErrBadSignature) {
			t.Fatalf("VerifyManifest() error = %v, want ErrBadSignature", err)
		}
	})

	t.Run("manifest bytes tampered after signing", func(t *testing.T) {
		signed := signManifest(t, priv, validManifest())
		var sm SignedManifest
		if err := json.Unmarshal(signed, &sm); err != nil {
			t.Fatalf("unmarshal signed: %v", err)
		}
		tampered := validManifest()
		tampered.Version = "9.9.9"
		rawTampered, err := json.Marshal(tampered)
		if err != nil {
			t.Fatalf("marshal tampered: %v", err)
		}
		sm.Manifest = rawTampered
		out, err := json.Marshal(sm)
		if err != nil {
			t.Fatalf("marshal signed: %v", err)
		}
		_, err = VerifyManifest(out, pub)
		if !errors.Is(err, ErrBadSignature) {
			t.Fatalf("VerifyManifest() error = %v, want ErrBadSignature", err)
		}
	})

	t.Run("malformed json", func(t *testing.T) {
		_, err := VerifyManifest([]byte("{not json"), pub)
		if !errors.Is(err, ErrInvalidManifest) {
			t.Fatalf("VerifyManifest() error = %v, want ErrInvalidManifest", err)
		}
	})

	t.Run("empty manifest field", func(t *testing.T) {
		sig := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, nil))
		out := []byte(`{"keyId":"` + KeyID + `","signature":"` + sig + `"}`)
		_, err := VerifyManifest(out, pub)
		if !errors.Is(err, ErrInvalidManifest) {
			t.Fatalf("VerifyManifest() error = %v, want ErrInvalidManifest", err)
		}
	})

	t.Run("incomparable version rejected after signature verifies", func(t *testing.T) {
		m := validManifest()
		m.Version = "latest-build"
		signed := signManifest(t, priv, m)
		_, err := VerifyManifest(signed, pub)
		if !errors.Is(err, ErrInvalidVersion) {
			t.Fatalf("VerifyManifest() error = %v, want ErrInvalidVersion", err)
		}
	})

	t.Run("artifact with path traversal name rejected", func(t *testing.T) {
		m := validManifest()
		m.Artifacts[0].Name = `..\evil.exe`
		signed := signManifest(t, priv, m)
		_, err := VerifyManifest(signed, pub)
		if !errors.Is(err, ErrInvalidArtifactName) {
			t.Fatalf("VerifyManifest() error = %v, want ErrInvalidArtifactName", err)
		}
	})

	t.Run("artifact with plain http url rejected", func(t *testing.T) {
		m := validManifest()
		m.Artifacts[0].URL = "http://cdn.example.com/typhon-setup.exe"
		signed := signManifest(t, priv, m)
		_, err := VerifyManifest(signed, pub)
		if !errors.Is(err, ErrInvalidArtifactURL) {
			t.Fatalf("VerifyManifest() error = %v, want ErrInvalidArtifactURL", err)
		}
	})
}

func TestManifestValidate(t *testing.T) {
	t.Run("no artifacts", func(t *testing.T) {
		m := validManifest()
		m.Artifacts = nil
		if err := m.Validate(); !errors.Is(err, ErrInvalidManifest) {
			t.Fatalf("Validate() error = %v, want ErrInvalidManifest", err)
		}
	})

	t.Run("duplicate os arch kind", func(t *testing.T) {
		m := validManifest()
		dup := m.Artifacts[0]
		dup.Name = "typhon-setup-2.exe"
		m.Artifacts = append(m.Artifacts, dup)
		if err := m.Validate(); !errors.Is(err, ErrInvalidManifest) {
			t.Fatalf("Validate() error = %v, want ErrInvalidManifest", err)
		}
	})

	t.Run("valid", func(t *testing.T) {
		m := validManifest()
		if err := m.Validate(); err != nil {
			t.Fatalf("Validate() error = %v, want nil", err)
		}
	})
}

func TestManifestArtifactFor(t *testing.T) {
	m := validManifest()

	a, err := m.ArtifactFor("windows", "amd64")
	if err != nil {
		t.Fatalf("ArtifactFor() error = %v, want nil", err)
	}
	if a.Name != "typhon-setup.exe" {
		t.Fatalf("ArtifactFor() name = %q, want typhon-setup.exe", a.Name)
	}

	_, err = m.ArtifactFor("linux", "arm64")
	if !errors.Is(err, ErrNoArtifact) {
		t.Fatalf("ArtifactFor() error = %v, want ErrNoArtifact", err)
	}
}

func TestIsNewer(t *testing.T) {
	cases := []struct {
		name      string
		available string
		current   string
		want      bool
		wantErr   error
	}{
		{"newer patch", "1.2.4", "1.2.3", true, nil},
		{"same version", "1.2.3", "1.2.3", false, nil},
		{"older version", "1.2.2", "1.2.3", false, nil},
		{"incomparable available", "not-a-version-!@#", "1.2.3", false, ErrInvalidVersion},
		{"incomparable current", "1.2.3", "not-a-version-!@#", false, ErrInvalidVersion},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := IsNewer(tc.available, tc.current)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("IsNewer() error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("IsNewer() unexpected error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("IsNewer() = %v, want %v", got, tc.want)
			}
		})
	}
}
