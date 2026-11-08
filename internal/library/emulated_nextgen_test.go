package library

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"typhon/internal/emulators"
	"typhon/internal/emulators/sfotest"
)

func sfoText(title, category string) string {
	return string(sfotest.Bytes(map[string]string{"TITLE_ID": title, "CATEGORY": category}))
}

func vitaPackage(t *testing.T, title string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "game.vpk")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	entry, err := w.Create("sce_sys/param.sfo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(sfotest.Bytes(map[string]string{"TITLE_ID": title, "CATEGORY": "gd"})); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func vitaGame(t *testing.T, s *Service, title string) Game {
	t.Helper()
	game, err := s.AddEmulatedGame(t.Context(), vitaPackage(t, title), emulators.Vita, "Game")
	if err != nil {
		t.Fatal(err)
	}
	return game
}

type vitaRun struct {
	lived time.Duration
	err   error
	kill  bool
	// program is the emulator; empty means one with no config beside it.
	program string
}

var errStartReached = errors.New("the emulator was started")

// playVita runs one session and returns the launch and its stopped event.
func playVita(t *testing.T, s *Service, id string, run vitaRun) (launch, SessionEvent) {
	t.Helper()
	var seen launch
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := newRegClock(base)
	s.now = clock.now
	proc := &exitProcess{id: 7, exit: make(chan struct{}), err: run.err}
	s.start = func(_ context.Context, req launch) (gameProcess, error) {
		seen = req
		return proc, nil
	}
	program := run.program
	if program == "" {
		program = vitaProgram(t, "")
	}
	s.SetEmulatorResolver(func(string) (string, error) { return program, nil })
	events := make(chan SessionEvent, 1)
	previous := emitStopped
	emitStopped = func(ev SessionEvent) { events <- ev }
	t.Cleanup(func() { emitStopped = previous })

	if err := s.PlayGame(id); err != nil {
		t.Fatal(err)
	}
	if run.kill {
		s.mu.Lock()
		s.running[id].stoppedByUser = true
		s.mu.Unlock()
	}
	clock.set(base.Add(run.lived))
	close(proc.exit)
	s.sessionWG.Wait()
	s.wg.Wait()
	return seen, <-events
}

func storedTitle(t *testing.T, s *Service, id string) string {
	t.Helper()
	game, err := s.Find(id)
	if err != nil {
		t.Fatal(err)
	}
	return game.Emulation.TitleID
}

func TestVitaTitleIsRememberedOnlyAfterTheEmulatorSettled(t *testing.T) {
	cases := []struct {
		name string
		run  vitaRun
		want string
	}{
		{"past the quick exit", vitaRun{lived: 5 * time.Second}, "PCSE00001"},
		{"a long session", vitaRun{lived: time.Hour}, "PCSE00001"},
		{"closed by the player past the limit", vitaRun{lived: 30 * time.Second, kill: true}, ""},
		{"failed past the limit", vitaRun{lived: time.Minute, err: codeError(1)}, ""},
		{"an unknown exit code past the limit", vitaRun{lived: time.Minute, err: errors.New("wait failed")}, ""},
		{"just under the limit", vitaRun{lived: 4 * time.Second}, ""},
		{"exits at once", vitaRun{lived: 0}, ""},
		{"fails at once", vitaRun{lived: time.Second, err: codeError(1)}, ""},
		{"closed by the player at once", vitaRun{lived: time.Second, kill: true}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newLibrary(t)
			game := vitaGame(t, s, "PCSE00001")
			req, _ := playVita(t, s, game.ID, c.run)
			want := []string{"--frontend", "sdl", "-F", game.Executable}
			if !slices.Equal(req.args, want) {
				t.Fatalf("args = %q, want %q", req.args, want)
			}
			if got := storedTitle(t, s, game.ID); got != c.want {
				t.Fatalf("title = %q, want %q", got, c.want)
			}
		})
	}
}

func TestVitaStartsByTitleOnceInstalled(t *testing.T) {
	s := newLibrary(t)
	game := vitaGame(t, s, "PCSE00001")
	playVita(t, s, game.ID, vitaRun{lived: time.Minute})

	req, _ := playVita(t, s, game.ID, vitaRun{lived: time.Minute})
	if want := []string{"--frontend", "sdl", "-F", "-r", "PCSE00001"}; !slices.Equal(req.args, want) {
		t.Fatalf("args = %q, want %q", req.args, want)
	}
	if got := storedTitle(t, s, game.ID); got != "PCSE00001" {
		t.Fatalf("title = %q", got)
	}
}

func TestVitaFolderIsGivenAsAFolder(t *testing.T) {
	s := newLibrary(t)
	dir := filepath.Join(t.TempDir(), "PCSE00001")
	writeFile(t, filepath.Join(dir, "sce_sys", "param.sfo"), sfoText("PCSE00001", "gd"))
	image := writeFile(t, filepath.Join(dir, "eboot.bin"), "x")
	game, err := s.AddEmulatedGame(t.Context(), image, emulators.Vita, "Game")
	if err != nil {
		t.Fatal(err)
	}
	if game.Executable != image || game.InstallDir != dir {
		t.Fatalf("card = %+v", game)
	}
	req, _ := playVita(t, s, game.ID, vitaRun{lived: time.Minute})
	if want := []string{"--frontend", "sdl", "-F", dir}; !slices.Equal(req.args, want) {
		t.Fatalf("args = %q, want %q", req.args, want)
	}
	if got := storedTitle(t, s, game.ID); got != "PCSE00001" {
		t.Fatalf("title = %q", got)
	}
}

func TestVitaForgetsATitleTheEmulatorLost(t *testing.T) {
	cases := []struct {
		name  string
		run   vitaRun
		want  string
		quick bool
	}{
		{"code 4 at once", vitaRun{lived: time.Second, err: codeError(4)}, "", true},
		{"a crash at once keeps the install", vitaRun{lived: time.Second, err: codeError(-1)}, "PCSE00001", true},
		{"a failed start keeps the install", vitaRun{lived: time.Second, err: codeError(1)}, "PCSE00001", true},
		{"a clean exit at once", vitaRun{lived: time.Second}, "PCSE00001", true},
		{"code 4 after a while", vitaRun{lived: time.Minute, err: codeError(4)}, "", false},
		{"code 4 after the player closed it", vitaRun{lived: time.Second, err: codeError(4), kill: true}, "PCSE00001", false},
		{"an unknown exit code", vitaRun{lived: time.Second, err: errors.New("wait failed")}, "PCSE00001", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newLibrary(t)
			game := vitaGame(t, s, "PCSE00001")
			playVita(t, s, game.ID, vitaRun{lived: time.Minute})

			_, ev := playVita(t, s, game.ID, c.run)
			if ev.EmulatorQuickExit != c.quick {
				t.Fatalf("event = %+v, want quick=%v", ev, c.quick)
			}
			if c.quick && ev.Emulator != "Vita3K" {
				t.Fatalf("emulator = %q", ev.Emulator)
			}
			if got := storedTitle(t, s, game.ID); got != c.want {
				t.Fatalf("title = %q, want %q", got, c.want)
			}
		})
	}
}

func TestVitaInstallsAgainAfterForgetting(t *testing.T) {
	s := newLibrary(t)
	game := vitaGame(t, s, "PCSE00001")
	playVita(t, s, game.ID, vitaRun{lived: time.Minute})
	playVita(t, s, game.ID, vitaRun{lived: time.Second, err: codeError(4)})

	req, _ := playVita(t, s, game.ID, vitaRun{lived: time.Minute})
	if !slices.Contains(req.args, game.Executable) {
		t.Fatalf("args = %q: the image was not given", req.args)
	}
	if got := storedTitle(t, s, game.ID); got != "PCSE00001" {
		t.Fatalf("title = %q", got)
	}
}

// With the detector on the session closes on a later tick, long after the
// emulator exited. Without the real moment of exit the id would never go.
func TestQuickExitIsMeasuredToTheExitNotToTheTick(t *testing.T) {
	s := newLibrary(t)
	game := vitaGame(t, s, "PCSE00001")
	playVita(t, s, game.ID, vitaRun{lived: time.Minute})

	base := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	clock := newRegClock(base.Add(30 * time.Second))
	s.now = clock.now
	events := make(chan SessionEvent, 1)
	previous := emitStopped
	emitStopped = func(ev SessionEvent) { events <- ev }
	t.Cleanup(func() { emitStopped = previous })
	s.mu.Lock()
	s.running[game.ID] = &session{
		startedAt: base,
		exit:      &exitInfo{code: 4, known: true, ended: base.Add(time.Second)},
		emulator:  emulatorRun{titleID: "PCSE00001", installed: true},
	}
	s.mu.Unlock()

	s.finishSession(t.Context(), game.ID, base)
	s.wg.Wait()
	if ev := <-events; !ev.EmulatorQuickExit {
		t.Fatalf("event = %+v", ev)
	}
	if got := storedTitle(t, s, game.ID); got != "" {
		t.Fatalf("title = %q", got)
	}
}

func TestVitaTitleRollsBackWhenItCannotBeSaved(t *testing.T) {
	t.Run("remembering", func(t *testing.T) {
		s := newLibrary(t)
		game := vitaGame(t, s, "PCSE00001")
		blockEmulated(t, s)
		playVita(t, s, game.ID, vitaRun{lived: time.Minute})
		if got := storedTitle(t, s, game.ID); got != "" {
			t.Fatalf("title = %q after a failed save", got)
		}
	})
	t.Run("forgetting", func(t *testing.T) {
		s := newLibrary(t)
		game := vitaGame(t, s, "PCSE00001")
		playVita(t, s, game.ID, vitaRun{lived: time.Minute})
		blockEmulated(t, s)
		playVita(t, s, game.ID, vitaRun{lived: time.Second, err: codeError(4)})
		if got := storedTitle(t, s, game.ID); got != "PCSE00001" {
			t.Fatalf("title = %q after a failed save", got)
		}
	})
}

// Copies of a card given out earlier share the Emulation pointer: writing through
// it would change them behind their readers' backs and race with them.
func TestSettlingATitleLeavesEarlierCopiesAlone(t *testing.T) {
	s := newLibrary(t)
	game := vitaGame(t, s, "PCSE00001")
	before, err := s.Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	playVita(t, s, game.ID, vitaRun{lived: time.Minute})
	if before.Emulation.TitleID != "" {
		t.Fatal("a copy of the card changed under its owner")
	}
}

func TestVitaLaunchRefusesWhatItCannotName(t *testing.T) {
	cases := []struct {
		name string
		edit func(t *testing.T, s *Service, game Game)
		want error
	}{
		{name: "a package with a bad id", want: emulators.ErrTitleIDInvalid, edit: func(t *testing.T, _ *Service, game Game) {
			replaced := vitaPackage(t, "../EVIL")
			body, err := os.ReadFile(replaced)
			if err != nil {
				t.Fatal(err)
			}
			writeFile(t, game.Executable, string(body))
		}},
		{name: "a stored id edited by hand", want: emulators.ErrTitleIDInvalid, edit: func(_ *testing.T, s *Service, game Game) {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.findLocked(game.ID).Emulation = &Emulation{Platform: emulators.Vita, TitleID: "--config"}
		}},
		{name: "a broken package", want: emulators.ErrPackageInvalid, edit: func(t *testing.T, _ *Service, game Game) {
			writeFile(t, game.Executable, "not a zip")
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newLibrary(t)
			game := vitaGame(t, s, "PCSE00001")
			c.edit(t, s, game)
			started := false
			s.start = func(context.Context, launch) (gameProcess, error) {
				started = true
				return nil, errors.New("started")
			}
			s.SetEmulatorResolver(func(string) (string, error) { return filepath.Join("emu", "Vita3K.exe"), nil })
			if err := s.PlayGame(game.ID); !errors.Is(err, c.want) {
				t.Fatalf("PlayGame = %v, want %v", err, c.want)
			}
			if started {
				t.Fatal("the emulator was started")
			}
		})
	}
}

func vitaFolder(t *testing.T, dir, title string) string {
	t.Helper()
	writeFile(t, filepath.Join(dir, "sce_sys", "param.sfo"), sfoText(title, "gd"))
	return writeFile(t, filepath.Join(dir, "eboot.bin"), "x")
}

var vitaByID = []string{"--frontend", "sdl", "-F", "-r", "PCSE00001"}

// vitaProgram is an emulator with a config beside it; an empty config means none.
func vitaProgram(t *testing.T, config string) string {
	t.Helper()
	dir := t.TempDir()
	if config != "" {
		writeFile(t, filepath.Join(dir, "config.yml"), config)
	}
	return filepath.Join(dir, "Vita3K.exe")
}

func prefConfig(root string) string {
	return "log-level: 0\npref-path: " + root + "\ncpu-opt: true\n"
}

func addVita(t *testing.T, s *Service, image string) Game {
	t.Helper()
	game, err := s.AddEmulatedGame(t.Context(), image, emulators.Vita, "Game")
	if err != nil {
		t.Fatal(err)
	}
	return game
}

// Installing removes ux0/app/<id> before it copies. Given that very folder as
// content, the emulator would delete the only copy of the game.
func TestStorageIsFoundByIdentity(t *testing.T) {
	const brokenConfig = "pref-path: \"unfinished\n"
	cases := []struct {
		name string
		// config gets the storage root; folder is relative to root or dump.
		config func(root string) string
		home   bool
		inDump bool
		folder []string
		byID   bool
	}{
		{name: "the storage", config: prefConfig, folder: []string{"ux0", "app", "PCSE00001"}, byID: true},
		{name: "deeper in the game", config: prefConfig, folder: []string{"ux0", "app", "PCSE00001", "extra"}, byID: true},
		{name: "a dump in another ux0", config: prefConfig, inDump: true, folder: []string{"ux0", "app", "PCSE00001"}},
		{name: "another title in the storage", config: prefConfig, folder: []string{"ux0", "app", "PCSE00002"}},
		{name: "default storage", home: true, folder: []string{"ux0", "app", "PCSE00001"}, byID: true},
		{name: "a dump with the default storage", home: true, inDump: true, folder: []string{"ux0", "app", "PCSE00001"}},
		{name: "an empty pref-path is the default", config: func(string) string { return "pref-path:\n" }, home: true,
			folder: []string{"ux0", "app", "PCSE00001"}, byID: true},
		{name: "no storage known, looks like one", config: func(string) string { return brokenConfig }, inDump: true,
			folder: []string{"ux0", "app", "PCSE00001"}, byID: true},
		{name: "no storage known, a short folder name", config: func(string) string { return brokenConfig }, inDump: true,
			folder: []string{"Ux0", "App", "PCSE00~1"}, byID: true},
		{name: "no storage known, a lookalike", config: func(string) string { return brokenConfig }, inDump: true,
			folder: []string{"ux0x", "app", "PCSE00001"}},
		{name: "no storage known, any folder", config: func(string) string { return brokenConfig }, inDump: true,
			folder: []string{"Games", "PCSE00001"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newLibrary(t)
			root, dump := t.TempDir(), t.TempDir()
			if c.home {
				s.emulatorHome = func() (emulators.StorageHome, error) { return emulators.StorageHome{Root: root}, nil }
			}
			base := root
			if c.inDump {
				base = dump
			}
			dir := filepath.Join(append([]string{base}, c.folder...)...)
			config := ""
			if c.config != nil {
				config = c.config(root)
			}
			game := addVita(t, s, vitaFolder(t, dir, "PCSE00001"))
			req, _ := playVita(t, s, game.ID, vitaRun{lived: time.Minute, program: vitaProgram(t, config)})
			want := []string{"--frontend", "sdl", "-F", dir}
			if c.byID {
				want = vitaByID
			}
			if !slices.Equal(req.args, want) {
				t.Fatalf("args = %q, want %q", req.args, want)
			}
		})
	}
}

func TestDefaultStorageThatCannotBeFoundIsUnknown(t *testing.T) {
	s := newLibrary(t)
	s.emulatorHome = func() (emulators.StorageHome, error) { return emulators.StorageHome{}, errors.New("no home") }
	dir := filepath.Join(t.TempDir(), "ux0", "app", "PCSE00001")
	game := addVita(t, s, vitaFolder(t, dir, "PCSE00001"))
	req, _ := playVita(t, s, game.ID, vitaRun{lived: time.Minute})
	if !slices.Equal(req.args, vitaByID) {
		t.Fatalf("args = %q: in doubt the folder goes by id", req.args)
	}
}

func TestStorageCopyIsNotInstalledAgainAfterADenial(t *testing.T) {
	s := newLibrary(t)
	root := t.TempDir()
	program := vitaProgram(t, prefConfig(root))
	game := addVita(t, s, vitaFolder(t, filepath.Join(root, "ux0", "app", "PCSE00001"), "PCSE00001"))
	playVita(t, s, game.ID, vitaRun{lived: time.Second, err: codeError(4), program: program})
	req, _ := playVita(t, s, game.ID, vitaRun{lived: time.Minute, program: program})
	if !slices.Equal(req.args, vitaByID) {
		t.Fatalf("args = %q: a retry must not turn into an install", req.args)
	}
}

func TestOwnedCardsFollowIdentityToo(t *testing.T) {
	target, ok := emulators.Lookup(emulators.Vita)
	if !ok {
		t.Fatal("no vita")
	}
	root := t.TempDir()
	storage := emulators.Storage{Root: root, Known: true}
	owned := Game{Owned: true, Emulation: &Emulation{Platform: emulators.Vita}}

	stored := filepath.Join(root, "ux0", "app", "PCSE00001")
	command, run, err := emulatorCommand(t.Context(), target, owned, vitaFolder(t, stored, "PCSE00001"), storage)
	if err != nil || !slices.Equal(command, vitaByID) || !run.stored {
		t.Fatalf("the storage itself: %q, %+v, %v", command, run, err)
	}

	copied := filepath.Join(t.TempDir(), "Consoles", "vita", "Game", "ux0", "app", "PCSE00001")
	command, run, err = emulatorCommand(t.Context(), target, owned, vitaFolder(t, copied, "PCSE00001"), storage)
	if err != nil || run.installed || !slices.Equal(command, []string{"--frontend", "sdl", "-F", copied}) {
		t.Fatalf("a copy elsewhere: %q, %+v, %v", command, run, err)
	}
}

func TestEmulatorsOwnDataIsRefusedAtLaunch(t *testing.T) {
	const brokenConfig = "pref-path: 'unfinished\n"
	cases := []struct {
		name    string
		folder  []string
		dump    bool
		unknown bool
		want    error
	}{
		{name: "patch", folder: []string{"ux0", "patch", "PCSE00001"}, want: emulators.ErrImageInStorage},
		{name: "add-on", folder: []string{"ux0", "addcont", "PCSE00001", "dlc"}, want: emulators.ErrImageInStorage},
		{name: "a dump of a patch", folder: []string{"ux0", "patch", "PCSE00001"}, dump: true},
		{name: "unknown storage, by name", folder: []string{"Ux0", "Patch", "PCSE00001"}, dump: true, unknown: true,
			want: emulators.ErrImageInStorage},
		{name: "unknown storage, a lookalike", folder: []string{"ux0x", "patch", "PCSE00001"}, dump: true, unknown: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newLibrary(t)
			root, base := t.TempDir(), ""
			base = root
			if c.dump {
				base = t.TempDir()
			}
			config := prefConfig(root)
			if c.unknown {
				config = brokenConfig
			}
			game := addVita(t, s, vitaFolder(t, filepath.Join(append([]string{base}, c.folder...)...), "PCSE00001"))
			program := vitaProgram(t, config)
			s.SetEmulatorResolver(func(string) (string, error) { return program, nil })
			s.start = func(context.Context, launch) (gameProcess, error) { return nil, errStartReached }
			err := s.PlayGame(game.ID)
			s.wg.Wait()
			if c.want != nil && !errors.Is(err, c.want) {
				t.Fatalf("PlayGame = %v, want %v", err, c.want)
			}
			if c.want == nil && !errors.Is(err, errStartReached) {
				t.Fatalf("PlayGame = %v: the game did not reach the emulator", err)
			}
		})
	}
}

func TestStoredGameIsRememberedOnACleanRun(t *testing.T) {
	cases := []struct {
		name string
		run  vitaRun
		want string
	}{
		{"a clean run", vitaRun{lived: time.Minute}, "PCSE00001"},
		{"exits at once", vitaRun{lived: time.Second}, ""},
		{"closed by the player", vitaRun{lived: time.Minute, kill: true}, ""},
		{"a crash", vitaRun{lived: time.Minute, err: codeError(-1)}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newLibrary(t)
			root := t.TempDir()
			c.run.program = vitaProgram(t, prefConfig(root))
			game := addVita(t, s, vitaFolder(t, filepath.Join(root, "ux0", "app", "PCSE00001"), "PCSE00001"))
			playVita(t, s, game.ID, c.run)
			if got := storedTitle(t, s, game.ID); got != c.want {
				t.Fatalf("title = %q, want %q", got, c.want)
			}
		})
	}
}

func TestGameInEmulator(t *testing.T) {
	s := newLibrary(t)
	root := t.TempDir()
	program := vitaProgram(t, prefConfig(root))
	stored := addVita(t, s, vitaFolder(t, filepath.Join(root, "ux0", "app", "PCSE00001"), "PCSE00001"))
	dump := addVita(t, s, vitaFolder(t, filepath.Join(t.TempDir(), "ux0", "app", "PCSE00002"), "PCSE00002"))
	plain := vitaGame(t, s, "PCSE00003")
	other := emulatedGame(t, s, "game.iso")

	s.SetEmulatorResolver(func(string) (string, error) { return program, nil })
	for _, c := range []struct {
		name string
		id   string
		want bool
	}{{"storage", stored.ID, true}, {"dump", dump.ID, false}, {"package", plain.ID, false}, {"another console", other.ID, false}} {
		if got, err := s.GameInEmulator(t.Context(), c.id); err != nil || got != c.want {
			t.Errorf("%s: %v, %v, want %v", c.name, got, err, c.want)
		}
	}
	if _, err := s.GameInEmulator(t.Context(), "gone"); !errors.Is(err, errNotFound) {
		t.Fatalf("unknown card: %v", err)
	}

	s.SetEmulatorResolver(func(string) (string, error) { return "", emulators.ErrNotConfigured })
	if got, err := s.GameInEmulator(t.Context(), dump.ID); err != nil || !got {
		t.Fatalf("no emulator goes by the look of the path: %v, %v", got, err)
	}
	if err := os.Remove(filepath.Join(filepath.Dir(dump.Executable), "sce_sys", "param.sfo")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GameInEmulator(t.Context(), dump.ID); !errors.Is(err, emulators.ErrImageIncomplete) {
		t.Fatalf("unreadable game: %v", err)
	}
}

func TestShadPS4WorksFromItsOwnFolder(t *testing.T) {
	s := newLibrary(t)
	dir := filepath.Join(t.TempDir(), "CUSA00001")
	writeFile(t, filepath.Join(dir, "sce_sys", "param.sfo"), sfoText("CUSA00001", "gd"))
	image := writeFile(t, filepath.Join(dir, "eboot.bin"), "x")
	game, err := s.AddEmulatedGame(t.Context(), image, emulators.PS4, "Game")
	if err != nil {
		t.Fatal(err)
	}
	program := filepath.Join(t.TempDir(), "shadPS4", "shadPS4.exe")
	var seen launch
	s.start = func(_ context.Context, req launch) (gameProcess, error) {
		seen = req
		return nil, errors.New("seen")
	}
	s.SetEmulatorResolver(func(string) (string, error) { return program, nil })
	if err := s.PlayGame(game.ID); err == nil {
		t.Fatal("the start was expected to stop")
	}
	s.wg.Wait()
	if seen.workDir != filepath.Dir(program) {
		t.Fatalf("workDir = %q, want the folder of the emulator", seen.workDir)
	}
	if want := []string{"-f", "true", "-g", image}; !slices.Equal(seen.args, want) {
		t.Fatalf("args = %q, want %q", seen.args, want)
	}
}

func TestOlderConsolesKeepTheirCommand(t *testing.T) {
	cases := []struct {
		platform string
		file     string
		want     func(image string) []string
	}{
		{emulators.PS1, "game.cue", func(image string) []string { return []string{"-batch", "-fullscreen", "--", image} }},
		{emulators.PS2, "game.iso", func(image string) []string { return []string{"-batch", "-fullscreen", "--", image} }},
		{emulators.PSP, "game.iso", func(image string) []string { return []string{"--pause-menu-exit", "--fullscreen", image} }},
	}
	for _, c := range cases {
		t.Run(c.platform, func(t *testing.T) {
			s := newLibrary(t)
			dir := t.TempDir()
			image := writeFile(t, filepath.Join(dir, c.file), `FILE "track.bin" BINARY`)
			writeFile(t, filepath.Join(dir, "track.bin"), "x")
			game, err := s.AddEmulatedGame(t.Context(), image, c.platform, "Game")
			if err != nil {
				t.Fatal(err)
			}
			req, _ := playVita(t, s, game.ID, vitaRun{lived: time.Minute})
			if !slices.Equal(req.args, c.want(image)) {
				t.Fatalf("args = %q, want %q", req.args, c.want(image))
			}
			if got := storedTitle(t, s, game.ID); got != "" {
				t.Fatalf("title = %q on a console that starts by image", got)
			}
		})
	}
}

func TestAddingUnpackedGames(t *testing.T) {
	root := func(t *testing.T) string { return filepath.Join(t.TempDir(), "Game") }
	t.Run("a disc takes the disc folder and its whole size", func(t *testing.T) {
		s := newLibrary(t)
		dir := root(t)
		writeFile(t, filepath.Join(dir, "PS3_DISC.SFB"), "mark")
		writeFile(t, filepath.Join(dir, "PS3_GAME", "PARAM.SFO"), sfoText("BLES00001", "DG"))
		writeFile(t, filepath.Join(dir, "PS3_GAME", "USRDIR", "data.dat"), strings.Repeat("x", 4096))
		image := writeFile(t, filepath.Join(dir, "PS3_GAME", "USRDIR", "EBOOT.BIN"), "x")
		game, err := s.AddEmulatedGame(t.Context(), image, emulators.PS3, "Game")
		if err != nil {
			t.Fatal(err)
		}
		if game.Executable != image || game.InstallDir != dir || game.Owned {
			t.Fatalf("card = %+v", game)
		}
		if game.SizeBytes < 4096 || game.SizeUnknown {
			t.Fatalf("size = %d, unknown %v", game.SizeBytes, game.SizeUnknown)
		}
	})
	t.Run("a folder that is not a disc never becomes the game", func(t *testing.T) {
		s := newLibrary(t)
		dir := root(t)
		writeFile(t, filepath.Join(dir, "PS3_GAME", "PARAM.SFO"), sfoText("BLES00001", "DG"))
		writeFile(t, filepath.Join(dir, "unrelated.mkv"), strings.Repeat("x", 8192))
		image := writeFile(t, filepath.Join(dir, "PS3_GAME", "USRDIR", "EBOOT.BIN"), "x")
		game, err := s.AddEmulatedGame(t.Context(), image, emulators.PS3, "Game")
		if err != nil {
			t.Fatal(err)
		}
		if game.InstallDir != filepath.Join(dir, "PS3_GAME") || game.SizeBytes >= 8192 {
			t.Fatalf("card = %+v", game)
		}
	})
	t.Run("rebinding follows the game folder", func(t *testing.T) {
		s := newLibrary(t)
		first := filepath.Join(t.TempDir(), "BLES00001")
		writeFile(t, filepath.Join(first, "PARAM.SFO"), sfoText("BLES00001", "DG"))
		image := writeFile(t, filepath.Join(first, "USRDIR", "EBOOT.BIN"), "x")
		game, err := s.AddEmulatedGame(t.Context(), image, emulators.PS3, "Game")
		if err != nil {
			t.Fatal(err)
		}
		moved := filepath.Join(t.TempDir(), "BLES00001")
		writeFile(t, filepath.Join(moved, "PARAM.SFO"), sfoText("BLES00001", "DG"))
		next := writeFile(t, filepath.Join(moved, "USRDIR", "EBOOT.BIN"), "x")
		bound, err := s.rebindImage(game.ID, next, emulators.PS3)
		if err != nil {
			t.Fatal(err)
		}
		if bound.Executable != next || bound.InstallDir != moved {
			t.Fatalf("card = %+v", bound)
		}
	})
	t.Run("another image is installed anew", func(t *testing.T) {
		s := newLibrary(t)
		game := vitaGame(t, s, "PCSE00001")
		playVita(t, s, game.ID, vitaRun{lived: time.Minute})
		bound, err := s.rebindImage(game.ID, vitaPackage(t, "PCSE00002"), emulators.Vita)
		if err != nil {
			t.Fatal(err)
		}
		if bound.Emulation.TitleID != "" {
			t.Fatalf("title = %q of the previous image", bound.Emulation.TitleID)
		}
	})
}

func TestAddingRefusesFormsThatDoNotPlay(t *testing.T) {
	dir := t.TempDir()
	eboot := writeFile(t, filepath.Join(dir, "Game", "USRDIR", "EBOOT.BIN"), "x")
	bare := writeFile(t, filepath.Join(dir, "Other", "eboot.bin"), "x")
	bin := writeFile(t, filepath.Join(dir, "game.bin"), "x")
	iso := writeFile(t, filepath.Join(dir, "game.iso"), "x")
	pkg := writeFile(t, filepath.Join(dir, "game.pkg"), "x")
	cases := []struct {
		name     string
		platform string
		path     string
		want     error
	}{
		{"ps3 without param.sfo", emulators.PS3, eboot, emulators.ErrImageIncomplete},
		{"ps4 without param.sfo", emulators.PS4, bare, emulators.ErrImageIncomplete},
		{"vita without param.sfo", emulators.Vita, bare, emulators.ErrImageIncomplete},
		{"ps3 eboot not in USRDIR", emulators.PS3, bare, emulators.ErrImageExtension},
		{"a generic bin on ps3", emulators.PS3, bin, emulators.ErrImageExtension},
		{"a generic bin on ps4", emulators.PS4, bin, emulators.ErrImageExtension},
		{"a generic bin on vita", emulators.Vita, bin, emulators.ErrImageExtension},
		{"iso on ps3", emulators.PS3, iso, emulators.ErrImageManualOnly},
		{"pkg on ps4", emulators.PS4, pkg, emulators.ErrImageManualOnly},
		{"pkg on vita", emulators.Vita, pkg, emulators.ErrImageManualOnly},
		{"a missing eboot is missing", emulators.PS3, filepath.Join(dir, "Lost", "USRDIR", "EBOOT.BIN"), emulators.ErrImageMissing},
		{"empty", emulators.PS3, "  ", emulators.ErrImagePathEmpty},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := newLibrary(t).AddEmulatedGame(t.Context(), c.path, c.platform, "Game"); !errors.Is(err, c.want) {
				t.Fatalf("AddEmulatedGame = %v, want %v", err, c.want)
			}
		})
	}
}

func TestEmulationStaysReadableByThePreviousFormat(t *testing.T) {
	plain, err := json.Marshal(Emulation{Platform: emulators.PS2})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(plain), "titleId") {
		t.Fatalf("an empty field reached the file: %s", plain)
	}

	s := newLibrary(t)
	game := vitaGame(t, s, "PCSE00001")
	playVita(t, s, game.ID, vitaRun{lived: time.Minute})
	reopened, err := NewServiceAt(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if got := storedTitle(t, reopened, game.ID); got != "PCSE00001" {
		t.Fatalf("title = %q after a restart", got)
	}
	raw, err := os.ReadFile(s.emulatedPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Version != emulatedVersion || emulatedVersion != 1 {
		t.Fatalf("version = %d: the previous launcher refuses a newer file", doc.Version)
	}
}
