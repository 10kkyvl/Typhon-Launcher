package library

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"typhon/internal/steamlink"
)

func profileGame(t *testing.T) (*Service, Game, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	game, err := s.AddGame(tempGameExe(t), "Half-Life 2")
	if err != nil {
		t.Fatal(err)
	}
	return s, game, path
}

func writeExe(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func addProfile(t *testing.T, s *Service, g Game, name, rel string) ProfileView {
	t.Helper()
	exe := writeExe(t, g.InstallDir, rel)
	v, err := s.AddProfile(g.ID, ProfileInput{Name: name, Executable: exe, ArgsLine: `-windowed "-x y"`})
	if err != nil {
		t.Fatalf("add profile: %v", err)
	}
	return v
}

func TestAddProfileStoresRelativeExecutable(t *testing.T) {
	s, game, path := profileGame(t)
	v := addProfile(t, s, game, "Mods", "bin/mod.exe")
	if len(v.ID) != 8 || !SafeProfileID(v.ID) {
		t.Fatalf("profile id = %q", v.ID)
	}
	if v.Executable != filepath.Join(game.InstallDir, "bin", "mod.exe") {
		t.Fatalf("view executable = %q", v.Executable)
	}
	var raw []struct {
		Profiles []struct {
			Executable string   `json:"executable"`
			Args       []string `json:"args"`
		} `json:"profiles"`
	}
	if err := json.Unmarshal(readFile(t, path), &raw); err != nil {
		t.Fatal(err)
	}
	if got := raw[0].Profiles[0].Executable; got != "bin/mod.exe" {
		t.Fatalf("stored executable = %q, want relative slash path", got)
	}
	if len(raw[0].Profiles[0].Args) != 2 || raw[0].Profiles[0].Args[1] != "-x y" {
		t.Fatalf("args = %v", raw[0].Profiles[0].Args)
	}
	views, err := s.Profiles(game.ID)
	if err != nil || len(views) != 2 || !views[0].IsDefault || !views[0].IsLast || views[0].Name != "Основной" {
		t.Fatalf("profiles = %+v, %v", views, err)
	}
}

func TestProfileValidation(t *testing.T) {
	s, game, _ := profileGame(t)
	inside := writeExe(t, game.InstallDir, "mod.exe")
	outsideDir := t.TempDir()
	outside := writeExe(t, outsideDir, "other.exe")
	dirPath := filepath.Join(game.InstallDir, "sub")
	if err := os.Mkdir(dirPath, 0o755); err != nil {
		t.Fatal(err)
	}
	addProfile(t, s, game, "Taken", "taken.exe")

	cases := []struct {
		name string
		in   ProfileInput
		want error
	}{
		{"outside", ProfileInput{Name: "A", Executable: outside}, errProfileOutside},
		{"dotdot", ProfileInput{Name: "A", Executable: filepath.Join(game.InstallDir, "..", filepath.Base(outsideDir), "other.exe")}, errProfileOutside},
		{"missing", ProfileInput{Name: "A", Executable: filepath.Join(game.InstallDir, "nope.exe")}, errProfileNotFile},
		{"directory", ProfileInput{Name: "A", Executable: dirPath}, errProfileNotFile},
		{"relative", ProfileInput{Name: "A", Executable: "mod.exe"}, errProfileNotFile},
		{"empty exe", ProfileInput{Name: "A"}, errProfileNotFile},
		{"empty name", ProfileInput{Name: "  ", Executable: inside}, errProfileName},
		{"long name", ProfileInput{Name: strings.Repeat("я", 41), Executable: inside}, errProfileName},
		{"control name", ProfileInput{Name: "a\x00b", Executable: inside}, errProfileName},
		{"duplicate", ProfileInput{Name: "taken", Executable: inside}, errProfileNameTaken},
		{"bad args", ProfileInput{Name: "A", Executable: inside, ArgsLine: `"unclosed`}, errLaunchArgsInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := s.AddProfile(game.ID, tc.in); !errors.Is(err, tc.want) {
				t.Fatalf("AddProfile err = %v, want %v", err, tc.want)
			}
		})
	}
	views := mustProfiles(t, s, game.ID)
	if len(views) != 2 {
		t.Fatalf("rejected input changed the profile list: %+v", views)
	}
}

func TestProfileSymlinkOutsideRejected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Log("symlink creation needs a privilege on Windows; covered on other platforms")
		return
	}
	s, game, _ := profileGame(t)
	target := writeExe(t, t.TempDir(), "real.exe")
	link := filepath.Join(game.InstallDir, "link.exe")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddProfile(game.ID, ProfileInput{Name: "L", Executable: link}); !errors.Is(err, errProfileOutside) {
		t.Fatalf("err = %v, want errProfileOutside", err)
	}
}

func TestProfileEmptyInstallDirRejected(t *testing.T) {
	s, game, _ := profileGame(t)
	s.mu.Lock()
	s.findLocked(game.ID).InstallDir = ""
	s.mu.Unlock()
	if _, err := s.AddProfile(game.ID, ProfileInput{Name: "A", Executable: game.Executable}); !errors.Is(err, errProfileNoInstallDir) {
		t.Fatalf("err = %v, want errProfileNoInstallDir", err)
	}
}

func TestProfileLimit(t *testing.T) {
	s, game, _ := profileGame(t)
	for i := 0; i < maxProfiles; i++ {
		n := "p" + strconv.Itoa(i)
		addProfile(t, s, game, n, n+".exe")
	}
	exe := writeExe(t, game.InstallDir, "extra.exe")
	if _, err := s.AddProfile(game.ID, ProfileInput{Name: "extra", Executable: exe}); !errors.Is(err, errProfileLimit) {
		t.Fatalf("err = %v, want errProfileLimit", err)
	}
}

func TestUpdateRemoveProfile(t *testing.T) {
	s, game, _ := profileGame(t)
	v := addProfile(t, s, game, "Mods", "mod.exe")
	other := addProfile(t, s, game, "Offline", "off.exe")
	exe := writeExe(t, game.InstallDir, "mod2.exe")

	if err := s.UpdateProfile(game.ID, v.ID, ProfileInput{Name: "offline", Executable: exe}); !errors.Is(err, errProfileNameTaken) {
		t.Fatalf("rename to taken: %v", err)
	}
	if err := s.UpdateProfile(game.ID, "nope", ProfileInput{Name: "x", Executable: exe}); !errors.Is(err, errProfileNotFound) {
		t.Fatalf("unknown profile: %v", err)
	}
	if err := s.UpdateProfile(game.ID, v.ID, ProfileInput{Name: "MODS", Executable: exe, RunAsAdmin: true}); err != nil {
		t.Fatalf("update same name: %v", err)
	}
	views := mustProfiles(t, s, game.ID)
	if views[1].Name != "MODS" || !views[1].RunAsAdmin || !strings.HasSuffix(views[1].Executable, "mod2.exe") {
		t.Fatalf("view = %+v", views[1])
	}
	if err := s.RemoveProfile(game.ID, "nope"); !errors.Is(err, errProfileNotFound) {
		t.Fatalf("remove unknown: %v", err)
	}
	if err := s.RemoveProfile(game.ID, other.ID); err != nil {
		t.Fatal(err)
	}
	views = mustProfiles(t, s, game.ID)
	if len(views) != 2 || views[1].ID != v.ID {
		t.Fatalf("after remove: %+v", views)
	}
}

func TestProfileMutationsRollBackWhenPersistFails(t *testing.T) {
	s, game, path := profileGame(t)
	v := addProfile(t, s, game, "Mods", "mod.exe")
	exe := writeExe(t, game.InstallDir, "x.exe")
	s.mu.Lock()
	s.findLocked(game.ID).LastProfile = v.ID
	s.mu.Unlock()
	s.path = t.TempDir()
	t.Cleanup(func() { s.path = path })

	if _, err := s.AddProfile(game.ID, ProfileInput{Name: "New", Executable: exe}); err == nil {
		t.Fatal("AddProfile must return the save error")
	}
	if err := s.UpdateProfile(game.ID, v.ID, ProfileInput{Name: "Renamed", Executable: exe}); err == nil {
		t.Fatal("UpdateProfile must return the save error")
	}
	if err := s.RemoveProfile(game.ID, v.ID); err == nil {
		t.Fatal("RemoveProfile must return the save error")
	}
	got, err := s.Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Profiles) != 1 || got.Profiles[0].Name != "Mods" || got.Profiles[0].Executable != "mod.exe" || got.LastProfile != v.ID {
		t.Fatalf("memory not rolled back: %+v last=%q", got.Profiles, got.LastProfile)
	}
}

func TestLoadLibraryWithoutProfiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "library.json")
	exe := writeExe(t, dir, "g/game.exe")
	old := `[{"id":"old-1","title":"Old","executable":` + jsonString(t, exe) + `,"installDir":` + jsonString(t, filepath.Dir(exe)) + `,"launchArgs":["-a"],"runAsAdmin":true}]`
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	s := mustServiceAt(t, path)
	views, err := s.Profiles("old-1")
	if err != nil || len(views) != 1 || views[0].ArgsLine != "-a" || !views[0].RunAsAdmin || !views[0].IsLast {
		t.Fatalf("views = %+v, err = %v", views, err)
	}
}

func jsonString(t *testing.T, v string) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func captureLaunches(s *Service) *[]launch {
	var seen []launch
	s.start = func(_ context.Context, req launch) (gameProcess, error) {
		seen = append(seen, req)
		return newRegProcess(len(seen)), nil
	}
	return &seen
}

func stopAndWait(t *testing.T, s *Service, id string) {
	t.Helper()
	if err := s.StopGame(id); err != nil {
		t.Fatal(err)
	}
	s.sessionWG.Wait()
	s.wg.Wait()
	if s.IsRunning(id) {
		t.Fatal("session still running")
	}
}

func TestPlayGameUsesLastProfileAndFallsBack(t *testing.T) {
	s, game, _ := profileGame(t)
	v := addProfile(t, s, game, "Mods", "mod.exe")
	launches := captureLaunches(s)

	if err := s.PlayGame(game.ID); err != nil {
		t.Fatal(err)
	}
	if (*launches)[0].executable != game.Executable {
		t.Fatalf("default launch used %q", (*launches)[0].executable)
	}
	stopAndWait(t, s, game.ID)

	if err := s.PlayGameProfile(game.ID, v.ID); err != nil {
		t.Fatal(err)
	}
	got := (*launches)[1]
	if got.executable != v.Executable || !equalStrings(got.args, []string{"-windowed", "-x y"}) || got.workDir != filepath.Dir(v.Executable) {
		t.Fatalf("profile launch = %+v", got)
	}
	if g := mustFind(t, s, game.ID); g.LastProfile != v.ID {
		t.Fatalf("LastProfile = %q", g.LastProfile)
	}
	stopAndWait(t, s, game.ID)

	if err := s.PlayGame(game.ID); err != nil {
		t.Fatal(err)
	}
	if (*launches)[2].executable != v.Executable {
		t.Fatalf("PlayGame did not pick the last profile: %q", (*launches)[2].executable)
	}
	stopAndWait(t, s, game.ID)

	if err := s.RemoveProfile(game.ID, v.ID); err != nil {
		t.Fatal(err)
	}
	if g := mustFind(t, s, game.ID); g.LastProfile != "" {
		t.Fatalf("removing the last profile kept LastProfile = %q", g.LastProfile)
	}
	if err := s.PlayGame(game.ID); err != nil {
		t.Fatal(err)
	}
	if (*launches)[3].executable != game.Executable {
		t.Fatalf("after removal launch used %q", (*launches)[3].executable)
	}
	stopAndWait(t, s, game.ID)
}

func TestPlayProfileFailures(t *testing.T) {
	s, game, _ := profileGame(t)
	v := addProfile(t, s, game, "Mods", "mod.exe")
	launches := captureLaunches(s)

	if err := s.PlayGameProfile(game.ID, "nope"); !errors.Is(err, errProfileNotFound) {
		t.Fatalf("unknown profile: %v", err)
	}
	if err := os.Remove(v.Executable); err != nil {
		t.Fatal(err)
	}
	if err := s.PlayGameProfile(game.ID, v.ID); !errors.Is(err, errProfileNotFile) {
		t.Fatalf("deleted profile exe: %v", err)
	}
	if len(*launches) != 0 {
		t.Fatalf("nothing must start, got %+v", *launches)
	}
}

func TestPlayGameFallsBackWhenLastProfileBroken(t *testing.T) {
	s, game, path := profileGame(t)
	v := addProfile(t, s, game, "Mods", "mod.exe")
	launches := captureLaunches(s)
	s.mu.Lock()
	s.findLocked(game.ID).LastProfile = v.ID
	s.mu.Unlock()
	if err := os.Remove(v.Executable); err != nil {
		t.Fatal(err)
	}

	if err := s.PlayGameProfile(game.ID, v.ID); !errors.Is(err, errProfileNotFile) {
		t.Fatalf("explicit launch must still fail: %v", err)
	}
	blocked := s.path
	s.path = t.TempDir()
	if err := s.PlayGame(game.ID); err == nil {
		t.Fatal("fallback that cannot be saved must abort the launch")
	}
	s.path = blocked
	if len(*launches) != 0 || mustFind(t, s, game.ID).LastProfile != v.ID {
		t.Fatalf("aborted fallback changed state: launches=%v last=%q", *launches, mustFind(t, s, game.ID).LastProfile)
	}

	if err := s.PlayGame(game.ID); err != nil {
		t.Fatalf("PlayGame must fall back to the default profile: %v", err)
	}
	if len(*launches) != 1 || (*launches)[0].executable != game.Executable {
		t.Fatalf("launches = %+v", *launches)
	}
	if mustFind(t, s, game.ID).LastProfile != "" {
		t.Fatal("LastProfile must be cleared")
	}
	var stored []Game
	if err := json.Unmarshal(readFile(t, path), &stored); err != nil || stored[0].LastProfile != "" {
		t.Fatalf("cleared LastProfile not persisted: %v %+v", err, stored)
	}
	stopAndWait(t, s, game.ID)
}

func TestPlayProfileSaveFailureDoesNotStart(t *testing.T) {
	s, game, _ := profileGame(t)
	v := addProfile(t, s, game, "Mods", "mod.exe")
	launches := captureLaunches(s)
	good := s.path
	s.path = t.TempDir()
	if err := s.PlayGameProfile(game.ID, v.ID); err == nil {
		t.Fatal("expected save error")
	}
	s.path = good
	if len(*launches) != 0 || mustFind(t, s, game.ID).LastProfile != "" {
		t.Fatalf("started despite save failure: %v", *launches)
	}
}

func TestRemoveProfileRefusesWhenCleanupFails(t *testing.T) {
	s, fake, _ := shortcutService(t)
	steam := &fakeSteam{present: true}
	s.steam = steam
	game := addShortcutGame(t, s)
	v := addProfile(t, s, game, "Mods", "mod.exe")
	if err := s.CreateProfileShortcut(game.ID, v.ID); err != nil {
		t.Fatal(err)
	}

	steam.removeErr = steamlink.ErrSteamRunning
	if err := s.RemoveProfile(game.ID, v.ID); !errors.Is(err, steamlink.ErrSteamRunning) {
		t.Fatalf("running Steam must refuse: %v", err)
	}
	steam.removeErr = nil
	fake.removeErr = errors.New("locked")
	if err := s.RemoveProfile(game.ID, v.ID); err == nil {
		t.Fatal("undeletable link must refuse")
	}
	if len(mustFind(t, s, game.ID).Profiles) != 1 {
		t.Fatal("profile must stay after a refused removal")
	}

	fake.removeErr = nil
	if err := s.RemoveProfile(game.ID, v.ID); err != nil {
		t.Fatal(err)
	}
	if len(steam.removed) < 1 || steam.removed[len(steam.removed)-1] != steamlink.ProfileAppID(game.ID, v.ID) {
		t.Fatalf("steam entry not removed: %v", steam.removed)
	}
	w := addProfile(t, s, mustFind(t, s, game.ID), "Other", "other.exe")
	steam.present = false
	steam.removed = nil
	if err := s.RemoveProfile(game.ID, w.ID); err != nil || len(steam.removed) != 0 {
		t.Fatalf("no steam entry must not be an error or a remove call: %v %v", err, steam.removed)
	}
}

func TestUpdateProfileRefreshesShortcut(t *testing.T) {
	s, fake, _ := shortcutService(t)
	game := addShortcutGame(t, s)
	v := addProfile(t, s, game, "Mods", "mod.exe")
	if err := s.CreateProfileShortcut(game.ID, v.ID); err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(fake.desktop, shortcutName(t, "Half-Life 2 (Mods)"))
	newPath := filepath.Join(fake.desktop, shortcutName(t, "Half-Life 2 (Retro)"))
	exe := writeExe(t, game.InstallDir, "retro.exe")

	fake.removeErr = errors.New("locked")
	if err := s.UpdateProfile(game.ID, v.ID, ProfileInput{Name: "Retro", Executable: exe}); err == nil {
		t.Fatal("undeletable old link must refuse the update")
	}
	if mustFind(t, s, game.ID).Profiles[0].Name != "Mods" {
		t.Fatal("refused update changed the profile")
	}
	fake.removeErr = nil
	if err := s.UpdateProfile(game.ID, v.ID, ProfileInput{Name: "Retro", Executable: exe}); err != nil {
		t.Fatal(err)
	}
	if _, ok := fake.created[oldPath]; ok {
		t.Fatal("old link still present")
	}
	link, ok := fake.created[newPath]
	if !ok || link.Icon != exe {
		t.Fatalf("new link = %+v ok=%v", link, ok)
	}
	if mustFind(t, s, game.ID).Profiles[0].ShortcutPath != newPath {
		t.Fatal("new link path not stored")
	}
}

func TestSteamPlanKeepsProfilesWhenDefaultTargetFails(t *testing.T) {
	s, game, _ := profileGame(t)
	v := addProfile(t, s, game, "Mods", "mod.exe")
	g := mustFind(t, s, game.ID)
	g.Executable = ""
	st := steamState{live: []Game{g}, liveIDs: map[string]struct{}{g.ID: {}}, launcherPath: os.Executable}
	plan := st.plan()
	if len(plan.Update) != 1 || plan.Update[0].AppID != steamlink.ProfileAppID(game.ID, v.ID) {
		t.Fatalf("update = %+v", plan.Update)
	}
}

func TestCreateProfileShortcutRejectsUnsafeID(t *testing.T) {
	s, _, _ := shortcutService(t)
	game := addShortcutGame(t, s)
	s.mu.Lock()
	g := s.findLocked(game.ID)
	g.Profiles = []LaunchProfile{{ID: "a b --x", Name: "Bad", Executable: "game.exe"}}
	s.mu.Unlock()
	if err := s.CreateProfileShortcut(game.ID, "a b --x"); !errors.Is(err, errProfileBadID) {
		t.Fatalf("err = %v", err)
	}
}

func TestProfileShortcutsAreSeparate(t *testing.T) {
	s, fake, _ := shortcutService(t)
	game := addShortcutGame(t, s)
	v := addProfile(t, s, game, "Mods", "mod.exe")

	if err := s.CreateShortcut(game.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateProfileShortcut(game.ID, v.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateProfileShortcut(game.ID, "nope"); !errors.Is(err, errProfileNotFound) {
		t.Fatalf("unknown profile: %v", err)
	}
	if len(fake.created) != 2 {
		t.Fatalf("created = %v, want two separate links", fake.created)
	}
	def := fake.created[filepath.Join(fake.desktop, shortcutName(t, "Half-Life 2"))]
	prof := fake.created[filepath.Join(fake.desktop, shortcutName(t, "Half-Life 2 (Mods)"))]
	if def.Args != "--play "+game.ID || def.AppUserModelID == "" {
		t.Fatalf("default link = %+v", def)
	}
	if prof.Args != "--play "+game.ID+" --profile "+v.ID || prof.Icon != v.Executable {
		t.Fatalf("profile link = %+v", prof)
	}
	if prof.AppUserModelID == def.AppUserModelID {
		t.Fatalf("both links share AppUserModelID %q", def.AppUserModelID)
	}

	for _, p := range []string{filepath.Join(fake.desktop, shortcutName(t, "Half-Life 2")), filepath.Join(fake.desktop, shortcutName(t, "Half-Life 2 (Mods)"))} {
		if err := os.WriteFile(p, []byte("lnk"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.refreshShortcuts(context.Background()); err != nil {
		t.Fatal(err)
	}
	g := mustFind(t, s, game.ID)
	if fake.appIDs[g.ShortcutPath] != def.AppUserModelID || fake.appIDs[g.Profiles[0].ShortcutPath] != prof.AppUserModelID {
		t.Fatalf("refresh merged or skipped links: %v", fake.appIDs)
	}
}

func TestProfileShortcutPersistFailureRollsBack(t *testing.T) {
	s, fake, path := shortcutService(t)
	game := addShortcutGame(t, s)
	v := addProfile(t, s, game, "Mods", "mod.exe")
	s.path = t.TempDir()
	t.Cleanup(func() { s.path = path })
	if err := s.CreateProfileShortcut(game.ID, v.ID); err == nil {
		t.Fatal("expected save error")
	}
	g := mustFind(t, s, game.ID)
	if g.Profiles[0].ShortcutPath != "" || len(fake.created) != 0 {
		t.Fatalf("not rolled back: %+v %v", g.Profiles, fake.created)
	}
}

func TestRemoveProfileRemovesItsShortcut(t *testing.T) {
	s, fake, _ := shortcutService(t)
	game := addShortcutGame(t, s)
	v := addProfile(t, s, game, "Mods", "mod.exe")
	if err := s.CreateProfileShortcut(game.ID, v.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveProfile(game.ID, v.ID); err != nil {
		t.Fatal(err)
	}
	if len(fake.created) != 0 || len(fake.removed) != 1 {
		t.Fatalf("created=%v removed=%v", fake.created, fake.removed)
	}
}

func TestSteamTargetPerProfile(t *testing.T) {
	s, game, _ := profileGame(t)
	v := addProfile(t, s, game, "Mods", "mod.exe")
	g := mustFind(t, s, game.ID)
	launcher := func() (string, error) { return filepath.Join(t.TempDir(), "typhon.exe"), nil }

	def, err := steamTarget(g, launcher)
	if err != nil {
		t.Fatal(err)
	}
	if def.AppID != steamlink.AppID(game.ID) || def.Name != "Half-Life 2" {
		t.Fatalf("default target changed: %+v", def)
	}
	prof, err := steamProfileTarget(g, v.ID, launcher)
	if err != nil {
		t.Fatal(err)
	}
	if prof.AppID == def.AppID || prof.AppID != steamlink.ProfileAppID(game.ID, v.ID) || prof.AppID&0x80000000 == 0 {
		t.Fatalf("profile app id = %d, default = %d", prof.AppID, def.AppID)
	}
	if prof.Name != "Half-Life 2 (Mods)" || prof.Exe != v.Executable || prof.StartDir != filepath.Dir(v.Executable) || prof.LaunchOptions != steamlink.JoinArgs([]string{"-windowed", "-x y"}) {
		t.Fatalf("profile target = %+v", prof)
	}
	if _, err := steamProfileTarget(g, "nope", launcher); !errors.Is(err, errProfileNotFound) {
		t.Fatalf("unknown profile: %v", err)
	}

	if err := s.UpdateProfile(game.ID, v.ID, ProfileInput{Name: "Mods", Executable: v.Executable, RunAsAdmin: true}); err != nil {
		t.Fatal(err)
	}
	g = mustFind(t, s, game.ID)
	admin, err := steamProfileTarget(g, v.ID, launcher)
	if err != nil {
		t.Fatal(err)
	}
	if admin.LaunchOptions != "--play "+game.ID+" --profile "+v.ID || !strings.HasSuffix(admin.Exe, "typhon.exe") {
		t.Fatalf("admin target = %+v", admin)
	}
	if steamlink.ProfileAppID("a", "") != steamlink.AppID("a") {
		t.Fatal("empty profile must keep the game's app id")
	}
}

func mustProfiles(t *testing.T, s *Service, id string) []ProfileView {
	t.Helper()
	views, err := s.Profiles(id)
	if err != nil {
		t.Fatal(err)
	}
	return views
}

func mustFind(t *testing.T, s *Service, id string) Game {
	t.Helper()
	g, err := s.Find(id)
	if err != nil {
		t.Fatal(err)
	}
	return g
}
