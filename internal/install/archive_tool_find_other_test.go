//go:build !windows

package install

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestFindArchiveToolsSources(t *testing.T) {
	cases := []struct {
		name  string
		files []string
		want  []string
	}{
		{name: "nothing installed"},
		{name: "both tools in the search directory", files: []string{"unrar", "7zz"}, want: []string{"UnRAR=unrar", "7-Zip=7zz"}},
		{name: "7zz is preferred over 7z", files: []string{"7z", "7zz"}, want: []string{"7-Zip=7zz"}},
		{name: "plain 7z is used when 7zz is absent", files: []string{"7z"}, want: []string{"7-Zip=7z"}},
		{name: "only unrar", files: []string{"unrar"}, want: []string{"UnRAR=unrar"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, name := range tc.files {
				writeTool(t, filepath.Join(dir, name))
			}
			t.Setenv("PATH", t.TempDir())
			prev := toolSearchDirs
			toolSearchDirs = []string{dir}
			t.Cleanup(func() { toolSearchDirs = prev })
			var got []string
			useBanners(t, currentBanners)
			for _, tool := range findArchiveTools(context.Background()).tools {
				got = append(got, tool.name+"="+filepath.Base(tool.path))
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("findArchiveTools = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFindArchiveToolsIgnoresRelativePath(t *testing.T) {
	dir := t.TempDir()
	writeTool(t, filepath.Join(dir, "unrar"))
	t.Chdir(dir)
	t.Setenv("PATH", ".")
	prev := toolSearchDirs
	toolSearchDirs = nil
	t.Cleanup(func() { toolSearchDirs = prev })
	useBanners(t, currentBanners)
	if got := findArchiveTools(context.Background()); len(got.tools) != 0 {
		t.Fatalf("findArchiveTools = %v, want nothing: a tool reached through a relative PATH entry must not run", got)
	}
}

func TestFindArchiveToolsAppliesTheUnixFloors(t *testing.T) {
	dir := t.TempDir()
	writeTool(t, filepath.Join(dir, "unrar"))
	writeTool(t, filepath.Join(dir, "7zz"))
	t.Setenv("PATH", t.TempDir())
	prev := toolSearchDirs
	toolSearchDirs = []string{dir}
	t.Cleanup(func() { toolSearchDirs = prev })
	cases := []struct {
		name       string
		unrar      string
		unrarErr   error
		sevenZip   string
		wantUnrar  bool
		wantSwitch bool
		wantSeven  bool
	}{
		{name: "both current", unrar: unrarBanner713, sevenZip: sevenZipBanner2501, wantUnrar: true, wantSwitch: true, wantSeven: true},
		{name: "UnRAR 6 is used without -ol-: the traversal fixes concern Windows only", unrar: "UNRAR 6.24 x64 freeware", sevenZip: sevenZipBanner2501, wantUnrar: true, wantSeven: true},
		{name: "UnRAR with an unknown version is used without -ol-", unrar: "usage", sevenZip: sevenZipBanner2501, wantUnrar: true, wantSeven: true},
		{name: "UnRAR that cannot be probed is used without -ol-", unrarErr: errors.New("cannot start"), sevenZip: sevenZipBanner2501, wantUnrar: true, wantSeven: true},
		{name: "old 7-Zip is refused", unrar: unrarBanner713, sevenZip: "7-Zip (z) 24.08 (arm64) : Copyright (c) 1999-2024 Igor Pavlov : 2024-08-11", wantUnrar: true, wantSwitch: true},
		{name: "p7zip is refused", unrar: unrarBanner713, sevenZip: "p7zip Version 16.02 (locale=en_US.UTF-8,Utf16=on,HugeFiles=on,64 bits,8 CPUs)", wantUnrar: true, wantSwitch: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			useBanners(t, func(path string) (string, error) {
				if strings.HasSuffix(path, "unrar") {
					return tc.unrar, tc.unrarErr
				}
				return tc.sevenZip, nil
			})
			set := findArchiveTools(context.Background())
			byName := map[string]archiveTool{}
			for _, tool := range set.tools {
				byName[tool.name] = tool
			}
			unrar, hasUnrar := byName["UnRAR"]
			if _, hasSeven := byName["7-Zip"]; hasUnrar != tc.wantUnrar || hasSeven != tc.wantSeven {
				t.Fatalf("tools = %v, want UnRAR %v and 7-Zip %v", set.tools, tc.wantUnrar, tc.wantSeven)
			}
			if hasUnrar && slices.Contains(unrar.args("a.rar", "/out"), "-ol-") != tc.wantSwitch {
				t.Fatalf("UnRAR args = %q, want -ol- present = %v", unrar.args("a.rar", "/out"), tc.wantSwitch)
			}
			if wantRefused := btoi(!tc.wantSeven); len(set.refused) != wantRefused {
				t.Fatalf("refused = %v, want %d", set.refused, wantRefused)
			}
		})
	}
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

func TestReadToolBannerReturnsEmptyOutputOfASilentTool(t *testing.T) {
	silent, err := exec.LookPath("true")
	if err != nil {
		t.Fatal(err)
	}
	out, err := readToolBanner(context.Background(), silent)
	if err != nil || out != "" {
		t.Fatalf("readToolBanner = %q, %v, want empty output and no error", out, err)
	}
	if _, err := readToolBanner(context.Background(), filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("readToolBanner succeeded for a tool that does not exist")
	}
}
