//go:build windows

package install

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestExtendedPath(t *testing.T) {
	cases := []struct {
		name string
		path string
		want string
	}{
		{name: "диск", path: `C:\Users\Public\Desktop\ti.lnk`, want: `\\?\C:\Users\Public\Desktop\ti.lnk`},
		{name: "прямые слеши", path: `C:/Users/Public/Desktop`, want: `\\?\C:\Users\Public\Desktop`},
		{name: "точки в пути", path: `C:\Users\Public\..\Public\Desktop\.\ti.lnk`, want: `\\?\C:\Users\Public\Desktop\ti.lnk`},
		{name: "UNC", path: `\\server\share\Desktop\ti.lnk`, want: `\\?\UNC\server\share\Desktop\ti.lnk`},
		{name: "UNC прямыми слешами", path: `//server/share/Desktop`, want: `\\?\UNC\server\share\Desktop`},
		{name: "уже расширенный", path: `\\?\C:\Desktop\ti.lnk`, want: `\\?\C:\Desktop\ti.lnk`},
		{name: "устройство", path: `\\.\pipe\x`, want: `\\.\pipe\x`},
		{name: "относительный", path: `Desktop\ti.lnk`, want: `Desktop\ti.lnk`},
		{name: "от корня без диска", path: `\Desktop\ti.lnk`, want: `\Desktop\ti.lnk`},
		{name: "пусто", path: "", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extendedPath(tc.path); got != tc.want {
				t.Fatalf("extendedPath(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

// Без \\?\ CreateFile упирается в MAX_PATH там, где длинные пути в системе
// выключены, а отказ «пути нет» выглядел бы как уже удалённый ярлык.
func TestCleanShellShortcutsLongPath(t *testing.T) {
	root := t.TempDir()
	before := shellSnapshotOf(t, root)
	deep := root
	for len(deep) < 300 {
		deep = filepath.Join(deep, strings.Repeat("d", 40))
	}
	if err := os.MkdirAll(extendedPath(deep), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", deep, err)
	}
	site := filepath.Join(deep, "ti.url")
	writeFile(t, extendedPath(site), siteURL)

	removed, err := cleanShellShortcuts(context.Background(), before, "", false)
	if err != nil || exists(extendedPath(site)) {
		t.Fatalf("removed = %v, err = %v, site exists = %v", removed, err, exists(extendedPath(site)))
	}
	if !slices.Contains(removed, strings.ToLower(site)) {
		t.Fatalf("removed = %v, want %s", removed, site)
	}
}

// Обычный путь Win32 нормализует: точка в конце имени срезается, и открылась
// бы не та запись, что дал обход каталога, — каталог «grp.» молча оставался.
func TestCleanShellShortcutsKeepsNameAsListed(t *testing.T) {
	root := t.TempDir()
	before := shellSnapshotOf(t, root)
	odd := extendedPath(filepath.Join(root, "grp."))
	if err := os.Mkdir(odd, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", odd, err)
	}
	t.Cleanup(func() {
		if err := os.Remove(odd); err != nil && !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("remove %s: %v", odd, err)
		}
	})

	removed, err := cleanShellShortcuts(context.Background(), before, "", true)
	if err != nil || len(removed) != 1 || exists(odd) {
		t.Fatalf("removed = %v, err = %v, dir exists = %v", removed, err, exists(odd))
	}
}

// Пока запись открыта, её не переименовать и не удалить ни через какой другой
// хэндл, и каталог над ней не переименовать: проверка и удаление видят один и
// тот же файл.
func TestShellEntryHoldsEntryInPlace(t *testing.T) {
	cases := []struct {
		name   string
		dir    bool
		action func(root, group, file string) error
	}{
		{name: "переименовать файл", action: func(_, group, file string) error { return os.Rename(file, filepath.Join(group, "moved.url")) }},
		{name: "удалить файл", action: func(_, _, file string) error { return os.Remove(file) }},
		{name: "переименовать каталог над файлом", action: func(root, group, _ string) error { return os.Rename(group, filepath.Join(root, "moved")) }},
		{name: "переименовать открытый каталог", dir: true, action: func(root, group, _ string) error { return os.Rename(group, filepath.Join(root, "moved")) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			group := filepath.Join(root, "group")
			mkdirs(t, group)
			file := filepath.Join(group, "ti.url")
			writeFile(t, file, siteURL)
			target := file
			if tc.dir {
				target = group
			}
			entry, err := openShellEntry(shellSnapshotOf(t, root), target, tc.dir)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			actionErr := tc.action(root, group, file)
			if err := entry.close(); err != nil {
				t.Fatalf("close: %v", err)
			}
			if actionErr == nil {
				t.Fatal("запись сдвинули, пока она была открыта")
			}
			if !exists(target) {
				t.Fatalf("%s пропал", target)
			}
		})
	}
}

func TestShellEntryRemovesReadOnlyThroughHandle(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "ti.url")
	writeFile(t, file, siteURL)
	if err := os.Chmod(file, 0o400); err != nil {
		t.Fatalf("chmod %s: %v", file, err)
	}
	entry, err := openShellEntry(shellSnapshotOf(t, root), file, false)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	removeErr := entry.remove()
	if err := entry.close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if removeErr != nil || exists(file) {
		t.Fatalf("remove = %v, file exists = %v", removeErr, exists(file))
	}
}

func setDACL(t *testing.T, path, sddl string) {
	t.Helper()
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		t.Fatalf("sddl %s: %v", sddl, err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatalf("dacl %s: %v", sddl, err)
	}
	info := windows.SECURITY_INFORMATION(windows.DACL_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION)
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, info, nil, nil, dacl, nil); err != nil {
		t.Fatalf("set dacl %s on %s: %v", sddl, path, err)
	}
}

// Лаунчер без прав администратора не получает DELETE в общем каталоге: ярлык,
// который трогать не надо, обязан пройти проверку без ошибки, а отказ
// всплывает только у того, который действительно надо удалить.
func TestCleanShellShortcutsWithoutDeleteAccess(t *testing.T) {
	root := t.TempDir()
	before := shellSnapshotOf(t, root)
	group := filepath.Join(root, "group")
	mkdirs(t, group)
	site := filepath.Join(group, "site.lnk")
	writeShortcut(t, site, `C:\Program Files (x86)\TI\TI.URL`)
	game := filepath.Join(group, "game.lnk")
	writeShortcut(t, game, `C:\Games\Other\other.exe`)
	// DELETE на файл даёт и FILE_DELETE_CHILD (0x40) на каталоге, поэтому
	// запрещать приходится оба.
	for _, path := range []string{site, game} {
		setDACL(t, path, "D:(D;;SD;;;WD)(A;;FA;;;WD)")
	}
	setDACL(t, group, "D:(D;;0x40;;;WD)(A;OICI;FA;;;WD)")
	t.Cleanup(func() {
		setDACL(t, group, "D:(A;OICI;FA;;;WD)")
		for _, path := range []string{site, game} {
			setDACL(t, path, "D:(A;;FA;;;WD)")
		}
	})

	removed, err := cleanShellShortcuts(context.Background(), before, filepath.Join(root, "dest", "game"), false)
	if err == nil || !strings.Contains(err.Error(), site) || strings.Contains(err.Error(), game) {
		t.Fatalf("err = %v, want only the site shortcut refusal", err)
	}
	if len(removed) != 0 || !exists(site) || !exists(game) {
		t.Fatalf("removed = %v, site exists = %v, game exists = %v", removed, exists(site), exists(game))
	}
}
