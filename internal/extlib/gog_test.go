package extlib

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeRegistry struct {
	keys      map[string]map[string]string
	subKeyErr error
	valuesErr error
}

func (f *fakeRegistry) SubKeys(path string) ([]string, error) {
	if f.subKeyErr != nil {
		return nil, f.subKeyErr
	}
	if path != gogGamesKey {
		return nil, fs.ErrNotExist
	}
	if f.keys == nil {
		return nil, fs.ErrNotExist
	}
	var out []string
	for k := range f.keys {
		out = append(out, k)
	}
	return out, nil
}

func (f *fakeRegistry) Values(path string) (map[string]string, error) {
	if f.valuesErr != nil {
		return nil, f.valuesErr
	}
	v, ok := f.keys[path[len(gogGamesKey)+1:]]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return v, nil
}

func TestScanGOG(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "Witcher")
	other := filepath.Join(root, "Outside")
	for _, d := range []string{game, other} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(game, "witcher.exe"), `x`)
	writeFile(t, filepath.Join(other, "evil.exe"), `x`)
	reg := &fakeRegistry{keys: map[string]map[string]string{
		"1207658924": {"gamename": "The Witcher", "path": game, "exe": filepath.Join(game, "witcher.exe"), "gameid": "1207658924"},
		"2":          {"gamename": "By File Name", "path": game, "exefile": "witcher.exe"},
		"3":          {"gamename": "Escapes", "path": game, "exe": filepath.Join(other, "evil.exe")},
		"4":          {"gamename": "Dot Dot", "path": game, "exe": filepath.Join(game, "..", "Outside", "evil.exe")},
		"5":          {"gamename": "Missing Exe", "path": game, "exe": filepath.Join(game, "none.exe")},
		"6":          {"gamename": "Missing Dir", "path": filepath.Join(root, "none"), "exefile": "a.exe"},
		"7":          {"gamename": "No Exe", "path": game},
		"8":          {"gamename": "Relative", "path": "games/x", "exefile": "a.exe"},
		"bad id":     {"gamename": "Bad Id", "path": game, "exefile": "witcher.exe"},
	}}

	got := scanGOG(context.Background(), reg)
	if got.Err != nil {
		t.Fatalf("scan error: %v", got.Err)
	}
	byID := map[string]Candidate{}
	for _, g := range got.Games {
		byID[g.AppID] = g
	}
	if len(byID) != 2 {
		t.Fatalf("games = %+v, want 1207658924 and 2", got.Games)
	}
	w := byID["1207658924"]
	if w.Title != "The Witcher" || w.InstallDir != game || w.Executable != filepath.Join(game, "witcher.exe") || w.LaunchURI != "" {
		t.Fatalf("witcher = %+v", w)
	}
	if byID["2"].Executable != filepath.Join(game, "witcher.exe") {
		t.Fatalf("exefile fallback = %+v", byID["2"])
	}
	reasons := map[string]string{}
	for _, s := range got.Skipped {
		reasons[s.Title] = s.Reason
	}
	for _, title := range []string{"Escapes", "Dot Dot", "Missing Exe", "Missing Dir", "No Exe", "Relative", "Bad Id"} {
		if reasons[title] == "" {
			t.Errorf("%q is not in Skipped: %v", title, reasons)
		}
	}
}

func TestScanGOGDropsDLCAndNamesMissingFolders(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "Game")
	if err := os.MkdirAll(game, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(game, "game.exe"), `x`)
	gone := filepath.Join(root, "gone")
	reg := &fakeRegistry{keys: map[string]map[string]string{
		"100": {"gamename": "Base", "path": game, "exefile": "game.exe"},
		"101": {"gamename": "Base DLC", "path": game, "exefile": "game.exe", "dependson": "100"},
		"102": {"gamename": "Stale DLC", "path": gone, "exefile": "a.exe", "dependson": "100"},
		"200": {"gamename": "Deleted", "path": gone, "exefile": "a.exe"},
	}}

	got := scanGOG(context.Background(), reg)
	if got.Err != nil {
		t.Fatalf("scan error: %v", got.Err)
	}
	if len(got.Games) != 1 || got.Games[0].AppID != "100" {
		t.Fatalf("games = %+v, want only 100", got.Games)
	}
	if len(got.Skipped) != 1 || got.Skipped[0].Title != "Deleted" {
		t.Fatalf("skipped = %+v, want only Deleted (DLC must be dropped silently)", got.Skipped)
	}
	reason := got.Skipped[0].Reason
	if !strings.Contains(reason, "папка не найдена") || strings.Contains(strings.ToLower(reason), "getfileattributes") || strings.Contains(reason, "system cannot find") {
		t.Fatalf("reason = %q, want a short Russian reason without the system text", reason)
	}
}

func TestCheckDirKeepsCauseBehindShortReason(t *testing.T) {
	_, err := checkDir(filepath.Join(t.TempDir(), "missing"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("checkDir error %v lost fs.ErrNotExist", err)
	}
	file := filepath.Join(t.TempDir(), "f")
	writeFile(t, file, `x`)
	if _, err := checkDir(file); err == nil || strings.Contains(err.Error(), "system") {
		t.Fatalf("checkDir(file) = %v, want a not-a-directory error", err)
	}
}

func TestScanGOGErrors(t *testing.T) {
	boom := errors.New("access denied")
	cases := []struct {
		name         string
		reg          *fakeRegistry
		wantNotInst  bool
		wantAnyError bool
	}{
		{"no key", &fakeRegistry{}, true, true},
		{"read error", &fakeRegistry{subKeyErr: boom}, false, true},
		{"values error", &fakeRegistry{keys: map[string]map[string]string{"1": {}}, valuesErr: boom}, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := scanGOG(context.Background(), tc.reg)
			if (got.Err != nil) != tc.wantAnyError {
				t.Fatalf("error = %v", got.Err)
			}
			if errors.Is(got.Err, ErrNotInstalled) != tc.wantNotInst {
				t.Fatalf("error = %v, ErrNotInstalled = %v, want %v", got.Err, errors.Is(got.Err, ErrNotInstalled), tc.wantNotInst)
			}
		})
	}
}
