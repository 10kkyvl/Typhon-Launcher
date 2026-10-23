package selfupdate

import (
	"fmt"
	"strings"

	"typhon/internal/version"
)

const deltaSuffix = ".delta"

// ExeSHA256, ExeSize and Deltas are optional, but never half a set: a patch
// without the hash of what it must produce could not be checked, and the
// signed manifest is the only place that hash can come from.
func (a Artifact) validateExeFields() error {
	if a.ExeSHA256 == "" && a.ExeSize == 0 && len(a.Deltas) == 0 {
		return nil
	}
	if a.Kind != KindInstaller && a.Kind != KindPortable {
		return fmt.Errorf("%w: only an installer carries a patched executable", ErrInvalidArtifact)
	}
	if a.OS != "windows" {
		return fmt.Errorf("%w: patches exist for windows only, not %s", ErrInvalidArtifact, a.OS)
	}
	if a.ExeSize <= 0 || a.ExeSize > MaxArtifactSize {
		return ErrInvalidArtifactSize
	}
	if err := validateHash(a.ExeSHA256); err != nil {
		return err
	}
	if len(a.Deltas) > MaxDeltas {
		return fmt.Errorf("%w: %d patches", ErrInvalidArtifact, len(a.Deltas))
	}
	names := map[string]bool{a.Name: true, a.PortableName: true, portableExeName: true}
	for i, d := range a.Deltas {
		if err := d.Validate(); err != nil {
			return fmt.Errorf("patch from %q: %w", d.From, err)
		}
		if names[d.Name] {
			return fmt.Errorf("%w: file name %q is used twice", ErrInvalidArtifact, d.Name)
		}
		names[d.Name] = true
		for _, earlier := range a.Deltas[:i] {
			if sameVersion(earlier.From, d.From) {
				return fmt.Errorf("%w: two patches from %q", ErrInvalidArtifact, d.From)
			}
		}
	}
	return nil
}

func (d Delta) Validate() error {
	if !version.Parse(d.From).Comparable {
		return fmt.Errorf("%w: version %q", ErrInvalidArtifact, d.From)
	}
	if err := validateHash(d.FromSHA256); err != nil {
		return err
	}
	if err := validateArtifactName(d.Name); err != nil {
		return err
	}
	if !strings.HasSuffix(d.Name, deltaSuffix) {
		return ErrInvalidArtifactName
	}
	if err := validateArtifactURL(d.URL); err != nil {
		return err
	}
	if d.Size <= 0 || d.Size > MaxArtifactSize {
		return ErrInvalidArtifactSize
	}
	return validateHash(d.SHA256)
}

// asArtifact lets the patch travel through the downloader like any other file.
func (d Delta) asArtifact(os, arch string) Artifact {
	return Artifact{OS: os, Arch: arch, Kind: KindDelta, Name: d.Name, URL: d.URL, Size: d.Size, SHA256: d.SHA256}
}

// validateDeltaVersions holds each patch to start from a release older than
// the one the manifest announces.
func (m Manifest) validateDeltaVersions(a Artifact) error {
	for _, d := range a.Deltas {
		newer, err := IsNewer(m.Version, d.From)
		if err != nil {
			return fmt.Errorf("%w: patch from %q: %w", ErrInvalidArtifact, d.From, err)
		}
		if !newer {
			return fmt.Errorf("%w: patch from %q does not lead to %q", ErrInvalidArtifact, d.From, m.Version)
		}
	}
	return nil
}
