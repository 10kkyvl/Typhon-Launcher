package library

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"typhon/internal/crashlog"
	"typhon/internal/emulators"
	"typhon/internal/procs"
	"typhon/internal/storage"
	"typhon/internal/uierr"
)

func writeFile(t *testing.T, path, body string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := storage.WriteAtomic(path, []byte(body)); err != nil {
		t.Fatal(err)
	}
	return path
}

func newLibrary(t *testing.T) *Service {
	t.Helper()
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	home := t.TempDir()
	s.emulatorHome = func() (emulators.StorageHome, error) { return emulators.StorageHome{Root: home}, nil }
	return s
}

func emulatedGame(t *testing.T, s *Service, name string) Game {
	t.Helper()
	game, err := s.AddEmulatedGame(t.Context(), writeFile(t, filepath.Join(t.TempDir(), name), "image"), emulators.PS2, "Game")
	if err != nil {
		t.Fatal(err)
	}
	return game
}

func blockEmulated(t *testing.T, s *Service) {
	t.Helper()
	s.emulatedPath = filepath.Join(writeFile(t, filepath.Join(t.TempDir(), "blocked"), "x"), emulatedFileName)
}

type linkInfo struct{ fs.FileInfo }

func (linkInfo) Mode() fs.FileMode { return fs.ModeSymlink }

func stubImageStat(t *testing.T, fn func(string) (fs.FileInfo, error)) {
	t.Helper()
	imageStat = fn
	t.Cleanup(func() { imageStat = os.Lstat })
}

type recordedLaunch struct {
	req  launch
	proc *stubProcess
}

// recordLaunches replaces the starter: the argv handed to it is the contract
// with the emulators, and no real process is needed to check it.
func recordLaunches(t *testing.T, s *Service) *recordedLaunch {
	t.Helper()
	rec := &recordedLaunch{proc: &stubProcess{done: make(chan struct{})}}
	s.start = func(_ context.Context, req launch) (gameProcess, error) {
		rec.req = req
		return rec.proc, nil
	}
	s.SetEmulatorResolver(func(string) (string, error) {
		return filepath.Join("emu", "dir", "emulator.exe"), nil
	})
	return rec
}

func (r *recordedLaunch) finish(s *Service) {
	close(r.proc.done)
	s.sessionWG.Wait()
}

func TestAddEmulatedGameRefuses(t *testing.T) {
	dir := t.TempDir()
	iso := writeFile(t, filepath.Join(dir, "game.iso"), "x")
	exe := writeFile(t, filepath.Join(dir, "game.exe"), "x")
	subdir := filepath.Join(dir, "disc.iso")
	if err := os.Mkdir(subdir, 0o700); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name     string
		rom      string
		platform string
		title    string
		want     error
	}{
		{"empty title", iso, emulators.PS2, "  ", emulators.ErrTitleEmpty},
		{"empty path", "  ", emulators.PS2, "G", emulators.ErrImagePathEmpty},
		{"relative path", "game.iso", emulators.PS2, "G", emulators.ErrImagePathRelative},
		{"unknown platform", iso, "gba", "G", emulators.ErrUnknownPlatform},
		{"empty platform", iso, "", "G", emulators.ErrUnknownPlatform},
		{"wrong extension", exe, emulators.PS2, "G", emulators.ErrImageExtension},
		{"extension of another console", filepath.Join(dir, "game.mds"), emulators.PSP, "G", emulators.ErrImageExtension},
		{"no file", filepath.Join(dir, "none.iso"), emulators.PS2, "G", emulators.ErrImageMissing},
		{"directory", subdir, emulators.PS2, "G", emulators.ErrImageNotFile},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newLibrary(t)
			if _, err := s.AddEmulatedGame(t.Context(), c.rom, c.platform, c.title); !errors.Is(err, c.want) {
				t.Fatalf("AddEmulatedGame = %v, want %v", err, c.want)
			}
			if len(s.GetGames()) != 0 {
				t.Fatal("a refused image reached the library")
			}
		})
	}
}

func TestAddEmulatedGameWithoutRightsIsNotAMissingFile(t *testing.T) {
	s := newLibrary(t)
	stubImageStat(t, func(string) (fs.FileInfo, error) { return nil, fs.ErrPermission })
	_, err := s.AddEmulatedGame(t.Context(), filepath.Join(t.TempDir(), "game.iso"), emulators.PS2, "G")
	if uierr.Code(err) != uierr.Code(emulators.ErrImageUnreadable) || !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("AddEmulatedGame = %v", err)
	}
}

func TestAddEmulatedGameRefusesASymlink(t *testing.T) {
	s := newLibrary(t)
	stubImageStat(t, func(string) (fs.FileInfo, error) { return linkInfo{}, nil })
	_, err := s.AddEmulatedGame(t.Context(), filepath.Join(t.TempDir(), "link.iso"), emulators.PS2, "G")
	if !errors.Is(err, emulators.ErrImageNotFile) {
		t.Fatalf("AddEmulatedGame = %v", err)
	}
}

func TestAddEmulatedGameStopsOnCancelledContext(t *testing.T) {
	s := newLibrary(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	rom := writeFile(t, filepath.Join(t.TempDir(), "game.iso"), "x")
	if _, err := s.AddEmulatedGame(ctx, rom, emulators.PS2, "G"); !errors.Is(err, context.Canceled) {
		t.Fatalf("AddEmulatedGame = %v", err)
	}
}

func TestAddEmulatedGameRollsBackAFailedSave(t *testing.T) {
	s := newLibrary(t)
	blockEmulated(t, s)
	rom := writeFile(t, filepath.Join(t.TempDir(), "game.iso"), "x")
	if _, err := s.AddEmulatedGame(t.Context(), rom, emulators.PS2, "G"); err == nil {
		t.Fatal("expected the save to fail")
	}
	if len(s.GetGames()) != 0 {
		t.Fatal("a game that was not saved stayed in memory")
	}
}

func TestAddEmulatedGameRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "other.bin"), strings.Repeat("x", 1000))
	rom := writeFile(t, filepath.Join(dir, "rom.iso"), "12345")

	game, err := s.AddEmulatedGame(t.Context(), rom, emulators.PS2, " My Game ")
	if err != nil {
		t.Fatal(err)
	}
	if game.Title != "My Game" || game.Executable != rom || game.InstallDir != dir || game.Owned || game.CanonicalGameID != "" ||
		game.SizeBytes != 5 || game.SizeUnknown || game.Emulation == nil || game.Emulation.Platform != emulators.PS2 {
		t.Fatalf("record: %+v", game)
	}
	if _, err := ReadMarker(dir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a marker appeared in the folder of the image: %v", err)
	}
	stored, err := mustServiceAt(t, path).Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Emulation == nil || stored.Emulation.Platform != emulators.PS2 {
		t.Fatalf("the marker of emulation was lost on reload: %+v", stored)
	}
}

func TestEmulatedGamesAreInvisibleToAnOlderLauncher(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "library.json")
	s := mustServiceAt(t, path)
	native, err := s.AddGame(t.Context(), tempGameExe(t), "Native")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddEmulatedGame(t.Context(), writeFile(t, filepath.Join(t.TempDir(), "g.iso"), "x"), emulators.PS2, "G"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var stored []Game
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].ID != native.ID {
		t.Fatalf("library.json holds %+v", stored)
	}
	if got := mustServiceAt(t, path).GetGames(); len(got) != 2 {
		t.Fatalf("reloaded: %+v", got)
	}
}

func TestOnlyTheChangedFileIsWritten(t *testing.T) {
	s := newLibrary(t)
	native, err := s.AddGame(t.Context(), tempGameExe(t), "Native")
	if err != nil {
		t.Fatal(err)
	}
	emulated := emulatedGame(t, s, "game.iso")
	blockEmulated(t, s)
	if _, err := s.SetFavorite(native.ID, true); err != nil {
		t.Fatalf("a native change touched the emulated file: %v", err)
	}
	s.emulatedPath = emulatedPathFor(s.path)
	if err := os.Remove(s.path); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetFavorite(emulated.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("an emulated change rewrote library.json: %v", err)
	}
}

func TestBrokenEmulatedFileStopsTheConstructor(t *testing.T) {
	good := `{"version":1,"data":[]}`
	cases := []struct {
		name string
		body string
		fail bool
	}{
		{"broken json", `{`, true},
		{"record without emulation", `{"version":1,"data":[{"id":"x","installDir":"d","executable":"e"}]}`, true},
		{"record without folder", `{"version":1,"data":[{"id":"x","executable":"e","emulation":{"platform":"ps2"}}]}`, true},
		{"empty list", good, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "library.json")
			writeFile(t, emulatedPathFor(path), c.body)
			_, err := NewServiceAt(path)
			if (err != nil) != c.fail {
				t.Fatalf("NewServiceAt = %v", err)
			}
		})
	}
}

func TestRemovalThatFailsOnTheSecondFileLeavesBothAsTheyWere(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s := mustServiceAt(t, path)
	game := emulatedGame(t, s, "game.iso")
	before, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	blockEmulated(t, s)
	if err := s.RemoveGame(game.ID); err == nil {
		t.Fatal("expected the save to fail")
	}
	if _, err := s.Find(game.ID); err != nil {
		t.Fatalf("memory not restored: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	if strings.Contains(string(after), game.ID) {
		t.Fatalf("library.json stayed ahead: before %q after %q", before, after)
	}
}

func TestPlainGameKeepsItsJSONShape(t *testing.T) {
	data, err := json.Marshal(Game{ID: "g"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "emulation") {
		t.Fatalf("emulation leaked into a plain record: %s", data)
	}
}

func TestSameImageTwiceIsRefusedTwoGamesInAFolderAreNot(t *testing.T) {
	s := newLibrary(t)
	dir := t.TempDir()
	first := writeFile(t, filepath.Join(dir, "first.iso"), "x")
	second := writeFile(t, filepath.Join(dir, "second.iso"), "x")
	if _, err := s.AddEmulatedGame(t.Context(), first, emulators.PS2, "A"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddEmulatedGame(t.Context(), second, emulators.PS2, "B"); err != nil {
		t.Fatalf("second game of the folder: %v", err)
	}
	if _, err := s.AddEmulatedGame(t.Context(), first, emulators.PSP, "C"); uierr.Code(err) != "library.already_added" {
		t.Fatalf("same image again: %v", err)
	}
	if len(s.GetGames()) != 2 {
		t.Fatalf("games: %+v", s.GetGames())
	}
}

func TestUninstalledImageComesBackWithItsHistory(t *testing.T) {
	s := newLibrary(t)
	game := emulatedGame(t, s, "game.iso")
	s.games[0].PlaytimeSeconds = 60
	if err := s.MarkUninstalled(game.ID); err != nil {
		t.Fatal(err)
	}
	again, err := s.AddEmulatedGame(t.Context(), game.Executable, emulators.PS2, "New")
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != game.ID || again.Uninstalled || again.PlaytimeSeconds != 60 || again.Title != "New" || again.SizeBytes != 5 {
		t.Fatalf("revived: %+v", again)
	}
}

func TestSetExecutableRebindsAnImage(t *testing.T) {
	s := newLibrary(t)
	game := emulatedGame(t, s, "game.iso")
	s.games[0].PlaytimeSeconds = 60
	moved := writeFile(t, filepath.Join(t.TempDir(), "moved.iso"), "1234567")

	after, err := s.SetExecutable(game.ID, moved)
	if err != nil {
		t.Fatal(err)
	}
	if after.Executable != moved || after.InstallDir != filepath.Dir(moved) || after.SizeBytes != 7 || after.PlaytimeSeconds != 60 || after.Title != game.Title {
		t.Fatalf("rebound: %+v", after)
	}
}

func TestSetExecutableRefusesAWrongImage(t *testing.T) {
	dir := t.TempDir()
	exe := writeFile(t, filepath.Join(dir, "game.exe"), "x")
	cases := []struct {
		name string
		path func(other Game) string
		want error
	}{
		{"wrong extension", func(Game) string { return exe }, emulators.ErrImageExtension},
		{"no file", func(Game) string { return filepath.Join(dir, "none.iso") }, emulators.ErrImageMissing},
		{"empty", func(Game) string { return " " }, emulators.ErrImagePathEmpty},
		{"image of another card", func(other Game) string { return other.Executable }, errAlreadyAdded},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newLibrary(t)
			game := emulatedGame(t, s, "game.iso")
			other := emulatedGame(t, s, "other.iso")
			if _, err := s.SetExecutable(game.ID, c.path(other)); !errors.Is(err, c.want) {
				t.Fatalf("SetExecutable = %v", err)
			}
			after, err := s.Find(game.ID)
			if err != nil {
				t.Fatal(err)
			}
			if after.Executable != game.Executable {
				t.Fatalf("record changed: %+v", after)
			}
		})
	}
}

func TestSetExecutableRollsBackAFailedSave(t *testing.T) {
	s := newLibrary(t)
	game := emulatedGame(t, s, "game.iso")
	blockEmulated(t, s)
	moved := writeFile(t, filepath.Join(t.TempDir(), "moved.iso"), "x")
	if _, err := s.SetExecutable(game.ID, moved); err == nil {
		t.Fatal("expected the save to fail")
	}
	after, err := s.Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Executable != game.Executable || after.InstallDir != game.InstallDir {
		t.Fatalf("memory not restored: %+v", after)
	}
}

func TestMissingImages(t *testing.T) {
	s := newLibrary(t)
	kept := emulatedGame(t, s, "kept.iso")
	gone := emulatedGame(t, s, "gone.iso")
	if err := os.Remove(gone.Executable); err != nil {
		t.Fatal(err)
	}
	got, err := s.MissingImages(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{gone.ID}) {
		t.Fatalf("missing %v, kept %s", got, kept.ID)
	}
	stubImageStat(t, func(string) (fs.FileInfo, error) { return nil, fs.ErrPermission })
	if got, err = s.MissingImages(t.Context()); err != nil || len(got) != 0 {
		t.Fatalf("an unreadable file was called gone: %v, %v", got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.MissingImages(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled = %v", err)
	}
}

func TestFindEmulatedCoverOutcomes(t *testing.T) {
	boom := uierr.Wrap("emulators.cover_lookup_failed", errors.New("offline"))
	cases := []struct {
		name   string
		result string
		err    error
		want   string
	}{
		{"found", "https://x/c.jpg", nil, "https://x/c.jpg"},
		{"not found", "", emulators.ErrCoverNotFound, ""},
		{"network", "", boom, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newLibrary(t)
			game := emulatedGame(t, s, "game.iso")
			var asked string
			s.SetCoverSearch(func(_ context.Context, title, platform string) (string, error) {
				asked = title + "|" + platform
				return c.result, c.err
			})
			got, err := s.FindEmulatedCover(t.Context(), game.ID)
			if !errors.Is(err, c.err) {
				t.Fatalf("FindEmulatedCover = %v", err)
			}
			if asked != "Game|ps2" {
				t.Fatalf("asked %q", asked)
			}
			after, findErr := s.Find(game.ID)
			if findErr != nil {
				t.Fatal(findErr)
			}
			if got.Cover != c.want || after.Cover != c.want || after.CanonicalGameID != "" {
				t.Fatalf("after: %+v", after)
			}
		})
	}
}

func TestFindEmulatedCoverRefuses(t *testing.T) {
	s := newLibrary(t)
	native, err := s.AddGame(t.Context(), tempGameExe(t), "Native")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.FindEmulatedCover(t.Context(), native.ID); !errors.Is(err, emulators.ErrUnsupported) {
		t.Fatalf("native = %v", err)
	}
	if _, err := s.FindEmulatedCover(t.Context(), "nope"); !IsGameNotFound(err) {
		t.Fatalf("unknown = %v", err)
	}
	game := emulatedGame(t, s, "game.iso")
	if _, err := s.FindEmulatedCover(t.Context(), game.ID); !errors.Is(err, emulators.ErrCoverLookup) {
		t.Fatalf("not wired = %v", err)
	}
}

func TestFindEmulatedCoverRollsBackAFailedSave(t *testing.T) {
	s := newLibrary(t)
	game := emulatedGame(t, s, "game.iso")
	s.SetCoverSearch(func(context.Context, string, string) (string, error) { return "c", nil })
	blockEmulated(t, s)
	if _, err := s.FindEmulatedCover(t.Context(), game.ID); err == nil {
		t.Fatal("expected the save to fail")
	}
	if after, err := s.Find(game.ID); err != nil || after.Cover != "" {
		t.Fatalf("memory not restored: %+v, %v", after, err)
	}
}

func TestLateCoverLeavesARemovedCardAlone(t *testing.T) {
	s := newLibrary(t)
	game := emulatedGame(t, s, "game.iso")
	s.SetCoverSearch(func(context.Context, string, string) (string, error) {
		if err := s.RemoveGame(game.ID); err != nil {
			t.Error(err)
		}
		return "c", nil
	})
	if _, err := s.FindEmulatedCover(t.Context(), game.ID); !IsGameNotFound(err) {
		t.Fatalf("FindEmulatedCover = %v", err)
	}
}

func TestReadImage(t *testing.T) {
	dir := t.TempDir()
	track := strings.Repeat("x", 100)
	one := writeFile(t, filepath.Join(dir, "track 1.bin"), track)
	two := writeFile(t, filepath.Join(dir, "track2.bin"), track+track)
	sheet := "FILE \"track 1.bin\" BINARY\n  TRACK 01 MODE2/2352\nFILE track2.bin BINARY\nFILE \"track 1.bin\" BINARY\n"
	good := writeFile(t, filepath.Join(dir, "good.cue"), sheet)
	list := "# discs\ngood.cue\n\nplain.iso\n"
	plain := writeFile(t, filepath.Join(dir, "plain.iso"), "12345")
	cases := []struct {
		name    string
		rom     string
		size    int64
		unknown bool
		files   []string
	}{
		{"plain image", plain, 5, false, nil},
		{"cue with its tracks", good, int64(len(sheet)) + 300, false, []string{one, two}},
		{"playlist with its discs", writeFile(t, filepath.Join(dir, "discs.m3u"), list),
			int64(len(list)) + int64(len(sheet)) + 305, false, []string{good, one, two, plain}},
		{"cue with a lost track", writeFile(t, filepath.Join(dir, "lost.cue"), "FILE \"gone.bin\" BINARY\n"), 0, true, []string{filepath.Join(dir, "gone.bin")}},
		{"cue reaching outside", writeFile(t, filepath.Join(dir, "evil.cue"), "FILE \"..\\..\\secret.bin\" BINARY\n"), 0, true, nil},
		{"cue with absolute track", writeFile(t, filepath.Join(dir, "abs.cue"), "FILE \"/etc/passwd\" BINARY\n"), 0, true, nil},
		{"cue with no tracks", writeFile(t, filepath.Join(dir, "empty.cue"), "REM nothing\n"), 0, true, nil},
		{"cue with an open quote", writeFile(t, filepath.Join(dir, "broken.cue"), "FILE \"track2.bin BINARY\n"), 0, true, nil},
		{"playlist reaching outside", writeFile(t, filepath.Join(dir, "evil.m3u"), "../secret.iso\n"), 0, true, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := readImage(c.rom, t.Context().Err)
			if err != nil {
				t.Fatal(err)
			}
			if got.size != c.size || got.unknown != c.unknown || !slices.Equal(got.files, c.files) {
				t.Fatalf("readImage = %+v", got)
			}
		})
	}
}

func TestReadImageStopsOnCancelledContext(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "t.bin"), "x")
	cue := writeFile(t, filepath.Join(dir, "g.cue"), "FILE t.bin BINARY\n")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := readImage(cue, ctx.Err); !errors.Is(err, context.Canceled) {
		t.Fatalf("readImage = %v", err)
	}
}

func TestFootprint(t *testing.T) {
	image := Game{Executable: "a.cue", InstallDir: "dir", Emulation: &Emulation{Files: []string{"a.bin"}}}
	if got := image.Footprint(); !slices.Equal(got, []string{"a.cue", "a.bin"}) {
		t.Fatalf("image: %q", got)
	}
	if got := (Game{InstallDir: "dir"}).Footprint(); !slices.Equal(got, []string{"dir"}) {
		t.Fatalf("native: %q", got)
	}
	if got := (Game{}).Footprint(); got != nil {
		t.Fatalf("no folder: %q", got)
	}
}

func TestPlayRunsTheEmulatorWithTheExactCommand(t *testing.T) {
	cases := []struct {
		platform string
		want     func(rom string) []string
	}{
		{emulators.PS1, func(rom string) []string { return []string{"-batch", "-fullscreen", "--", rom} }},
		{emulators.PS2, func(rom string) []string { return []string{"-batch", "-fullscreen", "--", rom} }},
		{emulators.PSP, func(rom string) []string { return []string{"--pause-menu-exit", "--fullscreen", rom} }},
	}
	for _, c := range cases {
		t.Run(c.platform, func(t *testing.T) {
			s := newLibrary(t)
			rec := recordLaunches(t, s)
			var asked string
			program := filepath.Join(t.TempDir(), "emulator.exe")
			s.SetEmulatorResolver(func(platform string) (string, error) {
				asked = platform
				return program, nil
			})
			rom := writeFile(t, filepath.Join(t.TempDir(), "My Game (USA).iso"), "x")
			game, err := s.AddEmulatedGame(t.Context(), rom, c.platform, "G")
			if err != nil {
				t.Fatal(err)
			}
			if err := s.PlayGame(game.ID); err != nil {
				t.Fatal(err)
			}
			defer rec.finish(s)
			if asked != c.platform || rec.req.executable != program || !slices.Equal(rec.req.args, c.want(rom)) ||
				rec.req.workDir != filepath.Dir(program) || rec.req.elevated {
				t.Fatalf("asked %q, launch %+v", asked, rec.req)
			}
		})
	}
}

func TestEmulatorIsResolvedOutsideTheLibraryLock(t *testing.T) {
	s := newLibrary(t)
	rec := recordLaunches(t, s)
	game := emulatedGame(t, s, "game.iso")
	s.SetEmulatorResolver(func(string) (string, error) {
		if !s.mu.TryLock() {
			t.Error("the resolver ran under the library mutex")
			return "emulator.exe", nil
		}
		s.mu.Unlock()
		return "emulator.exe", nil
	})
	if err := s.PlayGame(game.ID); err != nil {
		t.Fatal(err)
	}
	rec.finish(s)
}

func TestPlayPutsThePlayersArgumentsBeforeTheCommand(t *testing.T) {
	s := newLibrary(t)
	rec := recordLaunches(t, s)
	game := emulatedGame(t, s, "game.iso")
	if _, err := s.SetLaunchArgs(game.ID, []string{"-nogui"}); err != nil {
		t.Fatal(err)
	}
	if err := s.PlayGame(game.ID); err != nil {
		t.Fatal(err)
	}
	defer rec.finish(s)
	want := []string{"-nogui", "-batch", "-fullscreen", "--", game.Executable}
	if !slices.Equal(rec.req.args, want) {
		t.Fatalf("args %q", rec.req.args)
	}
}

func TestPlayIgnoresAStoredAdminFlag(t *testing.T) {
	s := newLibrary(t)
	rec := recordLaunches(t, s)
	game := emulatedGame(t, s, "game.iso")
	s.games[0].RunAsAdmin = true
	if err := s.PlayGame(game.ID); err != nil {
		t.Fatal(err)
	}
	defer rec.finish(s)
	if rec.req.elevated {
		t.Fatal("an emulator must never run elevated")
	}
}

func TestPlayFailures(t *testing.T) {
	resolverErr := func(err error) func(string) (string, error) {
		return func(string) (string, error) { return "", err }
	}
	cases := []struct {
		name     string
		resolver func(string) (string, error)
		mutate   func(t *testing.T, s *Service, game Game)
		code     string
	}{
		{"no resolver wired", nil, nil, "emulators.not_configured"},
		{"emulator not set", resolverErr(emulators.ErrNotConfigured), nil, "emulators.not_configured"},
		{"emulator file gone", resolverErr(emulators.ErrFileMissing), nil, "emulators.file_missing"},
		{"image gone", resolverErr(emulators.ErrNotConfigured), func(t *testing.T, _ *Service, game Game) {
			t.Helper()
			if err := os.Remove(game.Executable); err != nil {
				t.Fatal(err)
			}
		}, uierr.Code(errExecutableMissing)},
		{"unknown platform in the file", resolverErr(emulators.ErrNotConfigured), func(_ *testing.T, s *Service, _ Game) {
			s.games[0].Emulation = &Emulation{Platform: "gba"}
		}, "emulators.unknown_platform"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newLibrary(t)
			s.start = func(context.Context, launch) (gameProcess, error) {
				t.Error("nothing should have started")
				return nil, errors.New("unexpected")
			}
			if c.resolver != nil {
				s.SetEmulatorResolver(c.resolver)
			}
			game := emulatedGame(t, s, "game.iso")
			if c.mutate != nil {
				c.mutate(t, s, game)
			}
			if err := s.PlayGame(game.ID); uierr.Code(err) != c.code {
				t.Fatalf("PlayGame = %v, want code %s", err, c.code)
			}
			if s.IsRunning(game.ID) {
				t.Fatal("failed launch left a session")
			}
		})
	}
}

func TestNativeOnlyStarter(t *testing.T) {
	missing := errors.New("no runtime")
	started := ""
	start := nativeOnlyStarter(func(_ context.Context, req launch) (gameProcess, error) {
		started = req.executable
		return &stubProcess{}, nil
	}, missing)
	if _, err := start(t.Context(), launch{executable: "/Applications/DuckStation.app/Contents/MacOS/DuckStation"}); err != nil || started == "" {
		t.Fatalf("native program: %v, started %q", err, started)
	}
	started = ""
	if _, err := start(t.Context(), launch{executable: "/games/game.exe"}); !errors.Is(err, missing) || started != "" {
		t.Fatalf("windows program: %v, started %q", err, started)
	}
}

func TestDetectorDoesNotOpenASessionForAnEmulatedGame(t *testing.T) {
	s := newLibrary(t)
	game := emulatedGame(t, s, "game.iso")
	near := filepath.Join(game.InstallDir, "duckstation.exe")
	s.scan = func(context.Context) ([]procs.Process, bool, error) {
		return []procs.Process{{PID: 77, Path: near}}, true, nil
	}
	s.detectTick(t.Context())
	if s.IsRunning(game.ID) {
		t.Fatal("a program next to the image was taken for the game")
	}
}

func TestEmulatedSessionFollowsThePidOfTheEmulator(t *testing.T) {
	s := newLibrary(t)
	rec := recordLaunches(t, s)
	game := emulatedGame(t, s, "game.iso")
	start := time.Unix(1000, 0)
	s.now = func() time.Time { return start }
	s.watching = true
	alive := []procs.Process{{PID: 1234, Path: filepath.Join("emu", "dir", "emulator.exe")}}
	s.scan = func(context.Context) ([]procs.Process, bool, error) { return alive, true, nil }

	if err := s.PlayGame(game.ID); err != nil {
		t.Fatal(err)
	}
	s.detectTick(t.Context())
	if !s.IsRunning(game.ID) {
		t.Fatal("the session closed while the emulator was alive")
	}
	if pid, ok := s.RunningPID(game.ID); !ok || pid != 1234 {
		t.Fatalf("pid %d, %v", pid, ok)
	}

	s.now = func() time.Time { return start.Add(90 * time.Second) }
	alive = nil
	rec.finish(s)
	s.detectTick(t.Context())
	if s.IsRunning(game.ID) {
		t.Fatal("the session outlived the emulator")
	}
	stored, err := s.Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.PlaytimeSeconds != 90 || stored.LastPlayed == nil {
		t.Fatalf("playtime not recorded: %+v", stored)
	}
}

func TestEmulatedCrashIsAnalyzedAgainstTheEmulator(t *testing.T) {
	s := newLibrary(t)
	game := emulatedGame(t, s, "game.iso")
	program := filepath.Join(t.TempDir(), "emulator.exe")
	s.SetEmulatorResolver(func(string) (string, error) { return program, nil })
	proc := &exitProcess{id: 4100, exit: make(chan struct{}), err: codeError(int(0xC0000135))}
	s.start = func(context.Context, launch) (gameProcess, error) { return proc, nil }
	seen := make(chan string, 1)
	hooks := noJournalHooks()
	hooks.dumps = func(_ context.Context, dirs []crashlog.DumpDir, exe string, _, _ time.Time) (bool, error) {
		seen <- exe + "|" + dirs[len(dirs)-1].Path
		return false, nil
	}
	s.crash = hooks
	clock := newRegClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	s.now = clock.now

	if err := s.PlayGame(game.ID); err != nil {
		t.Fatal(err)
	}
	clock.set(clock.now().Add(emulatorQuickExitSeconds * time.Second))
	close(proc.exit)
	s.sessionWG.Wait()
	if got, want := <-seen, "emulator.exe|"+filepath.Dir(program); got != want {
		t.Fatalf("crash evidence looked at %q, want %q", got, want)
	}
}

func TestOperationsThatNeedAProgramAreRefused(t *testing.T) {
	cases := []struct {
		name string
		call func(s *Service, game Game) error
	}{
		{"run as admin", func(s *Service, game Game) error {
			_, err := s.SetRunAsAdmin(game.ID, true)
			return err
		}},
		{"saves folder", func(s *Service, game Game) error {
			_, err := s.SetSavesDir(game.ID, game.InstallDir)
			return err
		}},
		{"relocate", func(s *Service, game Game) error {
			_, err := s.Relocate(game.ID, t.TempDir())
			return err
		}},
		{"relocate check", func(s *Service, game Game) error { return s.CheckRelocatable(game.ID) }},
		{"native lookup", func(s *Service, game Game) error {
			_, err := s.FindNative(game.ID)
			return err
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newLibrary(t)
			game := emulatedGame(t, s, "game.iso")
			if err := c.call(s, game); !errors.Is(err, emulators.ErrUnsupported) {
				t.Fatalf("got %v", err)
			}
			after, err := s.Find(game.ID)
			if err != nil {
				t.Fatal(err)
			}
			if after.Executable != game.Executable || after.InstallDir != game.InstallDir || after.SavesDir != "" || after.RunAsAdmin {
				t.Fatalf("record changed: %+v", after)
			}
		})
	}
}

func TestRunAsAdminCanStillBeCleared(t *testing.T) {
	s := newLibrary(t)
	game := emulatedGame(t, s, "game.iso")
	s.games[0].RunAsAdmin = true
	if after, err := s.SetRunAsAdmin(game.ID, false); err != nil || after.RunAsAdmin {
		t.Fatalf("SetRunAsAdmin(false) = %+v, %v", after, err)
	}
}

func TestProfilesAreRefused(t *testing.T) {
	s := newLibrary(t)
	game := emulatedGame(t, s, "game.iso")
	in := ProfileInput{Name: "Alt", Executable: game.Executable}
	if _, err := s.AddProfile(game.ID, in); !errors.Is(err, errProfileExternal) {
		t.Fatalf("AddProfile = %v", err)
	}
	if err := s.UpdateProfile(game.ID, "p1", in); !errors.Is(err, errProfileExternal) {
		t.Fatalf("UpdateProfile = %v", err)
	}
}

func TestSavesAreNotLookedForNearTheImage(t *testing.T) {
	s := newLibrary(t)
	game := emulatedGame(t, s, "game.iso")
	writeFile(t, filepath.Join(game.InstallDir, "saves", "slot1.sav"), "x")
	s.saveRoots = nil
	got, err := s.LocateSaves(t.Context(), game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "" || len(got.Candidates) != 0 || len(got.Rejected) != 0 {
		t.Fatalf("the folder of the image was taken for saves: %+v", got)
	}
}

func TestRemovingAGameLeavesItsFolderAvailable(t *testing.T) {
	s := newLibrary(t)
	dir := t.TempDir()
	first := writeFile(t, filepath.Join(dir, "first.iso"), "x")
	second := writeFile(t, filepath.Join(dir, "second.iso"), "x")
	game, err := s.AddEmulatedGame(t.Context(), first, emulators.PS2, "A")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddEmulatedGame(t.Context(), second, emulators.PS2, "B"); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveGame(game.ID); err != nil {
		t.Fatal(err)
	}
	for _, rom := range []string{first, second} {
		if _, err := os.Stat(rom); err != nil {
			t.Fatalf("removing the record touched the file: %v", err)
		}
	}
	s.mu.Lock()
	ignored, err := s.excludedLocked(dir)
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if ignored {
		t.Fatal("removing an image excluded the whole folder from discovery")
	}
}

func TestDiscoveryLeavesAnEmulatedGameAlone(t *testing.T) {
	s := newLibrary(t)
	game := emulatedGame(t, s, "game.iso")
	exe := writeFile(t, filepath.Join(game.InstallDir, "real.exe"), "x")
	found, outcome, err := s.ApplyDiscovered(Discovered{Title: "Real", Executable: exe, InstallDir: game.InstallDir})
	if err != nil {
		t.Fatal(err)
	}
	if outcome != OutcomeCreated || found.ID == game.ID {
		t.Fatalf("discovered folder merged into the image: %v %+v", outcome, found)
	}
	after, err := s.Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Executable != game.Executable {
		t.Fatalf("image replaced: %+v", after)
	}
}

func TestLaterMeasurementKeepsTheSizeOfTheImage(t *testing.T) {
	s := newLibrary(t)
	game := emulatedGame(t, s, "game.iso")
	changed, err := s.UpdateMeasuredSize(game.ID, game.InstallDir, 1<<40, false)
	if err != nil {
		t.Fatal(err)
	}
	after, findErr := s.Find(game.ID)
	if findErr != nil {
		t.Fatal(findErr)
	}
	if changed || after.SizeBytes != game.SizeBytes {
		t.Fatalf("the folder size replaced the image size: %+v", after)
	}
}

func TestInstalledListIsForRealInstallations(t *testing.T) {
	s := newLibrary(t)
	emulatedGame(t, s, "game.iso")
	native, err := s.AddGame(t.Context(), tempGameExe(t), "Native")
	if err != nil {
		t.Fatal(err)
	}
	installed := s.GetInstalledGames()
	if len(installed) != 1 || installed[0].ID != native.ID {
		t.Fatalf("installed: %+v", installed)
	}
	if playable := s.GetPlayableGames(); len(playable) != 2 {
		t.Fatalf("playable: %+v", playable)
	}
	if got, err := s.FindNative(native.ID); err != nil || got.ID != native.ID {
		t.Fatalf("FindNative = %+v, %v", got, err)
	}
	if _, err := s.FindNative("nope"); !IsGameNotFound(err) {
		t.Fatalf("FindNative(unknown) = %v", err)
	}
}

func TestRecentGamesDoNotOfferTheImageAsAnIcon(t *testing.T) {
	s := newLibrary(t)
	game := emulatedGame(t, s, "game.iso")
	now := time.Now()
	s.games[0].LastPlayed = &now
	recent := s.RecentGames(5)
	if len(recent) != 1 || recent[0].ID != game.ID || recent[0].Executable != "" {
		t.Fatalf("recent: %+v", recent)
	}
}

func TestShortcutOfAnEmulatedGameRunsThroughTheLauncher(t *testing.T) {
	s := newLibrary(t)
	fake := newFakeShortcuts(t.TempDir())
	s.shortcuts = fake
	launcher := filepath.Join(t.TempDir(), "typhon.exe")
	s.launcherPath = func() (string, error) { return launcher, nil }
	game := emulatedGame(t, s, "game.iso")
	if err := s.CreateShortcut(game.ID); err != nil {
		t.Fatal(err)
	}
	for _, link := range fake.created {
		if link.Target != launcher || link.Icon != launcher {
			t.Fatalf("link: %+v", link)
		}
		return
	}
	t.Fatal("no shortcut created")
}

func TestSteamEntryOfAnEmulatedGameRunsThroughTheLauncher(t *testing.T) {
	launcher := filepath.Join(t.TempDir(), "typhon.exe")
	sc, err := steamProfileTarget(Game{
		ID: "abc", Title: "Game", Executable: filepath.Join(t.TempDir(), "game.iso"),
		Emulation: &Emulation{Platform: emulators.PS2},
	}, "", steamRoute{launcherPath: func() (string, error) { return launcher, nil }})
	if err != nil {
		t.Fatal(err)
	}
	if sc.Exe != launcher || sc.Icon != launcher || !strings.Contains(sc.LaunchOptions, "--play") {
		t.Fatalf("steam entry: %+v", sc)
	}
}
