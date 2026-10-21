package achievements

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

type layout struct {
	public, appData, programData, install string
}

func newLayout(t *testing.T) layout {
	t.Helper()
	root := t.TempDir()
	return layout{
		public:      filepath.Join(root, "public"),
		appData:     filepath.Join(root, "appdata"),
		programData: filepath.Join(root, "programdata"),
		install:     filepath.Join(root, "game"),
	}
}

func (l layout) scanner(profile bool) scanner {
	env := map[string]string{"PUBLIC": l.public, "APPDATA": l.appData, "PROGRAMDATA": l.programData}
	return scanner{getenv: func(k string) string { return env[k] }, profile: profile, ls: os.ReadDir}
}

func TestScanReadsEveryRootAndMergesThem(t *testing.T) {
	l := newLayout(t)
	writeFile(t, filepath.Join(l.public, "Documents", "Steam", "Any", "620", "achievements.ini"), "[PUB]\nachieved=1\nunlocktime=1700000300\n[SHARED]\nachieved=1\nunlocktime=1700000900\n")
	writeFile(t, filepath.Join(l.appData, "Some Vendor", "620", "achievements.json"), `{"APP":{"earned":true,"earned_time":1700000400},"SHARED":{"earned":true,"earned_time":1700000100}}`)
	writeFile(t, filepath.Join(l.appData, "Goldberg SteamEmu Saves", "620", "achievements.json"), `{"GB":{"earned":true,"earned_time":1700000500}}`)
	writeFile(t, filepath.Join(l.programData, "Steam", "Other", "620", "ACHIEVEMENTS.INI"), "[PD]\nachieved=1\n")
	writeFile(t, filepath.Join(l.install, "steam_settings", "achievements.json"), `[{"name":"DEFINITION"}]`)
	writeFile(t, filepath.Join(l.install, "620", "achievements.ini"), "[INST]\nachieved=1\nunlocktime=1700000600\n")
	writeFile(t, filepath.Join(l.install, "620", "notes.json"), `{"IGNORED":{"earned":true}}`)
	writeFile(t, filepath.Join(l.appData, "Some Vendor", "730", "achievements.json"), `{"OTHER_GAME":{"earned":true}}`)
	writeFile(t, filepath.Join(l.appData, "Some Vendor", "620", "deep", "achievements.json"), `{"TOO_DEEP":{"earned":true}}`)

	res, err := l.scanner(true).scan(context.Background(), GameRef{InstallDir: l.install, AppID: "620"})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	sameUnlocks(t, res.Unlocked, map[string]time.Time{
		"PUB": at(1700000300), "SHARED": at(1700000100), "APP": at(1700000400),
		"GB": at(1700000500), "PD": {}, "INST": at(1700000600),
	})
	if !res.Found || res.Partial || len(res.Failures) != 0 {
		t.Fatalf("found %v partial %v failures %v", res.Found, res.Partial, res.Failures)
	}
}

func TestScanWithoutProfileRootsReadsTheInstallDirOnly(t *testing.T) {
	l := newLayout(t)
	writeFile(t, filepath.Join(l.appData, "Vendor", "620", "achievements.json"), `{"PROFILE":{"earned":true}}`)
	writeFile(t, filepath.Join(l.install, "steam_settings", "achievements.ini"), "[INST]\nachieved=1\n")

	res, err := l.scanner(false).scan(context.Background(), GameRef{InstallDir: l.install, AppID: "620"})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	sameUnlocks(t, res.Unlocked, map[string]time.Time{"INST": {}})
}

func TestScanSkipsRootsThatAreNotConfigured(t *testing.T) {
	l := newLayout(t)
	writeFile(t, filepath.Join(l.install, "620", "achievements.ini"), "[INST]\nachieved=1\n")
	empty := scanner{getenv: func(string) string { return "" }, profile: true, ls: os.ReadDir}

	t.Run("empty environment", func(t *testing.T) {
		res, err := empty.scan(context.Background(), GameRef{InstallDir: l.install, AppID: "620"})
		if err != nil {
			t.Fatalf("scan: %v", err)
		}
		sameUnlocks(t, res.Unlocked, map[string]time.Time{"INST": {}})
		if res.Partial {
			t.Fatal("a root that is not configured is not a partial result")
		}
	})
	for _, dir := range []string{"", "relative/dir", "   "} {
		t.Run("install dir "+dir, func(t *testing.T) {
			res, err := empty.scan(context.Background(), GameRef{InstallDir: dir, AppID: "620"})
			if err != nil {
				t.Fatalf("scan: %v", err)
			}
			if res.Found || len(res.Unlocked) != 0 {
				t.Fatalf("got %+v from an install dir that cannot be trusted", res)
			}
		})
	}
	t.Run("missing roots", func(t *testing.T) {
		res, err := l.scanner(true).scan(context.Background(), GameRef{InstallDir: filepath.Join(l.install, "gone"), AppID: "620"})
		if err != nil || res.Found || res.Partial {
			t.Fatalf("res %+v err %v", res, err)
		}
	})
}

func TestScanRejectsBadAppIDs(t *testing.T) {
	l := newLayout(t)
	for _, appID := range []string{"", "62 0", "../620", "620/..", "1234567890123"} {
		if _, err := l.scanner(true).scan(context.Background(), GameRef{InstallDir: l.install, AppID: appID}); !errors.Is(err, ErrNoSteamID) {
			t.Fatalf("appid %q: err = %v", appID, err)
		}
	}
}

func TestScanFlagsPartialResults(t *testing.T) {
	l := newLayout(t)
	writeFile(t, filepath.Join(l.appData, "Good", "620", "achievements.json"), `{"GOOD":{"earned":true}}`)
	writeFile(t, filepath.Join(l.appData, "Broken", "620", "achievements.json"), `{"BAD":`)
	writeFile(t, filepath.Join(l.install, "620", "achievements.ini"), "[A\n")

	res, err := l.scanner(true).scan(context.Background(), GameRef{InstallDir: l.install, AppID: "620"})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	sameUnlocks(t, res.Unlocked, map[string]time.Time{"GOOD": {}})
	if !res.Partial || len(res.Failures) != 2 {
		t.Fatalf("partial %v failures %v", res.Partial, res.Failures)
	}
	var sawJSON, sawINI bool
	for _, f := range res.Failures {
		sawJSON = sawJSON || errors.Is(f, ErrBadJSON)
		sawINI = sawINI || errors.Is(f, ErrBadINI)
	}
	if !sawJSON || !sawINI {
		t.Fatalf("failures %v must keep their kinds", res.Failures)
	}
}

func TestScanListingErrors(t *testing.T) {
	l := newLayout(t)
	writeFile(t, filepath.Join(l.appData, "Vendor", "620", "achievements.json"), `{"A":{"earned":true}}`)
	denied := l.appData
	broken := errors.New("disk fell off")
	cases := []struct {
		name        string
		listErr     error
		wantErr     error
		wantPartial bool
	}{
		{"permission denied marks the result partial", fs.ErrPermission, nil, true},
		{"missing directory is nothing to read", fs.ErrNotExist, nil, false},
		{"any other failure is raised", broken, broken, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sc := l.scanner(true)
			sc.ls = func(path string) ([]fs.DirEntry, error) {
				if path == denied {
					return nil, tc.listErr
				}
				return os.ReadDir(path)
			}
			res, err := sc.scan(context.Background(), GameRef{AppID: "620"})
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("scan: %v", err)
			}
			if res.Partial != tc.wantPartial || len(res.Unlocked) != 0 {
				t.Fatalf("partial %v unlocked %v", res.Partial, res.Unlocked)
			}
		})
	}
}

func TestScanStopsOnCancelledContext(t *testing.T) {
	l := newLayout(t)
	writeFile(t, filepath.Join(l.install, "620", "achievements.ini"), "[A]\nachieved=1\n")
	writeFile(t, filepath.Join(l.appData, "Vendor", "620", "achievements.ini"), "[B]\nachieved=1\n")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := l.scanner(true).scan(ctx, GameRef{InstallDir: l.install, AppID: "620"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
