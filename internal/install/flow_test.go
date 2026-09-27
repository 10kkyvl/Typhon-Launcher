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

// TestExeInstallerDropsBrokerOnTheInteractivePath закрывает находку 3: только
// item.Silent && item.Destination != "" уходит в runSilent, а единственный
// defer s.DropBroker жил внутри неё. Интерактивная ветка (waitForUser) не
// звала DropBroker ни на одном пути, и брокер — процесс с правами
// администратора плюс горутина tendBroker с тикером — жил до закрытия
// лаунчера на каждый не-silent репак, для которого его подняли заранее.
func TestExeInstallerDropsBrokerOnTheInteractivePath(t *testing.T) {
	s, downloads, _ := newTestService(t)
	root := t.TempDir()
	dir := filepath.Join(root, "Game")
	mkFile(t, filepath.Join(dir, "setup.exe"), 4096)
	downloads.add("d1", "Game", root)

	programs := t.TempDir()
	installed := filepath.Join(programs, "MyGame")
	s.roots = []string{programs}
	s.runner = &fakeRunner{act: func(runSpec) {
		mkFile(t, filepath.Join(installed, "MyGame.exe"), 512<<10)
	}}

	brokerDir := t.TempDir()
	s.mu.Lock()
	s.brokers = map[string]*broker{"d1": {dir: brokerDir, gone: make(chan struct{})}}
	s.mu.Unlock()

	item, err := s.Start("d1", StartOptions{})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	s.waitStatus(t, item.ID, StatusWaitingForUser)

	// StatusWaitingForUser публикуется внутри waitForUser, а defer
	// s.DropBroker в runInstaller срабатывает только когда она вернётся и
	// стек развернётся до самого runInstaller — статус и снятие брокера не
	// один и тот же момент, поэтому снятие ждём отдельно, а не проверяем
	// синхронно сразу за статусом.
	waitFor(t, "broker dropped after interactive install", func() bool { return s.brokerFor("d1") == nil })
	// askBrokerToExit пишет маркер уже после того, как брокер снят с карты
	// (DropBroker), поэтому ждём отдельно и его — оба шага происходят в
	// одной горутине, но не одним атомарным действием.
	waitFor(t, "broker exit marker written", func() bool {
		_, statErr := os.Stat(brokerAbortPath(brokerDir))
		return statErr == nil
	})
}
