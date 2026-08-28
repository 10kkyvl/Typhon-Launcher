package settings

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func mustServiceAt(t testing.TB, path string) *Service {
	t.Helper()
	s, err := NewServiceAt(path)
	if err != nil {
		t.Fatalf("new settings service at %s: %v", path, err)
	}
	return s
}

func TestSaveAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := mustServiceAt(t, path)

	next := s.GetSettings()
	next.Theme = "dark"
	next.UIScale = 1.1
	next.LibraryPath = filepath.Join(t.TempDir(), LibraryFolderName)
	next = derivePaths(next)
	next.MinimizeToTray = false
	if err := s.SaveSettings(next); err != nil {
		t.Fatal(err)
	}

	reloaded := mustServiceAt(t, path).GetSettings()
	if reloaded != next {
		t.Fatalf("got %+v, want %+v", reloaded, next)
	}
}

func TestInvalidScaleFallsBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := mustServiceAt(t, path)
	next := s.GetSettings()
	next.UIScale = 3
	if err := s.SaveSettings(next); err != nil {
		t.Fatal(err)
	}
	if got := s.GetSettings().UIScale; got != 1 {
		t.Fatalf("ui scale = %v, want 1", got)
	}
}

func TestDownloadLimitsAreSanitized(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := mustServiceAt(t, path)
	next := s.GetSettings()
	next.MaxActiveDownloads = 42
	next.DownloadRateLimit = -1
	next.UploadRateLimit = -5
	if err := s.SaveSettings(next); err != nil {
		t.Fatal(err)
	}
	got := s.GetSettings()
	if got.MaxActiveDownloads != 10 || got.DownloadRateLimit != 0 || got.UploadRateLimit != 0 {
		t.Fatalf("got %+v", got)
	}

	next.MaxActiveDownloads = 0
	if err := s.SaveSettings(next); err != nil {
		t.Fatal(err)
	}
	if got := s.GetSettings().MaxActiveDownloads; got != 1 {
		t.Fatalf("max active = %d, want 1", got)
	}
}

func TestCleanupPolicyIsSanitized(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := mustServiceAt(t, path)

	next := s.GetSettings()
	next.InstallCleanupPolicy = "wipe"
	if err := s.SaveSettings(next); err != nil {
		t.Fatal(err)
	}
	if got := s.GetSettings().InstallCleanupPolicy; got != CleanupDelete {
		t.Fatalf("policy = %q, want %q", got, CleanupDelete)
	}

	next.InstallCleanupPolicy = CleanupKeep
	if err := s.SaveSettings(next); err != nil {
		t.Fatal(err)
	}
	if got := s.GetSettings().InstallCleanupPolicy; got != CleanupKeep {
		t.Fatalf("policy = %q, want %q", got, CleanupKeep)
	}
}

func TestSourceRefreshIntervalIsSanitized(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := mustServiceAt(t, path)

	if got := s.GetSettings().SourceRefreshInterval; got != RefreshSixHours {
		t.Fatalf("default interval = %q, want %q", got, RefreshSixHours)
	}

	next := s.GetSettings()
	next.SourceRefreshInterval = "every minute"
	if err := s.SaveSettings(next); err != nil {
		t.Fatal(err)
	}
	if got := s.GetSettings().SourceRefreshInterval; got != RefreshSixHours {
		t.Fatalf("interval = %q, want %q", got, RefreshSixHours)
	}

	next.SourceRefreshInterval = RefreshManual
	if err := s.SaveSettings(next); err != nil {
		t.Fatal(err)
	}
	if got := s.GetSettings().SourceRefreshInterval; got != RefreshManual {
		t.Fatalf("interval = %q, want %q", got, RefreshManual)
	}
}

func TestInstallDefaultsSurviveOldConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"theme":"dark","gamesPath":"D:\\Games"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got := mustServiceAt(t, path).GetSettings()
	if got.InstallCleanupPolicy != CleanupDelete || got.AutoInstall || !got.VerifyAfterInstall {
		t.Fatalf("install defaults lost: %+v", got)
	}
}

func TestInstallExtrasDeclinedByDefault(t *testing.T) {
	defaults := Defaults()
	if !defaults.InstallSkipShortcuts || !defaults.InstallSkipExtras {
		t.Fatalf("по умолчанию ярлыки и допы отклоняются: %+v", defaults)
	}

	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"theme":"dark","autoInstall":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got := mustServiceAt(t, path).GetSettings()
	if !got.InstallSkipShortcuts || !got.InstallSkipExtras {
		t.Fatalf("конфиг без новых ключей не должен включать ярлыки: %+v", got)
	}
}

func TestInstallExtrasSurviveRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	svc := mustServiceAt(t, path)
	next := svc.GetSettings()
	next.InstallSkipShortcuts = false
	next.InstallSkipExtras = false
	if err := svc.SaveSettings(next); err != nil {
		t.Fatalf("SaveSettings error = %v", err)
	}
	got := mustServiceAt(t, path).GetSettings()
	if got.InstallSkipShortcuts || got.InstallSkipExtras {
		t.Fatalf("выключенный отказ не сохранился: %+v", got)
	}
}

func TestSubscribersAreNotified(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := mustServiceAt(t, path)

	var seen Settings
	calls := 0
	unsubscribe := s.Subscribe(func(next Settings) {
		seen = next
		calls++
	})

	next := s.GetSettings()
	next.SeedAfterDownload = true
	if err := s.SaveSettings(next); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !seen.SeedAfterDownload {
		t.Fatalf("calls = %d, seen = %+v", calls, seen)
	}

	unsubscribe()
	if err := s.SaveSettings(next); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("calls after unsubscribe = %d, want 1", calls)
	}
}

func TestCorruptFileFailsConstruction(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewServiceAt(path); err == nil {
		t.Fatal("corrupt settings must not start the service")
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{broken" {
		t.Fatalf("settings file rewritten: %q", data)
	}
}

func TestUploadWhileDownloadingDefaultsOff(t *testing.T) {
	d := Defaults()
	if d.UploadWhileDownloading {
		t.Fatal("UploadWhileDownloading enabled by default")
	}
	if d.SeedAfterDownload {
		t.Fatal("SeedAfterDownload enabled by default")
	}
}

func TestLoadWithoutUploadFieldKeepsExistingSeeding(t *testing.T) {
	cases := []struct {
		name       string
		stored     string
		wantSeed   bool
		wantMax    int
		wantUpload bool
	}{
		{"legacy config", `{"seedAfterDownload":true,"maxActiveDownloads":3}`, true, 3, false},
		{"legacy config seeding off", `{"seedAfterDownload":false}`, false, 2, false},
		{"explicit upload on", `{"seedAfterDownload":false,"uploadWhileDownloading":true}`, false, 2, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			if err := os.WriteFile(path, []byte(c.stored), 0o644); err != nil {
				t.Fatal(err)
			}
			s, err := NewServiceAt(path)
			if err != nil {
				t.Fatalf("new settings service: %v", err)
			}
			got := s.GetSettings()
			if got.UploadWhileDownloading != c.wantUpload {
				t.Fatalf("UploadWhileDownloading = %v, want %v", got.UploadWhileDownloading, c.wantUpload)
			}
			if got.SeedAfterDownload != c.wantSeed {
				t.Fatalf("SeedAfterDownload = %v, want %v", got.SeedAfterDownload, c.wantSeed)
			}
			if got.MaxActiveDownloads != c.wantMax {
				t.Fatalf("MaxActiveDownloads = %d, want %d", got.MaxActiveDownloads, c.wantMax)
			}
		})
	}
}

func TestSaveKeepsUploadSettingsIndependent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s, err := NewServiceAt(path)
	if err != nil {
		t.Fatalf("new settings service: %v", err)
	}
	next := s.GetSettings()
	next.UploadWhileDownloading = true
	if err := s.SaveSettings(next); err != nil {
		t.Fatalf("save settings: %v", err)
	}

	reloaded, err := NewServiceAt(path)
	if err != nil {
		t.Fatalf("reload settings service: %v", err)
	}
	got := reloaded.GetSettings()
	if !got.UploadWhileDownloading {
		t.Fatal("UploadWhileDownloading not persisted")
	}
	if got.SeedAfterDownload {
		t.Fatal("SeedAfterDownload changed with UploadWhileDownloading")
	}
}

func TestDiscordRichPresenceDefaultsOff(t *testing.T) {
	if Defaults().DiscordRichPresence {
		t.Fatal("DiscordRichPresence enabled by default")
	}
}

func TestDiscordRichPresenceSurvivesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := mustServiceAt(t, path)
	next := s.GetSettings()
	next.DiscordRichPresence = true
	if err := s.SaveSettings(next); err != nil {
		t.Fatalf("save: %v", err)
	}
	if !mustServiceAt(t, path).GetSettings().DiscordRichPresence {
		t.Fatal("DiscordRichPresence not persisted")
	}
}

func TestDiscordRichPresenceMissingInOldConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"minimizeToTray":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if mustServiceAt(t, path).GetSettings().DiscordRichPresence {
		t.Fatal("legacy config must keep Discord presence off")
	}
}

func TestSourcesNoticeNotAcceptedByDefault(t *testing.T) {
	if Defaults().SourcesNoticeAccepted {
		t.Fatal("sources notice accepted by default")
	}
}

func TestSourcesNoticeAcceptanceSurvivesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := mustServiceAt(t, path)
	next := s.GetSettings()
	next.SourcesNoticeAccepted = true
	if err := s.SaveSettings(next); err != nil {
		t.Fatalf("save: %v", err)
	}
	if !mustServiceAt(t, path).GetSettings().SourcesNoticeAccepted {
		t.Fatal("acceptance not persisted")
	}
}

func TestSourcesNoticeMissingInOldConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"minimizeToTray":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if mustServiceAt(t, path).GetSettings().SourcesNoticeAccepted {
		t.Fatal("legacy config must not count as accepted")
	}
}

func TestAnonymousUsageStatsNotAcceptedByDefault(t *testing.T) {
	if Defaults().AnonymousUsageStats {
		t.Fatal("anonymous usage stats enabled by default")
	}
}

func TestAnonymousUsageStatsSurvivesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := mustServiceAt(t, path)
	next := s.GetSettings()
	next.AnonymousUsageStats = true
	if err := s.SaveSettings(next); err != nil {
		t.Fatalf("save: %v", err)
	}
	if !mustServiceAt(t, path).GetSettings().AnonymousUsageStats {
		t.Fatal("AnonymousUsageStats not persisted")
	}
}

func TestAnonymousUsageStatsMissingInOldConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"minimizeToTray":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if mustServiceAt(t, path).GetSettings().AnonymousUsageStats {
		t.Fatal("legacy config must not count as opted in")
	}
}

func TestAnonymousDiagnosticsNotAcceptedByDefault(t *testing.T) {
	if Defaults().AnonymousDiagnostics {
		t.Fatal("anonymous diagnostics enabled by default")
	}
}

func TestAnonymousDiagnosticsSurvivesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := mustServiceAt(t, path)
	next := s.GetSettings()
	next.AnonymousDiagnostics = true
	if err := s.SaveSettings(next); err != nil {
		t.Fatalf("save: %v", err)
	}
	if !mustServiceAt(t, path).GetSettings().AnonymousDiagnostics {
		t.Fatal("AnonymousDiagnostics not persisted")
	}
}

func TestAnonymousDiagnosticsMissingInOldConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"minimizeToTray":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if mustServiceAt(t, path).GetSettings().AnonymousDiagnostics {
		t.Fatal("legacy config must not count as opted in")
	}
}

func TestAnonymousDiagnosticsIndependentFromUsageStats(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := mustServiceAt(t, path)

	next := s.GetSettings()
	next.AnonymousDiagnostics = true
	if err := s.SaveSettings(next); err != nil {
		t.Fatalf("save: %v", err)
	}
	got := s.GetSettings()
	if !got.AnonymousDiagnostics {
		t.Fatal("AnonymousDiagnostics not saved")
	}
	if got.AnonymousUsageStats {
		t.Fatal("enabling AnonymousDiagnostics must not enable AnonymousUsageStats")
	}

	next = s.GetSettings()
	next.AnonymousUsageStats = true
	next.AnonymousDiagnostics = false
	if err := s.SaveSettings(next); err != nil {
		t.Fatalf("save: %v", err)
	}
	got = s.GetSettings()
	if !got.AnonymousUsageStats {
		t.Fatal("AnonymousUsageStats not saved")
	}
	if got.AnonymousDiagnostics {
		t.Fatal("enabling AnonymousUsageStats must not enable AnonymousDiagnostics")
	}
}

func TestSubscribeNotifiedOnAnonymousUsageStatsChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := mustServiceAt(t, path)

	var seen Settings
	calls := 0
	s.Subscribe(func(next Settings) {
		seen = next
		calls++
	})

	next := s.GetSettings()
	next.AnonymousUsageStats = true
	if err := s.SaveSettings(next); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("subscriber called %d times, want 1", calls)
	}
	if !seen.AnonymousUsageStats {
		t.Fatal("subscriber saw stale AnonymousUsageStats value")
	}
}

func TestApplierReceivesSanitizedSettingsBeforePersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := mustServiceAt(t, path)

	var seenScale float64
	var fileExistedDuringApply bool
	if err := s.AddApplier(func(_, next Settings) error {
		seenScale = next.UIScale
		_, statErr := os.Stat(path)
		fileExistedDuringApply = statErr == nil
		return nil
	}); err != nil {
		t.Fatalf("AddApplier: %v", err)
	}

	next := s.GetSettings()
	next.UIScale = 3
	if err := s.SaveSettings(next); err != nil {
		t.Fatal(err)
	}
	if seenScale != 1 {
		t.Fatalf("applier saw UIScale = %v, want sanitized 1", seenScale)
	}
	if fileExistedDuringApply {
		t.Fatal("applier ran after the settings file was already written")
	}
}

func TestApplierErrorPreventsPersistAndNotify(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := mustServiceAt(t, path)

	base := s.GetSettings()
	base.Theme = "dark"
	if err := s.SaveSettings(base); err != nil {
		t.Fatalf("baseline save: %v", err)
	}
	before, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}

	calls := 0
	s.Subscribe(func(Settings) { calls++ })

	applyErr := errors.New("registry denied")
	if err := s.AddApplier(func(Settings, Settings) error { return applyErr }); err != nil {
		t.Fatalf("AddApplier: %v", err)
	}

	next := s.GetSettings()
	next.Theme = "light"
	err = s.SaveSettings(next)
	if err == nil {
		t.Fatal("SaveSettings must fail when an applier errors")
	}
	if !errors.Is(err, applyErr) {
		t.Fatalf("SaveSettings error = %v, want wrapped %v", err, applyErr)
	}
	if calls != 0 {
		t.Fatalf("subscriber called %d times, want 0", calls)
	}
	if got := s.GetSettings(); got != base {
		t.Fatalf("in-memory settings changed: got %+v, want %+v", got, base)
	}
	after, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("settings file changed despite applier error")
	}
}

func TestApplierChainStopsAtFirstError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := mustServiceAt(t, path)

	firstErr := errors.New("first failed")
	firstCalls, secondCalls := 0, 0
	if err := s.AddApplier(func(Settings, Settings) error {
		firstCalls++
		return firstErr
	}); err != nil {
		t.Fatalf("AddApplier: %v", err)
	}
	if err := s.AddApplier(func(Settings, Settings) error {
		secondCalls++
		return nil
	}); err != nil {
		t.Fatalf("AddApplier: %v", err)
	}

	next := s.GetSettings()
	next.Theme = "light"
	if err := s.SaveSettings(next); err == nil {
		t.Fatal("SaveSettings must fail")
	}
	if firstCalls != 1 {
		t.Fatalf("first applier called %d times, want 1", firstCalls)
	}
	if secondCalls != 0 {
		t.Fatalf("second applier called %d times, want 0", secondCalls)
	}
}

func TestAddApplierRejectsNil(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := mustServiceAt(t, path)
	if err := s.AddApplier(nil); err == nil {
		t.Fatal("AddApplier(nil) must return an error")
	}
}

func TestApplierSuccessPathPersistsAndNotifies(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := mustServiceAt(t, path)

	applyCalls := 0
	if err := s.AddApplier(func(Settings, Settings) error {
		applyCalls++
		return nil
	}); err != nil {
		t.Fatalf("AddApplier: %v", err)
	}
	subCalls := 0
	s.Subscribe(func(Settings) { subCalls++ })

	next := s.GetSettings()
	next.Theme = "light"
	if err := s.SaveSettings(next); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	if applyCalls != 1 {
		t.Fatalf("applier called %d times, want 1", applyCalls)
	}
	if subCalls != 1 {
		t.Fatalf("subscriber called %d times, want 1", subCalls)
	}
	if got := mustServiceAt(t, path).GetSettings().Theme; got != "light" {
		t.Fatalf("theme = %q, want light", got)
	}
}

func TestApplierSeesStoredSettingsAsPrevious(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := mustServiceAt(t, path)

	base := s.GetSettings()
	base.LaunchOnStartup = false
	if err := s.SaveSettings(base); err != nil {
		t.Fatalf("baseline save: %v", err)
	}

	unrelatedErr := errors.New("registry denied")
	guardedCalls := 0
	if err := s.AddApplier(func(prev, next Settings) error {
		if prev.LaunchOnStartup == next.LaunchOnStartup {
			return nil
		}
		guardedCalls++
		return unrelatedErr
	}); err != nil {
		t.Fatalf("AddApplier: %v", err)
	}

	unrelated := s.GetSettings()
	unrelated.Theme = "light"
	if err := s.SaveSettings(unrelated); err != nil {
		t.Fatalf("saving an untouched field must not run the applier: %v", err)
	}
	if guardedCalls != 0 {
		t.Fatalf("applier acted %d times on an unrelated change, want 0", guardedCalls)
	}
	if got := s.GetSettings().Theme; got != "light" {
		t.Fatalf("Theme = %q, want light", got)
	}

	touched := s.GetSettings()
	touched.LaunchOnStartup = true
	if err := s.SaveSettings(touched); !errors.Is(err, unrelatedErr) {
		t.Fatalf("SaveSettings error = %v, want wrapped %v", err, unrelatedErr)
	}
	if guardedCalls != 1 {
		t.Fatalf("applier acted %d times on its own change, want 1", guardedCalls)
	}
	if s.GetSettings().LaunchOnStartup {
		t.Fatal("failed applier must not leave the setting stored as enabled")
	}
}

func TestDesktopShortcutsDefaultsOn(t *testing.T) {
	if !Defaults().DesktopShortcuts {
		t.Fatal("DesktopShortcuts disabled by default")
	}
}

func TestDesktopShortcutsSurvivesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := mustServiceAt(t, path)
	next := s.GetSettings()
	next.DesktopShortcuts = false
	if err := s.SaveSettings(next); err != nil {
		t.Fatalf("save: %v", err)
	}
	if mustServiceAt(t, path).GetSettings().DesktopShortcuts {
		t.Fatal("DesktopShortcuts not persisted")
	}
}

func TestDesktopShortcutsMissingInOldConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"minimizeToTray":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if !mustServiceAt(t, path).GetSettings().DesktopShortcuts {
		t.Fatal("legacy config must keep desktop shortcuts on")
	}
}
