package extlib

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func vdfPath(p string) string { return strings.ReplaceAll(p, `\`, `\\`) }

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestParseLibraryFolders(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    []string
		wantErr bool
	}{
		{"new format", `"libraryfolders"
{
	"0" { "path" "C:\\Steam" "apps" { "1" "2" } }
	"1" { "path" "D:\\Lib" }
}`, []string{`C:\Steam`, `D:\Lib`}, false},
		{"old format", `"LibraryFolders" { "TimeNextStatsReport" "123" "1" "E:\\Old" }`, []string{`E:\Old`}, false},
		{"comments and bom", "\xEF\xBB\xBF// c\n\"libraryfolders\" { \"0\" { \"path\" \"F:\\\\X\" } }", []string{`F:\X`}, false},
		{"empty", ``, nil, true},
		{"unclosed block", `"libraryfolders" { "0" { "path" "C:\\x" }`, nil, true},
		{"unterminated string", `"libraryfolders { }`, nil, true},
		{"no libraryfolders", `"other" { "a" "b" }`, nil, true},
		{"stray brace", `}`, nil, true},
		{"key without value", `"libraryfolders" { "0" }`, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseLibraryFolders([]byte(tc.in))
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if len(got) != len(tc.want) {
				t.Fatalf("paths = %v, want %v", got, tc.want)
			}
			have := map[string]bool{}
			for _, p := range got {
				have[p] = true
			}
			for _, w := range tc.want {
				if !have[w] {
					t.Fatalf("paths = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestParseVDFDepthLimit(t *testing.T) {
	deep := strings.Repeat(`"a" {`, maxVDFDepth+5) + strings.Repeat(`}`, maxVDFDepth+5)
	if _, err := parseVDF([]byte(deep)); !errors.Is(err, errVDF) {
		t.Fatalf("error = %v, want errVDF", err)
	}
}

func TestParseAppManifest(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    steamApp
		wantErr bool
	}{
		{"installed", `"AppState" { "appid" "730" "name" "Counter-Strike 2" "installdir" "cs2" "StateFlags" "4" }`,
			steamApp{AppID: "730", Name: "Counter-Strike 2", InstallDir: "cs2", Flags: 4}, false},
		{"escaped quote", `"AppState" { "appid" "1" "name" "A \"B\"" "installdir" "ab" "StateFlags" "4" }`,
			steamApp{AppID: "1", Name: `A "B"`, InstallDir: "ab", Flags: 4}, false},
		{"empty", ``, steamApp{}, true},
		{"no AppState", `"x" { "appid" "1" }`, steamApp{}, true},
		{"bad appid", `"AppState" { "appid" "12ab" "StateFlags" "4" }`, steamApp{}, true},
		{"bad flags", `"AppState" { "appid" "1" "StateFlags" "four" }`, steamApp{}, true},
		{"garbage", "\x00\x01\x02", steamApp{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseAppManifest([]byte(tc.in))
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.want {
				t.Fatalf("app = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func acf(appid, name, dir, flags string) string {
	return `"AppState" { "appid" "` + appid + `" "name" "` + name + `" "installdir" "` + dir + `" "StateFlags" "` + flags + `" }`
}

func TestScanSteam(t *testing.T) {
	root := t.TempDir()
	second := t.TempDir()
	apps := filepath.Join(root, "steamapps")
	for _, d := range []string{"Good", "Half", "Tool"} {
		if err := os.MkdirAll(filepath.Join(apps, "common", d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(second, "steamapps", "common", "Other"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(apps, "libraryfolders.vdf"),
		`"libraryfolders" { "0" { "path" "`+vdfPath(root)+`" } "1" { "path" "`+vdfPath(second)+`" } "2" { "path" "`+vdfPath(filepath.Join(root, "gone"))+`" } }`)
	writeFile(t, filepath.Join(apps, "appmanifest_1.acf"), acf("1", "Good\tGame", "Good", "4"))
	writeFile(t, filepath.Join(apps, "appmanifest_2.acf"), acf("2", "Half", "Half", "1026"))
	writeFile(t, filepath.Join(apps, "appmanifest_3.acf"), acf("3", "Escape", `../..`, "4"))
	writeFile(t, filepath.Join(apps, "appmanifest_4.acf"), acf("4", "Absolute", root, "4"))
	writeFile(t, filepath.Join(apps, "appmanifest_5.acf"), acf("5", "No Folder", "Missing", "4"))
	writeFile(t, filepath.Join(apps, "appmanifest_6.acf"), ``)
	writeFile(t, filepath.Join(apps, "appmanifest_7.acf"), `"AppState" {`)
	writeFile(t, filepath.Join(apps, "appmanifest_228980.acf"), acf("228980", "Steamworks Common Redistributables", "Tool", "4"))
	writeFile(t, filepath.Join(apps, "notes.txt"), `x`)
	writeFile(t, filepath.Join(second, "steamapps", "appmanifest_8.acf"), acf("8", "Other", "Other", "6"))
	writeFile(t, filepath.Join(second, "steamapps", "appmanifest_1.acf"), acf("1", "Good Duplicate", "Other", "4"))

	got := scanSteam(context.Background(), root)
	if got.Err != nil {
		t.Fatalf("scan error: %v", got.Err)
	}
	if len(got.Games) != 2 {
		t.Fatalf("games = %+v, want Good and Other", got.Games)
	}
	byID := map[string]Candidate{}
	for _, g := range got.Games {
		byID[g.AppID] = g
	}
	good := byID["1"]
	if good.Title != "Good Game" || good.LaunchURI != "steam://rungameid/1" || good.Executable != "" ||
		good.InstallDir != filepath.Join(apps, "common", "Good") {
		t.Fatalf("good game = %+v", good)
	}
	if byID["8"].InstallDir != filepath.Join(second, "steamapps", "common", "Other") {
		t.Fatalf("flags 6 includes the fully installed bit and must be imported: %+v", byID["8"])
	}
	reasons := map[string]string{}
	for _, s := range got.Skipped {
		reasons[s.Title] = s.Reason
	}
	for _, title := range []string{"Half", "Escape", "Absolute", "No Folder", "appmanifest_6.acf", "appmanifest_7.acf", filepath.Join(root, "gone")} {
		if reasons[title] == "" {
			t.Errorf("%q is not in Skipped: %v", title, reasons)
		}
	}
	if _, tool := reasons["Steamworks Common Redistributables"]; tool {
		t.Error("the service component must be dropped silently")
	}
}

func TestScanSteamReadErrorIsProviderError(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "steamapps", "libraryfolders.vdf"), `"libraryfolders" { "0" `)
	got := scanSteam(context.Background(), root)
	if got.Err == nil || errors.Is(got.Err, ErrNotInstalled) {
		t.Fatalf("error = %v, want a parse failure that is not ErrNotInstalled", got.Err)
	}
}

func TestScanSteamCancelled(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "steamapps", "appmanifest_1.acf"), acf("1", "A", "A", "4"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := scanSteam(ctx, root); !errors.Is(got.Err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", got.Err)
	}
}

func TestScanSteamOversizedManifest(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "steamapps", "appmanifest_1.acf"), strings.Repeat("x", maxFileBytes+1))
	got := scanSteam(context.Background(), root)
	if got.Err != nil || len(got.Games) != 0 || len(got.Skipped) != 1 || !strings.Contains(got.Skipped[0].Reason, errTooLarge.Error()) {
		t.Fatalf("result = %+v", got)
	}
}

func epicItemJSON(dir, name, ns, item, app, exe string, incomplete bool, main string) string {
	incompleteText := "false"
	if incomplete {
		incompleteText = "true"
	}
	return `{"FormatVersion":0,"DisplayName":"` + name + `","InstallLocation":"` + vdfPath(dir) +
		`","AppName":"` + app + `","CatalogNamespace":"` + ns + `","CatalogItemId":"` + item +
		`","LaunchExecutable":"` + exe + `","MainGameAppName":"` + main + `","bIsIncompleteInstall":` + incompleteText + `}`
}

func TestScanEpic(t *testing.T) {
	manifests := filepath.Join(t.TempDir(), "Manifests")
	games := t.TempDir()
	good := filepath.Join(games, "Good")
	if err := os.MkdirAll(good, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(good, "bin", "good.exe"), `x`)
	writeFile(t, filepath.Join(manifests, "a.item"), epicItemJSON(good, "Good\\nEpic", "ns1", "item1", "App1", "bin/good.exe", false, ""))
	writeFile(t, filepath.Join(manifests, "b.item"), epicItemJSON(good, "Unfinished", "ns2", "item2", "App2", "", true, ""))
	writeFile(t, filepath.Join(manifests, "c.item"), epicItemJSON(good, "Dlc", "ns3", "item3", "AppDlc", "", false, "App1"))
	writeFile(t, filepath.Join(manifests, "d.item"), epicItemJSON(filepath.Join(games, "missing"), "Gone", "ns4", "item4", "App4", "", false, ""))
	writeFile(t, filepath.Join(manifests, "e.item"), epicItemJSON(good, "Evil", "ns5%3A", "item5", "App5", "", false, ""))
	writeFile(t, filepath.Join(manifests, "f.item"), ``)
	writeFile(t, filepath.Join(manifests, "g.item"), `{"DisplayName": `)
	writeFile(t, filepath.Join(manifests, "h.item"), epicItemJSON("../../x", "Relative", "ns6", "item6", "App6", "", false, ""))
	writeFile(t, filepath.Join(manifests, "i.item"), epicItemJSON(good, "Escaping Exe", "ns7", "item7", "App7", "../../evil.exe", false, ""))
	writeFile(t, filepath.Join(manifests, "readme.txt"), `x`)

	got := scanEpic(context.Background(), manifests)
	if got.Err != nil {
		t.Fatalf("scan error: %v", got.Err)
	}
	byID := map[string]Candidate{}
	for _, g := range got.Games {
		byID[g.AppID] = g
	}
	if len(byID) != 2 {
		t.Fatalf("games = %+v, want App1 and App7", got.Games)
	}
	a := byID["App1"]
	if a.Title != "Good Epic" || a.InstallDir != good || a.Executable != filepath.Join(good, "bin", "good.exe") ||
		a.LaunchURI != "com.epicgames.launcher://apps/ns1%3Aitem1%3AApp1?action=launch&silent=true" {
		t.Fatalf("App1 = %+v", a)
	}
	if byID["App7"].Executable != "" {
		t.Fatalf("an executable that escapes the install dir must be dropped: %+v", byID["App7"])
	}
	reasons := map[string]string{}
	for _, s := range got.Skipped {
		reasons[s.Title] = s.Reason
	}
	for _, title := range []string{"Unfinished", "Gone", "Evil", "f.item", "g.item", "Relative"} {
		if reasons[title] == "" {
			t.Errorf("%q is not in Skipped: %v", title, reasons)
		}
	}
	if _, dlc := reasons["Dlc"]; dlc {
		t.Error("add-on content must be dropped silently")
	}
}

func TestScanEpicMissingFolderIsNotInstalled(t *testing.T) {
	got := scanEpic(context.Background(), filepath.Join(t.TempDir(), "none"))
	if !errors.Is(got.Err, ErrNotInstalled) {
		t.Fatalf("error = %v, want ErrNotInstalled", got.Err)
	}
}

func TestCleanTitle(t *testing.T) {
	cases := map[string]string{
		"  Plain  ":              "Plain",
		"A\x00B\x1b[31mC":        "A B [31mC",
		"Tab\tNew\nLine":         "Tab New Line",
		"\x01\x02":               "",
		strings.Repeat("я", 300): strings.Repeat("я", maxTitleRunes),
	}
	for in, want := range cases {
		if got := cleanTitle(in); got != want {
			t.Errorf("cleanTitle(%q) = %q, want %q", in, got, want)
		}
	}
}
