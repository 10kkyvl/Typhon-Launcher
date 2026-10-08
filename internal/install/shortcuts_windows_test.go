//go:build windows

package install

import (
	"path/filepath"
	"testing"
)

func TestSharedShortcutRootsAreSubsetOfShortcutRoots(t *testing.T) {
	shared, err := sharedShortcutRoots()
	if err != nil {
		t.Fatalf("sharedShortcutRoots error = %v", err)
	}
	if len(shared) != len(sharedShortcutFolders) {
		t.Fatalf("sharedShortcutRoots = %v, want %d roots", shared, len(sharedShortcutFolders))
	}
	all, err := shortcutRoots()
	if err != nil {
		t.Fatalf("shortcutRoots error = %v", err)
	}
	if len(all) != len(shared)+len(userShortcutFolders) {
		t.Fatalf("shortcutRoots = %v, want user and shared roots together", all)
	}
	user := all[:len(userShortcutFolders)]
	for _, root := range shared {
		if !filepath.IsAbs(root) {
			t.Fatalf("общий корень %q не абсолютный", root)
		}
		if !insideAny(all, filepath.Join(root, "x.lnk")) {
			t.Fatalf("общий корень %q не входит в shortcutRoots %v", root, all)
		}
		if insideAny(user, filepath.Join(root, "x.lnk")) {
			t.Fatalf("общий корень %q лежит внутри пользовательского %v: воркер и лаунчер делили бы каталог", root, user)
		}
	}
}
