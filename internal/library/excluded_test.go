package library

import (
	"os"
	"path/filepath"
	"testing"
)

func discoveredDir(t *testing.T, name string) (string, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	exe := filepath.Join(dir, name+".exe")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir, exe
}

func TestRemoveGameExcludesDirFromDiscovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	dir, exe := discoveredDir(t, "Found")

	game, _, err := s.ApplyDiscovered(Discovered{Title: "Found", Executable: exe, InstallDir: dir})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if err := s.RemoveGame(game.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}

	_, outcome, err := s.ApplyDiscovered(Discovered{Title: "Found", Executable: exe, InstallDir: dir})
	if err != nil {
		t.Fatalf("rediscover: %v", err)
	}
	if outcome != OutcomeIgnored {
		t.Fatalf("outcome = %q", outcome)
	}
	if games := s.GetInstalledGames(); len(games) != 0 {
		t.Fatalf("games = %+v", games)
	}

	reloaded := mustServiceAt(t, path)
	_, outcome, err = reloaded.ApplyDiscovered(Discovered{Title: "Found", Executable: exe, InstallDir: dir})
	if err != nil {
		t.Fatalf("rediscover after reload: %v", err)
	}
	if outcome != OutcomeIgnored {
		t.Fatalf("outcome after reload = %q", outcome)
	}
}

func TestRegisterInstalledClearsExclusion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	dir, exe := discoveredDir(t, "Reinstalled")

	game, _, err := s.ApplyDiscovered(Discovered{Title: "Reinstalled", Executable: exe, InstallDir: dir})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if err := s.RemoveGame(game.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}

	if _, err := s.RegisterInstalled(InstalledGame{
		Title:       "Reinstalled",
		Executable:  exe,
		InstallDir:  dir,
		InstallType: "portable",
		Owned:       true,
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	_, outcome, err := s.ApplyDiscovered(Discovered{Title: "Reinstalled", Executable: exe, InstallDir: dir})
	if err != nil {
		t.Fatalf("rediscover: %v", err)
	}
	if outcome == OutcomeIgnored {
		t.Fatal("reinstalled game stayed excluded")
	}
}

func TestRemoveGameKeepsExclusionsEmptyWhenDirGone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	dir, exe := discoveredDir(t, "Vanished")

	game, _, err := s.ApplyDiscovered(Discovered{Title: "Vanished", Executable: exe, InstallDir: dir})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("remove dir: %v", err)
	}
	if err := s.RemoveGame(game.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}

	s.mu.Lock()
	excluded := len(s.excluded)
	s.mu.Unlock()
	if excluded != 0 {
		t.Fatalf("excluded = %d", excluded)
	}
}

func TestRegisterInstalledStoresRemovalFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	dir, exe := discoveredDir(t, "Installed")

	game, err := s.RegisterInstalled(InstalledGame{
		Title:       "Installed",
		Executable:  exe,
		InstallDir:  dir,
		InstallType: "exe_installer",
		Uninstall:   Uninstall{Command: "unins000.exe", Key: `HKLM\Installed`},
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if game.InstallType != "exe_installer" || game.Uninstall.Command != "unins000.exe" {
		t.Fatalf("game = %+v", game)
	}

	reloaded := mustServiceAt(t, path).GetInstalledGames()
	if len(reloaded) != 1 {
		t.Fatalf("reloaded = %+v", reloaded)
	}
	if reloaded[0].Uninstall.Key != `HKLM\Installed` {
		t.Fatalf("uninstall = %+v", reloaded[0].Uninstall)
	}

	marker, err := ReadMarker(dir)
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}
	if marker.InstallType != "exe_installer" || marker.Uninstall.Command != "unins000.exe" {
		t.Fatalf("marker = %+v", marker)
	}
}
