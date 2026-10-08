package install

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func refusedEntry(err error) bool {
	return errors.Is(err, errShellEscaped) || errors.Is(err, errShellReparsePoint) || errors.Is(err, errShellRootChanged)
}

func mkdirs(t *testing.T, paths ...string) {
	t.Helper()
	for _, path := range paths {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
	}
}

func shellSnapshotOf(t *testing.T, root string) shellSnapshot {
	t.Helper()
	snap, err := takeShellSnapshot(context.Background(), []string{root})
	if err != nil {
		t.Fatalf("takeShellSnapshot error = %v", err)
	}
	return snap
}

// Подмена каталога ссылкой между снимком «после» и удалением: так повышенный
// воркер оказался бы обманут процессом без прав, которому доступен общий стол.
func TestRemoveShellShortcutsAfterSwap(t *testing.T) {
	cases := []struct {
		name   string
		layout func(t *testing.T, root string)
		swap   string
		victim func(t *testing.T, outside string) string
	}{
		{
			name: "ярлык сайта в подменённой папке установщика",
			layout: func(t *testing.T, root string) {
				mkdirs(t, filepath.Join(root, "sub"))
				writeFile(t, filepath.Join(root, "sub", "ti.url"), siteURL)
			},
			swap: "sub",
			victim: func(t *testing.T, outside string) string {
				path := filepath.Join(outside, "ti.url")
				writeFile(t, path, siteURL)
				return path
			},
		},
		{
			name:   "пустой каталог за подменённым родителем",
			layout: func(t *testing.T, root string) { mkdirs(t, filepath.Join(root, "a", "b")) },
			swap:   "a",
			victim: func(t *testing.T, outside string) string {
				path := filepath.Join(outside, "b")
				mkdirs(t, path)
				return path
			},
		},
		{
			name:   "сама папка установщика стала ссылкой на пустой каталог",
			layout: func(t *testing.T, root string) { mkdirs(t, filepath.Join(root, "group")) },
			swap:   "group",
			victim: func(_ *testing.T, outside string) string { return outside },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			outside := t.TempDir()
			before := shellSnapshotOf(t, root)
			tc.layout(t, root)
			after := shellSnapshotOf(t, root)
			victim := tc.victim(t, outside)
			swapped := filepath.Join(root, tc.swap)
			if err := os.RemoveAll(swapped); err != nil {
				t.Fatalf("remove %s: %v", swapped, err)
			}
			plantLink(t, swapped, outside)

			removed, err := removeShellShortcuts(context.Background(), before, after, "", true)
			if !exists(victim) {
				t.Fatalf("удалено %s за подменённым каталогом: removed = %v, err = %v", victim, removed, err)
			}
			if !refusedEntry(err) || len(removed) != 0 {
				t.Fatalf("removed = %v, err = %v, want a refusal", removed, err)
			}
			if _, statErr := os.Lstat(swapped); statErr != nil {
				t.Fatalf("ссылка %s удалена: %v", swapped, statErr)
			}
		})
	}
}

func TestCleanShellShortcutsRefusesLinkEntry(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "inner.url"), siteURL)
	before := shellSnapshotOf(t, root)
	link := filepath.Join(root, "ti.url")
	plantLink(t, link, outside)
	site := filepath.Join(root, "site.url")
	writeFile(t, site, siteURL)

	removed, err := cleanShellShortcuts(context.Background(), before, "", false)
	if !errors.Is(err, errShellReparsePoint) {
		t.Fatalf("err = %v, want errShellReparsePoint", err)
	}
	if _, statErr := os.Lstat(link); statErr != nil {
		t.Fatalf("ссылка удалена: %v", statErr)
	}
	if !exists(filepath.Join(outside, "inner.url")) {
		t.Fatal("удалён файл за ссылкой")
	}
	if len(removed) != 1 || exists(site) {
		t.Fatalf("removed = %v, обычный ярлык сайта должен уйти несмотря на отказ по ссылке", removed)
	}
}

func TestOpenShellEntry(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	file := filepath.Join(root, "dir", "ti.url")
	mkdirs(t, filepath.Dir(file))
	writeFile(t, file, siteURL)
	plantLink(t, filepath.Join(root, "link"), outside)
	writeFile(t, filepath.Join(outside, "x.url"), siteURL)

	snap := shellSnapshotOf(t, root)
	cases := []struct {
		name string
		snap shellSnapshot
		path string
		dir  bool
		want error
	}{
		{name: "файл", snap: snap, path: file},
		{name: "каталог", snap: snap, path: filepath.Dir(file), dir: true},
		{name: "нет файла", snap: snap, path: filepath.Join(root, "dir", "нет.url"), want: fs.ErrNotExist},
		{name: "файл вместо каталога", snap: snap, path: file, dir: true, want: errShellKindChanged},
		{name: "каталог вместо файла", snap: snap, path: filepath.Dir(file), want: errShellKindChanged},
		{name: "вне корней", snap: snap, path: filepath.Join(outside, "x.url"), want: errShellEscaped},
		{name: "сам корень", snap: snap, path: root, dir: true, want: errShellEscaped},
		{name: "корней нет", snap: shellSnapshot{}, path: file, want: errShellEscaped},
		{name: "корень не проверен снимком", snap: shellSnapshot{roots: []string{root}}, path: file, want: errShellRootUnknown},
		{name: "пустой путь", snap: snap, path: "", want: errShellEscaped},
		{name: "сама ссылка", snap: snap, path: filepath.Join(root, "link"), dir: true, want: errShellReparsePoint},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entry, err := openShellEntry(tc.snap, tc.path, tc.dir)
			if tc.want != nil {
				if !errors.Is(err, tc.want) {
					t.Fatalf("openShellEntry(%q) error = %v, want %v", tc.path, err, tc.want)
				}
				return
			}
			if err != nil {
				t.Fatalf("openShellEntry(%q) error = %v", tc.path, err)
			}
			if err := entry.close(); err != nil {
				t.Fatalf("close: %v", err)
			}
		})
	}
	t.Run("через ссылку в середине пути", func(t *testing.T) {
		_, err := openShellEntry(snap, filepath.Join(root, "link", "x.url"), false)
		if !refusedEntry(err) {
			t.Fatalf("err = %v, want a refusal", err)
		}
	})
}

func TestShellEntryReadsAndRemovesThroughOpenEntry(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "ti.url")
	writeFile(t, file, siteURL)
	dir := filepath.Join(root, "group")
	mkdirs(t, filepath.Join(dir, "inner"))
	snap := shellSnapshotOf(t, root)

	entry, err := openShellEntry(snap, file, false)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	data, err := entry.read(shortcutScanLimit)
	if err != nil || string(data) != siteURL {
		t.Fatalf("read = (%q, %v)", data, err)
	}
	if err := entry.remove(); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := entry.close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if exists(file) {
		t.Fatal("файл не удалён")
	}

	group, err := openShellEntry(snap, dir, true)
	if err != nil {
		t.Fatalf("open dir: %v", err)
	}
	if err := group.remove(); !errors.Is(err, errShellDirNotEmpty) {
		t.Fatalf("remove непустого каталога = %v, want errShellDirNotEmpty", err)
	}
	if err := group.close(); err != nil {
		t.Fatalf("close dir: %v", err)
	}
	if !exists(filepath.Join(dir, "inner")) {
		t.Fatal("содержимое непустого каталога пропало")
	}
}

// Корень сверяется с тем, каким его застал снимок: корень, подменённый ссылкой
// после снимка, не уводит удаление за свои пределы.
func TestRemoveShellShortcutsAfterRootSwap(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Desktop")
	mkdirs(t, root)
	outside := t.TempDir()
	before := shellSnapshotOf(t, root)
	writeFile(t, filepath.Join(root, "ti.url"), siteURL)
	after := shellSnapshotOf(t, root)
	victim := filepath.Join(outside, "ti.url")
	writeFile(t, victim, siteURL)
	if err := os.RemoveAll(root); err != nil {
		t.Fatalf("remove %s: %v", root, err)
	}
	plantLink(t, root, outside)

	removed, err := removeShellShortcuts(context.Background(), before, after, "", false)
	if !exists(victim) {
		t.Fatalf("удалён файл за подменённым корнем: removed = %v, err = %v", removed, err)
	}
	if !refusedEntry(err) || len(removed) != 0 {
		t.Fatalf("removed = %v, err = %v, want a refusal", removed, err)
	}
}

func TestShellEntryRemovesReadOnlyShortcut(t *testing.T) {
	root := t.TempDir()
	before := shellSnapshotOf(t, root)
	site := filepath.Join(root, "ti.url")
	writeFile(t, site, siteURL)
	if err := os.Chmod(site, 0o400); err != nil {
		t.Fatalf("chmod %s: %v", site, err)
	}

	removed, err := cleanShellShortcuts(context.Background(), before, "", false)
	if err != nil || len(removed) != 1 || exists(site) {
		t.Fatalf("removed = %v, err = %v, site exists = %v", removed, err, exists(site))
	}
}
