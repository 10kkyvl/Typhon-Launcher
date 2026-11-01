package discovery

import (
	"path/filepath"
	"testing"

	"typhon/internal/library"
)

func TestScanSkipsInterruptedLanTransfer(t *testing.T) {
	f := newFixture(t)
	dir := installedGame(t, f.root, ".typhon-receiving-0123456789abcdef")
	if err := library.WriteMarker(dir, library.Marker{
		GameID:     "sender-game",
		Title:      "Foreign",
		Executable: filepath.Base(dir) + ".exe",
	}); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	result := f.scan(t)

	if result.Candidates != 0 || result.Added != 0 {
		t.Fatalf("result = %+v, want the receiving folder unseen", result)
	}
	if games := f.lib.GetInstalledGames(); len(games) != 0 {
		t.Fatalf("scan adopted an interrupted transfer: %+v", games)
	}
}
