package settings

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func TestSaveSettingsCannotUndoAConsentAnsweredMeanwhile(t *testing.T) {
	tests := []struct {
		name        string
		usage, diag bool
	}{
		{"both refused", false, false},
		{"usage only", true, false},
		{"both accepted", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			svc := mustServiceAt(t, path)
			stale := svc.GetSettings()
			stale.Theme = "light"
			if stale.TelemetryConsentRecorded() {
				t.Fatalf("precondition: want an unanswered snapshot, got %+v", stale)
			}

			reached := make(chan struct{})
			release := make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			t.Cleanup(unblock)
			var calls atomic.Int32
			if err := svc.AddApplier(func(Settings, Settings) error {
				if calls.Add(1) == 1 {
					close(reached)
					<-release
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}

			saved := make(chan error, 1)
			go func() { saved <- svc.SaveSettings(stale) }()
			<-reached
			if _, err := svc.SaveConsent(tt.usage, tt.diag); err != nil {
				t.Fatalf("SaveConsent: %v", err)
			}
			unblock()
			if err := <-saved; err != nil {
				t.Fatalf("stale SaveSettings: %v", err)
			}

			for name, got := range map[string]Settings{
				"memory": svc.GetSettings(),
				"disk":   mustServiceAt(t, path).GetSettings(),
			} {
				if got.TelemetryConsentVersion != CurrentTelemetryConsent {
					t.Errorf("%s: consent version = %d, want %d", name, got.TelemetryConsentVersion, CurrentTelemetryConsent)
				}
				if got.AnonymousUsageStats != tt.usage || got.AnonymousDiagnostics != tt.diag {
					t.Errorf("%s: switches = (usage %v, diagnostics %v), want the answer (%v, %v)",
						name, got.AnonymousUsageStats, got.AnonymousDiagnostics, tt.usage, tt.diag)
				}
				if got.Theme != "light" {
					t.Errorf("%s: theme = %q, the stale save's own change was lost", name, got.Theme)
				}
			}
		})
	}
}

func TestSaveSettingsWithAnOlderConsentVersionLeavesTheAnswerAlone(t *testing.T) {
	tests := []struct {
		name    string
		version int
	}{
		{"never asked", 0},
		{"asked by an older prompt", CurrentTelemetryConsent - 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			svc := mustServiceAt(t, path)
			if _, err := svc.SaveConsent(true, false); err != nil {
				t.Fatalf("SaveConsent: %v", err)
			}

			next := svc.GetSettings()
			next.TelemetryConsentVersion = tt.version
			next.AnonymousUsageStats = false
			next.AnonymousDiagnostics = true
			if err := svc.SaveSettings(next); err != nil {
				t.Fatal(err)
			}

			got := mustServiceAt(t, path).GetSettings()
			if got.TelemetryConsentVersion != CurrentTelemetryConsent || !got.AnonymousUsageStats || got.AnonymousDiagnostics {
				t.Fatalf("stored consent = %+v, want the answer (usage true, diagnostics false) at version %d",
					got, CurrentTelemetryConsent)
			}
		})
	}
}

func TestSaveSettingsAtTheCurrentConsentVersionChangesTheSwitches(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	svc := mustServiceAt(t, path)
	if _, err := svc.SaveConsent(true, true); err != nil {
		t.Fatalf("SaveConsent: %v", err)
	}

	next := svc.GetSettings()
	next.AnonymousUsageStats = false
	if err := svc.SaveSettings(next); err != nil {
		t.Fatal(err)
	}

	got := mustServiceAt(t, path).GetSettings()
	if got.AnonymousUsageStats || !got.AnonymousDiagnostics || got.TelemetryConsentVersion != CurrentTelemetryConsent {
		t.Fatalf("stored consent = %+v, want usage off, diagnostics on, version %d", got, CurrentTelemetryConsent)
	}
}

func TestConsentSurvivesConcurrentStaleSaves(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	svc := mustServiceAt(t, path)
	stale := svc.GetSettings()

	const savers = 8
	start := make(chan struct{})
	errs := make(chan error, savers+1)
	var wg sync.WaitGroup
	for n := 1; n <= savers; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for range 5 {
				next := stale
				next.MaxActiveDownloads = n
				if err := svc.SaveSettings(next); err != nil {
					errs <- err
				}
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		if _, err := svc.SaveConsent(false, false); err != nil {
			errs <- err
		}
	}()
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent save: %v", err)
	}

	got := svc.GetSettings()
	if got.TelemetryConsentVersion != CurrentTelemetryConsent {
		t.Fatalf("consent version = %d, want %d", got.TelemetryConsentVersion, CurrentTelemetryConsent)
	}
	if got.AnonymousUsageStats || got.AnonymousDiagnostics {
		t.Fatalf("a refusal turned into %+v", got)
	}
	if reloaded := mustServiceAt(t, path).GetSettings(); reloaded != got {
		t.Fatalf("file and memory disagree: %+v vs %+v", reloaded, got)
	}
}
