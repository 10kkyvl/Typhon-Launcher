package discovery

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"typhon/internal/platform"
)

func TestScanLooksInEveryLibraryFolder(t *testing.T) {
	settingsService, root, _ := configuredSettings(t)
	extra := filepath.Join(filepath.Dir(filepath.Dir(root)), "Games2")
	if _, err := settingsService.AddLibraryFolder(context.Background(), extra); err != nil {
		t.Fatalf("add folder: %v", err)
	}
	inMain := installedGame(t, root, "Portal")
	inExtra := installedGame(t, extra, "Doom")
	lib := &fakeLibrary{}
	svc := start(t, settingsService, lib, newCatalog(t), nil)

	result, err := svc.Scan()
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if result.Added != 2 {
		t.Fatalf("added = %d, want a game from each folder (%+v)", result.Added, result.Problems)
	}
	lib.mu.Lock()
	defer lib.mu.Unlock()
	found := make([]string, 0, len(lib.applied))
	for _, d := range lib.applied {
		found = append(found, d.InstallDir)
	}
	for _, want := range []string{inMain, inExtra} {
		if !slices.ContainsFunc(found, func(got string) bool { return platform.SamePath(got, want) }) {
			t.Fatalf("registered %q lacks %q", found, want)
		}
	}
}

func TestRootsAreTheGamesFolderAndTheExtraFolders(t *testing.T) {
	settingsService, root, _ := configuredSettings(t)
	extra := filepath.Join(filepath.Dir(filepath.Dir(root)), "Games2")
	if _, err := settingsService.AddLibraryFolder(context.Background(), extra); err != nil {
		t.Fatalf("add folder: %v", err)
	}
	svc := start(t, settingsService, &fakeLibrary{}, newCatalog(t), nil)

	got, err := svc.roots()
	if err != nil {
		t.Fatalf("roots: %v", err)
	}
	if len(got) != 2 || !platform.SamePath(got[0], root) || !platform.SamePath(got[1], extra) {
		t.Fatalf("roots = %q, want %q then %q", got, root, extra)
	}
}
