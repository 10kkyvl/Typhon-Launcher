package settings

import (
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
