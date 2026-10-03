package install

import (
	"context"
	"encoding/binary"
	"errors"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func lnkBytes(t *testing.T, target string) []byte {
	t.Helper()
	info := make([]byte, 0x24)
	ansi := len(info)
	info = append(info, 0)
	wide := len(info)
	info = append(info, utf16Bytes(target)...)
	info = append(info, 0, 0)
	suffix := len(info)
	info = append(info, 0, 0)
	put32(t, info, 0, len(info))
	binary.LittleEndian.PutUint32(info[4:], 0x24)
	binary.LittleEndian.PutUint32(info[8:], 1)
	put32(t, info, 0x10, ansi)
	put32(t, info, 0x18, ansi)
	put32(t, info, 0x1c, wide)
	put32(t, info, 0x20, suffix)

	header := make([]byte, 0x4c)
	binary.LittleEndian.PutUint32(header[0:], 0x4c)
	binary.LittleEndian.PutUint32(header[0x14:], 0x2|0x80)
	return append(header, info...)
}

func put32(t *testing.T, b []byte, at, v int) {
	t.Helper()
	if v < 0 || v > math.MaxUint32 {
		t.Fatalf("значение %d не помещается в uint32", v)
		return
	}
	binary.LittleEndian.PutUint32(b[at:], uint32(v))
}

func writeShortcut(t *testing.T, path, target string) {
	t.Helper()
	if err := os.WriteFile(path, lnkBytes(t, target), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

const siteURL = "[InternetShortcut]\r\nURL=https://ti-url.com/igruha\r\nIconIndex=135\r\n"

func TestLnkTarget(t *testing.T) {
	info := make([]byte, 0x1c)
	base := len(info)
	info = append(info, `C:\TI\TI.URL`...)
	info = append(info, 0)
	suffix := len(info)
	info = append(info, 0)
	put32(t, info, 0, len(info))
	binary.LittleEndian.PutUint32(info[4:], 0x1c)
	binary.LittleEndian.PutUint32(info[8:], 1)
	put32(t, info, 0x10, base)
	put32(t, info, 0x18, suffix)
	ansiOnly := make([]byte, 0x4c)
	binary.LittleEndian.PutUint32(ansiOnly[0:], 0x4c)
	binary.LittleEndian.PutUint32(ansiOnly[0x14:], 0x2)
	ansiOnly = append(ansiOnly, info...)

	relative := make([]byte, 0x4c)
	binary.LittleEndian.PutUint32(relative[0:], 0x4c)
	binary.LittleEndian.PutUint32(relative[0x14:], 0x8|0x80)
	const name = `..\TI\TI.URL`
	relative = append(relative, byte(len(name)), 0)
	relative = append(relative, utf16Bytes(name)...)

	const tiURL = `C:\Program Files (x86)\TI\TI.URL`
	plain := lnkBytes(t, tiURL)
	withIDList := append([]byte(nil), plain[:0x4c]...)
	binary.LittleEndian.PutUint32(withIDList[0x14:], 0x1|0x2|0x80)
	withIDList = append(withIDList, 4, 0, 0x14, 0x00, 0x1f, 0x50)
	withIDList = append(withIDList, plain[0x4c:]...)
	mutate := func(src []byte, at int, value uint32) []byte {
		out := append([]byte(nil), src...)
		binary.LittleEndian.PutUint32(out[at:], value)
		return out
	}
	unterminated := append([]byte(nil), ansiOnly...)
	for i := 0x4c + base; i < len(unterminated); i++ {
		if unterminated[i] == 0 {
			unterminated[i] = 'x'
		}
	}

	cases := []struct {
		name    string
		data    []byte
		want    string
		wantErr bool
	}{
		{name: "широкий путь", data: lnkBytes(t, `C:\Program Files (x86)\TI\TI.URL`), want: `C:\Program Files (x86)\TI\TI.URL`},
		{name: "кириллица", data: lnkBytes(t, `G:\Игры\Тропико\game.exe`), want: `G:\Игры\Тропико\game.exe`},
		{name: "только ANSI", data: ansiOnly, want: `C:\TI\TI.URL`},
		{name: "относительный путь", data: relative, want: name},
		{name: "с IDList", data: withIDList, want: tiURL},
		{name: "LinkInfo длиннее файла", data: mutate(plain, 0x4c, 0xffff), wantErr: true},
		{name: "LinkInfo короче заголовка", data: mutate(plain, 0x4c, 0x10), wantErr: true},
		{name: "смещение строки за границей", data: mutate(plain, 0x4c+0x1c, 0xffff), wantErr: true},
		{name: "ANSI без нуля", data: unterminated, wantErr: true},
		{name: "IDList за границей", data: mutate(withIDList, 0x4c, 0xffff), wantErr: true},
		{name: "чужой формат", data: []byte("L\x00\x00\x00game.exe"), wantErr: true},
		{name: "обрезан", data: lnkBytes(t, `C:\Games\game.exe`)[:0x4c+0x10], wantErr: true},
		{name: "пусто", data: nil, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := lnkTarget(tc.data)
			if (err != nil) != tc.wantErr {
				t.Fatalf("lnkTarget error = %v, wantErr %v", err, tc.wantErr)
			}
			if err == nil && got != tc.want {
				t.Fatalf("lnkTarget = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSiteShortcut(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name    string
		file    string
		lnk     string
		content string
		want    bool
		wantErr bool
	}{
		{name: "url на сайт", file: "Торрент Игруха.url", content: siteURL, want: true},
		{name: "url на http", file: "site.URL", content: "[InternetShortcut]\nurl=http://example.com\n", want: true},
		{name: "url на файл игры", file: "game.url", content: "[InternetShortcut]\r\nURL=file:///G:/Games/Tropico/game.exe\r\n"},
		{name: "url на steam", file: "steam.url", content: "[InternetShortcut]\r\nURL=steam://rungameid/1\r\n"},
		{name: "lnk на .url", file: "Тoрpент-Игрyxа.lnk", lnk: `C:\Program Files (x86)\TI\TI.URL`, want: true},
		{name: "lnk на игру", file: "Tropico.lnk", lnk: `G:\Games\Tropico\Tropico.exe`},
		{name: "текст с адресом", file: "note.txt", content: siteURL},
		{name: "битый lnk", file: "broken.lnk", content: "L\x00\x00\x00", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.file)
			if tc.lnk != "" {
				writeShortcut(t, path, tc.lnk)
			} else {
				writeFile(t, path, tc.content)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			got, err := siteShortcut(path, data)
			if (err != nil) != tc.wantErr {
				t.Fatalf("siteShortcut error = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("siteShortcut = %v, want %v", got, tc.want)
			}
		})
	}
	if _, err := openShellEntry(shellSnapshotOf(t, dir), filepath.Join(dir, "нет.lnk"), false); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("отсутствующий ярлык: err = %v, want fs.ErrNotExist", err)
	}
}

func TestCleanShellShortcutsSite(t *testing.T) {
	for _, game := range []bool{false, true} {
		t.Run(map[bool]string{false: "ярлыки игры оставляем", true: "ярлыки игры убираем"}[game], func(t *testing.T) {
			public := t.TempDir()
			programs := t.TempDir()
			dest := filepath.Join(t.TempDir(), "Tropico 3")

			userSite := filepath.Join(public, "Мой сайт.url")
			writeFile(t, userSite, siteURL)
			rewrittenSite := filepath.Join(public, "Торрент  Игруха.lnk")
			writeShortcut(t, rewrittenSite, `C:\Program Files (x86)\TI\Torrent-Igruha.Org.URL`)
			oldGame := filepath.Join(public, "Старая игра.lnk")
			writeShortcut(t, oldGame, `G:\Games\Old\old.exe`)
			bookmarks := filepath.Join(public, "Закладки")
			if err := os.MkdirAll(bookmarks, 0o755); err != nil {
				t.Fatalf("mkdir bookmarks: %v", err)
			}
			old := time.Now().Add(-time.Hour)
			for _, path := range []string{userSite, rewrittenSite, oldGame} {
				if err := os.Chtimes(path, old, old); err != nil {
					t.Fatalf("chtimes: %v", err)
				}
			}

			ctx := context.Background()
			before, err := takeShellSnapshot(ctx, []string{public, programs})
			if err != nil {
				t.Fatalf("takeShellSnapshot error = %v", err)
			}

			newSite := filepath.Join(public, "Тoрpент-Игрyxа.lnk")
			writeShortcut(t, newSite, `C:\Program Files (x86)\TI\TI.URL`)
			writeShortcut(t, rewrittenSite, `C:\Program Files (x86)\TI\Torrent-Igruha.Org.URL`)
			writeShortcut(t, oldGame, `G:\Games\Old\old.exe`)
			gameLink := filepath.Join(public, "Tropico 3.lnk")
			writeShortcut(t, gameLink, filepath.Join(dest, "Tropico3.exe"))
			group := filepath.Join(programs, "Tropico 3")
			if err := os.MkdirAll(group, 0o755); err != nil {
				t.Fatalf("mkdir group: %v", err)
			}
			groupSite := filepath.Join(group, "Наш сайт.url")
			writeFile(t, groupSite, siteURL)
			bookmark := filepath.Join(bookmarks, "Новость.url")
			writeFile(t, bookmark, siteURL)
			userFolder := filepath.Join(public, "Новая папка")
			if err := os.MkdirAll(userFolder, 0o755); err != nil {
				t.Fatalf("mkdir user folder: %v", err)
			}

			removed, err := cleanShellShortcuts(ctx, before, dest, game)
			if err != nil {
				t.Fatalf("cleanShellShortcuts error = %v", err)
			}
			gone := []string{newSite, rewrittenSite, groupSite, group}
			kept := []string{userSite, oldGame, bookmark}
			if game {
				gone = append(gone, gameLink, userFolder)
			} else {
				kept = append(kept, gameLink, userFolder)
			}
			if len(removed) != len(gone) {
				t.Fatalf("removed = %v, want %d записей", removed, len(gone))
			}
			for _, path := range gone {
				if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
					t.Fatalf("%s остался: %v", path, err)
				}
			}
			for _, path := range kept {
				if _, err := os.Stat(path); err != nil {
					t.Fatalf("%s удалён: %v", path, err)
				}
			}
		})
	}
}

func TestCleanShellShortcutsSiteWithoutDestination(t *testing.T) {
	public := t.TempDir()
	before, err := takeShellSnapshot(context.Background(), []string{public})
	if err != nil {
		t.Fatalf("takeShellSnapshot error = %v", err)
	}
	site := filepath.Join(public, "Тoрpент-Игрyxа.lnk")
	writeShortcut(t, site, `C:\Program Files (x86)\TI\TI.URL`)

	removed, err := cleanShellShortcuts(context.Background(), before, "", true)
	if err != nil {
		t.Fatalf("cleanShellShortcuts error = %v", err)
	}
	if len(removed) != 1 {
		t.Fatalf("каталог игры не найден, но ярлык сайта всё равно убирается: removed = %v", removed)
	}
}

func TestCleanShellShortcutsBrokenLinkKeepsGoing(t *testing.T) {
	public := t.TempDir()
	before, err := takeShellSnapshot(context.Background(), []string{public})
	if err != nil {
		t.Fatalf("takeShellSnapshot error = %v", err)
	}
	broken := filepath.Join(public, "broken.lnk")
	writeFile(t, broken, "L\x00\x00\x00")
	site := filepath.Join(public, "site.url")
	writeFile(t, site, siteURL)

	removed, err := cleanShellShortcuts(context.Background(), before, "", false)
	if !errors.Is(err, errBadShellLink) {
		t.Fatalf("битый ярлык должен вернуть errBadShellLink, got %v", err)
	}
	if len(removed) != 1 || removed[0] != strings.ToLower(site) {
		t.Fatalf("removed = %v, want только %s", removed, site)
	}
	if _, err := os.Stat(broken); err != nil {
		t.Fatalf("непонятный ярлык удалён: %v", err)
	}
}

func TestReferencesPath(t *testing.T) {
	target := `C:\Games\GTA SA`
	cases := []struct {
		name string
		data []byte
		want bool
	}{
		{name: "ansi", data: []byte(`X` + target + `\gta_sa.exe`), want: true},
		{name: "utf16", data: append([]byte{0x4c, 0}, utf16Bytes(target+`\gta_sa.exe`)...), want: true},
		{name: "url с прямыми слешами", data: []byte("[InternetShortcut]\r\nURL=file:///C:/Games/GTA SA/gta_sa.exe\r\n"), want: true},
		{name: "другой регистр", data: []byte(strings.ToUpper(target)), want: true},
		{name: "чужой путь", data: []byte(`C:\Games\Other\game.exe`), want: false},
		{name: "пусто", data: nil, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := referencesPath(tc.data, target); got != tc.want {
				t.Fatalf("referencesPath = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestReferencesPathEmptyTarget(t *testing.T) {
	if referencesPath([]byte("anything"), "") {
		t.Fatal("пустая цель не должна совпадать ни с чем")
	}
}

func TestCleanShellShortcuts(t *testing.T) {
	desktop := t.TempDir()
	programs := t.TempDir()
	dest := filepath.Join(t.TempDir(), "GTA SA")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}

	mine := filepath.Join(desktop, "Моя папка.lnk")
	writeShortcut(t, mine, filepath.Join(dest, "gta_sa.exe"))
	oldGroup := filepath.Join(programs, "Старая группа")
	if err := os.MkdirAll(oldGroup, 0o755); err != nil {
		t.Fatalf("mkdir group: %v", err)
	}

	ctx := context.Background()
	before, err := takeShellSnapshot(ctx, []string{desktop, programs})
	if err != nil {
		t.Fatalf("takeShellSnapshot error = %v", err)
	}

	game := filepath.Join(desktop, "GTA SA.lnk")
	writeShortcut(t, game, filepath.Join(dest, "gta_sa.exe"))
	foreign := filepath.Join(desktop, "Браузер.lnk")
	writeShortcut(t, foreign, `C:\Program Files\Browser\browser.exe`)
	notShortcut := filepath.Join(desktop, "заметка.txt")
	if err := os.WriteFile(notShortcut, []byte(dest), 0o600); err != nil {
		t.Fatalf("write note: %v", err)
	}
	group := filepath.Join(programs, "GTA SA")
	if err := os.MkdirAll(group, 0o755); err != nil {
		t.Fatalf("mkdir new group: %v", err)
	}
	writeShortcut(t, filepath.Join(group, "Играть.lnk"), filepath.Join(dest, "gta_sa.exe"))
	writeShortcut(t, filepath.Join(oldGroup, "Играть.lnk"), filepath.Join(dest, "gta_sa.exe"))

	removed, err := cleanShellShortcuts(ctx, before, dest, true)
	if err != nil {
		t.Fatalf("cleanShellShortcuts error = %v", err)
	}
	if len(removed) != 4 {
		t.Fatalf("removed = %v, want 4 записи", removed)
	}
	for _, path := range []string{game, group, notShortcut} {
		if _, err := os.Stat(path); (err == nil) != (path == notShortcut) {
			t.Fatalf("stat %s = %v", path, err)
		}
	}
	if _, err := os.Stat(mine); err != nil {
		t.Fatalf("ярлык, существовавший до установки, удалён: %v", err)
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatalf("чужой ярлык удалён: %v", err)
	}
	if _, err := os.Stat(oldGroup); err != nil {
		t.Fatalf("существовавшая группа удалена: %v", err)
	}
}

func TestCleanShellShortcutsWithoutBaseline(t *testing.T) {
	desktop := t.TempDir()
	dest := t.TempDir()
	writeShortcut(t, filepath.Join(desktop, "GTA SA.lnk"), filepath.Join(dest, "gta_sa.exe"))

	removed, err := cleanShellShortcuts(context.Background(), shellSnapshot{}, dest, true)
	if err != nil {
		t.Fatalf("cleanShellShortcuts error = %v", err)
	}
	if len(removed) != 0 {
		t.Fatalf("без снимка до установки удалять нечего, removed = %v", removed)
	}
	if _, err := os.Stat(filepath.Join(desktop, "GTA SA.lnk")); err != nil {
		t.Fatalf("ярлык удалён без снимка: %v", err)
	}
}

func TestCleanShellShortcutsCancelled(t *testing.T) {
	desktop := t.TempDir()
	dest := t.TempDir()
	before, err := takeShellSnapshot(context.Background(), []string{desktop})
	if err != nil {
		t.Fatalf("takeShellSnapshot error = %v", err)
	}
	writeShortcut(t, filepath.Join(desktop, "GTA SA.lnk"), filepath.Join(dest, "gta_sa.exe"))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := cleanShellShortcuts(ctx, before, dest, true); err == nil {
		t.Fatal("отменённый ctx должен вернуть ошибку")
	}
	if _, err := os.Stat(filepath.Join(desktop, "GTA SA.lnk")); err != nil {
		t.Fatalf("ярлык удалён после отмены: %v", err)
	}
}

func TestTakeShellSnapshotMissingRoot(t *testing.T) {
	snap, err := takeShellSnapshot(context.Background(), []string{filepath.Join(t.TempDir(), "нет-такой-папки")})
	if err != nil {
		t.Fatalf("отсутствующий каталог не ошибка: %v", err)
	}
	if !snap.taken || len(snap.entries) != 0 {
		t.Fatalf("snapshot = %+v, want пустой но взятый", snap)
	}
}
