package app

import (
	"os"
	"runtime"
	"testing"
)

// configDirEnv names the variable os.UserConfigDir reads on this OS, so a test
// can point the launcher's config dir at a path that cannot be created.
func configDirEnv() string {
	switch runtime.GOOS {
	case "windows":
		return "AppData"
	case "darwin":
		return "HOME"
	default:
		return "XDG_CONFIG_HOME"
	}
}

// blockRename makes renaming backup inside dir fail until the returned func
// is called. On Windows a held-open handle does it: Go opens files without
// FILE_SHARE_DELETE, so the rename is refused like it would be under an AV
// scan. POSIX renames ignore open handles, so there the directory itself is
// made read-only instead; root ignores mode bits, so the test skips as root.
func blockRename(t *testing.T, dir, backup string) func() {
	t.Helper()
	if runtime.GOOS == "windows" {
		locked, err := os.Open(backup)
		if err != nil {
			t.Fatalf("lock backup: %v", err)
		}
		released := false
		return func() {
			if released {
				return
			}
			released = true
			if err := locked.Close(); err != nil {
				t.Fatalf("close lock: %v", err)
			}
		}
	}
	if os.Geteuid() == 0 {
		t.Skip("read-only directories do not block root")
	}
	//nolint:gosec // G302: каталог, а не файл: без бита x он не читается; тесту нужен read-only каталог, а не 0600
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("make dir read-only: %v", err)
	}
	released := false
	return func() {
		if released {
			return
		}
		released = true
		//nolint:gosec // G302: каталогу возвращается исходный режим (инвариант 8), иначе t.TempDir() не сможет его удалить
		if err := os.Chmod(dir, 0o755); err != nil {
			t.Fatalf("restore dir mode: %v", err)
		}
	}
}
