package shortcut

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// TestFileName сравнивает базовое имя (без расширения) с shortcutExt,
// а не с литеральным ".lnk": расширение платформенное (.lnk на Windows и в
// devmock-сборках, .app на настоящем macOS), и тест обязан проходить на
// всех платформах, где собирается пакет.
func TestFileName(t *testing.T) {
	cases := []struct {
		name     string
		title    string
		wantBase string
		wantErr  bool
	}{
		{
			name:     "normal title",
			title:    "Half-Life 2",
			wantBase: "Half-Life 2",
		},
		{
			name:     "slashes and colon stripped",
			title:    `Half-Life 2: Episode/One\Two`,
			wantBase: "Half-Life 2 EpisodeOneTwo",
		},
		{
			name:    "only forbidden characters",
			title:   `<>:"/\|?*`,
			wantErr: true,
		},
		{
			name:    "empty string",
			title:   "",
			wantErr: true,
		},
		{
			name:    "whitespace only",
			title:   "   \t\n  ",
			wantErr: true,
		},
		{
			name:     "reserved device name",
			title:    "CON",
			wantBase: "CON_",
		},
		{
			name:     "reserved device name with extension, lowercase",
			title:    "com1.exe",
			wantBase: "com1.exe_",
		},
		{
			name:     "trailing dot trimmed",
			title:    "Game.",
			wantBase: "Game",
		},
		{
			name:     "control characters stripped",
			title:    "Game\x00Name\x1f",
			wantBase: "GameName",
		},
		{
			name:     "repeated spaces collapsed",
			title:    "Half   Life   2",
			wantBase: "Half Life 2",
		},
		{
			name:     "tabs are control characters, stripped not collapsed",
			title:    "Half\tLife\t2",
			wantBase: "HalfLife2",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := FileName(tc.title)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("FileName(%q) = %q, want error", tc.title, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("FileName(%q) unexpected error: %v", tc.title, err)
			}
			want := tc.wantBase + shortcutExt
			if got != want {
				t.Fatalf("FileName(%q) = %q, want %q", tc.title, got, want)
			}
		})
	}
}

func TestFileNameLongMultibyteTitle(t *testing.T) {
	title := strings.Repeat("日本語ゲーム😀", 30) + ".exe"
	if utf8.RuneCountInString(title) <= maxNameRunes {
		t.Fatalf("test title too short: %d runes", utf8.RuneCountInString(title))
	}

	got, err := FileName(title)
	if err != nil {
		t.Fatalf("FileName unexpected error: %v", err)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("FileName(%q) = %q is not valid UTF-8", title, got)
	}
	base := strings.TrimSuffix(got, shortcutExt)
	if n := utf8.RuneCountInString(base); n > maxNameRunes {
		t.Fatalf("base name has %d runes, want <= %d", n, maxNameRunes)
	}
	if !strings.HasSuffix(got, shortcutExt) {
		t.Fatalf("FileName(%q) = %q, want suffix %q", title, got, shortcutExt)
	}
}

func TestRemoveMissingFileIsNotError(t *testing.T) {
	dir := t.TempDir()
	if err := Remove(filepath.Join(dir, "does-not-exist"+shortcutExt)); err != nil {
		t.Fatalf("Remove of missing file: %v", err)
	}
}

func TestRemoveExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing"+shortcutExt)
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Remove(path); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("stat after Remove: %v, want fs.ErrNotExist", err)
	}
}

// TestRemoveDirectoryRemovesRecursively проверяет ветку Remove для
// каталога-ярлыка (бандл .app на macOS — это каталог, не файл): содержимое
// внутри не должно мешать удалению, как мешало бы os.Remove на непустом
// каталоге.
func TestRemoveDirectoryRemovesRecursively(t *testing.T) {
	dir := t.TempDir()
	bundle := filepath.Join(dir, "Game"+shortcutExt)
	nested := filepath.Join(bundle, "Contents", "MacOS")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "Game"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := Remove(bundle); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(bundle); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("stat after Remove: %v, want fs.ErrNotExist", err)
	}
}

// TestRemoveSymlinkDoesNotFollowTarget фиксирует, почему Remove использует
// Lstat, а не Stat: если ярлык на диске оказался символической ссылкой на
// каталог, Remove должен снять саму ссылку, а не рекурсивно стереть то, на
// что она указывает.
func TestRemoveSymlinkDoesNotFollowTarget(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim")
	if err := os.MkdirAll(victim, 0o755); err != nil {
		t.Fatal(err)
	}
	victimFile := filepath.Join(victim, "keep-me")
	if err := os.WriteFile(victimFile, []byte("important"), 0o644); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(dir, "Game"+shortcutExt)
	if err := os.Symlink(victim, link); err != nil {
		t.Fatal(err)
	}

	if err := Remove(link); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Lstat(link); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("symlink still present: %v", err)
	}
	if _, err := os.Stat(victimFile); err != nil {
		t.Fatalf("Remove followed the symlink and deleted its target: %v", err)
	}
}
