package install

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func writeTool(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("tool"), 0o755); err != nil {
		t.Fatal(err)
	}
}

const (
	unrarBanner713     = "UNRAR 7.13 x64 freeware      Copyright (c) 1993-2025 Alexander Roshal\n\nUsage:     unrar <command>"
	sevenZipBanner2501 = "\n7-Zip 25.01 (x64) : Copyright (c) 1999-2025 Igor Pavlov : 2025-08-03\n"
)

func useBanners(t *testing.T, banner func(path string) (string, error)) {
	t.Helper()
	prev := toolBanner
	toolBanner = func(_ context.Context, path string) (string, error) { return banner(path) }
	t.Cleanup(func() { toolBanner = prev })
}

func useBanner(t *testing.T, text string, err error) {
	t.Helper()
	useBanners(t, func(string) (string, error) { return text, err })
}

func currentBanners(path string) (string, error) {
	if strings.HasPrefix(strings.ToLower(filepath.Base(path)), "unrar") {
		return unrarBanner713, nil
	}
	return sevenZipBanner2501, nil
}

func TestRegularFiles(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.bin")
	second := filepath.Join(dir, "second.bin")
	writeTool(t, first)
	writeTool(t, second)
	absent := filepath.Join(dir, "absent.bin")
	cases := []struct {
		name       string
		candidates []string
		want       []string
		wantNotes  []string
	}{
		{name: "no candidates"},
		{name: "empty string is skipped", candidates: []string{"", first}, want: []string{first}, wantNotes: []string{"путь не определён"}},
		{name: "relative path is refused even though the file exists next to the test", candidates: []string{"archive_tool.go"}, wantNotes: []string{"archive_tool.go: путь не абсолютный"}},
		{name: "relative path is skipped for the next candidate", candidates: []string{"archive_tool.go", first}, want: []string{first}, wantNotes: []string{"archive_tool.go: путь не абсолютный"}},
		{name: "missing file is skipped", candidates: []string{absent, first}, want: []string{first}, wantNotes: []string{absent + ": нет файла"}},
		{name: "directory is refused", candidates: []string{dir}, wantNotes: []string{dir + ": не обычный файл"}},
		{name: "directory is skipped for the next candidate", candidates: []string{dir, second}, want: []string{second}, wantNotes: []string{dir + ": не обычный файл"}},
		{name: "path that cannot be stat'ed is skipped", candidates: []string{filepath.Join(dir, "bad\x00name"), first}, want: []string{first}, wantNotes: []string{"ошибка stat"}},
		{name: "every existing candidate is returned in order", candidates: []string{first, second}, want: []string{first, second}},
		{name: "a file named twice is returned once", candidates: []string{first, first}, want: []string{first}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, notes := regularFiles(tc.candidates)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("regularFiles(%q) = %q, want %q", tc.candidates, got, tc.want)
			}
			if len(notes) != len(tc.wantNotes) {
				t.Fatalf("notes = %q, want %d entries containing %q", notes, len(tc.wantNotes), tc.wantNotes)
			}
			for i, want := range tc.wantNotes {
				if !strings.Contains(notes[i], want) {
					t.Fatalf("notes[%d] = %q, want it to contain %q", i, notes[i], want)
				}
			}
		})
	}
}

func TestLookPathAbs(t *testing.T) {
	name := "typhon-fake-archive-tool"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	t.Run("tool on PATH is returned as an absolute path", func(t *testing.T) {
		dir := t.TempDir()
		writeTool(t, filepath.Join(dir, name))
		t.Setenv("PATH", dir)
		got := lookPathAbs(name)
		if !filepath.IsAbs(got) || !strings.EqualFold(got, filepath.Join(dir, name)) {
			t.Fatalf("lookPathAbs(%q) = %q, want %q", name, got, filepath.Join(dir, name))
		}
	})
	t.Run("tool that is not installed", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		if got := lookPathAbs(name); got != "" {
			t.Fatalf("lookPathAbs(%q) = %q, want nothing", name, got)
		}
	})
	t.Run("tool in the current directory is not run", func(t *testing.T) {
		dir := t.TempDir()
		writeTool(t, filepath.Join(dir, name))
		t.Chdir(dir)
		t.Setenv("PATH", t.TempDir())
		if got := lookPathAbs(name); got != "" {
			t.Fatalf("lookPathAbs(%q) = %q, want nothing: a lookup relative to the launcher's directory must not run", name, got)
		}
	})
	t.Run("relative PATH entry gives a relative result that is refused", func(t *testing.T) {
		dir := t.TempDir()
		writeTool(t, filepath.Join(dir, "relbin", name))
		t.Chdir(dir)
		t.Setenv("PATH", "relbin")
		if got := lookPathAbs(name); got != "" {
			t.Fatalf("lookPathAbs(%q) = %q, want nothing", name, got)
		}
	})
	t.Run("dot in PATH gives a relative result that is refused", func(t *testing.T) {
		dir := t.TempDir()
		writeTool(t, filepath.Join(dir, name))
		t.Chdir(dir)
		t.Setenv("PATH", ".")
		if got := lookPathAbs(name); got != "" {
			t.Fatalf("lookPathAbs(%q) = %q, want nothing", name, got)
		}
	})
}
