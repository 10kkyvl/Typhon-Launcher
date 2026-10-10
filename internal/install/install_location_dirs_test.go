package install

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallDirsFromEntriesPrefersTheNamedEntry(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "GamersGoMakers")
	tool := filepath.Join(root, "SomeRedist")
	for _, dir := range []string{game, tool} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	added := map[string]uninstallEntry{
		"HKLM\\Redist": {Key: "HKLM\\Redist", DisplayName: "Some Redistributable", Command: uninstaller(tool), InstallLocation: tool},
		"HKLM\\Game":   {Key: "HKLM\\Game", DisplayName: "GamersGoMakers 1.1.7", Command: uninstaller(game), InstallLocation: game},
	}
	cases := []struct {
		name  string
		title string
		want  []string
	}{
		{"named entry wins", "GamersGoMakers", []string{game}},
		{"nothing matches the name, both stay", "Unrelated", []string{game, tool}},
		{"empty name keeps both", "", []string{game, tool}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := installDirsFromEntries(nil, added, tc.title, dirLimits{})
			if len(got) != len(tc.want) {
				t.Fatalf("dirs = %v, want %v", got, tc.want)
			}
			for _, want := range tc.want {
				found := false
				for _, dir := range got {
					found = found || samePath(dir, want)
				}
				if !found {
					t.Fatalf("dirs = %v, want %v among them", got, tc.want)
				}
			}
		})
	}
}

func TestInstallDirsFromEntriesRespectsLimits(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "Library")
	games := filepath.Join(library, "games")
	downloads := filepath.Join(library, "downloads")
	state := filepath.Join(root, "State")
	for _, dir := range []string{games, downloads, state, filepath.Join(downloads, "nested"), filepath.Join(games, "Inside"), filepath.Join(root, "Elsewhere")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	file := filepath.Join(root, "file.txt")
	mkFile(t, file, 4)
	limits := dirLimits{closed: []string{downloads, state}, roots: []string{games}}
	cases := []struct {
		name     string
		location string
		want     bool
	}{
		{"the downloads folder", downloads, false},
		{"inside the downloads folder", filepath.Join(downloads, "nested"), false},
		{"the launcher state folder", state, false},
		{"a parent of the downloads folder", library, false},
		{"the games root", games, false},
		{"a parent of the games root", root, false},
		{"a folder inside the games root", filepath.Join(games, "Inside"), true},
		{"a sibling of the library", filepath.Join(root, "Elsewhere"), true},
		{"a regular file", file, false},
		{"a quoted path", `"` + filepath.Join(root, "Elsewhere") + `"`, true},
		{"an empty value", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			added := map[string]uninstallEntry{"k": {Key: "k", DisplayName: "Game", Command: "x.exe", InstallLocation: tc.location}}
			got := installDirsFromEntries(nil, added, "Game", limits)
			if (len(got) == 1) != tc.want {
				t.Fatalf("dirs for %q = %v, want accepted=%v", tc.location, got, tc.want)
			}
		})
	}
}

func TestInstallDirFallsBackAcrossSources(t *testing.T) {
	root := t.TempDir()
	system := filepath.Join(root, "System32")
	game := filepath.Join(root, "Game")
	for _, dir := range []string{system, game} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	limits := dirLimits{closed: []string{system}}
	entry := uninstallEntry{
		Command: `"` + filepath.Join(system, "msiexec.exe") + `" /x {A}`,
		Icon:    `"` + filepath.Join(game, "Game.exe") + `",0`,
	}
	got, ok := entry.installDir(limits)
	if !ok || !samePath(got, game) {
		t.Fatalf("installDir = %q, %v, want %q from the icon after the command pointed at a closed directory", got, ok, game)
	}
	if got, ok := (uninstallEntry{Command: entry.Command}).installDir(limits); ok {
		t.Fatalf("installDir = %q for an uninstaller inside a closed directory, want none", got)
	}
}

func TestIconPath(t *testing.T) {
	cases := []struct {
		icon string
		want string
	}{
		{`"C:\Games\Game\Game.exe",0`, `C:\Games\Game\Game.exe`},
		{`C:\Games\Game\Game.exe,0`, `C:\Games\Game\Game.exe`},
		{`C:\Games\Game\Game.exe,-101`, `C:\Games\Game\Game.exe`},
		{`C:\Games\Game\Game.exe`, `C:\Games\Game\Game.exe`},
		{`C:\Games, Inc\Game.exe`, `C:\Games, Inc\Game.exe`},
		{`"C:\Games\Game\Game.exe`, ``},
		{``, ``},
	}
	for _, tc := range cases {
		if got := iconPath(tc.icon); got != tc.want {
			t.Errorf("iconPath(%q) = %q, want %q", tc.icon, got, tc.want)
		}
	}
}
