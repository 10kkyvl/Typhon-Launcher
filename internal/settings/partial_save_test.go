package settings

import (
	"path/filepath"
	"sync"
	"testing"
)

// gateFirst blocks the first applier call that satisfies match until release
// is closed, holding a save between reading its starting point and writing.
func gateFirst(t *testing.T, svc *Service, match func(prev, next Settings) bool) (reached <-chan struct{}, release func()) {
	t.Helper()
	in := make(chan struct{})
	out := make(chan struct{})
	var once, free sync.Once
	release = func() { free.Do(func() { close(out) }) }
	t.Cleanup(release)
	if err := svc.AddApplier(func(prev, next Settings) error {
		if match(prev, next) {
			once.Do(func() {
				close(in)
				<-out
			})
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return in, release
}

func TestSetupLibraryKeepsASettingSavedWhileItWasInFlight(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	svc := mustServiceAt(t, path)
	reached, release := gateFirst(t, svc, func(_, next Settings) bool { return next.LibraryPath != "" })

	done := make(chan error, 1)
	go func() {
		_, err := svc.SetupLibrary(t.TempDir())
		done <- err
	}()
	<-reached

	other := svc.GetSettings()
	other.Theme = "light"
	other.MaxActiveDownloads = 7
	if err := svc.SaveSettings(other); err != nil {
		t.Fatalf("concurrent save: %v", err)
	}
	release()
	if err := <-done; err != nil {
		t.Fatalf("SetupLibrary: %v", err)
	}

	for name, got := range map[string]Settings{
		"memory": svc.GetSettings(),
		"disk":   mustServiceAt(t, path).GetSettings(),
	} {
		if got.LibraryPath == "" {
			t.Errorf("%s: the library path was lost", name)
		}
		if got.Theme != "light" || got.MaxActiveDownloads != 7 {
			t.Errorf("%s: theme %q, downloads %d: SetupLibrary wrote a stale copy over a newer save", name, got.Theme, got.MaxActiveDownloads)
		}
	}
}

func TestSaveConsentKeepsASettingSavedWhileItWasInFlight(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	svc := mustServiceAt(t, path)
	reached, release := gateFirst(t, svc, func(_, next Settings) bool { return next.TelemetryConsentVersion == CurrentTelemetryConsent })

	done := make(chan error, 1)
	go func() {
		_, err := svc.SaveConsent(true, false)
		done <- err
	}()
	<-reached

	other := svc.GetSettings()
	other.Theme = "light"
	if err := svc.SaveSettings(other); err != nil {
		t.Fatalf("concurrent save: %v", err)
	}
	release()
	if err := <-done; err != nil {
		t.Fatalf("SaveConsent: %v", err)
	}

	got := mustServiceAt(t, path).GetSettings()
	if got.Theme != "light" {
		t.Fatalf("theme = %q: SaveConsent wrote a stale copy over a newer save", got.Theme)
	}
	if got.TelemetryConsentVersion != CurrentTelemetryConsent || !got.AnonymousUsageStats || got.AnonymousDiagnostics {
		t.Fatalf("consent = %+v, want the answer recorded", got)
	}
}
