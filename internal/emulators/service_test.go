package emulators

import (
	"context"
	"errors"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"typhon/internal/settings"
	"typhon/internal/uierr"
)

func newTestService(t *testing.T, goos string) (*Service, *settings.Service) {
	t.Helper()
	store, err := settings.NewServiceAt(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewService(store)
	if err != nil {
		t.Fatal(err)
	}
	s.goos = goos
	s.home = func() (StorageHome, error) { return StorageHome{}, errors.New("no home in tests") }
	return s, store
}

func touch(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestNewServiceRefusesMissingSettings(t *testing.T) {
	if _, err := NewService(nil); err == nil {
		t.Fatal("service built without settings")
	}
}

func TestTableIsTheSingleSourceOfTruth(t *testing.T) {
	want := map[string][]string{
		PS1:  {"-batch", "-fullscreen", "--", "image"},
		PS2:  {"-batch", "-fullscreen", "--", "image"},
		PSP:  {"--pause-menu-exit", "--fullscreen", "image"},
		PS3:  {"--no-gui", "--fullscreen", "image"},
		Vita: {"--frontend", "sdl", "-F", "image"},
		PS4:  {"-f", "true", "-g", "image"},
	}
	for _, p := range Platforms() {
		page, err := url.Parse(p.Homepage)
		if err != nil || page.Scheme != "https" || page.Host == "" {
			t.Errorf("%s: homepage %q is not https", p.ID, p.Homepage)
		}
		if p.Name == "" || p.Emulator == "" || len(p.windowsExes) == 0 {
			t.Errorf("%s: incomplete row: %+v", p.ID, p)
		}
		if len(p.Extensions) == 0 && !p.HasGameFiles() {
			t.Errorf("%s: takes no file at all", p.ID)
		}
		// Bundle names are listed only as far as they were seen on a real Mac.
		if !p.HasGameFiles() && len(p.macBundles) == 0 {
			t.Errorf("%s: no macOS bundle", p.ID)
		}
		if got := p.Command("image"); !slices.Equal(got, want[p.ID]) {
			t.Errorf("%s: command %q, want %q", p.ID, got, want[p.ID])
		}
		for _, ext := range p.Extensions {
			if ext == ".exe" {
				t.Errorf("%s: a program must never count as an image", p.ID)
			}
		}
	}
	if len(Platforms()) != len(want) {
		t.Fatalf("platforms: %d, want %d", len(Platforms()), len(want))
	}
}

func TestCommandDoesNotEditTheTable(t *testing.T) {
	p, _ := Lookup(PSP)
	p.Command("first")
	if got := p.Command("second"); got[len(got)-1] != "second" {
		t.Fatalf("second command %q", got)
	}
}

func TestAcceptsImageIgnoresCase(t *testing.T) {
	p, _ := Lookup(PS2)
	if !p.AcceptsImage(`C:\Games\Game.ISO`) {
		t.Fatal("upper case extension refused")
	}
	if p.AcceptsImage(`C:\Games\Game.exe`) {
		t.Fatal("program accepted as an image")
	}
}

func TestSetPathValidates(t *testing.T) {
	dir := t.TempDir()
	exe := touch(t, filepath.Join(dir, "duckstation.exe"))
	macBinary := touch(t, filepath.Join(dir, "duckstation"))
	bat := touch(t, filepath.Join(dir, "run.bat"))
	cases := []struct {
		name     string
		goos     string
		platform string
		path     string
		want     error
	}{
		{"windows exe", "windows", PS1, exe, nil},
		{"mac binary", "darwin", PS1, macBinary, nil},
		{"unknown platform", "windows", "gba", exe, ErrUnknownPlatform},
		{"empty platform", "windows", "", exe, ErrUnknownPlatform},
		{"relative", "windows", PS1, "duckstation.exe", ErrPathRelative},
		{"control character", "windows", PS1, exe + "\n", nil},
		{"control character inside", "windows", PS1, filepath.Join(dir, "a\x01.exe"), ErrPathInvalid},
		{"no file", "windows", PS1, filepath.Join(dir, "none.exe"), ErrFileMissing},
		{"directory", "windows", PS1, dir, ErrNotRegularFile},
		{"not exe on windows", "windows", PS1, bat, ErrNotExe},
		{"exe refused on mac", "darwin", PS1, exe, ErrWindowsProgram},
		{"bat refused on mac", "darwin", PS1, bat, ErrWindowsProgram},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, store := newTestService(t, c.goos)
			row, err := s.SetPath(c.platform, c.path)
			if !errors.Is(err, c.want) {
				t.Fatalf("SetPath = %v, want %v", err, c.want)
			}
			if c.want != nil {
				if len(store.GetSettings().EmulatorPaths) != 0 {
					t.Fatalf("a refused path was stored: %v", store.GetSettings().EmulatorPaths)
				}
				return
			}
			if row.Status != StatusReady {
				t.Fatalf("status %q", row.Status)
			}
		})
	}
}

type linkInfo struct{ fs.FileInfo }

func (linkInfo) Mode() fs.FileMode { return fs.ModeSymlink }

func TestSetPathRefusesASymlink(t *testing.T) {
	s, _ := newTestService(t, "windows")
	s.lstat = func(string) (fs.FileInfo, error) { return linkInfo{}, nil }
	if _, err := s.SetPath(PS1, filepath.Join(t.TempDir(), "link.exe")); !errors.Is(err, ErrNotRegularFile) {
		t.Fatalf("SetPath = %v", err)
	}
}

func TestImagePatterns(t *testing.T) {
	psp, err := ImagePatterns([]string{PSP})
	if err != nil || psp != "*.iso;*.cso;*.chd;*.pbp" {
		t.Fatalf("PSP = %q, %v", psp, err)
	}
	all, err := ImagePatterns(nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(all, "*.iso") != 1 || !strings.Contains(all, "*.m3u") || !strings.Contains(all, "*.zso") {
		t.Fatalf("all = %q", all)
	}
	if _, err := ImagePatterns([]string{"gba"}); !errors.Is(err, ErrUnknownPlatform) {
		t.Fatalf("unknown = %v", err)
	}
}

func TestIsWindowsProgram(t *testing.T) {
	for path, want := range map[string]bool{"a.EXE": true, "a.bat": true, "a.msi": true, "a": false, "a.iso": false} {
		if got := IsWindowsProgram(path); got != want {
			t.Errorf("%s: %v", path, got)
		}
	}
}

func TestSetPathWithoutRightsIsNotAMissingFile(t *testing.T) {
	s, _ := newTestService(t, "windows")
	s.lstat = func(string) (fs.FileInfo, error) { return nil, fs.ErrPermission }
	_, err := s.SetPath(PS1, touch(t, filepath.Join(t.TempDir(), "a.exe")))
	if uierr.Code(err) != uierr.Code(ErrFileUnreadable) || !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("SetPath = %v", err)
	}
}

func TestSetPathReducesAnAppBundleToItsBinary(t *testing.T) {
	root := t.TempDir()
	known := filepath.Join(root, "DuckStation.app")
	touch(t, filepath.Join(known, "Contents", "MacOS", "helper"))
	binary := touch(t, filepath.Join(known, "Contents", "MacOS", "DuckStation"))
	single := filepath.Join(root, "Renamed.app")
	only := touch(t, filepath.Join(single, "Contents", "MacOS", "whatever"))
	ambiguous := filepath.Join(root, "Two.app")
	touch(t, filepath.Join(ambiguous, "Contents", "MacOS", "a"))
	touch(t, filepath.Join(ambiguous, "Contents", "MacOS", "b"))
	empty := filepath.Join(root, "Empty.app")
	if err := os.MkdirAll(empty, 0o700); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		path string
		want string
		err  error
	}{
		{"known name wins", known, binary, nil},
		{"single binary", single, only, nil},
		{"ambiguous", ambiguous, "", ErrBadAppBundle},
		{"no binary", empty, "", ErrBadAppBundle},
		{"no bundle", filepath.Join(root, "None.app"), "", ErrBadAppBundle},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, store := newTestService(t, "darwin")
			_, err := s.SetPath(PS1, c.path)
			if !errors.Is(err, c.err) {
				t.Fatalf("SetPath = %v, want %v", err, c.err)
			}
			if got := store.GetSettings().EmulatorPaths[PS1]; got != c.want {
				t.Fatalf("stored %q, want %q", got, c.want)
			}
		})
	}
}

func TestAppBundleIsADirectoryOutsideMacOS(t *testing.T) {
	bundle := filepath.Join(t.TempDir(), "DuckStation.app")
	touch(t, filepath.Join(bundle, "Contents", "MacOS", "DuckStation"))
	s, _ := newTestService(t, "windows")
	if _, err := s.SetPath(PS1, bundle); !errors.Is(err, ErrNotRegularFile) {
		t.Fatalf("SetPath = %v", err)
	}
}

func TestEmptyPathForgetsTheEmulator(t *testing.T) {
	s, store := newTestService(t, "windows")
	exe := touch(t, filepath.Join(t.TempDir(), "a.exe"))
	other := touch(t, filepath.Join(t.TempDir(), "b.exe"))
	if _, err := s.SetPath(PS1, exe); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetPath(PS2, other); err != nil {
		t.Fatal(err)
	}
	row, err := s.SetPath(PS1, "  ")
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != StatusNotSet || row.Path != "" {
		t.Fatalf("row after reset: %+v", row)
	}
	paths := store.GetSettings().EmulatorPaths
	if _, ok := paths[PS1]; ok || paths[PS2] != other {
		t.Fatalf("paths after reset: %v", paths)
	}
}

func TestSetPathDoesNotEditSharedSettingsMap(t *testing.T) {
	s, store := newTestService(t, "windows")
	before := store.GetSettings()
	if _, err := s.SetPath(PS1, touch(t, filepath.Join(t.TempDir(), "a.exe"))); err != nil {
		t.Fatal(err)
	}
	if len(before.EmulatorPaths) != 0 {
		t.Fatalf("a copy taken earlier changed: %v", before.EmulatorPaths)
	}
}

func TestSetPathReportsAFailedSaveAndKeepsMemory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	store, err := settings.NewServiceAt(path)
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewService(store)
	if err != nil {
		t.Fatal(err)
	}
	s.goos = "windows"
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	_, err = s.SetPath(PS1, touch(t, filepath.Join(dir, "a.exe")))
	if uierr.Code(err) != uierr.Code(ErrSaveFailed) {
		t.Fatalf("SetPath = %v", err)
	}
	if got := store.GetSettings().EmulatorPaths; len(got) != 0 {
		t.Fatalf("failed save stayed in memory: %v", got)
	}
}

func TestListTellsTheStatesApart(t *testing.T) {
	s, _ := newTestService(t, "windows")
	ready := touch(t, filepath.Join(t.TempDir(), "a.exe"))
	gone := touch(t, filepath.Join(t.TempDir(), "b.exe"))
	if _, err := s.SetPath(PS1, ready); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetPath(PS2, gone); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}
	rows, err := s.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != len(platforms) {
		t.Fatalf("rows: %+v", rows)
	}
	want := []string{StatusReady, StatusMissing, StatusNotSet, StatusNotSet, StatusNotSet, StatusNotSet}
	for i, row := range rows {
		if row.Status != want[i] {
			t.Errorf("%s: status %q, want %q", row.ID, row.Status, want[i])
		}
		if row.Homepage == "" || row.Emulator == "" || (len(row.Extensions) == 0 && len(row.ImageNames) == 0) {
			t.Errorf("%s: incomplete row %+v", row.ID, row)
		}
	}
	if rows[1].Path != gone {
		t.Fatalf("the lost path is not shown: %+v", rows[1])
	}
}

func TestListDoesNotCallAnUnreadableFileMissing(t *testing.T) {
	s, _ := newTestService(t, "windows")
	if _, err := s.SetPath(PS1, touch(t, filepath.Join(t.TempDir(), "a.exe"))); err != nil {
		t.Fatal(err)
	}
	s.lstat = func(string) (fs.FileInfo, error) { return nil, fs.ErrPermission }
	rows, err := s.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Status != StatusUnavailable || rows[0].Reason != uierr.Code(ErrFileUnreadable) {
		t.Fatalf("row: %+v", rows[0])
	}
}

func TestListStopsOnCancelledContext(t *testing.T) {
	s, _ := newTestService(t, "windows")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.List(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("List = %v", err)
	}
}

func TestResolve(t *testing.T) {
	exe := touch(t, filepath.Join(t.TempDir(), "a.exe"))
	gone := touch(t, filepath.Join(t.TempDir(), "gone.exe"))
	cases := []struct {
		name     string
		platform string
		stored   string
		want     string
		err      error
	}{
		{"ready", PS1, exe, exe, nil},
		{"not set", PS1, "", "", ErrNotConfigured},
		{"unknown", "gba", "", "", ErrUnknownPlatform},
		{"file gone", PS1, gone, "", ErrFileMissing},
		{"hand edited relative path", PS1, "a.exe", "", ErrPathRelative},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, store := newTestService(t, "windows")
			if c.stored != "" {
				if _, err := store.Update(func(next *settings.Settings) error {
					next.EmulatorPaths = map[string]string{c.platform: c.stored}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Remove(gone); err != nil && !errors.Is(err, fs.ErrNotExist) {
				t.Fatal(err)
			}
			got, err := s.Resolve(c.platform)
			if !errors.Is(err, c.err) || got != c.want {
				t.Fatalf("Resolve = %q, %v", got, err)
			}
		})
	}
}

func TestResolveRefusesAWindowsProgramOnMac(t *testing.T) {
	s, store := newTestService(t, "darwin")
	exe := touch(t, filepath.Join(t.TempDir(), "a.exe"))
	if _, err := store.Update(func(next *settings.Settings) error {
		next.EmulatorPaths = map[string]string{PS1: exe}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Resolve(PS1); !errors.Is(err, ErrWindowsProgram) {
		t.Fatalf("Resolve = %v", err)
	}
}

func TestOfficialPage(t *testing.T) {
	s, _ := newTestService(t, "windows")
	page, err := s.OfficialPage(PS2)
	if err != nil || page != "https://pcsx2.net/" {
		t.Fatalf("OfficialPage = %q, %v", page, err)
	}
	if _, err := s.OfficialPage(""); !errors.Is(err, ErrUnknownPlatform) {
		t.Fatalf("OfficialPage(empty) = %v", err)
	}
}

func TestDetectFindsOnlyExistingFilesAndStoresNothing(t *testing.T) {
	root := t.TempDir()
	duck := touch(t, filepath.Join(root, "DuckStation", "duckstation-qt-x64-ReleaseLTCG.exe"))
	touch(t, filepath.Join(root, "PCSX2", "unrelated.exe"))
	if err := os.MkdirAll(filepath.Join(root, "PPSSPP", "PPSSPPWindows64.exe"), 0o700); err != nil {
		t.Fatal(err)
	}
	s, store := newTestService(t, "windows")
	s.roots = func() ([]string, error) { return []string{root}, nil }

	got, err := s.Detect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Found) != 1 || got.Found[0].Platform != PS1 || got.Found[0].Path != duck {
		t.Fatalf("Detect = %+v", got)
	}
	if len(store.GetSettings().EmulatorPaths) != 0 {
		t.Fatalf("Detect stored a path: %v", store.GetSettings().EmulatorPaths)
	}
}

func TestDetectLooksInsideMacBundles(t *testing.T) {
	root := t.TempDir()
	binary := touch(t, filepath.Join(root, "PCSX2.app", "Contents", "MacOS", "PCSX2"))
	s, _ := newTestService(t, "darwin")
	s.roots = func() ([]string, error) { return []string{root}, nil }
	got, err := s.Detect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Found) != 1 || got.Found[0].Platform != PS2 || got.Found[0].Path != binary {
		t.Fatalf("Detect = %+v", got)
	}
}

// The layouts are the ones of RPCS3 v0.0.43 (macos_aarch64) and the Vita3K
// continuous build of 2026-11-03.
func TestDetectFindsNextGenMacBundles(t *testing.T) {
	root := t.TempDir()
	rpcs3 := touch(t, filepath.Join(root, "RPCS3.app", "Contents", "MacOS", "rpcs3"))
	vita3k := touch(t, filepath.Join(root, "Vita3K.app", "Contents", "MacOS", "Vita3K"))
	s, _ := newTestService(t, "darwin")
	s.roots = func() ([]string, error) { return []string{root}, nil }
	got, err := s.Detect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := []Found{{Platform: PS3, Path: rpcs3}, {Platform: Vita, Path: vita3k}}
	if !slices.Equal(got.Found, want) {
		t.Fatalf("Detect = %+v, want %+v", got.Found, want)
	}
}

func TestDetectCountsWhatItCouldNotCheck(t *testing.T) {
	s, _ := newTestService(t, "windows")
	s.roots = func() ([]string, error) { return []string{t.TempDir()}, nil }
	s.lstat = func(string) (fs.FileInfo, error) { return nil, fs.ErrPermission }
	got, err := s.Detect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got.Unreadable == 0 || len(got.Found) != 0 {
		t.Fatalf("Detect = %+v", got)
	}
}

func TestDetectStopsOnCancelledContext(t *testing.T) {
	s, _ := newTestService(t, "windows")
	s.roots = func() ([]string, error) { return []string{t.TempDir()}, nil }
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.Detect(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Detect = %v", err)
	}
}

func TestDetectReportsWhenTheFoldersCannotBeResolved(t *testing.T) {
	s, _ := newTestService(t, "darwin")
	boom := errors.New("no home")
	s.roots = func() ([]string, error) { return nil, boom }
	if _, err := s.Detect(t.Context()); !errors.Is(err, boom) {
		t.Fatalf("Detect = %v", err)
	}
}

func TestImageConsolesFollowTheLayoutOnDisk(t *testing.T) {
	s, _ := newTestService(t, "darwin")
	root := t.TempDir()
	ps3 := touch(t, filepath.Join(root, "Game", "PS3_GAME", "USRDIR", "EBOOT.BIN"))
	touch(t, filepath.Join(root, "Game", "PS3_GAME", "PARAM.SFO"))
	next := touch(t, filepath.Join(root, "Next", "eboot.bin"))
	touch(t, filepath.Join(root, "Next", "sce_sys", "param.sfo"))
	bare := touch(t, filepath.Join(root, "Bare", "eboot.bin"))
	cases := []struct {
		name, image string
		want        []string
	}{
		{"a ps3 game folder", ps3, []string{PS3}},
		{"a ps vita or ps4 folder", next, []string{Vita, PS4}},
		{"no proof beside the file", bare, []string{}},
		{"an image of another console", touch(t, filepath.Join(root, "game.iso")), []string{}},
		{"a relative path", filepath.Join("Game", "PS3_GAME", "USRDIR", "EBOOT.BIN"), []string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := s.ImageConsoles(t.Context(), c.image)
			if err != nil || !slices.Equal(got, c.want) {
				t.Fatalf("ImageConsoles = %q, %v; want %q", got, err, c.want)
			}
		})
	}
}

func TestStatusWarnsOfTheDiscordHangOnMac(t *testing.T) {
	pref := t.TempDir()
	program := touch(t, filepath.Join(t.TempDir(), "Vita3K.app", "Contents", "MacOS", "Vita3K"))
	cases := []struct {
		name, goos, config string
		want               []string
	}{
		{"on", "darwin", "log-level: 0\ndiscord-rich-presence: true\n", []string{WarnDiscordExitHang}},
		{"off", "darwin", "discord-rich-presence: false\n", []string{}},
		{"not said", "darwin", "log-level: 0\n", []string{}},
		{"said twice", "darwin", "discord-rich-presence: true\ndiscord-rich-presence: true\n", []string{}},
		{"windows", "windows", "discord-rich-presence: true\n", []string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, _ := newTestService(t, c.goos)
			s.home = func() (StorageHome, error) { return StorageHome{ConfigDir: pref, Root: filepath.Join(pref, "fs")}, nil }
			plant(t, filepath.Join(pref, storageConfig), []byte(c.config))
			p, _ := Lookup(Vita)
			got := s.statusOf(p, program)
			if c.goos == "darwin" && got.Status != StatusReady {
				t.Fatalf("status = %+v", got)
			}
			if !slices.Equal(got.Warnings, c.want) {
				t.Fatalf("warnings = %q, want %q", got.Warnings, c.want)
			}
		})
	}
	s, _ := newTestService(t, "darwin")
	s.home = func() (StorageHome, error) { return StorageHome{ConfigDir: pref}, nil }
	plant(t, filepath.Join(pref, storageConfig), []byte("discord-rich-presence: true\n"))
	ps3, _ := Lookup(PS3)
	if got := s.statusOf(ps3, program); len(got.Warnings) != 0 {
		t.Fatalf("another console warned: %q", got.Warnings)
	}
}
