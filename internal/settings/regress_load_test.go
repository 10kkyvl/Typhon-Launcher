package settings

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func readBytes(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestNewServiceAtRefusesAnUnreadableSettingsPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(path, "marker.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	svc, err := NewServiceAt(path)
	if err == nil {
		t.Fatalf("a path that cannot be read must not fall back to defaults, got %+v", svc.GetSettings())
	}
	if errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("an unreadable path was reported as missing: %v", err)
	}
	if got := readBytes(t, marker); got != "keep" {
		t.Fatalf("marker = %q", got)
	}
}

func TestNewServiceAtRefusesDamagedSettings(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"empty file", ""},
		{"whitespace only", " \n\t "},
		{"nul bytes after a torn write", "\x00\x00\x00\x00\x00\x00"},
		{"utf-8 byte order mark", "\xef\xbb\xbf{\"theme\":\"light\"}"},
		{"scalar root", `"dark"`},
		{"trailing garbage", `{"theme":"light"} tail`},
		{"two documents", `{"theme":"light"}{"theme":"dark"}`},
		{"scale as a string", `{"uiScale":"big"}`},
		{"downloads as a float string", `{"maxActiveDownloads":"3"}`},
		{"flag as a number", `{"minimizeToTray":1}`},
		{"list where a text belongs", `{"theme":["dark"]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeConfig(t, tc.raw)
			if svc, err := NewServiceAt(path); err == nil {
				t.Fatalf("damaged settings must not start the service, got %+v", svc.GetSettings())
			}
			if got := readBytes(t, path); got != tc.raw {
				t.Fatalf("file rewritten: %q", got)
			}
		})
	}
}

func TestLoadRefusesInvalidStoredValues(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want error
	}{
		{"accent that is not a hex colour", `{"accentColor":"red"}`, nil},
		{"accent without the hash", `{"accentColor":"123456"}`, nil},
		{"relative library path", `{"libraryPath":"games/library"}`, ErrLibraryPathRelative},
		{"proxy type nobody knows", `{"proxyType":"ftp"}`, ErrProxyTypeInvalid},
		{"proxy host with a space", `{"proxyHost":"bad host"}`, ErrProxyHostInvalid},
		{"proxy host with a scheme", `{"proxyHost":"http://proxy.example.com"}`, ErrProxyHostInvalid},
		{"proxy port above the range", `{"proxyPort":70000}`, ErrProxyPortInvalid},
		{"negative proxy port", `{"proxyPort":-1}`, ErrProxyPortInvalid},
		{"proxy mode without a host", `{"networkMode":"proxy","proxyPort":1080}`, ErrProxyHostRequired},
		{"proxy mode without a port", `{"networkMode":"proxy","proxyHost":"proxy.example.com"}`, ErrProxyPortInvalid},
		{"interface mode without an adapter", `{"networkMode":"interface"}`, ErrNetworkInterfaceRequired},
		{"adapter name with a control character", `{"networkInterface":"eth\u0001"}`, ErrNetworkInterfaceInvalid},
		{"proxy login with a colon", `{"proxyUsername":"user:name"}`, ErrProxyUsernameInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeConfig(t, tc.raw)
			svc, err := NewServiceAt(path)
			if err == nil {
				t.Fatalf("a stored value that would fail the save must stop the start, got %+v", svc.GetSettings())
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			if got := readBytes(t, path); got != tc.raw {
				t.Fatalf("file rewritten: %q", got)
			}
		})
	}
}

func TestLoadClampsOutOfRangeValuesFromDisk(t *testing.T) {
	defaults := Defaults()
	cases := []struct {
		name  string
		raw   string
		check func(Settings) bool
	}{
		{"ui scale too large", `{"uiScale":9}`, func(s Settings) bool { return s.UIScale == 1 }},
		{"ui scale too small", `{"uiScale":0.1}`, func(s Settings) bool { return s.UIScale == 1 }},
		{"downloads below one", `{"maxActiveDownloads":0}`, func(s Settings) bool { return s.MaxActiveDownloads == 1 }},
		{"downloads negative", `{"maxActiveDownloads":-4}`, func(s Settings) bool { return s.MaxActiveDownloads == 1 }},
		{"downloads above ten", `{"maxActiveDownloads":99}`, func(s Settings) bool { return s.MaxActiveDownloads == 10 }},
		{"negative download limit", `{"downloadRateLimit":-5}`, func(s Settings) bool { return s.DownloadRateLimit == 0 }},
		{"negative upload limit", `{"uploadRateLimit":-5}`, func(s Settings) bool { return s.UploadRateLimit == 0 }},
		{"backup limit below the minimum", `{"saveBackupLimit":0}`, func(s Settings) bool { return s.SaveBackupLimit == MinSaveBackupLimit }},
		{"backup limit above the maximum", `{"saveBackupLimit":9999}`, func(s Settings) bool { return s.SaveBackupLimit == MaxSaveBackupLimit }},
		{"unknown cleanup policy", `{"installCleanupPolicy":"shred"}`, func(s Settings) bool { return s.InstallCleanupPolicy == CleanupDelete }},
		{"unknown refresh interval", `{"sourceRefreshInterval":"5m"}`, func(s Settings) bool { return s.SourceRefreshInterval == RefreshSixHours }},
		{"unknown language", `{"language":"fr"}`, func(s Settings) bool { return s.Language == LanguageSystem }},
		{"unknown presence", `{"presenceStatus":"dnd"}`, func(s Settings) bool { return s.PresenceStatus == PresenceOnline }},
		{"unknown keep-previous mode", `{"keepPreviousVersion":"forever"}`, func(s Settings) bool { return s.KeepPreviousVersion == KeepPreviousFirstLaunch }},
		{"negative consent version", `{"telemetryConsentVersion":-3}`, func(s Settings) bool { return s.TelemetryConsentVersion == 0 && !s.DiagnosticsAllowed() }},
		{"lowercase accent is normalised", `{"accentColor":"#abcdef"}`, func(s Settings) bool { return s.AccentColor == "#ABCDEF" }},
		{"missing keys inherit the defaults", `{"uiScale":9}`, func(s Settings) bool {
			return s.MaxActiveDownloads == defaults.MaxActiveDownloads && s.Theme == defaults.Theme && s.MinimizeToTray == defaults.MinimizeToTray
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeConfig(t, tc.raw)
			svc, err := NewServiceAt(path)
			if err != nil {
				t.Fatalf("a value that can be repaired must not stop the start: %v", err)
			}
			if got := svc.GetSettings(); !tc.check(got) {
				t.Fatalf("settings after load = %+v", got)
			}
			if got := readBytes(t, path); got != tc.raw {
				t.Fatalf("loading rewrote the file: %q", got)
			}
		})
	}
}

func TestLoadToleratesUnknownFields(t *testing.T) {
	raw := `{"theme":"light","futureOption":{"nested":[1,2,3]},"anotherOne":"x","maxActiveDownloads":4}`
	path := writeConfig(t, raw)

	got := mustServiceAt(t, path).GetSettings()
	if got.Theme != "light" || got.MaxActiveDownloads != 4 {
		t.Fatalf("known fields were lost next to unknown ones: %+v", got)
	}
	if after := readBytes(t, path); after != raw {
		t.Fatalf("loading rewrote the file: %q", after)
	}
}

func TestMissingSettingsDirectoryStartsWithDefaultsAndSaveCreatesIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "deeper", "settings.json")
	svc := mustServiceAt(t, path)
	if svc.GetSettings() != Defaults() {
		t.Fatalf("got %+v, want defaults", svc.GetSettings())
	}

	next := svc.GetSettings()
	next.Theme = "light"
	if err := svc.SaveSettings(next); err != nil {
		t.Fatalf("save: %v", err)
	}
	if got := mustServiceAt(t, path).GetSettings().Theme; got != "light" {
		t.Fatalf("theme after reload = %q", got)
	}
}
