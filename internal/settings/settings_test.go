package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := newServiceAt(path)

	next := s.GetSettings()
	next.Theme = "dark"
	next.UIScale = 1.1
	next.GamesPath = `D:\Games`
	next.MinimizeToTray = false
	if err := s.SaveSettings(next); err != nil {
		t.Fatal(err)
	}

	reloaded := newServiceAt(path).GetSettings()
	if reloaded != next {
		t.Fatalf("got %+v, want %+v", reloaded, next)
	}
}

func TestInvalidScaleFallsBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := newServiceAt(path)
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
	s := newServiceAt(path)
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
	s := newServiceAt(path)

	next := s.GetSettings()
	next.InstallCleanupPolicy = "wipe"
	if err := s.SaveSettings(next); err != nil {
		t.Fatal(err)
	}
	if got := s.GetSettings().InstallCleanupPolicy; got != CleanupKeep {
		t.Fatalf("policy = %q, want %q", got, CleanupKeep)
	}

	next.InstallCleanupPolicy = CleanupDelete
	if err := s.SaveSettings(next); err != nil {
		t.Fatal(err)
	}
	if got := s.GetSettings().InstallCleanupPolicy; got != CleanupDelete {
		t.Fatalf("policy = %q, want %q", got, CleanupDelete)
	}
}

func TestSourceRefreshIntervalIsSanitized(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := newServiceAt(path)

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
	got := newServiceAt(path).GetSettings()
	if got.InstallCleanupPolicy != CleanupKeep || got.AutoInstall || !got.VerifyAfterInstall {
		t.Fatalf("install defaults lost: %+v", got)
	}
}

func TestSubscribersAreNotified(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := newServiceAt(path)

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

func TestCorruptFileFallsBackToDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := newServiceAt(path).GetSettings()
	if got != Defaults() {
		t.Fatalf("got %+v, want defaults", got)
	}
}
