package settings

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

var portableNames = []string{
	"theme",
	"uiScale",
	"animationsEnabled",
	"minimizeToTray",
	"discordRichPresence",
	"seedAfterDownload",
	"uploadWhileDownloading",
	"installCleanupPolicy",
	"autoInstall",
	"sourceRefreshInterval",
	"verifyAfterInstall",
	"installSkipShortcuts",
	"installSkipExtras",
	"desktopShortcuts",
	"updateCheckAutomatically",
	"updateAutoDownload",
	"updateAutoInstall",
	"updateSaveBackup",
	"keepPreviousVersion",
	"allowTorrentReuse",
}

var localNames = []string{
	"lanSharing",
	"libraryPath",
	"downloadsPath",
	"gamesPath",
	"screenshotsPath",
	"launchOnStartup",
	"hardwareAcceleration",
	"maxActiveDownloads",
	"downloadRateLimit",
	"uploadRateLimit",
	"accountSync",
	"sourcesNoticeAccepted",
	"anonymousUsageStats",
	"anonymousDiagnostics",
	"telemetryConsentVersion",
}

func jsonNames(v any) []string {
	t := reflect.TypeOf(v)
	names := make([]string, 0, t.NumField())
	for i := range t.NumField() {
		tag := t.Field(i).Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")
		if name == "" || name == "-" {
			continue
		}
		names = append(names, name)
	}
	return names
}

func TestEverySettingIsClassified(t *testing.T) {
	classified := map[string]string{}
	for _, name := range portableNames {
		classified[name] = "portable"
	}
	for _, name := range localNames {
		if kind, ok := classified[name]; ok {
			t.Fatalf("%q is listed as both %s and local", name, kind)
		}
		classified[name] = "local"
	}

	seen := map[string]bool{}
	for _, name := range jsonNames(Settings{}) {
		seen[name] = true
		if _, ok := classified[name]; !ok {
			t.Errorf("настройка %q не отнесена ни к переносимым, ни к локальным: "+
				"решение обязательно, иначе поле уедет на сервер или потеряется", name)
		}
	}
	for name := range classified {
		if !seen[name] {
			t.Errorf("%q классифицирована, но в Settings такого поля нет", name)
		}
	}
}

func TestPortableCarriesOnlyClassifiedFields(t *testing.T) {
	got := jsonNames(Portable{})
	if !reflect.DeepEqual(got, portableNames) {
		t.Fatalf("поля Portable разошлись со списком переносимых\nполучено: %v\nожидалось: %v", got, portableNames)
	}
}

func TestPortableNeverCarriesLocalValues(t *testing.T) {
	s := Settings{
		LibraryPath:           `E:\TyphonLibrary`,
		GamesPath:             `E:\TyphonLibrary\Games`,
		DownloadsPath:         `E:\TyphonLibrary\Downloads`,
		ScreenshotsPath:       `E:\TyphonLibrary\Screenshots`,
		LaunchOnStartup:       true,
		HardwareAcceleration:  true,
		MaxActiveDownloads:    7,
		DownloadRateLimit:     1234567,
		UploadRateLimit:       7654321,
		SourcesNoticeAccepted: true,
		AnonymousUsageStats:   true,
		AnonymousDiagnostics:  true,
		Theme:                 "dark",
	}

	data, err := json.Marshal(PortableOf(s))
	if err != nil {
		t.Fatalf("marshal portable: %v", err)
	}
	payload := string(data)

	for _, forbidden := range []string{
		`E:\TyphonLibrary`, "TyphonLibrary", "1234567", "7654321", "7",
	} {
		if strings.Contains(payload, forbidden) {
			t.Errorf("в переносимых настройках оказалось локальное значение %q: %s", forbidden, payload)
		}
	}
	for _, name := range localNames {
		if strings.Contains(payload, name) {
			t.Errorf("в переносимых настройках оказалось локальное поле %q: %s", name, payload)
		}
	}
}

func TestApplyPortableKeepsLocalFields(t *testing.T) {
	local := Settings{
		LibraryPath:          `E:\TyphonLibrary`,
		GamesPath:            `E:\TyphonLibrary\Games`,
		LaunchOnStartup:      true,
		HardwareAcceleration: true,
		MaxActiveDownloads:   7,
		DownloadRateLimit:    1234567,
		AnonymousUsageStats:  true,
		Theme:                "dark",
		AutoInstall:          false,
	}
	remote := Settings{
		LibraryPath:          `D:\Other`,
		GamesPath:            `D:\Other\Games`,
		LaunchOnStartup:      false,
		HardwareAcceleration: false,
		MaxActiveDownloads:   1,
		DownloadRateLimit:    0,
		AnonymousUsageStats:  false,
		Theme:                "light",
		AutoInstall:          true,
	}

	got := ApplyPortable(local, PortableOf(remote))

	if got.LibraryPath != local.LibraryPath || got.GamesPath != local.GamesPath {
		t.Errorf("пути перезаписаны удалёнными: %q %q", got.LibraryPath, got.GamesPath)
	}
	if got.LaunchOnStartup != local.LaunchOnStartup || got.HardwareAcceleration != local.HardwareAcceleration {
		t.Error("машинные переключатели перезаписаны удалёнными")
	}
	if got.MaxActiveDownloads != local.MaxActiveDownloads || got.DownloadRateLimit != local.DownloadRateLimit {
		t.Error("лимиты канала перезаписаны удалёнными")
	}
	if got.AnonymousUsageStats != local.AnonymousUsageStats {
		t.Error("согласие на сбор статистики перенесено с другого устройства")
	}
	if got.Theme != "light" || !got.AutoInstall {
		t.Errorf("переносимые настройки не применились: theme=%q autoInstall=%v", got.Theme, got.AutoInstall)
	}
}

func TestApplyPortableIgnoresUnsetFields(t *testing.T) {
	local := Defaults()
	local.Theme = "light"
	local.AutoInstall = true

	got := ApplyPortable(local, Portable{})

	if got.Theme != "light" || !got.AutoInstall {
		t.Errorf("пустой набор переносимых настроек затёр локальные: theme=%q autoInstall=%v", got.Theme, got.AutoInstall)
	}
}
