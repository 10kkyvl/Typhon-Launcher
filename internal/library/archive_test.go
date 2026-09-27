package library

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRemoveArchiveRollsBackOnSaveFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	s.games = []Game{{ID: "game", Title: "Game", PlaytimeSeconds: 3600}}
	if err := s.persist(); err != nil {
		t.Fatal(err)
	}
	s.path = t.TempDir() // atomic replacement of a directory must fail
	if err := s.RemoveGame("game"); err == nil {
		t.Fatal("expected save failure")
	}
	if len(s.GetGames()) != 1 || len(s.archived) != 0 {
		t.Fatal("partial removal survived save failure")
	}
	restored := mustServiceAt(t, path)
	if len(restored.GetGames()) != 1 {
		t.Fatal("disk changed")
	}
}

func TestSessionEndingAfterRemovalUpdatesArchive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	now := time.Now()
	s.now = func() time.Time { return now }
	s.games = []Game{{ID: "game", Title: "Game", PlaytimeSeconds: 3600}}
	if err := s.RemoveGame("game"); err != nil {
		t.Fatal(err)
	}
	s.finishSession("game", now.Add(-time.Hour))
	restored := mustServiceAt(t, path)
	history := restored.GetHistoryGames()
	if len(history) != 1 || history[0].PlaytimeSeconds != 7200 || len(restored.GetGames()) != 0 {
		t.Fatalf("history=%+v", history)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}
