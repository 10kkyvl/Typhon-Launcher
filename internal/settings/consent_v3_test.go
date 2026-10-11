package settings

import (
	"path/filepath"
	"testing"
)

func TestSessionHeartbeatDefaultsOnForEveryStoredFile(t *testing.T) {
	cases := []struct {
		name   string
		stored string
		want   bool
	}{
		{"file predates the field", `{"minimizeToTray":true}`, true},
		{"file predates the prompt with switches off", `{"anonymousUsageStats":false,"anonymousDiagnostics":false}`, true},
		{"file answered at version 2 without the field", `{"anonymousUsageStats":true,"telemetryConsentVersion":2}`, true},
		{"field stored on", `{"sessionHeartbeat":true}`, true},
		{"field stored off", `{"sessionHeartbeat":false}`, false},
		{"field stored off next to a consent answer", `{"sessionHeartbeat":false,"anonymousUsageStats":true,"telemetryConsentVersion":3}`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := mustServiceAt(t, writeConfig(t, c.stored)).GetSettings()
			if got.SessionHeartbeat != c.want {
				t.Fatalf("SessionHeartbeat = %v, want %v", got.SessionHeartbeat, c.want)
			}
		})
	}
}

func TestSessionHeartbeatOffSurvivesSaveRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	svc := mustServiceAt(t, path)

	next := svc.GetSettings()
	next.SessionHeartbeat = false
	if err := svc.SaveSettings(next); err != nil {
		t.Fatalf("save: %v", err)
	}
	if mustServiceAt(t, path).GetSettings().SessionHeartbeat {
		t.Fatal("SessionHeartbeat=false was not persisted")
	}

	again := svc.GetSettings()
	again.Theme = "light"
	if err := svc.SaveSettings(again); err != nil {
		t.Fatalf("save unrelated: %v", err)
	}
	if mustServiceAt(t, path).GetSettings().SessionHeartbeat {
		t.Fatal("an unrelated save switched the heartbeat back on")
	}
}

func TestFreshInstallDefaultsForTelemetry(t *testing.T) {
	got := mustServiceAt(t, filepath.Join(t.TempDir(), "settings.json")).GetSettings()
	if got.AnonymousDiagnostics || got.AnonymousUsageStats {
		t.Fatalf("a switch starts on: usage=%v diagnostics=%v", got.AnonymousUsageStats, got.AnonymousDiagnostics)
	}
	if !got.SessionHeartbeat {
		t.Fatal("SessionHeartbeat starts off")
	}
	if got.DiagnosticsAllowed() || got.UsageStatsAllowed() || got.UsageStatsAllowedCurrent() {
		t.Fatal("something is allowed before the prompt is answered")
	}
}

func TestUsageStatsAllowedCurrent(t *testing.T) {
	cases := []struct {
		name    string
		version int
		usage   bool
		want    bool
	}{
		{"never asked", 0, true, false},
		{"answered version 1", 1, true, false},
		{"answered version 2", 2, true, false},
		{"answered the current version", CurrentTelemetryConsent, true, true},
		{"answered the current version, declined", CurrentTelemetryConsent, false, false},
		{"answered a newer version", CurrentTelemetryConsent + 1, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := Settings{TelemetryConsentVersion: c.version, AnonymousUsageStats: c.usage}
			if got := s.UsageStatsAllowedCurrent(); got != c.want {
				t.Fatalf("UsageStatsAllowedCurrent = %v, want %v", got, c.want)
			}
			if got := s.CompatReportsAllowed(); got != c.want {
				t.Fatalf("CompatReportsAllowed = %v, want %v", got, c.want)
			}
		})
	}
}

const answeredV2 = `{"anonymousUsageStats":true,"anonymousDiagnostics":false,"telemetryConsentVersion":2}`

func TestVersionTwoAnswerKeepsItsSwitchesUntilVersionThreeIsAnswered(t *testing.T) {
	path := writeConfig(t, answeredV2)
	svc := mustServiceAt(t, path)

	got := svc.GetSettings()
	if got.TelemetryConsentVersion != 2 {
		t.Fatalf("TelemetryConsentVersion after load = %d, want 2", got.TelemetryConsentVersion)
	}
	if !got.UsageStatsAllowed() {
		t.Fatal("UsageStatsAllowed = false, the version 2 answer must keep its meaning")
	}
	if got.UsageStatsAllowedCurrent() {
		t.Fatal("UsageStatsAllowedCurrent = true, a version 2 answer does not cover the version 3 data")
	}
	if got.DiagnosticsAllowed() {
		t.Fatal("DiagnosticsAllowed = true, the user declined it")
	}

	saved, err := svc.SaveConsent(true, false)
	if err != nil {
		t.Fatalf("SaveConsent: %v", err)
	}
	if saved.TelemetryConsentVersion != 3 || !saved.UsageStatsAllowedCurrent() {
		t.Fatalf("SaveConsent result = version %d, current allowed %v", saved.TelemetryConsentVersion, saved.UsageStatsAllowedCurrent())
	}
	reloaded := mustServiceAt(t, path).GetSettings()
	if reloaded.TelemetryConsentVersion != 3 || !reloaded.UsageStatsAllowedCurrent() {
		t.Fatalf("reloaded = version %d, current allowed %v", reloaded.TelemetryConsentVersion, reloaded.UsageStatsAllowedCurrent())
	}
}

func TestVersionTwoDeclineStaysDeclinedAfterVersionThreeIsAnswered(t *testing.T) {
	path := writeConfig(t, `{"anonymousUsageStats":false,"anonymousDiagnostics":true,"telemetryConsentVersion":2}`)
	svc := mustServiceAt(t, path)

	got := svc.GetSettings()
	if got.UsageStatsAllowed() || got.UsageStatsAllowedCurrent() {
		t.Fatal("usage statistics turned on by loading a version 2 file that declined them")
	}
	saved, err := svc.SaveConsent(false, true)
	if err != nil {
		t.Fatalf("SaveConsent: %v", err)
	}
	if saved.UsageStatsAllowedCurrent() || !saved.DiagnosticsAllowed() {
		t.Fatalf("answer was not kept as given: %+v", saved)
	}
}

func TestGenericSaveFromVersionTwoCallerDoesNotRaiseTheVersion(t *testing.T) {
	cases := []struct {
		name string
		save func(t *testing.T, svc *Service)
	}{
		{"SaveSettings", func(t *testing.T, svc *Service) {
			next := svc.GetSettings()
			next.Theme = "light"
			if err := svc.SaveSettings(next); err != nil {
				t.Fatalf("SaveSettings: %v", err)
			}
		}},
		{"Update", func(t *testing.T, svc *Service) {
			if _, err := svc.Update(func(next *Settings) error {
				next.Theme = "light"
				return nil
			}); err != nil {
				t.Fatalf("Update: %v", err)
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := writeConfig(t, answeredV2)
			svc := mustServiceAt(t, path)
			c.save(t, svc)

			for label, got := range map[string]Settings{"in memory": svc.GetSettings(), "reloaded": mustServiceAt(t, path).GetSettings()} {
				if got.Theme != "light" {
					t.Fatalf("%s: the save itself was lost: theme = %q", label, got.Theme)
				}
				if got.TelemetryConsentVersion != 2 {
					t.Fatalf("%s: TelemetryConsentVersion = %d, a generic save must not answer version 3", label, got.TelemetryConsentVersion)
				}
				if got.UsageStatsAllowedCurrent() {
					t.Fatalf("%s: UsageStatsAllowedCurrent = true after a save that answered nothing", label)
				}
				if !got.UsageStatsAllowed() {
					t.Fatalf("%s: the version 2 answer was lost", label)
				}
			}
		})
	}
}
