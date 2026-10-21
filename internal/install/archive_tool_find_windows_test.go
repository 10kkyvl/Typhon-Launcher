//go:build windows

package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type fakeSources struct {
	registry     map[string][]string
	programFiles []string
	path         map[string]string
}

func (f fakeSources) sources() toolSources {
	return toolSources{
		registry: func(key string, names ...string) []string {
			var out []string
			for _, name := range names {
				out = append(out, f.registry[key+"|"+name]...)
			}
			if len(out) > 0 {
				return out
			}
			return f.registry[key]
		},
		programFiles: func() []string { return f.programFiles },
		lookPath:     func(name string) string { return f.path[name] },
	}
}

func toolNames(tools []archiveTool) []string {
	var out []string
	for _, tool := range tools {
		out = append(out, tool.name+"="+tool.path)
	}
	return out
}

func TestFindToolsSources(t *testing.T) {
	root := t.TempDir()
	winrarReg := filepath.Join(root, "reg", "WinRAR", "UnRAR.exe")
	sevenReg := filepath.Join(root, "reg", "7-Zip", "7z.exe")
	winrarPF := filepath.Join(root, "pf", "WinRAR", "UnRAR.exe")
	sevenPF := filepath.Join(root, "pf", "7-Zip", "7z.exe")
	winrarPath := filepath.Join(root, "path", "UnRAR.exe")
	sevenPath := filepath.Join(root, "path", "7z.exe")
	sevenOnly := filepath.Join(root, "pf7", "7-Zip", "7z.exe")
	for _, p := range []string{winrarReg, sevenReg, winrarPF, sevenPF, winrarPath, sevenPath, sevenOnly} {
		writeTool(t, p)
	}
	missing := filepath.Join(root, "absent", "WinRAR", "WinRAR.exe")
	asDir := filepath.Join(root, "dirs", "UnRAR.exe")
	if err := os.MkdirAll(asDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		src  fakeSources
		want []string
	}{
		{name: "nothing installed"},
		{
			name: "registry beats Program Files and PATH",
			src: fakeSources{
				registry:     map[string][]string{`SOFTWARE\WinRAR`: {filepath.Join(root, "reg", "WinRAR", "WinRAR.exe")}, `SOFTWARE\7-Zip`: {filepath.Join(root, "reg", "7-Zip")}},
				programFiles: []string{filepath.Join(root, "pf")},
				path:         map[string]string{"UnRAR.exe": winrarPath, "7z.exe": sevenPath},
			},
			want: []string{"UnRAR=" + winrarReg, "7-Zip=" + sevenReg},
		},
		{
			name: "registry points at a missing install: Program Files is next",
			src: fakeSources{
				registry:     map[string][]string{`SOFTWARE\WinRAR`: {missing}, `SOFTWARE\7-Zip`: {filepath.Join(root, "absent", "7-Zip")}},
				programFiles: []string{filepath.Join(root, "pf")},
				path:         map[string]string{"UnRAR.exe": winrarPath, "7z.exe": sevenPath},
			},
			want: []string{"UnRAR=" + winrarPF, "7-Zip=" + sevenPF},
		},
		{
			name: "only PATH knows the tools",
			src:  fakeSources{path: map[string]string{"UnRAR.exe": winrarPath, "7z.exe": sevenPath}},
			want: []string{"UnRAR=" + winrarPath, "7-Zip=" + sevenPath},
		},
		{
			name: "only 7-Zip is installed",
			src:  fakeSources{programFiles: []string{filepath.Join(root, "pf7")}},
			want: []string{"7-Zip=" + sevenOnly},
		},
		{
			name: "a directory named like the tool is not a tool",
			src: fakeSources{
				registry: map[string][]string{`SOFTWARE\WinRAR`: {filepath.Join(root, "dirs", "WinRAR.exe")}},
			},
		},
		{
			name: "empty lookup results are skipped",
			src:  fakeSources{path: map[string]string{"UnRAR.exe": ""}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			useBanners(t, currentBanners)
			if got := toolNames(findTools(context.Background(), tc.src.sources()).tools); !slices.Equal(got, tc.want) {
				t.Fatalf("findTools = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFindToolsAppliesTheWindowsFloors(t *testing.T) {
	root := t.TempDir()
	writeTool(t, filepath.Join(root, "WinRAR", "UnRAR.exe"))
	writeTool(t, filepath.Join(root, "7-Zip", "7z.exe"))
	src := fakeSources{programFiles: []string{root}}
	cases := []struct {
		name        string
		unrar       string
		sevenZip    string
		wantTools   []string
		wantRefused int
	}{
		{name: "both current", unrar: unrarBanner713, sevenZip: sevenZipBanner2501, wantTools: []string{"UnRAR", "7-Zip"}},
		{name: "UnRAR before the traversal fixes", unrar: "UNRAR 7.12 x64 freeware", sevenZip: sevenZipBanner2501, wantTools: []string{"7-Zip"}, wantRefused: 1},
		{name: "old 7-Zip", unrar: unrarBanner713, sevenZip: "\n7-Zip 24.09 (x64) : Copyright (c) 1999-2024 Igor Pavlov : 2024-11-29\n", wantTools: []string{"UnRAR"}, wantRefused: 1},
		{name: "nothing usable", unrar: "UNRAR 6.24 x64 freeware", sevenZip: "7-Zip 9.20  Copyright (c) 1999-2010 Igor Pavlov", wantRefused: 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			useBanners(t, func(path string) (string, error) {
				if strings.HasSuffix(strings.ToLower(path), "unrar.exe") {
					return tc.unrar, nil
				}
				return tc.sevenZip, nil
			})
			set := findTools(context.Background(), src.sources())
			var names []string
			for _, tool := range set.tools {
				names = append(names, tool.name)
			}
			if !slices.Equal(names, tc.wantTools) || len(set.refused) != tc.wantRefused {
				t.Fatalf("tools = %v, refused = %v, want %v and %d refusals", names, set.refused, tc.wantTools, tc.wantRefused)
			}
			for _, err := range set.refused {
				if !errors.Is(err, errToolVersion) {
					t.Fatalf("refusal %v is not errToolVersion", err)
				}
			}
		})
	}
}

func TestReadToolBannerRunsTheToolWithoutArguments(t *testing.T) {
	findstr, err := systemExecutable("findstr.exe")
	if err != nil {
		t.Fatal(err)
	}
	out, err := readToolBanner(context.Background(), findstr)
	if err != nil || !strings.Contains(strings.ToUpper(out), "FINDSTR") {
		t.Fatalf("readToolBanner = %q, %v: want the usage text even though findstr exits with an error", out, err)
	}
	if _, err := readToolBanner(context.Background(), filepath.Join(t.TempDir(), "absent.exe")); err == nil {
		t.Fatal("readToolBanner succeeded for a tool that does not exist")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := readToolBanner(ctx, findstr); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestFindToolsSeesAToolInstalledBetweenCalls(t *testing.T) {
	root := t.TempDir()
	src := fakeSources{programFiles: []string{root}}
	useBanners(t, currentBanners)
	if got := toolNames(findTools(context.Background(), src.sources()).tools); len(got) != 0 {
		t.Fatalf("findTools before the install = %q, want nothing", got)
	}
	writeTool(t, filepath.Join(root, "WinRAR", "UnRAR.exe"))
	want := []string{"UnRAR=" + filepath.Join(root, "WinRAR", "UnRAR.exe")}
	if got := toolNames(findTools(context.Background(), src.sources()).tools); !slices.Equal(got, want) {
		t.Fatalf("findTools after the install = %q, want %q: the search must not remember the empty result", got, want)
	}
}

func TestFindToolsReportsWhereItLooked(t *testing.T) {
	root := t.TempDir()
	regExe := filepath.Join(root, "reg", "WinRAR.exe")
	src := fakeSources{
		registry:     map[string][]string{`SOFTWARE\WinRAR`: {regExe}},
		programFiles: []string{filepath.Join(root, "pf")},
		path:         map[string]string{"UnRAR.exe": ""},
	}
	useBanners(t, currentBanners)
	set := findTools(context.Background(), src.sources())
	if len(set.tools) != 0 || len(set.refused) != 0 {
		t.Fatalf("tools = %v, refused = %v, want nothing", set.tools, set.refused)
	}
	for _, want := range []string{
		"UnRAR: " + filepath.Join(root, "reg", "UnRAR.exe") + ": нет файла",
		"UnRAR: " + filepath.Join(root, "pf", "WinRAR", "UnRAR.exe") + ": нет файла",
		"UnRAR: путь не определён",
		"7-Zip: " + filepath.Join(root, "pf", "7-Zip", "7z.exe") + ": нет файла",
	} {
		if !slices.Contains(set.searched, want) {
			t.Errorf("searched = %q, want it to contain %q", set.searched, want)
		}
	}
}

func TestFindToolsNamesTheVersionItRefused(t *testing.T) {
	root := t.TempDir()
	writeTool(t, filepath.Join(root, "WinRAR", "UnRAR.exe"))
	useBanner(t, "UNRAR 7.12 x64 freeware", nil)
	set := findTools(context.Background(), fakeSources{programFiles: []string{root}}.sources())
	if len(set.tools) != 0 || len(set.refused) != 1 {
		t.Fatalf("tools = %v, refused = %v, want one refusal", set.tools, set.refused)
	}
	if text := set.refused[0].Error(); !strings.Contains(text, "7.12") || !strings.Contains(text, "7.13") {
		t.Fatalf("refusal = %q, want it to name both the found version and the floor", text)
	}
}

func TestFindToolsReadsEveryRegistryRecord(t *testing.T) {
	root := t.TempDir()
	trailing := filepath.Join(root, "inst", "WinRAR") + string(filepath.Separator)
	icon := `"` + filepath.Join(root, "ico", "WinRAR.exe") + `",0`
	cases := []struct {
		name     string
		registry map[string][]string
		tool     string
	}{
		{
			name:     "App Paths without the WinRAR key",
			registry: map[string][]string{appPathsKey + "WinRAR.exe|": {filepath.Join(root, "ap", "WinRAR.exe")}},
			tool:     filepath.Join(root, "ap", "UnRAR.exe"),
		},
		{
			name:     "uninstall record with a trailing slash in InstallLocation",
			registry: map[string][]string{uninstallKey + "WinRAR archiver|InstallLocation": {trailing}},
			tool:     filepath.Join(root, "inst", "WinRAR", "UnRAR.exe"),
		},
		{
			name:     "uninstall record with a quoted DisplayIcon and an icon index",
			registry: map[string][]string{uninstallKey + "WinRAR archiver|DisplayIcon": {icon}},
			tool:     filepath.Join(root, "ico", "UnRAR.exe"),
		},
		{
			name:     "relative registry value is not a candidate",
			registry: map[string][]string{appPathsKey + "WinRAR.exe|": {"WinRAR.exe"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.tool != "" {
				writeTool(t, tc.tool)
			}
			useBanners(t, currentBanners)
			set := findTools(context.Background(), fakeSources{registry: tc.registry}.sources())
			var want []string
			if tc.tool != "" {
				want = []string{"UnRAR=" + tc.tool}
			}
			if got := toolNames(set.tools); !slices.Equal(got, want) {
				t.Fatalf("findTools = %q, want %q", got, want)
			}
		})
	}
	t.Run("7-Zip from App Paths and from the uninstall record", func(t *testing.T) {
		appExe := filepath.Join(root, "7ap", "7z.exe")
		instExe := filepath.Join(root, "7inst", "7z.exe")
		writeTool(t, appExe)
		writeTool(t, instExe)
		useBanners(t, currentBanners)
		src := fakeSources{registry: map[string][]string{
			appPathsKey + "7zFM.exe|":              {filepath.Join(root, "7ap", "7zFM.exe")},
			uninstallKey + "7-Zip|InstallLocation": {filepath.Join(root, "7inst") + string(filepath.Separator)},
		}}
		set := findTools(context.Background(), src.sources())
		if got, want := toolNames(set.tools), []string{"7-Zip=" + appExe}; !slices.Equal(got, want) {
			t.Fatalf("findTools = %q, want %q", got, want)
		}
		only := fakeSources{registry: map[string][]string{
			uninstallKey + "7-Zip|InstallLocation": {filepath.Join(root, "7inst") + string(filepath.Separator)},
		}}
		if got, want := toolNames(findTools(context.Background(), only.sources()).tools), []string{"7-Zip=" + instExe}; !slices.Equal(got, want) {
			t.Fatalf("findTools with only the uninstall record = %q, want %q", got, want)
		}
	})
}

func TestFindToolsPassesOverAnOutdatedCandidate(t *testing.T) {
	root := t.TempDir()
	oldExe := filepath.Join(root, "old", "UnRAR.exe")
	newExe := filepath.Join(root, "pf", "WinRAR", "UnRAR.exe")
	writeTool(t, oldExe)
	writeTool(t, newExe)
	banners := func(old, fresh string) func(string) (string, error) {
		return func(path string) (string, error) {
			if path == oldExe {
				return old, nil
			}
			return fresh, nil
		}
	}
	src := fakeSources{
		registry:     map[string][]string{`SOFTWARE\WinRAR`: {filepath.Join(root, "old", "WinRAR.exe")}},
		programFiles: []string{filepath.Join(root, "pf")},
	}
	t.Run("the newer one further down is found", func(t *testing.T) {
		useBanners(t, banners("UNRAR 7.12 x64 freeware", unrarBanner713))
		set := findTools(context.Background(), src.sources())
		if got, want := toolNames(set.tools), []string{"UnRAR=" + newExe}; !slices.Equal(got, want) {
			t.Fatalf("findTools = %q, want %q", got, want)
		}
		if len(set.refused) != 0 {
			t.Fatalf("refused = %v, want none: a later candidate was accepted", set.refused)
		}
		found := false
		for _, note := range set.searched {
			found = found || strings.Contains(note, oldExe) && strings.Contains(note, "7.12") && strings.Contains(note, "отклонён")
		}
		if !found {
			t.Fatalf("searched = %q, want a note that names %s and version 7.12", set.searched, oldExe)
		}
	})
	t.Run("both outdated is a refusal for each file", func(t *testing.T) {
		useBanners(t, banners("UNRAR 7.12 x64 freeware", "UNRAR 7.10 x64 freeware"))
		set := findTools(context.Background(), src.sources())
		if len(set.tools) != 0 || len(set.refused) != 2 {
			t.Fatalf("tools = %v, refused = %v, want two refusals", set.tools, set.refused)
		}
	})
	t.Run("one file named twice is probed once", func(t *testing.T) {
		useBanners(t, banners("UNRAR 7.12 x64 freeware", "UNRAR 7.12 x64 freeware"))
		same := fakeSources{
			registry:     map[string][]string{`SOFTWARE\WinRAR`: {filepath.Join(root, "pf", "WinRAR", "WinRAR.exe")}},
			programFiles: []string{filepath.Join(root, "pf")},
		}
		if set := findTools(context.Background(), same.sources()); len(set.refused) != 1 {
			t.Fatalf("refused = %v, want one: the same file reached through the registry and Program Files", set.refused)
		}
	})
}

func TestCleanRegistryPath(t *testing.T) {
	cases := []struct{ in, want string }{
		{`C:\Program Files\WinRAR\WinRAR.exe`, `C:\Program Files\WinRAR\WinRAR.exe`},
		{`"C:\Program Files\WinRAR\WinRAR.exe",0`, `C:\Program Files\WinRAR\WinRAR.exe`},
		{`C:\Program Files\WinRAR\WinRAR.exe,1`, `C:\Program Files\WinRAR\WinRAR.exe`},
		{`  C:\Tools, Inc\WinRAR  `, `C:\Tools, Inc\WinRAR`},
		{"", ""},
		{"   ", ""},
	}
	for _, tc := range cases {
		if got := cleanRegistryPath(tc.in); got != tc.want {
			t.Errorf("cleanRegistryPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
