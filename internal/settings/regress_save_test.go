package settings

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

func TestEveryFlagAndValueSurvivesSaveAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	svc := mustServiceAt(t, path)

	next := svc.GetSettings()
	value := reflect.ValueOf(&next).Elem()
	flipped := 0
	for i := range value.NumField() {
		if field := value.Field(i); field.Kind() == reflect.Bool {
			field.SetBool(!field.Bool())
			flipped++
		}
	}
	if flipped < 20 {
		t.Fatalf("only %d flags found, the reflection walk is broken", flipped)
	}
	next.Theme = "light"
	next.AccentColor = "#ABCDEF"
	next.UIScale = 1.1
	next.Language = LanguageEN
	next.LibraryPath = filepath.Join(t.TempDir(), LibraryFolderName)
	next = derivePaths(next)
	next.MaxActiveDownloads = 7
	next.DownloadRateLimit = 123456
	next.UploadRateLimit = 654321
	next.InstallCleanupPolicy = CleanupAsk
	next.SourceRefreshInterval = RefreshHalfDay
	next.KeepPreviousVersion = KeepPreviousDay
	next.SaveBackupLimit = 17
	next.NetworkMode = NetworkProxy
	next.NetworkInterface = "Ethernet 2"
	next.ProxyType = ProxyHTTP
	next.ProxyHost = "proxy.example.com"
	next.ProxyPort = 8080
	next.ProxyUsername = "player"
	next.OverlayHotkey = OverlayHotkeyShiftF1
	next.PresenceStatus = PresenceBusy
	next.TelemetryConsentVersion = CurrentTelemetryConsent

	if err := svc.SaveSettings(next); err != nil {
		t.Fatalf("save: %v", err)
	}
	if got := svc.GetSettings(); got != next {
		t.Fatalf("the save changed a valid value: got %+v, want %+v", got, next)
	}

	reloaded := reflect.ValueOf(mustServiceAt(t, path).GetSettings())
	want := reflect.ValueOf(next)
	for i := range want.NumField() {
		if !reflect.DeepEqual(want.Field(i).Interface(), reloaded.Field(i).Interface()) {
			t.Errorf("%s did not survive the reload: saved %v, reloaded %v",
				want.Type().Field(i).Name, want.Field(i).Interface(), reloaded.Field(i).Interface())
		}
	}
}

func TestRejectedSaveChangesNothing(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Settings)
	}{
		{"accent that is not a colour", func(s *Settings) { s.AccentColor = "red" }},
		{"relative library path", func(s *Settings) { s.LibraryPath = filepath.Join("relative", LibraryFolderName) }},
		{"unknown network mode", func(s *Settings) { s.NetworkMode = "vpn" }},
		{"proxy mode without a host", func(s *Settings) { s.NetworkMode = NetworkProxy; s.ProxyPort = 1080 }},
		{"unknown overlay hotkey", func(s *Settings) { s.OverlayHotkey = "F13" }},
		{"proxy login with a colon", func(s *Settings) { s.ProxyUsername = "a:b" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			svc := mustServiceAt(t, path)
			base := svc.GetSettings()
			base.Theme = "light"
			if err := svc.SaveSettings(base); err != nil {
				t.Fatalf("baseline save: %v", err)
			}
			stored := readBytes(t, path)

			applied, notified := 0, 0
			if err := svc.AddApplier(func(Settings, Settings) error { applied++; return nil }); err != nil {
				t.Fatal(err)
			}
			svc.Subscribe(func(Settings) { notified++ })

			next := svc.GetSettings()
			tc.mutate(&next)
			if err := svc.SaveSettings(next); err == nil {
				t.Fatal("expected the save to be rejected")
			}

			if got := svc.GetSettings(); got != base {
				t.Fatalf("memory changed by a rejected save: %+v", got)
			}
			if got := readBytes(t, path); got != stored {
				t.Fatal("file changed by a rejected save")
			}
			if applied != 0 || notified != 0 {
				t.Fatalf("applier ran %d times and subscribers %d times for a rejected save", applied, notified)
			}
		})
	}
}

func TestFailedWriteKeepsMemorySubscribersAndFileUntouched(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "cfg")
	path := filepath.Join(dir, "settings.json")
	svc := mustServiceAt(t, path)
	base := svc.GetSettings()
	base.Theme = "light"
	if err := svc.SaveSettings(base); err != nil {
		t.Fatalf("baseline save: %v", err)
	}

	notified := 0
	svc.Subscribe(func(Settings) { notified++ })

	// The directory is swapped for a file so the next write cannot create
	// anything under it, on Windows as well as on Unix.
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}

	next := base
	next.Theme = "dark"
	next.MaxActiveDownloads = 9
	if err := svc.SaveSettings(next); err == nil {
		t.Fatal("expected the write to fail")
	}

	if got := svc.GetSettings(); got != base {
		t.Fatalf("memory ran ahead of the disk: %+v", got)
	}
	if notified != 0 {
		t.Fatalf("subscribers told about a save that failed: %d", notified)
	}
	if got := readBytes(t, dir); got != "occupied" {
		t.Fatalf("blocker rewritten: %q", got)
	}
}

func TestFailedConsentWriteKeepsThePromptPending(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "cfg")
	svc := mustServiceAt(t, filepath.Join(dir, "settings.json"))
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := svc.SaveConsent(true, true)
	if err == nil {
		t.Fatal("expected the consent write to fail")
	}
	if got != (Settings{}) {
		t.Fatalf("a failed answer returned settings: %+v", got)
	}
	current := svc.GetSettings()
	if current.TelemetryConsentRecorded() || current.UsageStatsAllowed() || current.DiagnosticsAllowed() {
		t.Fatalf("an answer that was never stored is already in effect: %+v", current)
	}
}

func TestConcurrentSavesNeverTearTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	svc := mustServiceAt(t, path)

	const savers = 8
	var wg sync.WaitGroup
	errs := make(chan error, savers*10*2)
	for n := 1; n <= savers; n++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for range 4 {
				next := svc.GetSettings()
				next.MaxActiveDownloads = n
				if err := svc.SaveSettings(next); err != nil {
					errs <- err
				}
			}
		}()
		go func() {
			defer wg.Done()
			for range 10 {
				unsubscribe := svc.Subscribe(func(Settings) {})
				if n := svc.GetSettings().MaxActiveDownloads; n < 1 {
					errs <- fmt.Errorf("read a downloads limit of %d", n)
				}
				unsubscribe()
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent use: %v", err)
	}

	reloaded := mustServiceAt(t, path).GetSettings()
	if reloaded != svc.GetSettings() {
		t.Fatalf("file and memory disagree after concurrent saves: %+v vs %+v", reloaded, svc.GetSettings())
	}
	if n := reloaded.MaxActiveDownloads; n < 1 || n > savers {
		t.Fatalf("stored value %d was never saved by anyone", n)
	}
}
