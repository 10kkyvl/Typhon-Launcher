package discovery

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/catalog"
	"typhon/internal/library"
	"typhon/internal/settings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type fixture struct {
	svc      *Service
	lib      *library.Service
	cat      *catalog.Service
	settings *settings.Service
	root     string
}

func configuredSettings(t *testing.T) (*settings.Service, string, string) {
	t.Helper()
	base := t.TempDir()
	config := filepath.Join(base, "config")
	if err := os.MkdirAll(config, 0o755); err != nil {
		t.Fatal(err)
	}
	settingsService, err := settings.NewServiceAt(filepath.Join(config, "settings.json"))
	if err != nil {
		t.Fatalf("settings service: %v", err)
	}
	if _, err := settingsService.SetupLibrary(filepath.Join(base, settings.LibraryFolderName)); err != nil {
		t.Fatalf("setup library: %v", err)
	}
	return settingsService, settingsService.GetSettings().GamesPath, config
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	settingsService, root, config := configuredSettings(t)
	lib, err := library.NewServiceAt(filepath.Join(config, "library.json"))
	if err != nil {
		t.Fatalf("library service: %v", err)
	}
	cat, err := catalog.NewServiceAt(config)
	if err != nil {
		t.Fatalf("catalog service: %v", err)
	}
	return &fixture{
		svc:      start(t, settingsService, lib, cat, nil),
		lib:      lib,
		cat:      cat,
		settings: settingsService,
		root:     root,
	}
}

func newCatalog(t *testing.T) *catalog.Service {
	t.Helper()
	cat, err := catalog.NewServiceAt(t.TempDir())
	if err != nil {
		t.Fatalf("catalog service: %v", err)
	}
	return cat
}

func start(t *testing.T, settingsService *settings.Service, lib gameLibrary, cat gameCatalog, meta metadataResolver) *Service {
	t.Helper()
	svc, err := NewService(settingsService, lib, cat, meta)
	if err != nil {
		t.Fatalf("discovery service: %v", err)
	}
	if err := svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("startup: %v", err)
	}
	t.Cleanup(func() {
		if err := svc.ServiceShutdown(); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})
	return svc
}

func (f *fixture) scan(t *testing.T) Result {
	t.Helper()
	result, err := f.svc.Scan()
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	return result
}

func (f *fixture) game(t *testing.T, title string) library.Game {
	t.Helper()
	for _, game := range f.lib.GetInstalledGames() {
		if game.Title == title {
			return game
		}
	}
	t.Fatalf("игра %q не найдена в библиотеке: %+v", title, f.lib.GetInstalledGames())
	return library.Game{}
}

func writeFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	payload := make([]byte, size)
	for i := range payload {
		payload[i] = byte('a' + i%26)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
}

// installedGame повторяет то, что оставляет после себя установщик Typhon:
// каталог игры в корне библиотеки, исполняемый файл по имени игры и каталог с
// ресурсами рядом.
func installedGame(t *testing.T, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	writeFile(t, filepath.Join(dir, name+".exe"), 4096)
	writeFile(t, filepath.Join(dir, "Data", "content.pak"), 2048)
	return dir
}

func unrelatedFolder(t *testing.T, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	writeFile(t, filepath.Join(dir, "readme.txt"), 32)
	writeFile(t, filepath.Join(dir, "notes", "todo.txt"), 32)
	return dir
}

func incompleteInstall(t *testing.T, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	writeFile(t, filepath.Join(dir, "setup.exe"), 1024)
	writeFile(t, filepath.Join(dir, "data1.bin"), 512)
	return dir
}

func ambiguousInstall(t *testing.T, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	writeFile(t, filepath.Join(dir, "alpha.exe"), 2048)
	writeFile(t, filepath.Join(dir, "beta.exe"), 2048)
	writeFile(t, filepath.Join(dir, "Data", "content.pak"), 1024)
	return dir
}
