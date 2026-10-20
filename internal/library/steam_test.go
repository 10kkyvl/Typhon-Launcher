package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"typhon/internal/steamlink"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type reconcileCall struct {
	update []steamlink.Shortcut
	drop   []uint32
}

type fakeSteam struct {
	unsupported  bool
	addErr       error
	removeErr    error
	reconcileErr error
	containsErr  error
	present      bool
	lockProbe    func() bool

	added      []steamlink.Shortcut
	arts       []steamlink.Art
	addPlans   []steamlink.Sync
	removed    []uint32
	removePlan []steamlink.Sync
	reconciled []reconcileCall
	checked    []uint32
	lockHeld   bool
}

func (f *fakeSteam) Supported() bool { return !f.unsupported }

func (f *fakeSteam) probe() {
	if f.lockProbe != nil && !f.lockProbe() {
		f.lockHeld = true
	}
}

func (f *fakeSteam) Add(_ context.Context, sc steamlink.Shortcut, art steamlink.Art, plan steamlink.Sync) error {
	f.probe()
	if f.addErr != nil {
		return f.addErr
	}
	f.added = append(f.added, sc)
	f.arts = append(f.arts, art)
	f.addPlans = append(f.addPlans, plan)
	return nil
}

func (f *fakeSteam) Remove(_ context.Context, appID uint32, plan steamlink.Sync) error {
	f.probe()
	f.removed = append(f.removed, appID)
	f.removePlan = append(f.removePlan, plan)
	return f.removeErr
}

func (f *fakeSteam) Reconcile(_ context.Context, update []steamlink.Shortcut, drop []uint32) error {
	f.probe()
	f.reconciled = append(f.reconciled, reconcileCall{update: update, drop: drop})
	return f.reconcileErr
}

func (f *fakeSteam) Contains(_ context.Context, appID uint32) (bool, error) {
	f.probe()
	f.checked = append(f.checked, appID)
	return f.present, f.containsErr
}

// stateSteam keeps the shortcuts a real Steam would hold and refuses to write
// while "running", which is the behaviour the library has to live with.
type stateSteam struct {
	fakeSteam
	running bool
	entries map[uint32]steamlink.Shortcut
}

func newStateSteam() *stateSteam {
	return &stateSteam{entries: map[uint32]steamlink.Shortcut{}}
}

func (f *stateSteam) changes(plan steamlink.Sync) bool {
	for _, id := range plan.Drop {
		if _, ok := f.entries[id]; ok {
			return true
		}
	}
	for _, sc := range plan.Update {
		if cur, ok := f.entries[sc.AppID]; ok && cur != sc {
			return true
		}
	}
	return false
}

func (f *stateSteam) apply(plan steamlink.Sync) {
	for _, id := range plan.Drop {
		delete(f.entries, id)
	}
	for _, sc := range plan.Update {
		if _, ok := f.entries[sc.AppID]; ok {
			f.entries[sc.AppID] = sc
		}
	}
}

func (f *stateSteam) Add(ctx context.Context, sc steamlink.Shortcut, art steamlink.Art, plan steamlink.Sync) error {
	if f.running {
		return steamlink.ErrSteamRunning
	}
	if err := f.fakeSteam.Add(ctx, sc, art, plan); err != nil {
		return err
	}
	f.apply(plan)
	f.entries[sc.AppID] = sc
	return nil
}

func (f *stateSteam) Remove(ctx context.Context, appID uint32, plan steamlink.Sync) error {
	plan.Drop = append(slices.Clone(plan.Drop), appID)
	if f.changes(plan) && f.running {
		return steamlink.ErrSteamRunning
	}
	f.apply(plan)
	return f.fakeSteam.Remove(ctx, appID, plan)
}

func (f *stateSteam) Reconcile(ctx context.Context, update []steamlink.Shortcut, drop []uint32) error {
	plan := steamlink.Sync{Update: update, Drop: drop}
	if err := f.fakeSteam.Reconcile(ctx, update, drop); err != nil {
		return err
	}
	if !f.changes(plan) {
		return nil
	}
	if f.running {
		return steamlink.ErrSteamRunning
	}
	f.apply(plan)
	return nil
}

func (f *stateSteam) Contains(ctx context.Context, appID uint32) (bool, error) {
	if _, err := f.fakeSteam.Contains(ctx, appID); err != nil {
		return false, err
	}
	_, ok := f.entries[appID]
	return ok, nil
}

func probeFor(s *Service) func() bool {
	return func() bool {
		if !s.mu.TryLock() {
			return false
		}
		s.mu.Unlock()
		return true
	}
}

func steamService(t *testing.T) (*Service, *fakeSteam, Game) {
	t.Helper()
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	fake := &fakeSteam{lockProbe: probeFor(s)}
	s.steam = fake
	launcher := filepath.Join(t.TempDir(), "typhon.exe")
	s.launcherPath = func() (string, error) { return launcher, nil }
	game, err := s.AddGame(t.Context(), tempGameExe(t), "Half-Life 2")
	if err != nil {
		t.Fatalf("add game: %v", err)
	}
	return s, fake, game
}

func editGame(t *testing.T, s *Service, id string, edit func(*Game)) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.findLocked(id)
	if g == nil {
		t.Fatalf("game %s not found", id)
	}
	edit(g)
}

func TestServiceStartsWithSteamDisabled(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	if s.steam.Supported() {
		t.Fatal("a fresh service talks to the real Steam")
	}
	if s.steamSync {
		t.Fatal("a fresh service reconciles at startup")
	}
	game, err := s.AddGame(t.Context(), tempGameExe(t), "Game")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddToSteam(game.ID); !errors.Is(err, steamlink.ErrUnsupported) {
		t.Fatalf("AddToSteam err = %v, want ErrUnsupported", err)
	}
	status, err := s.SteamStatus(game.ID)
	if err != nil || status != (SteamStatus{}) {
		t.Fatalf("status = %+v, %v", status, err)
	}
	s.EnableSteam()
	if _, ok := s.steam.(systemSteam); !ok || !s.steamSync {
		t.Fatalf("EnableSteam left steam = %T, sync = %v", s.steam, s.steamSync)
	}
	s.steam = disabledSteam{}
}

func TestAddToSteamLaunchesTheGameDirectly(t *testing.T) {
	s, fake, game := steamService(t)
	editGame(t, s, game.ID, func(g *Game) { g.LaunchArgs = []string{"-windowed", "-w", "1280"} })

	if err := s.AddToSteam(game.ID); err != nil {
		t.Fatalf("AddToSteam: %v", err)
	}
	if len(fake.added) != 1 {
		t.Fatalf("Add called %d times", len(fake.added))
	}
	got := fake.added[0]
	want := steamlink.Shortcut{
		AppID:         steamlink.AppID(game.ID),
		Name:          "Half-Life 2",
		Exe:           game.Executable,
		StartDir:      filepath.Dir(game.Executable),
		Icon:          game.Executable,
		LaunchOptions: steamlink.JoinArgs([]string{"-windowed", "-w", "1280"}),
	}
	if got != want {
		t.Fatalf("shortcut = %+v\n want %+v", got, want)
	}
	if fake.lockHeld {
		t.Fatal("Steam was called with the library mutex held")
	}
}

func TestAddToSteamRunAsAdminGoesThroughTheLauncher(t *testing.T) {
	s, fake, game := steamService(t)
	editGame(t, s, game.ID, func(g *Game) {
		g.RunAsAdmin = true
		g.LaunchArgs = []string{"-ignored"}
	})
	launcher, err := s.launcherPath()
	if err != nil {
		t.Fatal(err)
	}

	if err := s.AddToSteam(game.ID); err != nil {
		t.Fatalf("AddToSteam: %v", err)
	}
	got := fake.added[0]
	if got.Exe != launcher || got.StartDir != filepath.Dir(launcher) || got.LaunchOptions != "--play "+game.ID {
		t.Fatalf("shortcut = %+v", got)
	}
	if got.Icon != game.Executable {
		t.Fatalf("icon = %q, want the game executable", got.Icon)
	}
}

func TestAddToSteamRefusals(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*testing.T, *Service, *fakeSteam, *Game) string
		want  error
	}{
		{"unknown game", func(_ *testing.T, _ *Service, _ *fakeSteam, _ *Game) string { return "nope" }, errNotFound},
		{"uninstalled", func(t *testing.T, s *Service, _ *fakeSteam, g *Game) string {
			editGame(t, s, g.ID, func(x *Game) { x.Uninstalled = true })
			return g.ID
		}, errShortcutUninstalled},
		{"already a steam game", func(t *testing.T, s *Service, _ *fakeSteam, g *Game) string {
			editGame(t, s, g.ID, func(x *Game) { x.External = &External{Provider: ProviderSteam, AppID: "42"} })
			return g.ID
		}, errSteamExternal},
		{"no executable", func(t *testing.T, s *Service, _ *fakeSteam, g *Game) string {
			editGame(t, s, g.ID, func(x *Game) { x.Executable = "" })
			return g.ID
		}, errShortcutNoExecutable},
		{"executable gone", func(t *testing.T, _ *Service, _ *fakeSteam, g *Game) string {
			if err := os.Remove(g.Executable); err != nil {
				t.Fatal(err)
			}
			return g.ID
		}, os.ErrNotExist},
		{"unsupported platform", func(_ *testing.T, _ *Service, f *fakeSteam, g *Game) string {
			f.unsupported = true
			return g.ID
		}, steamlink.ErrUnsupported},
		{"id unusable on a command line", func(t *testing.T, s *Service, _ *fakeSteam, g *Game) string {
			editGame(t, s, g.ID, func(x *Game) {
				x.ID = "bad id"
				x.RunAsAdmin = true
			})
			return "bad id"
		}, errShortcutBadID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, fake, game := steamService(t)
			id := tc.setup(t, s, fake, &game)
			if err := s.AddToSteam(id); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if len(fake.added) != 0 {
				t.Fatal("Steam was called despite the refusal")
			}
		})
	}
}

func TestAddToSteamBackendErrorIsReturned(t *testing.T) {
	s, fake, game := steamService(t)
	fake.addErr = steamlink.ErrSteamRunning
	if err := s.AddToSteam(game.ID); !errors.Is(err, steamlink.ErrSteamRunning) {
		t.Fatalf("err = %v, want ErrSteamRunning", err)
	}
}

func TestAddToSteamArt(t *testing.T) {
	t.Run("art of the catalog game is passed on", func(t *testing.T) {
		s, fake, game := steamService(t)
		editGame(t, s, game.ID, func(g *Game) { g.CanonicalGameID = "cat-7" })
		var asked []string
		want := steamlink.Art{Cover: "/c.png", Hero: "/h.jpg"}
		s.SetSteamArt(func(id string) (steamlink.Art, error) {
			asked = append(asked, id)
			return want, nil
		})
		if err := s.AddToSteam(game.ID); err != nil {
			t.Fatal(err)
		}
		if len(asked) != 1 || asked[0] != "cat-7" || fake.arts[0] != want {
			t.Fatalf("asked %v, art %+v", asked, fake.arts)
		}
	})

	t.Run("port failure is returned and nothing is added", func(t *testing.T) {
		s, fake, game := steamService(t)
		editGame(t, s, game.ID, func(g *Game) { g.CanonicalGameID = "cat-7" })
		boom := errors.New("catalog unreadable")
		s.SetSteamArt(func(string) (steamlink.Art, error) { return steamlink.Art{}, boom })
		if err := s.AddToSteam(game.ID); !errors.Is(err, boom) {
			t.Fatalf("err = %v, want the port error", err)
		}
		if len(fake.added) != 0 {
			t.Fatal("shortcut added without art after the port failed")
		}
	})

	t.Run("game outside the catalog skips the port", func(t *testing.T) {
		s, fake, game := steamService(t)
		s.SetSteamArt(func(string) (steamlink.Art, error) {
			t.Error("port called for a game without a catalog id")
			return steamlink.Art{}, nil
		})
		if err := s.AddToSteam(game.ID); err != nil {
			t.Fatal(err)
		}
		if fake.arts[0] != (steamlink.Art{}) {
			t.Fatalf("art = %+v", fake.arts[0])
		}
	})
}

func TestSteamStatus(t *testing.T) {
	cases := []struct {
		name    string
		fake    fakeSteam
		want    SteamStatus
		wantErr error
	}{
		{"unsupported", fakeSteam{unsupported: true}, SteamStatus{}, nil},
		{"steam not installed is a state", fakeSteam{containsErr: steamlink.ErrNotInstalled}, SteamStatus{Supported: true}, nil},
		{"installed, not added", fakeSteam{}, SteamStatus{Supported: true, Installed: true}, nil},
		{"installed and added", fakeSteam{present: true}, SteamStatus{Supported: true, Installed: true, Added: true}, nil},
		{"other failure is an error", fakeSteam{containsErr: os.ErrPermission}, SteamStatus{}, os.ErrPermission},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, _, game := steamService(t)
			fake := tc.fake
			s.steam = &fake
			got, err := s.SteamStatus(game.ID)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("status = %+v, want %+v", got, tc.want)
			}
			if len(fake.checked) > 0 && fake.checked[0] != steamlink.AppID(game.ID) {
				t.Fatalf("asked about %d", fake.checked[0])
			}
		})
	}
}

func TestRemoveFromSteam(t *testing.T) {
	s, fake, game := steamService(t)
	if err := s.RemoveFromSteam(game.ID); err != nil {
		t.Fatal(err)
	}
	if len(fake.removed) != 1 || fake.removed[0] != steamlink.AppID(game.ID) {
		t.Fatalf("removed = %v", fake.removed)
	}
	fake.removeErr = steamlink.ErrSteamRunning
	if err := s.RemoveFromSteam(game.ID); !errors.Is(err, steamlink.ErrSteamRunning) {
		t.Fatalf("err = %v, want ErrSteamRunning", err)
	}
	fake.unsupported = true
	if err := s.RemoveFromSteam(game.ID); !errors.Is(err, steamlink.ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
}

type dropper struct {
	name string
	run  func(*Service, string) error
	gone func(*Service, string) bool
}

func droppers() []dropper {
	return []dropper{
		{
			name: "RemoveGame",
			run:  func(s *Service, id string) error { return s.RemoveGame(id) },
			gone: func(s *Service, id string) bool { return s.findLocked(id) == nil },
		},
		{
			name: "MarkUninstalled",
			run:  func(s *Service, id string) error { return s.MarkUninstalled(id) },
			gone: func(s *Service, id string) bool { g := s.findLocked(id); return g != nil && g.Uninstalled },
		},
	}
}

func TestGameRemovalDropsTheSteamShortcut(t *testing.T) {
	for _, d := range droppers() {
		cases := []struct {
			name       string
			fake       fakeSteam
			wantCalled bool
		}{
			{"reconciled", fakeSteam{}, true},
			{"steam not installed", fakeSteam{reconcileErr: steamlink.ErrNotInstalled}, true},
			{"steam running keeps the removal", fakeSteam{reconcileErr: steamlink.ErrSteamRunning}, true},
			{"other failure keeps the removal", fakeSteam{reconcileErr: errors.New("damaged vdf")}, true},
			{"unsupported platform", fakeSteam{unsupported: true}, false},
		}
		for _, tc := range cases {
			t.Run(d.name+"/"+tc.name, func(t *testing.T) {
				s, _, game := steamService(t)
				fake := tc.fake
				fake.lockProbe = probeFor(s)
				s.steam = &fake

				if err := d.run(s, game.ID); err != nil {
					t.Fatalf("%s: %v", d.name, err)
				}
				s.mu.Lock()
				gone := d.gone(s, game.ID)
				s.mu.Unlock()
				if !gone {
					t.Fatal("game was not removed")
				}
				if !tc.wantCalled {
					if len(fake.reconciled) != 0 {
						t.Fatalf("Steam called: %+v", fake.reconciled)
					}
					return
				}
				if len(fake.reconciled) != 1 {
					t.Fatalf("Reconcile calls = %d, want 1", len(fake.reconciled))
				}
				call := fake.reconciled[0]
				if len(call.update) != 0 || !slices.Equal(call.drop, []uint32{steamlink.AppID(game.ID)}) {
					t.Fatalf("Reconcile(%+v, %v), want a drop of this game only", call.update, call.drop)
				}
				if len(fake.checked) != 0 || len(fake.removed) != 0 {
					t.Fatalf("lookup and removal were done separately: checked %v removed %v", fake.checked, fake.removed)
				}
				if fake.lockHeld {
					t.Fatal("Steam was called with the library mutex held")
				}
			})
		}
	}
}

func TestFailedRemovalLeavesTheSteamShortcut(t *testing.T) {
	for _, d := range droppers() {
		t.Run(d.name, func(t *testing.T) {
			s, fake, _ := steamService(t)
			if err := d.run(s, "nope"); !errors.Is(err, errNotFound) {
				t.Fatalf("err = %v, want errNotFound", err)
			}
			if len(fake.reconciled) != 0 {
				t.Fatalf("Steam touched: %+v", fake.reconciled)
			}
		})
	}
}

func addExeGame(t *testing.T, s *Service, title string) Game {
	t.Helper()
	game, err := s.AddGame(t.Context(), tempGameExe(t), title)
	if err != nil {
		t.Fatalf("add game: %v", err)
	}
	return game
}

func TestSteamPlanFollowsTheLibraryState(t *testing.T) {
	s, fake, first := steamService(t)
	admin := addExeGame(t, s, "Admin")
	editGame(t, s, admin.ID, func(g *Game) { g.RunAsAdmin = true })
	noExe := addExeGame(t, s, "No exe")
	editGame(t, s, noExe.ID, func(g *Game) { g.Executable = "" })
	nul := addExeGame(t, s, "Has NUL")
	editGame(t, s, nul.ID, func(g *Game) { g.Title = "bad\x00title" })
	steamOwned := addExeGame(t, s, "Steam owned")
	editGame(t, s, steamOwned.ID, func(g *Game) { g.External = &External{Provider: ProviderSteam, AppID: "1"} })
	uninstalled := addExeGame(t, s, "Uninstalled")
	archived := addExeGame(t, s, "Archived")
	if err := s.MarkUninstalled(uninstalled.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveGame(archived.ID); err != nil {
		t.Fatal(err)
	}
	fake.reconciled = nil

	target := addExeGame(t, s, "Target")
	if err := s.AddToSteam(target.ID); err != nil {
		t.Fatalf("AddToSteam: %v", err)
	}
	plan := fake.addPlans[0]

	var updated []string
	for _, sc := range plan.Update {
		updated = append(updated, sc.Name)
	}
	slices.Sort(updated)
	if want := []string{"Admin", "Half-Life 2", "Target"}; !slices.Equal(updated, want) {
		t.Fatalf("update = %v, want %v", updated, want)
	}
	for _, sc := range plan.Update {
		if sc.Name == "Admin" && sc.LaunchOptions != "--play "+admin.ID {
			t.Fatalf("admin game updated with %+v", sc)
		}
	}
	wantDrop := []uint32{steamlink.AppID(uninstalled.ID), steamlink.AppID(archived.ID)}
	slices.Sort(wantDrop)
	gotDrop := slices.Clone(plan.Drop)
	slices.Sort(gotDrop)
	if !slices.Equal(gotDrop, wantDrop) {
		t.Fatalf("drop = %v, want %v", gotDrop, wantDrop)
	}
	for _, id := range []string{first.ID, admin.ID, target.ID} {
		if slices.Contains(plan.Drop, steamlink.AppID(id)) {
			t.Fatalf("live game %s is in the drop list", id)
		}
	}
}

func TestStaleEntryLeftByRunningSteamIsRemovedByTheNextAdd(t *testing.T) {
	s, _, gone := steamService(t)
	fake := newStateSteam()
	fake.lockProbe = probeFor(s)
	s.steam = fake
	other := addExeGame(t, s, "Other")

	if err := s.AddToSteam(gone.ID); err != nil {
		t.Fatal(err)
	}
	fake.running = true
	if err := s.RemoveGame(gone.ID); err != nil {
		t.Fatalf("RemoveGame: %v", err)
	}
	if _, ok := fake.entries[steamlink.AppID(gone.ID)]; !ok {
		t.Fatal("the entry vanished while Steam was running")
	}

	fake.running = false
	if err := s.AddToSteam(other.ID); err != nil {
		t.Fatalf("AddToSteam: %v", err)
	}
	if _, ok := fake.entries[steamlink.AppID(gone.ID)]; ok {
		t.Fatal("the orphan survived the next add")
	}
	if _, ok := fake.entries[steamlink.AppID(other.ID)]; !ok {
		t.Fatal("the new entry is missing")
	}
}

func TestRelocateRefreshesExistingEntryOnly(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	fake := newStateSteam()
	fake.lockProbe = probeFor(s)
	s.steam = fake
	moved, oldDir := addRelocateGame(t, s)
	notInSteam, _ := addRelocateGame(t, s)

	sc, err := steamTarget(moved, s.launcherPath)
	if err != nil {
		t.Fatal(err)
	}
	fake.entries[sc.AppID] = sc

	newDir := filepath.Join(t.TempDir(), "new")
	if _, err := s.Relocate(moved.ID, newDir); err != nil {
		t.Fatalf("Relocate: %v", err)
	}
	got, ok := fake.entries[sc.AppID]
	if !ok {
		t.Fatal("entry disappeared")
	}
	if want := filepath.Join(newDir, "game.exe"); got.Exe != want || got.StartDir != newDir {
		t.Fatalf("entry = %+v, want exe %q in %q", got, want, newDir)
	}
	if got.Exe == filepath.Join(oldDir, "game.exe") {
		t.Fatal("entry still points at the old directory")
	}
	if _, ok := fake.entries[steamlink.AppID(notInSteam.ID)]; ok {
		t.Fatal("a game that was never added to Steam got an entry")
	}
	if len(fake.added) != 0 {
		t.Fatal("relocate added a shortcut")
	}
}

func TestStartupReconcile(t *testing.T) {
	t.Run("runs once in the background when enabled", func(t *testing.T) {
		s, fake, game := steamService(t)
		fake.reconcileErr = steamlink.ErrSteamRunning
		s.steamSync = true
		gone := addExeGame(t, s, "Gone")
		if err := s.MarkUninstalled(gone.ID); err != nil {
			t.Fatal(err)
		}
		fake.reconciled = nil

		if err := s.ServiceStartup(t.Context(), application.ServiceOptions{}); err != nil {
			t.Fatalf("ServiceStartup: %v", err)
		}
		if err := s.ServiceShutdown(); err != nil {
			t.Fatal(err)
		}
		if len(fake.reconciled) != 1 {
			t.Fatalf("Reconcile calls = %d, want 1", len(fake.reconciled))
		}
		call := fake.reconciled[0]
		if !slices.Equal(call.drop, []uint32{steamlink.AppID(gone.ID)}) || len(call.update) != 1 || call.update[0].AppID != steamlink.AppID(game.ID) {
			t.Fatalf("Reconcile(%+v, %v)", call.update, call.drop)
		}
	})

	t.Run("does nothing unless enabled", func(t *testing.T) {
		s, fake, _ := steamService(t)
		if err := s.ServiceStartup(t.Context(), application.ServiceOptions{}); err != nil {
			t.Fatalf("ServiceStartup: %v", err)
		}
		if err := s.ServiceShutdown(); err != nil {
			t.Fatal(err)
		}
		if len(fake.reconciled) != 0 {
			t.Fatalf("Steam touched: %+v", fake.reconciled)
		}
	})

	t.Run("not started after shutdown", func(t *testing.T) {
		s, fake, _ := steamService(t)
		s.steamSync = true
		if err := s.ServiceShutdown(); err != nil {
			t.Fatal(err)
		}
		s.startSteamSync(t.Context())
		if len(fake.reconciled) != 0 {
			t.Fatalf("Steam touched after shutdown: %+v", fake.reconciled)
		}
	})
}

func TestReconcileLeavesUnchangedSteamAlone(t *testing.T) {
	s, _, _ := steamService(t)
	fake := newStateSteam()
	fake.lockProbe = probeFor(s)
	s.steam = fake
	fake.running = true
	s.reconcileSteam(t.Context(), "test")
	if len(fake.entries) != 0 || len(fake.reconciled) != 1 {
		t.Fatalf("entries %v, calls %d", fake.entries, len(fake.reconciled))
	}
}
