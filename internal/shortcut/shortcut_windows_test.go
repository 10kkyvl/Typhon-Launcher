//go:build windows

package shortcut

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestCreateAndRemove(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "Test Game.lnk")
	link := Link{
		Target:      exe,
		Args:        `--launch "Test Game"`,
		WorkDir:     dir,
		Description: "Test Game shortcut",
	}

	if err := Create(path, link); err != nil {
		t.Fatalf("Create: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat created shortcut: %v", err)
	}
	if info.Size() == 0 {
		t.Fatalf("created shortcut %s is empty", path)
	}

	if err := Create(path, link); err != nil {
		t.Fatalf("second Create (overwrite) failed: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat overwritten shortcut: %v", err)
	}

	if err := Remove(path); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("shortcut still present after Remove: err=%v", err)
	}

	if err := Remove(path); err != nil {
		t.Fatalf("second Remove of already-missing file: %v", err)
	}
}

func TestCreateMissingDirectory(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}

	path := filepath.Join(t.TempDir(), "no-such-subdir", "Game.lnk")
	link := Link{Target: exe}

	if err := Create(path, link); err == nil {
		t.Fatalf("Create into missing directory: want error, got nil")
	}
}

func TestCreateEmptyTarget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Game.lnk")
	link := Link{Target: ""}

	if err := Create(path, link); err == nil {
		t.Fatalf("Create with empty Target: want error, got nil")
	}
}

func TestSupported(t *testing.T) {
	if !Supported() {
		t.Fatalf("Supported() = false on windows, want true")
	}
}

func TestDesktopDir(t *testing.T) {
	dir, err := DesktopDir()
	if err != nil {
		t.Fatalf("DesktopDir: %v", err)
	}
	if dir == "" {
		t.Fatalf("DesktopDir returned empty path with nil error")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("stat desktop dir %s: %v", dir, err)
	}
}

// readAppUserModelID loads a .lnk back through IPersistFile and reads
// PKEY_AppUserModel_ID from its IPropertyStore. ok is false when the
// property is absent (VT_EMPTY).
//
//nolint:gosec // G103: addresses of live COM objects and locals for the duration of each call. G115: HRESULT is the 32-bit value in r0.
func readAppUserModelID(t *testing.T, path string) (string, bool) {
	t.Helper()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := coInitialize(); err != nil {
		t.Fatal(err)
	}
	defer windows.CoUninitialize()

	sl, err := createShellLink()
	if err != nil {
		t.Fatal(err)
	}
	defer sl.release()
	pf, err := sl.queryPersistFile()
	if err != nil {
		t.Fatal(err)
	}
	defer pf.release()
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	if hr := int32(callMethod(pf.vtbl.Load, uintptr(unsafe.Pointer(pf)), uintptr(unsafe.Pointer(p)), 0)); hr < 0 {
		t.Fatalf("IPersistFile.Load: hresult 0x%08X", uint32(hr))
	}
	ps, err := sl.queryPropertyStore()
	if err != nil {
		t.Fatal(err)
	}
	defer ps.release()

	v, err := ps.getString(&pkeyAppUserModelID)
	if err != nil {
		t.Fatal(err)
	}
	return v, v != ""
}

func TestCreateWritesAppUserModelID(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "Game.lnk")
	if err := Create(path, Link{Target: exe, Args: "--play x", WorkDir: dir, AppUserModelID: "Typhon.Game.x"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, ok := readAppUserModelID(t, path)
	if !ok || got != "Typhon.Game.x" {
		t.Fatalf("AppUserModelID = %q (present %v), want Typhon.Game.x", got, ok)
	}
}

func TestCreateWithoutAppUserModelIDLeavesPropertyEmpty(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "Plain.lnk")
	if err := Create(path, Link{Target: exe}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got, ok := readAppUserModelID(t, path); ok {
		t.Fatalf("unexpected AppUserModelID %q", got)
	}
}

func TestCreateRejectsInvalidAppUserModelID(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for name, id := range map[string]string{"space": "a b", "slash": "a/b", "long": strings.Repeat("a", 129)} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "Bad.lnk")
			err := Create(path, Link{Target: exe, AppUserModelID: id})
			if !errors.Is(err, ErrAppIDInvalid) && !errors.Is(err, ErrAppIDTooLong) {
				t.Fatalf("err = %v, want invalid or too long", err)
			}
			if _, statErr := os.Stat(path); !errors.Is(statErr, fs.ErrNotExist) {
				t.Fatalf("shortcut written despite invalid id: %v", statErr)
			}
		})
	}
}

func TestSetProcessAppID(t *testing.T) {
	if err := SetProcessAppID(""); !errors.Is(err, ErrAppIDEmpty) {
		t.Fatalf("empty: %v", err)
	}
	if err := SetProcessAppID("bad id"); !errors.Is(err, ErrAppIDInvalid) {
		t.Fatalf("invalid: %v", err)
	}
}
