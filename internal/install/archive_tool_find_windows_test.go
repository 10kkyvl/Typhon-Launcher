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
		registry:     func(key string, _ ...string) []string { return f.registry[key] },
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
