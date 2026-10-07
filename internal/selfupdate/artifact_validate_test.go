package selfupdate

import (
	"errors"
	"strings"
	"testing"
)

func TestArtifactValidateBoundaries(t *testing.T) {
	valid := func() Artifact {
		return Artifact{
			OS: "windows", Arch: "amd64", Kind: KindInstaller, Name: "typhon-setup.exe",
			URL: "https://cdn.example.com/typhon-setup.exe", Size: 1024, SHA256: strings.Repeat("ab", 32),
		}
	}
	tests := []struct {
		name   string
		mutate func(a *Artifact)
		want   error
	}{
		{"baseline", func(*Artifact) {}, nil},
		{"bundle kind", func(a *Artifact) { a.Kind = KindBundle }, nil},
		{"smallest size", func(a *Artifact) { a.Size = 1 }, nil},
		{"largest size", func(a *Artifact) { a.Size = MaxArtifactSize }, nil},
		{"size one past the largest", func(a *Artifact) { a.Size = MaxArtifactSize + 1 }, ErrInvalidArtifactSize},
		{"zero size", func(a *Artifact) { a.Size = 0 }, ErrInvalidArtifactSize},
		{"negative size", func(a *Artifact) { a.Size = -1 }, ErrInvalidArtifactSize},
		{"longest name", func(a *Artifact) { a.Name = strings.Repeat("a", 128) }, nil},
		{"name one past the longest", func(a *Artifact) { a.Name = strings.Repeat("a", 129) }, ErrInvalidArtifactName},
		{"dotted name", func(a *Artifact) { a.Name = "typhon-0.4.2-amd64.setup.exe" }, nil},
		{"hidden name", func(a *Artifact) { a.Name = ".hidden" }, nil},
		{"loopback ipv4 over http", func(a *Artifact) { a.URL = "http://127.0.0.1:8080/setup.exe" }, nil},
		{"loopback ipv6 over http", func(a *Artifact) { a.URL = "http://[::1]:8080/setup.exe" }, nil},
		{"localhost over http", func(a *Artifact) { a.URL = "http://localhost/setup.exe" }, nil},
		{"loopback lookalike host", func(a *Artifact) { a.URL = "http://127.0.0.1.evil.example.com/setup.exe" }, ErrInvalidArtifactURL},
		{"localhost lookalike host", func(a *Artifact) { a.URL = "http://localhost.evil.example.com/setup.exe" }, ErrInvalidArtifactURL},
		{"loopback in the userinfo", func(a *Artifact) { a.URL = "http://localhost@evil.example.com/setup.exe" }, ErrInvalidArtifactURL},
		{"private address over http", func(a *Artifact) { a.URL = "http://192.168.1.10/setup.exe" }, ErrInvalidArtifactURL},
		{"ftp", func(a *Artifact) { a.URL = "ftp://cdn.example.com/setup.exe" }, ErrInvalidArtifactURL},
		{"scheme-less", func(a *Artifact) { a.URL = "cdn.example.com/setup.exe" }, ErrInvalidArtifactURL},
		{"protocol relative", func(a *Artifact) { a.URL = "//cdn.example.com/setup.exe" }, ErrInvalidArtifactURL},
		{"javascript", func(a *Artifact) { a.URL = "javascript:alert(1)" }, ErrInvalidArtifactURL},
		{"control character in the url", func(a *Artifact) { a.URL = "https://cdn.example.com/set\x7fup.exe" }, ErrInvalidArtifactURL},
		{"hash with a non hex digit", func(a *Artifact) { a.SHA256 = strings.Repeat("ab", 31) + "zz" }, ErrInvalidHash},
		{"hash one digit too long", func(a *Artifact) { a.SHA256 = strings.Repeat("ab", 32) + "a" }, ErrInvalidHash},
		{"hash with surrounding space", func(a *Artifact) { a.SHA256 = " " + strings.Repeat("ab", 31) + "a" }, ErrInvalidHash},
		{"no kind", func(a *Artifact) { a.Kind = "" }, ErrUnsupportedKind},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := valid()
			tt.mutate(&a)
			err := a.Validate()
			if tt.want == nil {
				if err != nil {
					t.Fatalf("Validate() error = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tt.want) {
				t.Fatalf("Validate() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestArtifactNamesAndVersionDirsCannotEscapeTheCache(t *testing.T) {
	unsafe := []string{
		"", ".", "..", "a/b", `a\b`, `..\evil.exe`, "../evil.exe", "C:evil.exe", "setup.exe:stream",
		"a*b", "a?b", `a"b`, "a<b", "a>b", "a|b", "a\x00b", "a\x1fb", "a\x7fb", "a\nb",
	}
	for _, name := range unsafe {
		t.Run("name "+name, func(t *testing.T) {
			if err := validateArtifactName(name); !errors.Is(err, ErrInvalidArtifactName) {
				t.Fatalf("validateArtifactName(%q) = %v, want ErrInvalidArtifactName", name, err)
			}
			if _, err := ArtifactPath(t.TempDir(), "1.2.3", name); !errors.Is(err, ErrInvalidArtifactName) {
				t.Fatalf("ArtifactPath(%q) = %v, want ErrInvalidArtifactName", name, err)
			}
		})
	}
	for _, version := range []string{"", ".", "..", "1/2", `1\2`, "1.2.3:evil", "1*", "1?", `1"`, "1<", "1>", "1|", "1\x00", "1\x7f", "1\n"} {
		t.Run("version "+version, func(t *testing.T) {
			if _, err := VersionDir(t.TempDir(), version); !errors.Is(err, ErrInvalidVersionPath) {
				t.Fatalf("VersionDir(%q) = %v, want ErrInvalidVersionPath", version, err)
			}
		})
	}
	for _, version := range []string{"1.2.3", "0.10.0", "1.2.3-rc1"} {
		if _, err := VersionDir(t.TempDir(), version); err != nil {
			t.Fatalf("VersionDir(%q) error = %v", version, err)
		}
	}
}
