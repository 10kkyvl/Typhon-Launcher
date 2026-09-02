package selfupdate

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"strings"
	"testing"
	"time"
)

func testKeyPair(t *testing.T) (ed25519.PrivateKey, ed25519.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	return priv, pub
}

func sampleManifest(notes string) Manifest {
	return Manifest{
		Version:     "1.2.3",
		PublishedAt: time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC),
		Notes:       notes,
		Artifacts: []Artifact{{
			OS: "windows", Arch: "amd64", Kind: KindInstaller, Name: "setup.exe",
			URL: "https://api.typhon-launcher.com/launcher/download/1.2.3/setup.exe", Size: 42, SHA256: strings.Repeat("a", 64),
		}},
	}
}

func TestSignManifestRoundTrip(t *testing.T) {
	priv, pub := testKeyPair(t)
	signed, err := SignManifest(sampleManifest("hello"), priv)
	if err != nil {
		t.Fatalf("SignManifest: %v", err)
	}
	m, err := VerifyManifest(signed, pub)
	if err != nil {
		t.Fatalf("VerifyManifest: %v", err)
	}
	if m.Version != "1.2.3" || m.Notes != "hello" || len(m.Artifacts) != 1 || m.Artifacts[0].Name != "setup.exe" {
		t.Fatalf("verified manifest = %+v", m)
	}
}

func TestSignManifestWrongKeyFails(t *testing.T) {
	priv, _ := testKeyPair(t)
	_, otherPub := testKeyPair(t)
	signed, err := SignManifest(sampleManifest(""), priv)
	if err != nil {
		t.Fatalf("SignManifest: %v", err)
	}
	if _, err := VerifyManifest(signed, otherPub); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("VerifyManifest with another key = %v, want ErrBadSignature", err)
	}
}

func TestSignManifestTamperedByteFails(t *testing.T) {
	priv, pub := testKeyPair(t)
	signed, err := SignManifest(sampleManifest("hello"), priv)
	if err != nil {
		t.Fatalf("SignManifest: %v", err)
	}
	tampered := []byte(strings.Replace(string(signed), `"hello"`, `"hellp"`, 1))
	if string(tampered) == string(signed) {
		t.Fatal("tampering did not change the payload")
	}
	if _, err := VerifyManifest(tampered, pub); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("VerifyManifest on tampered bytes = %v, want ErrBadSignature", err)
	}
}

func TestSignManifestTooLarge(t *testing.T) {
	priv, _ := testKeyPair(t)
	if _, err := SignManifest(sampleManifest(strings.Repeat("x", MaxManifestSize)), priv); !errors.Is(err, ErrManifestTooLarge) {
		t.Fatalf("SignManifest = %v, want ErrManifestTooLarge", err)
	}
}

func TestSignManifestRejectsBadPrivateKey(t *testing.T) {
	if _, err := SignManifest(sampleManifest(""), ed25519.PrivateKey([]byte("short"))); !errors.Is(err, ErrBadPrivateKey) {
		t.Fatalf("SignManifest = %v, want ErrBadPrivateKey", err)
	}
	if _, err := SignManifest(sampleManifest(""), nil); !errors.Is(err, ErrBadPrivateKey) {
		t.Fatalf("SignManifest(nil key) = %v, want ErrBadPrivateKey", err)
	}
}

func TestSignManifestRejectsInvalidManifest(t *testing.T) {
	priv, _ := testKeyPair(t)
	m := sampleManifest("")
	m.Artifacts = nil
	if _, err := SignManifest(m, priv); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("SignManifest = %v, want ErrInvalidManifest", err)
	}
}
