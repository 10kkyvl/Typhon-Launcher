package library

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLegacyOwnershipDistinguishesMissingMarkerFromUnavailableVolume(t *testing.T) {
	for _, unavailable := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing_marker", true: "unavailable_volume"}[unavailable], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "library.json")
			s := mustServiceAt(t, path)
			dir, exe := discoveredDir(t, "Legacy")
			g, err := s.AddGame(exe, "Legacy")
			if err != nil {
				t.Fatal(err)
			}
			s.games[0].Owned = true
			s.games[0].InstallType = ""
			s.games[0].CanonicalGameID = "legacy"
			if err := s.persist(); err != nil {
				t.Fatal(err)
			}
			if unavailable {
				if err := os.Rename(dir, dir+"-disconnected"); err != nil {
					t.Fatal(err)
				}
			}
			for i := 0; i < 2; i++ {
				s = mustServiceAt(t, path)
				got, err := s.Find(g.ID)
				if err != nil || got.Owned != unavailable {
					t.Fatalf("ownership = %v, error = %v; unavailable=%v", got.Owned, err, unavailable)
				}
				if snap := s.SyncSnapshot(); len(snap) != 1 || snap[0].Owned != unavailable {
					t.Fatalf("sync snapshot = %+v", snap)
				}
				if err := s.persist(); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
