package install

import (
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/uierr"
)

// PendingRemovals is a look at the journal, not a load of it: a journal it
// could not read is not marked failed, so the removals still refuse and the
// startup's late sweep still runs exactly as without the look.
func TestPendingRemovalsOnlyReadsTheJournal(t *testing.T) {
	dir := t.TempDir()
	journal := filepath.Join(dir, "removals.json")
	mkFile(t, filepath.Join(journal, "keep.txt"), 4)
	s := mustServiceAt(t, dir)

	if _, err := s.PendingRemovals(); uierr.Code(err) != "install.removals_unreadable" {
		t.Fatalf("PendingRemovals on an unreadable journal = %v, want install.removals_unreadable", err)
	}
	if err := os.RemoveAll(journal); err != nil {
		t.Fatal(err)
	}
	staged := filepath.Join(t.TempDir(), "Game"+removingSuffix)
	writeJournal(t, dir, journalEntry(t, staged, "g1", filepath.Join(filepath.Dir(staged), "Game"), true))

	got, err := s.PendingRemovals()
	if err != nil || len(got) != 1 || got[0] != staged {
		t.Fatalf("PendingRemovals = %v, %v; want the journaled folder", got, err)
	}
	if _, err := s.removals.load(); err != nil {
		t.Fatal(err)
	}
	if late, ok := s.removals.takeLate(); ok {
		t.Fatalf("looking at the journal marked it as read late: %v", late)
	}
	got, err = s.PendingRemovals()
	if err != nil || len(got) != 1 || got[0] != staged {
		t.Fatalf("PendingRemovals once loaded = %v, %v", got, err)
	}
}

func TestPendingRemovalsOfAMissingJournalIsEmpty(t *testing.T) {
	s := mustServiceAt(t, t.TempDir())
	got, err := s.PendingRemovals()
	if err != nil || len(got) != 0 {
		t.Fatalf("PendingRemovals = %v, %v; want none", got, err)
	}
}
