package settings

import (
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/uierr"
)

func TestSaveLegalAcceptanceSurvivesReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	got, err := mustServiceAt(t, path).SaveLegalAcceptance("2026-10-10")
	if err != nil {
		t.Fatalf("SaveLegalAcceptance: %v", err)
	}
	if got.LegalAcceptedVersion != "2026-10-10" {
		t.Fatalf("returned version = %q", got.LegalAcceptedVersion)
	}
	if reloaded := mustServiceAt(t, path).GetSettings(); reloaded.LegalAcceptedVersion != "2026-10-10" {
		t.Fatalf("reloaded version = %q", reloaded.LegalAcceptedVersion)
	}
}

func TestSaveLegalAcceptanceRejectsEmptyVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	svc := mustServiceAt(t, path)
	if _, err := svc.SaveLegalAcceptance(""); err == nil {
		t.Fatal("SaveLegalAcceptance(\"\") = nil, want an error")
	}
	if got := svc.GetSettings().LegalAcceptedVersion; got != "" {
		t.Fatalf("version after a rejected save = %q", got)
	}
}

func TestSaveLegalAcceptanceFailureCarriesAUICode(t *testing.T) {
	dir := t.TempDir()
	svc := mustServiceAt(t, filepath.Join(dir, "settings.json"))
	blocked := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(blocked, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc.path = filepath.Join(blocked, "settings.json")

	_, err := svc.SaveLegalAcceptance("2026-10-10")
	if err == nil {
		t.Fatal("SaveLegalAcceptance with an invalid parent path = nil, want an error")
	}
	if code := uierr.Code(err); code != ErrCodeLegalAcceptanceSaveFailed {
		t.Fatalf("uierr.Code(err) = %q, want %q (raw text: %v)", code, ErrCodeLegalAcceptanceSaveFailed, err)
	}
	if got := svc.GetSettings().LegalAcceptedVersion; got != "" {
		t.Fatalf("in-memory version after a failed save = %q", got)
	}
}

func TestSettingsFileWithoutLegalFieldLoadsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"theme":"dark","telemetryConsentVersion":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := mustServiceAt(t, path).GetSettings().LegalAcceptedVersion; got != "" {
		t.Fatalf("version from a file without the field = %q, want empty", got)
	}
}

func TestSaveSettingsCannotEraseAcceptedLegalVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	svc := mustServiceAt(t, path)
	if _, err := svc.SaveLegalAcceptance("2026-10-10"); err != nil {
		t.Fatal(err)
	}
	stale := svc.GetSettings()
	stale.LegalAcceptedVersion = ""
	stale.Theme = "light"
	if err := svc.SaveSettings(stale); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	if got := mustServiceAt(t, path).GetSettings().LegalAcceptedVersion; got != "2026-10-10" {
		t.Fatalf("version after a stale save = %q, want it kept", got)
	}
}

func TestPortableDoesNotCarryLegalAcceptance(t *testing.T) {
	local := Defaults()
	local.LegalAcceptedVersion = "2026-10-10"
	merged := ApplyPortable(Defaults(), PortableOf(local))
	if merged.LegalAcceptedVersion != "" {
		t.Fatalf("legal acceptance travelled through sync: %q", merged.LegalAcceptedVersion)
	}
}
