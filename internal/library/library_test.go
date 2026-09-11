package library

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func tempGameExe(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	exe := filepath.Join(dir, "game.exe")
	if err := os.WriteFile(exe, []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}
	return exe
}

func mustServiceAt(t testing.TB, path string) *Service {
	t.Helper()
	s, err := NewServiceAt(path)
	if err != nil {
		t.Fatalf("new library service at %s: %v", path, err)
	}
	// Existing tests assert on real child processes (pid, exit code, working
	// directory): forcing the exec-based starter keeps that true under
	// -tags devmock too, where newGameStarter would otherwise hand back a
	// fake process. Only process_devmock_test.go exercises the devmock
	// starter, and it sets s.start itself.
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.start = execStarter
	// Настоящая подготовка окружения на macOS заводит бутыль CrossOver:
	// секунды и сотни мегабайт на каждый запуск. Тесты запускают обычные
	// процессы, и оставлять после прогона настоящие бутыли нельзя.
	s.prepare = func(context.Context, launch) error { return nil }
	// Registered after the t.TempDir() that produced path, so it runs before
	// that directory is removed: a session goroutine still persisting into it
	// would otherwise race the cleanup.
	t.Cleanup(func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Errorf("library shutdown: %v", err)
		}
	})
	return s
}

func TestAddAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)

	exe := tempGameExe(t)
	game, err := s.AddGame(exe, "  ")
	if err != nil {
		t.Fatal(err)
	}
	if game.Title != "game" {
		t.Fatalf("title = %q", game.Title)
	}
	if game.InstallDir != filepath.Dir(exe) {
		t.Fatalf("install dir = %q", game.InstallDir)
	}
	if game.SizeBytes == 0 {
		t.Fatal("size not computed")
	}

	if _, err := s.AddGame(exe, "Duplicate"); err == nil {
		t.Fatal("expected duplicate error")
	}

	reloaded := mustServiceAt(t, path).GetInstalledGames()
	if len(reloaded) != 1 || reloaded[0].ID != game.ID {
		t.Fatalf("reloaded = %+v", reloaded)
	}

	if err := s.RemoveGame(game.ID); err != nil {
		t.Fatal(err)
	}
	if len(mustServiceAt(t, path).GetInstalledGames()) != 0 {
		t.Fatal("remove not persisted")
	}
}

func TestPlayMissingExecutable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	exe := tempGameExe(t)
	game, err := s.AddGame(exe, "Ghost")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(exe); err != nil {
		t.Fatal(err)
	}
	if err := s.PlayGame(game.ID); err == nil {
		t.Fatal("expected error for missing executable")
	}
}

func TestRegisterInstalledAddsGame(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	exe := tempGameExe(t)

	uploadedAt := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	game, err := s.RegisterInstalled(InstalledGame{
		Title:             "  Space Game  ",
		Executable:        exe,
		InstallDir:        filepath.Dir(exe),
		Version:           "1.2.3",
		SourceDownloadID:  "d1",
		ReleaseID:         "release-1",
		SourceID:          "source-1",
		DistributionID:    "space-game-main",
		ReleaseUploadedAt: &uploadedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if game.Title != "Space Game" || game.Version != "1.2.3" || game.SourceDownloadID != "d1" {
		t.Fatalf("game = %+v", game)
	}
	if game.SizeBytes == 0 || game.InstalledAt.IsZero() {
		t.Fatalf("game = %+v", game)
	}

	reloaded := mustServiceAt(t, path).GetInstalledGames()
	if len(reloaded) != 1 || reloaded[0].SourceDownloadID != "d1" || reloaded[0].DistributionID != "space-game-main" ||
		reloaded[0].ReleaseUploadedAt == nil || !reloaded[0].ReleaseUploadedAt.Equal(uploadedAt) {
		t.Fatalf("reloaded = %+v", reloaded)
	}
}

func TestRegisterInstalledUpdatesExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	exe := tempGameExe(t)

	first, err := s.AddGame(exe, "Original")
	if err != nil {
		t.Fatal(err)
	}

	updated, err := s.RegisterInstalled(InstalledGame{
		Executable:       strings.ToUpper(exe),
		InstallDir:       filepath.Dir(exe),
		Version:          "2.0",
		SourceDownloadID: "d2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != first.ID {
		t.Fatalf("id = %q, want %q", updated.ID, first.ID)
	}
	if updated.Title != "Original" {
		t.Fatalf("title = %q, want the original one", updated.Title)
	}
	if updated.Version != "2.0" || updated.SourceDownloadID != "d2" {
		t.Fatalf("game = %+v", updated)
	}
	if games := s.GetInstalledGames(); len(games) != 1 {
		t.Fatalf("games = %+v", games)
	}
}

func TestRegisterInstalledRejectsMissingExecutable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	if _, err := s.RegisterInstalled(InstalledGame{Executable: filepath.Join(t.TempDir(), "nope.exe")}); err == nil {
		t.Fatal("expected error for missing executable")
	}
}

// Сборка нужна общей статистике: репаки разных сборщиков ведут себя по-разному,
// и сложить их в одну цифру значит соврать.
func TestRegisterInstalledKeepsTheBuild(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	exe, _ := testExecutable(t)

	game, err := s.RegisterInstalled(InstalledGame{
		Title: "Game", Executable: exe, InstallDir: filepath.Dir(exe),
		ReleaseID: "rel-1", CanonicalGameID: "canon-1",
		Repacker: "fitgirl", ReleaseVersion: "1.0.28518",
	})
	if err != nil {
		t.Fatalf("RegisterInstalled: %v", err)
	}
	if game.Repacker != "fitgirl" || game.ReleaseVersion != "1.0.28518" {
		t.Fatalf("сборка потеряна: %+v", game)
	}
}

// Переустановка другой сборкой обязана переписать прошлую: иначе статистика
// припишет исход не тому репаку.
func TestReinstallReplacesTheBuild(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	exe, _ := testExecutable(t)
	base := InstalledGame{
		Title: "Game", Executable: exe, InstallDir: filepath.Dir(exe),
		ReleaseID: "rel-1", CanonicalGameID: "canon-1",
		Repacker: "fitgirl", ReleaseVersion: "1.0",
	}
	if _, err := s.RegisterInstalled(base); err != nil {
		t.Fatalf("первая установка: %v", err)
	}

	next := base
	next.ReleaseID = "rel-2"
	next.Repacker = "dodi"
	next.ReleaseVersion = "1.2"
	game, err := s.RegisterInstalled(next)
	if err != nil {
		t.Fatalf("переустановка: %v", err)
	}
	if game.Repacker != "dodi" || game.ReleaseVersion != "1.2" {
		t.Fatalf("сборка не обновилась: %+v", game)
	}
}

func TestReinstallReplacesDistributionBindingAtomically(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	exe, _ := testExecutable(t)
	base := InstalledGame{
		Title: "Game", Executable: exe, InstallDir: filepath.Dir(exe),
		ReleaseID: "rel-a", SourceID: "src", DistributionID: "distribution-a",
	}
	if _, err := s.RegisterInstalled(base); err != nil {
		t.Fatalf("first install: %v", err)
	}

	next := base
	next.ReleaseID = "rel-b"
	next.DistributionID = ""
	game, err := s.RegisterInstalled(next)
	if err != nil {
		t.Fatalf("reinstall: %v", err)
	}
	if game.ReleaseID != "rel-b" || game.SourceID != "src" || game.DistributionID != "" {
		t.Fatalf("stale distribution survived reinstall: %+v", game)
	}
}

func TestBindDistributionPersistsOnlyExactLegacyRelease(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "Game")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(dir, "game.exe")
	if err := os.WriteFile(executable, []byte("stub"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "library.json")
	s := mustServiceAt(t, path)
	game, err := s.RegisterInstalled(InstalledGame{
		Title: "Game", Executable: executable, InstallDir: dir, ReleaseID: "release-a", SourceID: "source-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	uploadedAt := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	bound, err := s.BindDistribution(game.ID, "source-a", "release-a", "distribution-a", &uploadedAt)
	if err != nil {
		t.Fatal(err)
	}
	if bound.DistributionID != "distribution-a" || bound.ReleaseUploadedAt == nil || !bound.ReleaseUploadedAt.Equal(uploadedAt) {
		t.Fatalf("bound game = %+v", bound)
	}
	if _, err := s.BindDistribution(game.ID, "source-a", "release-a", "distribution-b", &uploadedAt); err == nil {
		t.Fatal("existing distribution binding was overwritten")
	}
	if _, err := s.BindDistribution(game.ID, "source-a", "foreign-release", "distribution-a", &uploadedAt); err == nil {
		t.Fatal("foreign saved release was accepted")
	}
	reloaded := mustServiceAt(t, path).GetInstalledGames()
	if len(reloaded) != 1 || reloaded[0].DistributionID != "distribution-a" || reloaded[0].ReleaseUploadedAt == nil ||
		!reloaded[0].ReleaseUploadedAt.Equal(uploadedAt) {
		t.Fatalf("persisted game = %+v", reloaded)
	}
}
