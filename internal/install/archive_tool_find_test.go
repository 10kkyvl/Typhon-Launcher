package install

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
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

func TestFirstRegularFile(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.bin")
	second := filepath.Join(dir, "second.bin")
	writeTool(t, first)
	writeTool(t, second)
	cases := []struct {
		name       string
		candidates []string
		want       string
	}{
		{name: "no candidates"},
		{name: "empty string is skipped", candidates: []string{"", first}, want: first},
		{name: "relative path is refused even though the file exists next to the test", candidates: []string{"archive_tool.go"}},
		{name: "relative path is skipped for the next candidate", candidates: []string{"archive_tool.go", first}, want: first},
		{name: "missing file is skipped", candidates: []string{filepath.Join(dir, "absent.bin"), first}, want: first},
		{name: "directory is refused", candidates: []string{dir}},
		{name: "directory is skipped for the next candidate", candidates: []string{dir, second}, want: second},
		{name: "path that cannot be stat'ed is skipped", candidates: []string{filepath.Join(dir, "bad\x00name"), first}, want: first},
		{name: "first existing candidate wins", candidates: []string{first, second}, want: first},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := firstRegularFile(tc.candidates); got != tc.want {
				t.Fatalf("firstRegularFile(%q) = %q, want %q", tc.candidates, got, tc.want)
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
