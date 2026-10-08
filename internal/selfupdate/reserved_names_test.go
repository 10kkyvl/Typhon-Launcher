package selfupdate

import (
	"errors"
	"testing"
)

// The cache is a Windows directory: CreateFile on a reserved device name opens
// the device, not a file, and a trailing dot or space is silently stripped, so
// two names the manifest keeps apart land on one file. Every host rejects them
// because the same signed manifest is served to all of them.
var windowsUnsafeNames = []string{
	"NUL", "nul", "Nul", "CON", "PRN", "AUX",
	"con.exe", "CON.EXE", "nul.txt", "aux.tar.gz", "prn.zip",
	"COM0", "COM1", "com5.exe", "COM9.zip", "LPT0", "LPT1", "lpt9.txt",
	"COM¹", "COM².exe", "LPT³",
	"CONIN$", "CONOUT$", "conout$.txt",
	"nul .txt", "con  .exe", "NUL ",
	"setup.exe.", "setup.exe ", "typhon.", "typhon ", "1.2.3.",
}

var windowsSafeNames = []string{
	"typhon-setup.exe", "setup.exe", ".hidden", "console.exe", "com10.exe", "COM.exe",
	"lpt.zip", "nullable.exe", "auxiliary.exe", "prn1.exe", "xcon.exe", "typhon-0.4.2-amd64.setup.exe",
	"1.2.3", "1.2.3-rc1", "0.10.0",
}

func TestArtifactNamesRejectWindowsUnsafeNames(t *testing.T) {
	for _, name := range windowsUnsafeNames {
		t.Run("name "+name, func(t *testing.T) {
			if err := validateArtifactName(name); !errors.Is(err, ErrInvalidArtifactName) {
				t.Fatalf("validateArtifactName(%q) = %v, want ErrInvalidArtifactName", name, err)
			}
			if _, err := ArtifactPath(t.TempDir(), "1.2.3", name); !errors.Is(err, ErrInvalidArtifactName) {
				t.Fatalf("ArtifactPath(%q) = %v, want ErrInvalidArtifactName", name, err)
			}
			art := artifactNamed(name)
			if err := art.Validate(); !errors.Is(err, ErrInvalidArtifactName) {
				t.Fatalf("Artifact{Name: %q}.Validate() = %v, want ErrInvalidArtifactName", name, err)
			}
		})
	}
	for _, name := range windowsSafeNames {
		t.Run("safe "+name, func(t *testing.T) {
			if err := validateArtifactName(name); err != nil {
				t.Fatalf("validateArtifactName(%q) = %v, want nil", name, err)
			}
		})
	}
}

func TestVersionDirsRejectWindowsUnsafeNames(t *testing.T) {
	for _, version := range windowsUnsafeNames {
		t.Run("version "+version, func(t *testing.T) {
			if _, err := VersionDir(t.TempDir(), version); !errors.Is(err, ErrInvalidVersionPath) {
				t.Fatalf("VersionDir(%q) = %v, want ErrInvalidVersionPath", version, err)
			}
		})
	}
	for _, version := range []string{"1.2.3", "0.10.0", "1.2.3-rc1", ".hidden", "console"} {
		t.Run("safe "+version, func(t *testing.T) {
			if _, err := VersionDir(t.TempDir(), version); err != nil {
				t.Fatalf("VersionDir(%q) = %v, want nil", version, err)
			}
		})
	}
}

func artifactNamed(name string) Artifact {
	a := testArtifact([]byte("x"), name)
	a.URL = "https://cdn.example.com/setup.exe"
	return a
}
