package install

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// requireUnreadableDir removes read permission from dir so os.ReadDir fails
// with a permission error rather than reporting an empty listing. Mirrors
// requireUnwritableDir in persist_test.go: POSIX permission bits don't apply
// the same way on Windows, and root ignores them entirely, so both cases
// skip just this scenario instead of the whole test.
func requireUnreadableDir(t *testing.T, dir string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX-права каталога не действуют на windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("запущено от root: chmod не мешает чтению")
	}
	if err := os.Chmod(dir, 0o000); err != nil { //nolint:gosec // G302: тест инварианта 26 требует нечитаемый каталог
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(dir, 0o700); err != nil { //nolint:gosec // G302: возврат прав, выставленных выше по той же причине
			t.Errorf("restore chmod: %v", err)
		}
	})
}

// TestVerifyInstallDistinguishesReadErrorFromEmpty закрывает finding 2:
// verifyInstall схлопывал любую ошибку os.ReadDir (нет прав, диск отвалился,
// путь исчез) в errEmptyInstall, выдавая пользователю неверную причину.
// Ошибка чтения каталога и реально пустой каталог обязаны различаться.
func TestVerifyInstallDistinguishesReadErrorFromEmpty(t *testing.T) {
	t.Run("truly empty directory reports errEmptyInstall", func(t *testing.T) {
		dir := t.TempDir()
		item := Installation{Destination: dir}
		err := verifyInstall(item)
		if !errors.Is(err, errEmptyInstall) {
			t.Fatalf("err = %v, want errEmptyInstall", err)
		}
	})

	t.Run("unreadable directory reports a read error, not errEmptyInstall", func(t *testing.T) {
		dir := t.TempDir()
		mkFile(t, filepath.Join(dir, "keep.txt"), 4)
		requireUnreadableDir(t, dir)

		item := Installation{Destination: dir}
		err := verifyInstall(item)
		if err == nil {
			t.Fatal("verifyInstall returned nil for an unreadable destination")
		}
		if errors.Is(err, errEmptyInstall) {
			t.Fatalf("err = %v, want a read error distinct from errEmptyInstall", err)
		}
		if !errors.Is(err, os.ErrPermission) {
			t.Fatalf("err = %v, want it to wrap the underlying permission error", err)
		}
	})

	t.Run("populated directory passes without an executable to check", func(t *testing.T) {
		dir := t.TempDir()
		mkFile(t, filepath.Join(dir, "data.bin"), 4)
		item := Installation{Destination: dir}
		if err := verifyInstall(item); err != nil {
			t.Fatalf("verifyInstall = %v, want nil for a populated destination", err)
		}
	})
}
