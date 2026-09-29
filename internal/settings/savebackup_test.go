package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveBackupDefaults(t *testing.T) {
	got := Defaults()
	if !got.SaveBackupAfterSession {
		t.Error("SaveBackupAfterSession = false, want on by default")
	}
	if got.SaveBackupLimit != DefaultSaveBackupLimit || DefaultSaveBackupLimit != 5 {
		t.Errorf("SaveBackupLimit = %d, want 5", got.SaveBackupLimit)
	}
}

func TestSaveBackupLimitIsSanitized(t *testing.T) {
	cases := []struct {
		name string
		in   int
		want int
	}{
		{"zero", 0, MinSaveBackupLimit},
		{"negative", -3, MinSaveBackupLimit},
		{"min", 1, 1},
		{"inside", 12, 12},
		{"max", 50, 50},
		{"above", 51, MaxSaveBackupLimit},
		{"huge", 1 << 30, MaxSaveBackupLimit},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := mustServiceAt(t, filepath.Join(t.TempDir(), "settings.json"))
			next := s.GetSettings()
			next.SaveBackupLimit = c.in
			if err := s.SaveSettings(next); err != nil {
				t.Fatal(err)
			}
			if got := s.GetSettings().SaveBackupLimit; got != c.want {
				t.Fatalf("SaveBackupLimit = %d, want %d", got, c.want)
			}
		})
	}
}

func TestSaveBackupFieldsMissingInOldConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"theme":"dark"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got := mustServiceAt(t, path).GetSettings()
	if !got.SaveBackupAfterSession || got.SaveBackupLimit != DefaultSaveBackupLimit {
		t.Fatalf("old config lost the defaults: after=%v limit=%d", got.SaveBackupAfterSession, got.SaveBackupLimit)
	}
}

func TestSaveBackupFieldsSurviveRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := mustServiceAt(t, path)
	next := s.GetSettings()
	next.SaveBackupAfterSession = false
	next.SaveBackupLimit = 9
	if err := s.SaveSettings(next); err != nil {
		t.Fatal(err)
	}
	got := mustServiceAt(t, path).GetSettings()
	if got.SaveBackupAfterSession || got.SaveBackupLimit != 9 {
		t.Fatalf("after=%v limit=%d, want false and 9", got.SaveBackupAfterSession, got.SaveBackupLimit)
	}
}

func TestSaveBackupFieldsArePortable(t *testing.T) {
	local := Defaults()
	remote := Defaults()
	remote.SaveBackupAfterSession = false
	remote.SaveBackupLimit = 20
	got := ApplyPortable(local, PortableOf(remote))
	if got.SaveBackupAfterSession || got.SaveBackupLimit != 20 {
		t.Fatalf("after=%v limit=%d, want false and 20", got.SaveBackupAfterSession, got.SaveBackupLimit)
	}
}
