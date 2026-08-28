package selfupdate

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"typhon/internal/version"
)

func VerifyManifest(signed []byte, key ed25519.PublicKey) (Manifest, error) {
	var sm SignedManifest
	if err := json.Unmarshal(signed, &sm); err != nil {
		return Manifest{}, fmt.Errorf("%w: %w", ErrInvalidManifest, err)
	}
	if sm.KeyID != KeyID {
		return Manifest{}, ErrUnknownKey
	}
	if len(sm.Manifest) == 0 {
		return Manifest{}, ErrInvalidManifest
	}
	sig, err := base64.StdEncoding.DecodeString(sm.Signature)
	if err != nil {
		return Manifest{}, fmt.Errorf("%w: %w", ErrBadSignature, err)
	}
	if !ed25519.Verify(key, sm.Manifest, sig) {
		return Manifest{}, ErrBadSignature
	}

	var m Manifest
	if err := json.Unmarshal(sm.Manifest, &m); err != nil {
		return Manifest{}, fmt.Errorf("%w: %w", ErrInvalidManifest, err)
	}
	if err := m.Validate(); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

func (m Manifest) Validate() error {
	v := version.Parse(m.Version)
	if !v.Comparable {
		return ErrInvalidVersion
	}
	if len(m.Artifacts) == 0 {
		return ErrInvalidManifest
	}
	if err := validateReleaseNotes(m.Releases); err != nil {
		return err
	}
	seen := make(map[string]bool, len(m.Artifacts))
	for _, a := range m.Artifacts {
		if err := a.Validate(); err != nil {
			return err
		}
		key := a.OS + "|" + a.Arch + "|" + string(a.Kind)
		if seen[key] {
			return ErrInvalidManifest
		}
		seen[key] = true
	}
	return nil
}

func (m Manifest) ArtifactFor(goos, goarch string) (Artifact, error) {
	for _, a := range m.Artifacts {
		if a.OS == goos && a.Arch == goarch {
			return a, nil
		}
	}
	return Artifact{}, ErrNoArtifact
}

func IsNewer(available, current string) (bool, error) {
	av := version.Parse(available)
	cv := version.Parse(current)
	newer, ok := version.Newer(av, cv)
	if !ok {
		return false, ErrInvalidVersion
	}
	return newer, nil
}
