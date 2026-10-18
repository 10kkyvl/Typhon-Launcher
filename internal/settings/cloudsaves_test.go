package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCloudSavesOffByDefault(t *testing.T) {
	if Defaults().CloudSaves {
		t.Fatal("CloudSaves = true in Defaults, want opt-in")
	}
}

func TestCloudSavesOffInOldConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"theme":"dark"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if mustServiceAt(t, path).GetSettings().CloudSaves {
		t.Fatal("old config without cloudSaves turned the cloud on")
	}
}

func TestCloudSavesSurvivesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := mustServiceAt(t, path)
	next := s.GetSettings()
	next.CloudSaves = true
	if err := s.SaveSettings(next); err != nil {
		t.Fatal(err)
	}
	if !mustServiceAt(t, path).GetSettings().CloudSaves {
		t.Fatal("CloudSaves lost after reload")
	}
}

func TestCloudSavesIsNotPortable(t *testing.T) {
	local := Defaults()
	local.CloudSaves = true
	remote := Defaults()
	got := ApplyPortable(local, PortableOf(remote))
	if !got.CloudSaves {
		t.Fatal("account sync switched the local cloud saves setting off")
	}
	remote.CloudSaves = true
	if ApplyPortable(Defaults(), PortableOf(remote)).CloudSaves {
		t.Fatal("account sync switched the local cloud saves setting on")
	}
}
