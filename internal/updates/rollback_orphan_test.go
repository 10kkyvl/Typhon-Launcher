package updates

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	"typhon/internal/library"
)

// Если старую версию не удалось удалить сразу после отката (файл занят,
// нет прав, выключение между шагами), каталог .replaced не должен остаться
// сиротой без журнала: иначе следующий откат этой игры навсегда отвечает
// «требуется восстановление предыдущего отката». Удаление идёт фазой
// cleanup под журналом, и следующий запуск доводит его до конца.
func TestRollbackCleansUpTheReplacedVersionOnTheNextStartWhenRemovalFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	root := t.TempDir()
	current := filepath.Join(root, "game")
	previous := current + previousSuffix
	replaced := current + replacedSuffix
	for dir, version := range map[string]string{current: "new", previous: "old"} {
		mkTree(t, dir, version)
		if err := os.WriteFile(filepath.Join(dir, "game.exe"), []byte(version), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Подкаталог без права записи: RemoveAll не сможет удалить файл внутри.
	locked := filepath.Join(current, "locked")
	if err := os.MkdirAll(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(locked, "busy.bin"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o500); err != nil {
		t.Fatal(err)
	}
	unlock := func() {
		if err := os.Chmod(filepath.Join(replaced, "locked"), 0o755); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Logf("unlock: %v", err)
		}
	}
	t.Cleanup(unlock)

	libraryPath := filepath.Join(root, "library.json")
	lib, err := library.NewServiceAt(libraryPath)
	if err != nil {
		t.Fatal(err)
	}
	game, err := lib.RegisterInstalled(t.Context(), library.InstalledGame{Title: "Game", InstallDir: current, Executable: filepath.Join(current, "game.exe"), Version: "2", ReleaseID: "r2", SourceID: "s", DistributionID: "d"})
	if err != nil {
		t.Fatal(err)
	}
	configDir := filepath.Join(root, "updates")
	s, err := newServiceAt(configDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.library = lib
	s.ctx = context.Background()
	s.downloads = newFakeDownloads()
	s.rollbacks[game.ID] = &Rollback{GameID: game.ID, Path: previous, InstallDir: current, Executable: game.Executable, Version: "1", ReleaseID: "r1", SourceID: "s", DistributionID: "d"}
	s.updates[game.ID] = &Update{GameID: game.ID, Plan: &UpdatePlan{GameID: game.ID}}
	if err := s.persistRollbacksLocked(); err != nil {
		t.Fatal(err)
	}

	if err := s.Rollback(t.Context(), game.ID); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if readMarker(t, current) != "old" {
		t.Fatal("rollback did not restore the old version")
	}
	if !exists(replaced) {
		t.Skip("the locked directory was removable on this filesystem; the scenario needs a stuck deletion")
	}
	journals, err := s.store.loadJournals()
	if err != nil {
		t.Fatal(err)
	}
	if len(journals) != 1 || journals[0].Kind != JournalCleanup || journals[0].RetainedPrevious != replaced {
		t.Fatalf("journal after a stuck removal = %+v, want a cleanup phase pointing at %s", journals, replaced)
	}

	// Следующий запуск: файл освободился, запись должна исчезнуть вместе с журналом.
	unlock()
	fresh, err := newServiceAt(configDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	onDisk, err := library.NewServiceAt(libraryPath)
	if err != nil {
		t.Fatal(err)
	}
	fresh.library = onDisk
	fresh.downloads = newFakeDownloads()
	if err := fresh.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := fresh.ServiceShutdown(); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})
	if exists(replaced) {
		t.Fatal("startup recovery left the replaced version behind")
	}
	left, err := fresh.store.loadJournals()
	if err != nil || len(left) != 0 {
		t.Fatalf("journals after recovery = %+v, error = %v", left, err)
	}

	// И откат снова возможен: новая запись не упирается в сироту.
	again := filepath.Join(root, "game.previous-2")
	mkTree(t, again, "older")
	fresh.mu.Lock()
	fresh.rollbacks[game.ID] = &Rollback{GameID: game.ID, Path: again, InstallDir: current, Executable: game.Executable, Version: "0", ReleaseID: "r0", SourceID: "s", DistributionID: "d"}
	fresh.updates[game.ID] = &Update{GameID: game.ID}
	err = fresh.persistRollbacksLocked()
	fresh.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if err := fresh.Rollback(t.Context(), game.ID); err != nil {
		t.Fatalf("second rollback refused: %v", err)
	}
	if readMarker(t, current) != "older" {
		t.Fatal("second rollback did not land")
	}
}
