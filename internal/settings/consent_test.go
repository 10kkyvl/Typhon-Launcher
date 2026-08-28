package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// The launcher ships with the diagnostics switch selected. An installation
// that already exists must not inherit that, whatever its settings file
// happens to contain, because nobody there has been asked. Each case states
// what the stored file means and what the user must see next.
func TestConsentMigrationNeverOptsInAnExistingInstall(t *testing.T) {
	cases := []struct {
		name         string
		stored       string
		wantUsage    bool
		wantDiag     bool
		wantAnswered bool
	}{
		{
			name:         "config predates both switches",
			stored:       `{"minimizeToTray":true}`,
			wantUsage:    false,
			wantDiag:     false,
			wantAnswered: false,
		},
		{
			name:         "config predates the prompt with both switches off",
			stored:       `{"anonymousUsageStats":false,"anonymousDiagnostics":false}`,
			wantUsage:    false,
			wantDiag:     false,
			wantAnswered: false,
		},
		{
			name:         "diagnostics turned on by hand counts as an answer",
			stored:       `{"anonymousUsageStats":false,"anonymousDiagnostics":true}`,
			wantUsage:    false,
			wantDiag:     true,
			wantAnswered: true,
		},
		{
			name:         "usage stats turned on by hand counts as an answer",
			stored:       `{"anonymousUsageStats":true,"anonymousDiagnostics":false}`,
			wantUsage:    true,
			wantDiag:     false,
			wantAnswered: true,
		},
		{
			name:         "prompt answered with both declined is never re-asked",
			stored:       `{"anonymousUsageStats":false,"anonymousDiagnostics":false,"telemetryConsentVersion":1}`,
			wantUsage:    false,
			wantDiag:     false,
			wantAnswered: true,
		},
		{
			name:         "prompt answered with diagnostics accepted",
			stored:       `{"anonymousUsageStats":false,"anonymousDiagnostics":true,"telemetryConsentVersion":1}`,
			wantUsage:    false,
			wantDiag:     true,
			wantAnswered: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := mustServiceAt(t, writeConfig(t, c.stored)).GetSettings()
			if got.AnonymousUsageStats != c.wantUsage {
				t.Errorf("AnonymousUsageStats = %v, want %v", got.AnonymousUsageStats, c.wantUsage)
			}
			if got.AnonymousDiagnostics != c.wantDiag {
				t.Errorf("AnonymousDiagnostics = %v, want %v", got.AnonymousDiagnostics, c.wantDiag)
			}
			if got.TelemetryConsentRecorded() != c.wantAnswered {
				t.Errorf("TelemetryConsentRecorded = %v, want %v", got.TelemetryConsentRecorded(), c.wantAnswered)
			}
			if !got.TelemetryConsentRecorded() && got.DiagnosticsAllowed() {
				t.Error("diagnostics allowed while the prompt is still pending")
			}
		})
	}
}

// A fresh install is the only case the new default applies to, and even there
// it only preselects the prompt.
func TestFreshInstallStartsThePromptWithoutSending(t *testing.T) {
	got := mustServiceAt(t, filepath.Join(t.TempDir(), "settings.json")).GetSettings()
	if !got.AnonymousDiagnostics {
		t.Fatal("fresh install must start the prompt with diagnostics selected")
	}
	if got.TelemetryConsentRecorded() {
		t.Fatal("fresh install must not count as answered")
	}
	if got.DiagnosticsAllowed() || got.UsageStatsAllowed() {
		t.Fatal("fresh install sends before the prompt is answered")
	}
}

// Updating the launcher reloads the same file. Nothing about the second load
// may differ from the first, or an update would change an answer.
func TestConsentSurvivesReloadAndAnUnrelatedSave(t *testing.T) {
	path := writeConfig(t, `{"anonymousUsageStats":false,"anonymousDiagnostics":false}`)

	first := mustServiceAt(t, path).GetSettings()
	if first.AnonymousDiagnostics || first.TelemetryConsentRecorded() {
		t.Fatalf("first load already wrong: %+v", first)
	}

	svc := mustServiceAt(t, path)
	next := svc.GetSettings()
	next.Theme = "light"
	if err := svc.SaveSettings(next); err != nil {
		t.Fatalf("save unrelated setting: %v", err)
	}

	after := mustServiceAt(t, path).GetSettings()
	if after.AnonymousDiagnostics {
		t.Fatal("saving an unrelated setting turned diagnostics on")
	}
	if after.TelemetryConsentRecorded() {
		t.Fatal("saving an unrelated setting counted as an answer")
	}
}

// Persisting settings writes the whole struct, preselection included. If the
// prompt is still pending, that stored preselection must not read back as an
// answer on the next launch — otherwise anyone who opened settings before
// answering would be opted in by a file they never agreed to.
func TestSavingSettingsBeforeAnsweringIsNotAnAnswer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	svc := mustServiceAt(t, path)

	before := svc.GetSettings()
	if !before.AnonymousDiagnostics || before.TelemetryConsentRecorded() {
		t.Fatalf("precondition: want preselected and unanswered, got %+v", before)
	}

	next := before
	next.Theme = "light"
	if err := svc.SaveSettings(next); err != nil {
		t.Fatalf("save: %v", err)
	}

	after := mustServiceAt(t, path).GetSettings()
	if after.TelemetryConsentRecorded() {
		t.Fatal("a settings save before the prompt was answered counted as consent")
	}
	if after.DiagnosticsAllowed() {
		t.Fatal("diagnostics allowed after a save that answered nothing")
	}
}

func TestSaveConsentRecordsAnswerAndVersionTogether(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	svc := mustServiceAt(t, path)

	got, err := svc.SaveConsent(false, true)
	if err != nil {
		t.Fatalf("SaveConsent: %v", err)
	}
	if !got.DiagnosticsAllowed() || got.UsageStatsAllowed() {
		t.Fatalf("SaveConsent returned the wrong answer: %+v", got)
	}

	reloaded := mustServiceAt(t, path).GetSettings()
	if reloaded.TelemetryConsentVersion != CurrentTelemetryConsent {
		t.Fatalf("consent version = %d, want %d", reloaded.TelemetryConsentVersion, CurrentTelemetryConsent)
	}
	if !reloaded.DiagnosticsAllowed() || reloaded.UsageStatsAllowed() {
		t.Fatalf("answer did not survive the reload: %+v", reloaded)
	}
}

// Declining is an answer and must be recorded as one, or the prompt returns on
// the next launch and eventually wears the user down into accepting.
func TestSaveConsentRecordsARefusal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if _, err := mustServiceAt(t, path).SaveConsent(false, false); err != nil {
		t.Fatalf("SaveConsent: %v", err)
	}
	reloaded := mustServiceAt(t, path).GetSettings()
	if !reloaded.TelemetryConsentRecorded() {
		t.Fatal("a refusal was not recorded, so the prompt will come back")
	}
	if reloaded.DiagnosticsAllowed() || reloaded.UsageStatsAllowed() {
		t.Fatal("a refusal still allows sending")
	}
}

// The frontend assembles the settings struct itself and predates this field.
// A caller that omits it sends a zero, and a zero must not erase the record
// that the user was asked.
func TestSaveSettingsCannotEraseARecordedConsent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	svc := mustServiceAt(t, path)
	if _, err := svc.SaveConsent(true, false); err != nil {
		t.Fatalf("SaveConsent: %v", err)
	}

	stale := svc.GetSettings()
	stale.TelemetryConsentVersion = 0
	stale.Theme = "light"
	if err := svc.SaveSettings(stale); err != nil {
		t.Fatalf("save: %v", err)
	}

	reloaded := mustServiceAt(t, path).GetSettings()
	if !reloaded.TelemetryConsentRecorded() {
		t.Fatal("a caller that omitted the field erased the consent record")
	}
	if reloaded.Theme != "light" {
		t.Fatalf("the rest of the save was lost: theme = %q", reloaded.Theme)
	}
}

// The consent answer is per device. Syncing it would move one machine's
// answer onto another machine whose owner was never asked.
func TestConsentIsNotCarriedBetweenDevices(t *testing.T) {
	raw, err := json.Marshal(PortableOf(Settings{
		AnonymousUsageStats:     true,
		AnonymousDiagnostics:    true,
		TelemetryConsentVersion: CurrentTelemetryConsent,
	}))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"anonymousUsageStats", "anonymousDiagnostics", "telemetryConsentVersion"} {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		if _, ok := fields[key]; ok {
			t.Fatalf("portable settings carry %q between devices", key)
		}
	}
}
