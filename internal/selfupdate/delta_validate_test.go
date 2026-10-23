package selfupdate

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
)

func validDelta(from string) Delta {
	return Delta{
		From:       from,
		FromSHA256: strings.Repeat("ab", 32),
		Name:       "typhon-" + from + "-to-1.3.0.delta",
		URL:        "https://cdn.example.com/1.3.0/typhon-" + from + "-to-1.3.0.delta",
		Size:       4096,
		SHA256:     strings.Repeat("cd", 32),
	}
}

func deltaArtifact(mutate func(*Artifact)) Artifact {
	a := Artifact{
		OS: "windows", Arch: "amd64", Kind: KindInstaller, Name: "typhon-amd64-installer.exe",
		URL: "https://cdn.example.com/1.3.0/typhon-amd64-installer.exe", Size: 1 << 20, SHA256: strings.Repeat("01", 32),
		ExeSHA256: strings.Repeat("ef", 32), ExeSize: 2 << 20,
		Deltas: []Delta{validDelta("1.2.0"), validDelta("1.1.0")},
	}
	if mutate != nil {
		mutate(&a)
	}
	return a
}

func TestArtifactValidateDeltaFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Artifact)
		want   error
	}{
		{"installer with patches", nil, nil},
		{"executable fields without patches", func(a *Artifact) { a.Deltas = nil }, nil},
		{"no executable fields at all", func(a *Artifact) { a.Deltas, a.ExeSHA256, a.ExeSize = nil, "", 0 }, nil},
		{"patches without the executable size", func(a *Artifact) { a.ExeSize = 0 }, ErrInvalidArtifactSize},
		{"executable size past the limit", func(a *Artifact) { a.ExeSize = MaxArtifactSize + 1 }, ErrInvalidArtifactSize},
		{"negative executable size", func(a *Artifact) { a.ExeSize = -1 }, ErrInvalidArtifactSize},
		{"patches without the executable hash", func(a *Artifact) { a.ExeSHA256 = "" }, ErrInvalidHash},
		{"executable hash in capitals", func(a *Artifact) { a.ExeSHA256 = strings.ToUpper(a.ExeSHA256) }, ErrInvalidHash},
		{"executable hash too short", func(a *Artifact) { a.ExeSHA256 = "abcd" }, ErrInvalidHash},
		{"a bundle carries no patches", func(a *Artifact) { a.Kind = KindBundle }, ErrInvalidArtifact},
		{"patches for another platform", func(a *Artifact) { a.OS = "darwin" }, ErrInvalidArtifact},
		{"too many patches", func(a *Artifact) {
			a.Deltas = nil
			for i := 0; i <= MaxDeltas; i++ {
				d := validDelta("1.0." + strconv.Itoa(i))
				a.Deltas = append(a.Deltas, d)
			}
		}, ErrInvalidArtifact},
		{"patch from a version that is not a version", func(a *Artifact) { a.Deltas[0].From = "latest" }, ErrInvalidArtifact},
		{"patch without a start hash", func(a *Artifact) { a.Deltas[0].FromSHA256 = "" }, ErrInvalidHash},
		{"patch without a hash", func(a *Artifact) { a.Deltas[0].SHA256 = "xyz" }, ErrInvalidHash},
		{"patch name without the suffix", func(a *Artifact) { a.Deltas[0].Name = "patch.bin" }, ErrInvalidArtifactName},
		{"patch name with a path", func(a *Artifact) { a.Deltas[0].Name = "../x.delta" }, ErrInvalidArtifactName},
		{"patch name is a reserved device", func(a *Artifact) { a.Deltas[0].Name = "NUL.delta" }, ErrInvalidArtifactName},
		{"patch name repeats the installer", func(a *Artifact) { a.Name = "same.delta"; a.Deltas[0].Name = "same.delta" }, ErrInvalidArtifact},
		{"patch name repeats the portable zip", func(a *Artifact) {
			a.PortableName, a.PortableURL, a.PortableSize, a.PortableSHA256 = "z.zip", a.URL, 10, strings.Repeat("01", 32)
			a.Deltas[0].Name = "z.zip"
		}, ErrInvalidArtifactName},
		{"two patches with one name", func(a *Artifact) { a.Deltas[1].Name = a.Deltas[0].Name }, ErrInvalidArtifact},
		{"two patches from one version", func(a *Artifact) { a.Deltas[1].From = "1.2.0.0" }, ErrInvalidArtifact},
		{"patch url over plain http", func(a *Artifact) { a.Deltas[0].URL = "http://cdn.example.com/p.delta" }, ErrInvalidArtifactURL},
		{"patch url without a host", func(a *Artifact) { a.Deltas[0].URL = "https:///p.delta" }, ErrInvalidArtifactURL},
		{"empty patch", func(a *Artifact) { a.Deltas[0].Size = 0 }, ErrInvalidArtifactSize},
		{"patch past the limit", func(a *Artifact) { a.Deltas[0].Size = MaxArtifactSize + 1 }, ErrInvalidArtifactSize},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := deltaArtifact(tt.mutate).Validate()
			if tt.want == nil && err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
			if tt.want != nil && !errors.Is(err, tt.want) {
				t.Fatalf("Validate() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestManifestValidateDeltas(t *testing.T) {
	manifest := func(version string, mutate func(*Artifact)) Manifest {
		return Manifest{Version: version, PublishedAt: time.Now(), Artifacts: []Artifact{deltaArtifact(mutate)}}
	}
	tests := []struct {
		name string
		m    Manifest
		want error
	}{
		{"patches from older releases", manifest("1.3.0", nil), nil},
		{"patch from the announced version", manifest("1.2.0", nil), ErrInvalidArtifact},
		{"patch from a newer version", manifest("1.1.5", nil), ErrInvalidArtifact},
		{"a patch as a manifest artifact", manifest("1.3.0", func(a *Artifact) { a.Kind = KindDelta }), ErrUnsupportedKind},
		{"a rebuilt executable as a manifest artifact", manifest("1.3.0", func(a *Artifact) { a.Kind = KindExe }), ErrUnsupportedKind},
		{"portable as a manifest artifact", manifest("1.3.0", func(a *Artifact) { a.Kind = KindPortable }), ErrUnsupportedKind},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.m.Validate()
			if tt.want == nil && err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
			if tt.want != nil && !errors.Is(err, tt.want) {
				t.Fatalf("Validate() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestPortableArtifactKeepsThePatches(t *testing.T) {
	m := Manifest{Version: "1.3.0", PublishedAt: time.Now(), Artifacts: []Artifact{deltaArtifact(func(a *Artifact) {
		a.PortableName, a.PortableURL, a.PortableSize, a.PortableSHA256 = "typhon-windows-portable.zip", "https://cdn.example.com/1.3.0/typhon-windows-portable.zip", 500, strings.Repeat("12", 32)
	})}}
	if err := m.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	got, err := m.PortableArtifactFor("windows", "amd64")
	if err != nil {
		t.Fatalf("PortableArtifactFor() error = %v", err)
	}
	if got.Kind != KindPortable || got.ExeSHA256 != strings.Repeat("ef", 32) || got.ExeSize != 2<<20 || len(got.Deltas) != 2 {
		t.Fatalf("portable artifact = %+v, want the executable fields and both patches carried over", got)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("the derived portable artifact does not validate: %v", err)
	}
}

// Launchers up to 1.2.0 decode the manifest with the plain json decoder and
// know none of the new keys. If they ever had to refuse a document for an
// unknown key, every installed launcher would stop seeing updates.
func TestReleasedLaunchersStillReadAManifestWithPatches(t *testing.T) {
	type legacyArtifact struct {
		OS             string `json:"os"`
		Arch           string `json:"arch"`
		Kind           string `json:"kind"`
		Name           string `json:"name"`
		URL            string `json:"url"`
		Size           int64  `json:"size"`
		SHA256         string `json:"sha256"`
		PortableName   string `json:"portableName,omitempty"`
		PortableURL    string `json:"portableUrl,omitempty"`
		PortableSize   int64  `json:"portableSize,omitempty"`
		PortableSHA256 string `json:"portableSha256,omitempty"`
	}
	type legacyManifest struct {
		Version   string           `json:"version"`
		Artifacts []legacyArtifact `json:"artifacts"`
	}

	a := deltaArtifact(nil)
	raw, err := json.Marshal(Manifest{Version: "1.3.0", PublishedAt: time.Now(), Artifacts: []Artifact{a}})
	if err != nil {
		t.Fatal(err)
	}
	var old legacyManifest
	if err := json.Unmarshal(raw, &old); err != nil {
		t.Fatalf("a released launcher cannot decode the manifest: %v", err)
	}
	if old.Version != "1.3.0" || len(old.Artifacts) != 1 || old.Artifacts[0].Name != a.Name ||
		old.Artifacts[0].URL != a.URL || old.Artifacts[0].Size != a.Size || old.Artifacts[0].SHA256 != a.SHA256 {
		t.Fatalf("a released launcher reads %+v, want the installer fields unchanged", old)
	}
}

func TestManifestWithPatchesFitsTheSizeLimitReleasedLaunchersEnforce(t *testing.T) {
	priv, pub := testKeyPair(t)
	m := Manifest{Version: "1.3.0", PublishedAt: time.Now(), Artifacts: []Artifact{deltaArtifact(func(a *Artifact) {
		a.Deltas = nil
		for _, from := range []string{"1.2.0", "1.1.0", "1.0.2"} {
			a.Deltas = append(a.Deltas, validDelta(from))
		}
	})}}
	signed, err := SignManifest(m, priv)
	if err != nil {
		t.Fatalf("SignManifest() error = %v", err)
	}
	got, err := VerifyManifest(signed, pub)
	if err != nil {
		t.Fatalf("VerifyManifest() error = %v", err)
	}
	if len(got.Artifacts[0].Deltas) != 3 {
		t.Fatalf("deltas after the round trip = %d, want 3", len(got.Artifacts[0].Deltas))
	}
	if len(signed) > MaxManifestSize/4 {
		t.Fatalf("three patches alone make a %d byte manifest; the 64 KiB limit is shared with the release notes", len(signed))
	}
}
