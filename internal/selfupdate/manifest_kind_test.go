//go:build !devmock

package selfupdate

import (
	"context"
	"crypto/ed25519"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"

	"typhon/internal/uierr"
)

func kindArtifact(goos, arch string, kind Kind, name string) Artifact {
	return Artifact{
		OS: goos, Arch: arch, Kind: kind, Name: name,
		URL: "https://cdn.example.com/" + name, Size: 1024, SHA256: strings.Repeat("0123456789abcdef", 4),
	}
}

// A manifest may carry several kinds for one os/arch. The client has to take
// the one this platform applies, not whichever the publisher listed first: a
// bundle .zip handed to the Windows installer path is run as an executable.
func TestArtifactForTakesTheKindThePlatformApplies(t *testing.T) {
	tests := []struct {
		name      string
		goos      string
		artifacts []Artifact
		want      string
		wantErr   error
	}{
		{
			name: "windows picks the installer listed after a bundle",
			goos: "windows",
			artifacts: []Artifact{
				kindArtifact("windows", "amd64", KindBundle, "typhon.zip"),
				kindArtifact("windows", "amd64", KindInstaller, "typhon-setup.exe"),
			},
			want: "typhon-setup.exe",
		},
		{
			name: "windows picks the installer listed first",
			goos: "windows",
			artifacts: []Artifact{
				kindArtifact("windows", "amd64", KindInstaller, "typhon-setup.exe"),
				kindArtifact("windows", "amd64", KindBundle, "typhon.zip"),
			},
			want: "typhon-setup.exe",
		},
		{
			name: "darwin picks the bundle listed after an installer",
			goos: "darwin",
			artifacts: []Artifact{
				kindArtifact("darwin", "arm64", KindInstaller, "typhon.pkg"),
				kindArtifact("darwin", "arm64", KindBundle, "typhon-darwin-arm64.zip"),
			},
			want: "typhon-darwin-arm64.zip",
		},
		{
			name:      "windows with only a bundle has nothing to install",
			goos:      "windows",
			artifacts: []Artifact{kindArtifact("windows", "amd64", KindBundle, "typhon.zip")},
			wantErr:   ErrNoArtifact,
		},
		{
			name:      "darwin with only an installer has nothing to apply",
			goos:      "darwin",
			artifacts: []Artifact{kindArtifact("darwin", "arm64", KindInstaller, "typhon.pkg")},
			wantErr:   ErrNoArtifact,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			arch := tt.artifacts[0].Arch
			m := Manifest{Version: "1.2.3", PublishedAt: time.Now(), Artifacts: tt.artifacts}
			if err := m.Validate(); err != nil {
				t.Fatalf("Validate: %v", err)
			}

			got, err := m.ArtifactFor(tt.goos, arch)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ArtifactFor() = %+v, %v; want %v", got, err, tt.wantErr)
				}
				if code := uierr.Code(err); code != "selfupdate.no_artifact" {
					t.Fatalf("error code = %q, want selfupdate.no_artifact so the UI can explain it", code)
				}
				return
			}
			if err != nil {
				t.Fatalf("ArtifactFor() error = %v", err)
			}
			if got.Name != tt.want {
				t.Fatalf("ArtifactFor() = %q (%s), want %q", got.Name, got.Kind, tt.want)
			}
		})
	}
}

// A release that ships only a kind this platform cannot apply must stop at the
// check: offering it would download a file the apply step then refuses or,
// worse, runs as something it is not.
func TestCheckRefusesAManifestWithOnlyTheWrongKind(t *testing.T) {
	priv, pub := testKeyPair(t)
	m := releaseManifest("1.2.3")
	m.Artifacts[0].Kind = KindBundle
	if runtime.GOOS == "darwin" {
		m.Artifacts[0].Kind = KindInstaller
	}
	ps := newPollServer(t)
	ps.serve(signManifest(t, priv, m))
	client, err := newClientWithKey(ps.srv.URL, pub)
	if err != nil {
		t.Fatalf("newClientWithKey: %v", err)
	}
	dir := t.TempDir()
	s := &Service{dir: dir, notes: mustNotesStore(t, dir), store: mustStore(t, dir), client: client, currentVersion: "1.0.0"}

	status, err := s.CheckForUpdate(context.Background())
	if !errors.Is(err, ErrNoArtifact) {
		t.Fatalf("CheckForUpdate() error = %v, want ErrNoArtifact", err)
	}
	if status.State == StateAvailable || status.ErrorCode != "artifact" {
		t.Fatalf("status = %+v, want no update offered and the artifact error reported", status)
	}
	if v, err := mustStore(t, dir).Load(); err != nil || v.AvailableVersion != "" {
		t.Fatalf("saved state = %+v, %v; a release this platform cannot apply must not be recorded", v, err)
	}
}

// The same decision, reached the way the launcher reaches it: a signed
// manifest off the wire, through verification, to the artifact it downloads.
func TestSignedManifestWithBothKindsResolvesToTheInstallerOnWindows(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	m := Manifest{
		Version:     "1.2.3",
		PublishedAt: time.Now(),
		Artifacts: []Artifact{
			kindArtifact("windows", "amd64", KindBundle, "typhon.zip"),
			kindArtifact("windows", "amd64", KindInstaller, "typhon-setup.exe"),
		},
	}

	verified, err := VerifyManifest(signManifest(t, priv, m), pub)
	if err != nil {
		t.Fatalf("VerifyManifest: %v", err)
	}
	got, err := verified.ArtifactFor("windows", "amd64")
	if err != nil {
		t.Fatalf("ArtifactFor: %v", err)
	}
	if got.Kind != KindInstaller || got.Name != "typhon-setup.exe" {
		t.Fatalf("ArtifactFor() = %q (%s), want the installer", got.Name, got.Kind)
	}
}
