//go:build windows

package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateInstallerPathNotAbsolute(t *testing.T) {
	dir := t.TempDir()
	cacheDir, err := CacheDir(dir)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}
	if err := validateInstallerPath(dir, filepath.Join(cacheDir, "setup.exe")[2:]); !errors.Is(err, errInstallerPathNotAbsolute) {
		t.Fatalf("validateInstallerPath() error = %v, want errInstallerPathNotAbsolute", err)
	}
}

func TestValidateInstallerPathNotClean(t *testing.T) {
	dir := t.TempDir()
	cacheDir, err := CacheDir(dir)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}
	dirty := filepath.Join(cacheDir, "1.2.3") + string(filepath.Separator) + ".." + string(filepath.Separator) + "1.2.3" + string(filepath.Separator) + "setup.exe"
	if err := validateInstallerPath(dir, dirty); !errors.Is(err, errInstallerPathNotClean) {
		t.Fatalf("validateInstallerPath() error = %v, want errInstallerPathNotClean", err)
	}
}

func TestValidateInstallerPathOutsideCache(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(dir, "setup.exe")
	writeTestFile(t, outside, []byte("x"))
	if err := validateInstallerPath(dir, outside); !errors.Is(err, errInstallerOutsideCache) {
		t.Fatalf("validateInstallerPath() error = %v, want errInstallerOutsideCache", err)
	}
}

func TestValidateInstallerPathCacheRootItselfRejected(t *testing.T) {
	dir := t.TempDir()
	cacheDir, err := CacheDir(dir)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := validateInstallerPath(dir, cacheDir); !errors.Is(err, errInstallerOutsideCache) {
		t.Fatalf("validateInstallerPath() error = %v, want errInstallerOutsideCache for the cache root itself", err)
	}
}

func TestValidateInstallerPathMissingFile(t *testing.T) {
	dir := t.TempDir()
	cacheDir, err := CacheDir(dir)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}
	missing := filepath.Join(cacheDir, "1.2.3", "setup.exe")
	if err := validateInstallerPath(dir, missing); err == nil {
		t.Fatal("validateInstallerPath() error = nil, want an error for a missing file")
	}
}

func TestValidateInstallerPathDirectoryRejected(t *testing.T) {
	dir := t.TempDir()
	cacheDir, err := CacheDir(dir)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}
	target := filepath.Join(cacheDir, "1.2.3")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := validateInstallerPath(dir, target); !errors.Is(err, errInstallerNotRegularFile) {
		t.Fatalf("validateInstallerPath() error = %v, want errInstallerNotRegularFile for a directory", err)
	}
}

// TestValidateInstallerPathSymlinkRejected closes invariant 11/32: a symlink
// inside the cache dir must not be accepted as the installer to run, even if
// its target is a legitimate regular file.
func TestValidateInstallerPathSymlinkRejected(t *testing.T) {
	dir := t.TempDir()
	cacheDir, err := CacheDir(dir)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}
	target := filepath.Join(dir, "real-setup.exe")
	writeTestFile(t, target, []byte("real installer"))
	link := filepath.Join(cacheDir, "1.2.3", "setup.exe")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create symlink on this machine (needs developer mode or admin): %v", err)
	}
	if err := validateInstallerPath(dir, link); !errors.Is(err, errInstallerNotRegularFile) {
		t.Fatalf("validateInstallerPath() error = %v, want errInstallerNotRegularFile for a symlink", err)
	}
}

func TestValidateInstallerPathValid(t *testing.T) {
	dir := t.TempDir()
	cacheDir, err := CacheDir(dir)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}
	valid := filepath.Join(cacheDir, "1.2.3", "setup.exe")
	writeTestFile(t, valid, []byte("installer"))
	if err := validateInstallerPath(dir, valid); err != nil {
		t.Fatalf("validateInstallerPath() error = %v, want nil", err)
	}
}

func TestApplyRejectsPathOutsideCache(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)

	outside := filepath.Join(dir, "not-cached-setup.exe")
	writeTestFile(t, outside, []byte("x"))

	if err := Apply(context.Background(), outside, dir, filepath.Join(dir, "typhon.exe")); err == nil {
		t.Fatal("Apply() error = nil, want an error for a path outside the selfupdate cache")
	}
}

func TestApplyNotReadyWhenNothingStored(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)

	configDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir: %v", err)
	}
	cacheDir, err := CacheDir(filepath.Join(configDir, "Typhon"))
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}
	installerPath := filepath.Join(cacheDir, "1.2.3", "setup.exe")
	writeTestFile(t, installerPath, []byte("installer"))

	if err := Apply(context.Background(), installerPath, dir, filepath.Join(dir, "typhon.exe")); !errors.Is(err, ErrNotReady) {
		t.Fatalf("Apply() error = %v, want ErrNotReady when nothing was ever downloaded", err)
	}
}

func TestApplyHashMismatchAfterTampering(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)

	configDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir: %v", err)
	}
	launcherDir := filepath.Join(configDir, "Typhon")

	content := []byte("original installer bytes")
	installerPath, err := ArtifactPath(launcherDir, "1.2.3", "setup.exe")
	if err != nil {
		t.Fatalf("ArtifactPath: %v", err)
	}
	writeTestFile(t, installerPath, content)

	art := Artifact{
		OS: "windows", Arch: "amd64", Kind: KindInstaller, Name: "setup.exe",
		URL: "https://example.com/setup.exe", Size: int64(len(content)), SHA256: sha256Hex(t, content),
	}
	store, err := NewStore(launcherDir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.Save(stored{AvailableVersion: "1.2.3", Artifact: &art, ReadyPath: installerPath}); err != nil {
		t.Fatalf("seed store: %v", err)
	}

	// Tamper with the file after it was recorded as verified, keeping the same
	// length so this exercises the hash check rather than the size check:
	// Apply must re-verify against the recorded hash, not trust the path alone.
	tampered := make([]byte, len(content))
	copy(tampered, content)
	tampered[0] ^= 0xff
	writeTestFile(t, installerPath, tampered)

	if err := Apply(context.Background(), installerPath, dir, filepath.Join(dir, "typhon.exe")); !errors.Is(err, ErrHashMismatch) {
		t.Fatalf("Apply() error = %v, want ErrHashMismatch", err)
	}
}

func TestApplyNotReadyWhenReadyPathDiffers(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)

	configDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir: %v", err)
	}
	launcherDir := filepath.Join(configDir, "Typhon")

	content := []byte("installer bytes")
	recordedPath, err := ArtifactPath(launcherDir, "1.2.3", "setup.exe")
	if err != nil {
		t.Fatalf("ArtifactPath: %v", err)
	}
	writeTestFile(t, recordedPath, content)

	otherPath, err := ArtifactPath(launcherDir, "1.2.4", "setup.exe")
	if err != nil {
		t.Fatalf("ArtifactPath: %v", err)
	}
	writeTestFile(t, otherPath, content)

	art := Artifact{
		OS: "windows", Arch: "amd64", Kind: KindInstaller, Name: "setup.exe",
		URL: "https://example.com/setup.exe", Size: int64(len(content)), SHA256: sha256Hex(t, content),
	}
	store, err := NewStore(launcherDir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.Save(stored{AvailableVersion: "1.2.3", Artifact: &art, ReadyPath: recordedPath}); err != nil {
		t.Fatalf("seed store: %v", err)
	}

	if err := Apply(context.Background(), otherPath, dir, filepath.Join(dir, "typhon.exe")); !errors.Is(err, ErrNotReady) {
		t.Fatalf("Apply() error = %v, want ErrNotReady when the path does not match the recorded ReadyPath", err)
	}
}

func TestValidateInstallDir(t *testing.T) {
	existing := t.TempDir()
	regular := filepath.Join(existing, "typhon.exe")
	writeTestFile(t, regular, []byte("x"))

	tests := []struct {
		name string
		dir  string
		want error
	}{
		{name: "empty", dir: "", want: errInstallDirEmpty},
		{name: "relative", dir: `Programs\Typhon`, want: errInstallDirNotAbsolute},
		{name: "trailing separator", dir: existing + string(filepath.Separator), want: errInstallDirNotClean},
		{name: "dot dot", dir: filepath.Join(existing, "sub", "..") + string(filepath.Separator) + ".", want: errInstallDirNotClean},
		{name: "quote", dir: `C:\Programs\Ty"phon`, want: errInstallDirUnsafe},
		{name: "control char", dir: "C:" + string(filepath.Separator) + "Typ" + string(rune(1)) + "hon", want: errInstallDirUnsafe},
		{name: "missing", dir: filepath.Join(existing, "gone"), want: os.ErrNotExist},
		{name: "regular file", dir: regular, want: errInstallDirNotDir},
		{name: "valid", dir: existing, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateInstallDir(tt.dir)
			if tt.want == nil {
				if err != nil {
					t.Fatalf("validateInstallDir(%q) = %v, want nil", tt.dir, err)
				}
				return
			}
			if !errors.Is(err, tt.want) {
				t.Fatalf("validateInstallDir(%q) = %v, want %v", tt.dir, err, tt.want)
			}
		})
	}
}

func TestInstallerCmdLine(t *testing.T) {
	installDir := filepath.Join(t.TempDir(), "Program Files", "Typhon")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	installer := filepath.Join(t.TempDir(), "typhon-amd64-installer.exe")

	got, err := installerCmdLine(installer, installDir)
	if err != nil {
		t.Fatalf("installerCmdLine: %v", err)
	}
	want := `"` + installer + `" /S /D=` + installDir
	if got != want {
		t.Fatalf("installerCmdLine() = %q, want %q", got, want)
	}
	if !strings.HasSuffix(got, " /D="+installDir) {
		t.Fatalf("installerCmdLine() = %q, /D= must come last and stay unquoted", got)
	}
}

func TestInstallerCmdLineRejectsBadInput(t *testing.T) {
	installDir := t.TempDir()
	installer := filepath.Join(t.TempDir(), "setup.exe")

	if _, err := installerCmdLine(installer, filepath.Join(installDir, "gone")); err == nil {
		t.Fatal("installerCmdLine() error = nil, want an error for a missing install dir")
	}
	if _, err := installerCmdLine(installer+"\"", installDir); !errors.Is(err, errInstallerPathUnsafe) {
		t.Fatalf("installerCmdLine() error = %v, want errInstallerPathUnsafe", err)
	}
}
