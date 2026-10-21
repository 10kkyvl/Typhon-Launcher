//go:build !devmock

package settings

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestConfigDirUsesThePortableDirectory(t *testing.T) {
	isolatedUserConfigDir(t)
	marker := "state"
	exeDir := fakeLauncher(t, &marker)

	dir, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir() error = %v", err)
	}
	if want := filepath.Join(exeDir, "state"); dir != want {
		t.Fatalf("ConfigDir() = %q, want %q", dir, want)
	}
}

func TestConfigDirRefusesToFallBackWhenTheMarkerIsInvalid(t *testing.T) {
	base := isolatedUserConfigDir(t)
	marker := `..\elsewhere`
	fakeLauncher(t, &marker)

	dir, err := ConfigDir()
	if !errors.Is(err, ErrPortableMarkerInvalid) {
		t.Fatalf("ConfigDir() = %q, %v; want ErrPortableMarkerInvalid", dir, err)
	}
	if dir == filepath.Join(base, "Typhon") {
		t.Fatalf("ConfigDir() fell back to the profile directory %q", dir)
	}
}

func TestConfigDirKeepsTheProfileDirectoryWithoutMarker(t *testing.T) {
	base := isolatedUserConfigDir(t)
	fakeLauncher(t, nil)

	dir, err := ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(base, "Typhon"); dir != want {
		t.Fatalf("ConfigDir() = %q, want %q", dir, want)
	}
}
