package library

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoteRemovalPreservesLocalInstallation(t *testing.T) {
	for _, offline := range []bool{false, true} {
		t.Run(map[bool]string{false: "live", true: "unavailable"}[offline], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "library.json")
			s := mustServiceAt(t, path)
			dir, exe := discoveredDir(t, "Installed")
			g, err := s.RegisterInstalled(InstalledGame{Title: "Installed", CanonicalGameID: "catalog-game", InstallDir: dir, Executable: exe, InstallType: "portable"})
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if offline {
				if err := os.Rename(dir, dir+"-unavailable"); err != nil {
					t.Fatal(err)
				}
			}
			for i := 0; i < 2; i++ {
				if err := s.RemoveSyncedGame("catalog-game"); err != nil {
					t.Fatal(err)
				}
			}
			if got, err := s.Find(g.ID); err != nil || got.InstallDir != dir || got.Executable != exe {
				t.Fatalf("installation = %+v, %v", got, err)
			}
			if len(s.excluded) != 0 || len(s.archived) != 0 {
				t.Fatal("remote removal excluded or archived local installation")
			}
			after, err := os.ReadFile(path)
			if err != nil || string(before) != string(after) {
				t.Fatalf("remote removal modified library.json: %v", err)
			}
			fresh := mustServiceAt(t, path)
			if got, err := fresh.Find(g.ID); err != nil || got.InstallDir != dir {
				t.Fatalf("installation after restart = %+v, %v", got, err)
			}
		})
	}
}

func TestRemoteRemovalStillArchivesCloudOnlyCard(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	g, err := s.AddCatalogGame("cloud-game", "Cloud", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveSyncedGame("cloud-game"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Find(g.ID); err == nil {
		t.Fatal("cloud-only card remained in library")
	}
	if len(s.archived) != 1 || len(s.excluded) != 0 {
		t.Fatal("cloud card removal did not preserve archive/discovery semantics")
	}
}
