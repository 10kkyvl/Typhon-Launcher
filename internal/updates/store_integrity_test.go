package updates

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewServiceAtRejectsEmptyDir(t *testing.T) {
	if _, err := newServiceAt("", nil); err == nil {
		t.Fatal("empty dir must not produce a service")
	}
}

func TestCorruptStateFailsLoadAndKeepsFile(t *testing.T) {
	cases := []struct {
		name string
		file string
		load func(*store) error
	}{
		{"updates", "updates.json", func(s *store) error { _, err := s.loadUpdates(); return err }},
		{"history", "update_history.json", func(s *store) error { _, err := s.loadHistory(); return err }},
		{"rollbacks", "rollbacks.json", func(s *store) error { _, err := s.loadRollbacks(); return err }},
		{"verifications", "verify.json", func(s *store) error { _, err := s.loadVerifications(); return err }},
		{"journal", "journal.json", func(s *store) error { _, err := s.loadJournals(); return err }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, tc.file)
			const raw = `{"version":1,"data":[{"gameId":`
			if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := tc.load(newStore(dir)); err == nil {
				t.Fatal("corrupt state must not load as empty")
			}
			got, err := os.ReadFile(filepath.Clean(path))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != raw {
				t.Fatalf("file rewritten: %q", got)
			}
		})
	}
}

func TestMissingStateLoadsEmptyAndSaves(t *testing.T) {
	dir := t.TempDir()
	s := newStore(dir)
	list, err := s.loadUpdates()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if list != nil {
		t.Fatalf("updates = %+v, want nil", list)
	}
	if err := s.saveUpdates([]Update{{GameID: "g1"}}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "updates.json")); err != nil {
		t.Fatalf("updates not saved: %v", err)
	}
}

func TestJournalRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := newStore(dir)
	list, err := s.loadJournals()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if list != nil {
		t.Fatalf("journals = %+v, want nil", list)
	}
	want := []SwapJournal{{GameID: "g1", Kind: JournalSwap, InstallDir: "/games/g1", Previous: "/games/g1.previous"}}
	if err := s.saveJournals(want); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := s.loadJournals()
	if err != nil {
		t.Fatalf("load after save: %v", err)
	}
	if len(got) != 1 || got[0].GameID != "g1" || got[0].Kind != JournalSwap {
		t.Fatalf("got %+v", got)
	}
}

func TestStagingDirRejectsEmptyInput(t *testing.T) {
	cases := []struct {
		name       string
		installDir string
		gameID     string
	}{
		{"empty install dir", "", "g1"},
		{"empty game id", filepath.Join(t.TempDir(), "Game"), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := stagingDir(tc.installDir, tc.gameID); err == nil {
				t.Fatalf("stagingDir = %q, want error", got)
			}
		})
	}
}

func TestPreviousDirRejectsEmptyInstallDir(t *testing.T) {
	if got, err := previousDir(""); err == nil {
		t.Fatalf("previousDir = %q, want error", got)
	}
}
