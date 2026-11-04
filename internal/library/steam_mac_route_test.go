package library

import (
	"errors"
	"io/fs"
	"path/filepath"
	"runtime"
	"testing"

	"typhon/internal/steamlink"
)

func TestOnlyTheMacSendsSteamShortcutsThroughTheLauncher(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	if want := runtime.GOOS == "darwin"; s.steamViaLauncher != want {
		t.Fatalf("steamViaLauncher = %v on %s, want %v", s.steamViaLauncher, runtime.GOOS, want)
	}
}

// The Mac Steam cannot start the .exe of a game that lives in a CrossOver
// bottle, so its entry runs the launcher, which brings the game up the way
// the Play button does: own arguments, bottle and all.
func TestAddToSteamOnMacGoesThroughTheLauncher(t *testing.T) {
	s, fake, game := steamService(t)
	s.steamViaLauncher = true
	editGame(t, s, game.ID, func(g *Game) { g.LaunchArgs = []string{"-windowed"} })
	launcher, err := s.launcherPath()
	if err != nil {
		t.Fatal(err)
	}

	if err := s.AddToSteam(game.ID); err != nil {
		t.Fatalf("AddToSteam: %v", err)
	}
	want := steamlink.Shortcut{
		AppID:         steamlink.AppID(game.ID),
		Name:          "Half-Life 2",
		Exe:           launcher,
		StartDir:      filepath.Dir(launcher),
		LaunchOptions: "--play " + game.ID,
	}
	if got := fake.added[0]; got != want {
		t.Fatalf("shortcut = %+v\n want %+v", got, want)
	}
}

// The entry no longer points at the .exe, but adding it still needs the game
// to be there: Steam would otherwise get a shortcut to nothing.
func TestAddToSteamOnMacRefusesAMissingGame(t *testing.T) {
	s, fake, game := steamService(t)
	s.steamViaLauncher = true
	editGame(t, s, game.ID, func(g *Game) { g.Executable = filepath.Join(t.TempDir(), "gone.exe") })

	if err := s.AddToSteam(game.ID); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("AddToSteam err = %v, want a missing executable", err)
	}
	if len(fake.added) != 0 {
		t.Fatalf("Steam got %+v", fake.added)
	}
}

func TestAddProfileToSteamOnMacGoesThroughTheLauncher(t *testing.T) {
	s, game, _ := profileGame(t)
	fake := &fakeSteam{lockProbe: probeFor(s)}
	s.steam = fake
	launcher := filepath.Join(t.TempDir(), "Typhon")
	s.launcherPath = func() (string, error) { return launcher, nil }
	s.steamViaLauncher = true
	v := addProfile(t, s, game, "Mods", "mod.exe")

	if err := s.AddProfileToSteam(game.ID, v.ID); err != nil {
		t.Fatalf("AddProfileToSteam: %v", err)
	}
	got := fake.added[0]
	if got.Exe != launcher || got.LaunchOptions != "--play "+game.ID+" --profile "+v.ID || got.Name != "Half-Life 2 (Mods)" {
		t.Fatalf("profile shortcut = %+v", got)
	}
}

// Reconcile rewrites every entry the launcher owns, so on the Mac it must keep
// them on the launcher route too, or a restart would turn them back into
// .exe targets that the Mac Steam cannot run.
func TestSteamPlanOnMacKeepsEntriesOnTheLauncher(t *testing.T) {
	s, fake, game := steamService(t)
	s.steamViaLauncher = true
	other := addExeGame(t, s, "Other")
	launcher, err := s.launcherPath()
	if err != nil {
		t.Fatal(err)
	}

	if err := s.AddToSteam(other.ID); err != nil {
		t.Fatalf("AddToSteam: %v", err)
	}
	plan := fake.addPlans[0]
	if len(plan.Update) != 2 {
		t.Fatalf("update = %+v, want both games", plan.Update)
	}
	for _, sc := range plan.Update {
		id := game.ID
		if sc.Name == "Other" {
			id = other.ID
		}
		if sc.Exe != launcher || sc.LaunchOptions != "--play "+id || sc.Icon != "" {
			t.Fatalf("plan entry %q = %+v, want the launcher", sc.Name, sc)
		}
	}
}

// {steam} save templates describe Windows games; on the Mac they are resolved
// in the shared Steam bottle only, never in the native Mac Steam.
func TestSaveTemplatesNeverPointIntoTheMacSteam(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the Windows Steam is where the templates point")
	}
	if _, err := saveSteamRoot(); !errors.Is(err, steamlink.ErrUnsupported) {
		t.Fatalf("saveSteamRoot err = %v, want ErrUnsupported", err)
	}
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	root, err := steamRootFor(s.steamRoot)
	if err != nil || root != "" {
		t.Fatalf("steam root for saves = %q, %v; want none", root, err)
	}
}
