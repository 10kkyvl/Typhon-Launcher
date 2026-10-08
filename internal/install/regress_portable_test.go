package install

import (
	"errors"
	"path/filepath"
	"testing"
)

// Источник перемещаемой раздачи — единственная копия, пока игра не записана в
// библиотеку: отказ регистрации не должен лишать пользователя файлов.
func TestPortableMoveKeepsTheSourceUntilTheGameIsRegistered(t *testing.T) {
	r := newRig(t)
	root := t.TempDir()
	source := portableSource(t, root, "Game")
	r.download("d1", "Game", root)
	dest := filepath.Join(t.TempDir(), "Games", "Game")
	lib := &lockedLibrary{fakeRegistrar: r.reg}
	lib.locked.Store(true)
	r.lib = lib
	r.s.library = lib

	item, err := r.s.Start("d1", StartOptions{Destination: dest, Mode: ModeMove})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	got := r.settle(item.ID)
	if got.Status != StatusFailed || got.Mode != ModeMove {
		t.Fatalf("status %s mode %q (%q)", got.Status, got.Mode, got.Error)
	}
	if !exists(filepath.Join(source, "Game.exe")) {
		t.Fatal("the only copy of the game was removed although the library never accepted it")
	}
	if !exists(filepath.Join(dest, "Game.exe")) {
		t.Fatal("the copied files were removed together with the failed registration")
	}
}

// Не удалившийся источник после удачной установки — предупреждение, а не провал:
// игра уже на месте и записана.
func TestPortableMoveSourceCleanupFailureDoesNotFailTheInstall(t *testing.T) {
	r := newRig(t)
	root := t.TempDir()
	source := portableSource(t, root, "Game")
	r.download("d1", "Game", root)
	dest := filepath.Join(t.TempDir(), "Games", "Game")
	previous := removeInstalledSource
	removeInstalledSource = func(string) error { return errors.New("files are locked by the torrent client") }
	t.Cleanup(func() { removeInstalledSource = previous })

	item, err := r.s.Start("d1", StartOptions{Destination: dest, Mode: ModeMove})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	got := r.settle(item.ID)
	if got.Status != StatusCompleted || got.Error != "" || got.GameID == "" {
		t.Fatalf("after a source cleanup failure: %+v", viewOf(got))
	}
	if !exists(filepath.Join(source, "Game.exe")) {
		t.Fatal("the source disappeared although its removal was refused")
	}
	r.assertDurable(item.ID)
}

// «Отмена» на шаге выбора исполняемого файла закрывает установку без повторного
// запуска установщика; выбранный после этого файл уже ничего не завершит.
func TestCancelWhileWaitingForTheExecutable(t *testing.T) {
	r := newRig(t)
	installed := interactiveSource(t, r)
	run := newRgRunner(t, rgStep{act: func(runSpec) { mkFile(t, filepath.Join(installed, "MyGame.exe"), 512<<10) }})
	r.setRunner(run)
	log := r.finished()
	item, err := r.s.Start("d1", StartOptions{})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := r.settle(item.ID); got.Status != StatusWaitingForUser {
		t.Fatalf("status = %s", got.Status)
	}

	if err := r.s.Cancel(item.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	got := r.get(item.ID)
	if got.Status != StatusCancelled || got.Error != "" {
		t.Fatalf("after cancel: status %s error %q", got.Status, got.Error)
	}
	if notified := log.next(t); notified.Status != StatusCancelled {
		t.Fatalf("onFinished status = %s", notified.Status)
	}
	if err := r.s.ConfirmExecutable(item.ID, filepath.Join(installed, "MyGame.exe")); errCode(err) != "install.unavailable" {
		t.Fatalf("Confirm after cancel = %v, want install.unavailable", err)
	}
	if games := r.reg.registered(); len(games) != 0 {
		t.Fatalf("a cancelled install registered %+v", games)
	}
	if n := len(run.calls()); n != 1 {
		t.Fatalf("installer runs = %d, want 1", n)
	}
	r.assertDurable(item.ID)
}
