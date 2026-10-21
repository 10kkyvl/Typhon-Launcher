package settings

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"typhon/internal/platform"
	"typhon/internal/uierr"
)

func foldersFixture(t *testing.T) (*Service, string, string) {
	t.Helper()
	base := t.TempDir()
	s := mustServiceAt(t, filepath.Join(base, "settings.json"))
	if _, err := s.SetupLibrary(filepath.Join(base, "main")); err != nil {
		t.Fatalf("setup library: %v", err)
	}
	return s, base, s.GetSettings().LibraryPath
}

func TestSanitizeExtraLibraryPaths(t *testing.T) {
	base := t.TempDir()
	library := filepath.Join(base, LibraryFolderName)
	other := filepath.Join(base, "Other")
	driveRoot := filepath.VolumeName(base) + string(filepath.Separator)

	cases := []struct {
		name    string
		extras  []string
		want    []string
		wantErr error
	}{
		{"nil", nil, []string{}, nil},
		{"blank entries are dropped", []string{"", "   "}, []string{}, nil},
		{"relative", []string{filepath.Join("relative", "games")}, nil, ErrLibraryPathRelative},
		{"drive root", []string{driveRoot}, nil, ErrLibraryPathRoot},
		{"cleaned", []string{other + string(filepath.Separator) + "." + string(filepath.Separator)}, []string{other}, nil},
		{"duplicate", []string{other, other + string(filepath.Separator)}, []string{other}, nil},
		{"two folders keep their order", []string{other, filepath.Join(base, "Second")}, []string{other, filepath.Join(base, "Second")}, nil},
		{"equals library", []string{library}, nil, ErrExtraLibraryPathNested},
		{"equals games folder", []string{filepath.Join(library, dirGames)}, nil, ErrExtraLibraryPathNested},
		{"inside games folder", []string{filepath.Join(library, dirGames, "More")}, nil, ErrExtraLibraryPathNested},
		{"inside downloads folder", []string{filepath.Join(library, dirDownloads, "More")}, nil, ErrExtraLibraryPathNested},
		{"contains library", []string{base}, nil, ErrExtraLibraryPathNested},
		{"inside another extra", []string{other, filepath.Join(other, "Inner")}, nil, ErrExtraLibraryPathNested},
		{"contains another extra", []string{filepath.Join(other, "Inner"), other}, nil, ErrExtraLibraryPathNested},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := Defaults()
			s.LibraryPath = library
			s.ExtraLibraryPaths = tc.extras
			got, err := sanitize(s)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("sanitize: %v", err)
			}
			if got.ExtraLibraryPaths == nil || !slices.Equal(got.ExtraLibraryPaths, tc.want) {
				t.Fatalf("extras = %#v, want %#v", got.ExtraLibraryPaths, tc.want)
			}
		})
	}
}

func TestSanitizeExtraLibraryPathsCaseFollowsTheFilesystem(t *testing.T) {
	want := 2
	if runtime.GOOS == "windows" {
		want = 1
	}
	base := t.TempDir()
	s := Defaults()
	s.LibraryPath = filepath.Join(base, LibraryFolderName)
	s.ExtraLibraryPaths = []string{filepath.Join(base, "Other"), strings.ToUpper(filepath.Join(base, "Other"))}
	got, err := sanitize(s)
	if err != nil {
		t.Fatalf("sanitize: %v", err)
	}
	if len(got.ExtraLibraryPaths) != want {
		t.Fatalf("extras = %q, want %d", got.ExtraLibraryPaths, want)
	}
}

func TestSanitizeLastInstallRoot(t *testing.T) {
	base := t.TempDir()
	library := filepath.Join(base, LibraryFolderName)
	extra := filepath.Join(base, "Other")
	cases := []struct {
		name string
		last string
		want string
	}{
		{"empty", "", ""},
		{"main games folder", filepath.Join(library, dirGames), filepath.Join(library, dirGames)},
		{"extra folder", extra, extra},
		{"no longer a root", filepath.Join(base, "Removed"), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := Defaults()
			s.LibraryPath = library
			s.ExtraLibraryPaths = []string{extra}
			s.LastInstallRoot = tc.last
			got, err := sanitize(s)
			if err != nil {
				t.Fatalf("sanitize: %v", err)
			}
			if got.LastInstallRoot != tc.want {
				t.Fatalf("LastInstallRoot = %q, want %q", got.LastInstallRoot, tc.want)
			}
		})
	}
}

func TestGamesRoots(t *testing.T) {
	s := Defaults()
	if got := s.GamesRoots(); len(got) != 0 {
		t.Fatalf("unconfigured roots = %q, want none", got)
	}
	s.ExtraLibraryPaths = []string{"X"}
	if got := s.GamesRoots(); !slices.Equal(got, []string{"X"}) {
		t.Fatalf("roots without a games folder = %q", got)
	}
	s.GamesPath = "G"
	if got := s.GamesRoots(); !slices.Equal(got, []string{"G", "X"}) {
		t.Fatalf("roots = %q, want games folder first", got)
	}
}

func TestExtraLibraryPathsSurviveReload(t *testing.T) {
	s, base, _ := foldersFixture(t)
	extra := filepath.Join(base, "second", "Games")
	if _, err := s.AddLibraryFolder(context.Background(), extra); err != nil {
		t.Fatalf("add: %v", err)
	}
	reloaded := mustServiceAt(t, s.path)
	if got := reloaded.GetSettings().ExtraLibraryPaths; !slices.Equal(got, []string{extra}) {
		t.Fatalf("after reload extras = %q, want %q", got, extra)
	}
}

func TestOlderSettingsFileLoadsWithNoExtraFolders(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"theme":"dark"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got := mustServiceAt(t, path).GetSettings()
	if got.ExtraLibraryPaths == nil || len(got.ExtraLibraryPaths) != 0 {
		t.Fatalf("extras = %#v, want an empty list", got.ExtraLibraryPaths)
	}
}

func TestNestedExtraFolderInStoredFileStopsTheService(t *testing.T) {
	base := t.TempDir()
	library := filepath.Join(base, LibraryFolderName)
	path := filepath.Join(base, "settings.json")
	body := fmt.Sprintf(`{"libraryPath":%q,"extraLibraryPaths":[%q]}`, library, filepath.Join(library, dirGames, "More"))
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewServiceAt(path); !errors.Is(err, ErrExtraLibraryPathNested) {
		t.Fatalf("err = %v, want ErrExtraLibraryPathNested", err)
	}
}

func TestAddLibraryFolderCreatesAndRecords(t *testing.T) {
	s, base, _ := foldersFixture(t)
	extra := filepath.Join(base, "disk2", "Games")

	saved, err := s.AddLibraryFolder(context.Background(), extra)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if !slices.Equal(saved.ExtraLibraryPaths, []string{extra}) {
		t.Fatalf("extras = %q, want %q", saved.ExtraLibraryPaths, extra)
	}
	if info, err := os.Stat(extra); err != nil || !info.IsDir() {
		t.Fatalf("folder was not created: %v", err)
	}
	if !slices.Equal(s.GetSettings().ExtraLibraryPaths, saved.ExtraLibraryPaths) {
		t.Fatal("stored settings differ from the returned ones")
	}
	again, err := s.AddLibraryFolder(context.Background(), extra)
	if err != nil {
		t.Fatalf("add twice: %v", err)
	}
	if len(again.ExtraLibraryPaths) != 1 {
		t.Fatalf("adding the same folder twice gave %q", again.ExtraLibraryPaths)
	}
}

func TestAddLibraryFolderRejectsBeforeTouchingTheDisk(t *testing.T) {
	s, base, library := foldersFixture(t)
	cases := []struct {
		name    string
		path    string
		wantErr error
		made    string
	}{
		{"relative", filepath.Join("relative", "games"), ErrLibraryPathRelative, ""},
		{"empty", "  ", ErrLibraryParentEmpty, ""},
		{"drive root", filepath.VolumeName(base) + string(filepath.Separator), ErrLibraryPathRoot, ""},
		{"inside games folder", filepath.Join(library, dirGames, "More", "Deeper"), ErrExtraLibraryPathNested, filepath.Join(library, dirGames, "More")},
		{"contains the library", base, ErrExtraLibraryPathNested, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := s.AddLibraryFolder(context.Background(), tc.path)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if tc.made != "" {
				if _, statErr := os.Stat(tc.made); !errors.Is(statErr, fs.ErrNotExist) {
					t.Fatalf("%s was created before the rejection: %v", tc.made, statErr)
				}
			}
			if got := s.GetSettings().ExtraLibraryPaths; len(got) != 0 {
				t.Fatalf("extras = %q, want none", got)
			}
		})
	}
	if code := uierr.Code(ErrExtraLibraryPathNested); code != "settings.extra_library_nested" {
		t.Fatalf("code = %q", code)
	}
}

func TestAddLibraryFolderLeavesNoDirectoryWhenTheSaveFails(t *testing.T) {
	s, base, _ := foldersFixture(t)
	if err := s.AddApplier(func(prev, next Settings) error {
		if len(next.ExtraLibraryPaths) != len(prev.ExtraLibraryPaths) {
			return errors.New("applier refuses")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	top := filepath.Join(base, "disk3")
	extra := filepath.Join(top, "Games", "Typhon")

	if _, err := s.AddLibraryFolder(context.Background(), extra); err == nil {
		t.Fatal("add succeeded though saving failed")
	}
	if _, err := os.Stat(top); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("created folders were left behind: %v", err)
	}
	if got := s.GetSettings().ExtraLibraryPaths; len(got) != 0 {
		t.Fatalf("extras = %q, want none", got)
	}
}

func TestAddLibraryFolderKeepsAFolderItDidNotCreate(t *testing.T) {
	s, base, _ := foldersFixture(t)
	if err := s.AddApplier(func(prev, next Settings) error {
		if len(next.ExtraLibraryPaths) != len(prev.ExtraLibraryPaths) {
			return errors.New("applier refuses")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(base, "mine")
	keep := filepath.Join(existing, "save.dat")
	if err := os.MkdirAll(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keep, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := s.AddLibraryFolder(context.Background(), existing); err == nil {
		t.Fatal("add succeeded though saving failed")
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("the user's folder was damaged: %v", err)
	}
}

func TestAddLibraryFolderUnderAFileFails(t *testing.T) {
	s, base, _ := foldersFixture(t)
	file := filepath.Join(base, "afile")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := s.AddLibraryFolder(context.Background(), filepath.Join(file, "Games"))
	if code := uierr.Code(err); code != "settings.library_create_failed" {
		t.Fatalf("err = %v, code %q, want settings.library_create_failed", err, code)
	}
	if got := s.GetSettings().ExtraLibraryPaths; len(got) != 0 {
		t.Fatalf("extras = %q, want none", got)
	}
}

func TestAddLibraryFolderCancelledContext(t *testing.T) {
	s, base, _ := foldersFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	extra := filepath.Join(base, "never")
	if _, err := s.AddLibraryFolder(ctx, extra); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if _, err := os.Stat(extra); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("folder was created for a cancelled call: %v", err)
	}
}

func TestRemoveLibraryFolderOnlyTakesItOffTheList(t *testing.T) {
	s, base, _ := foldersFixture(t)
	extra := filepath.Join(base, "disk2")
	if _, err := s.AddLibraryFolder(context.Background(), extra); err != nil {
		t.Fatal(err)
	}
	game := filepath.Join(extra, "Game", "game.exe")
	if err := os.MkdirAll(filepath.Dir(game), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(game, []byte("exe"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(func(next *Settings) error {
		next.LastInstallRoot = extra
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	saved, err := s.RemoveLibraryFolder(context.Background(), extra+string(filepath.Separator))
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if len(saved.ExtraLibraryPaths) != 0 {
		t.Fatalf("extras = %q, want none", saved.ExtraLibraryPaths)
	}
	if saved.LastInstallRoot != "" {
		t.Fatalf("LastInstallRoot = %q, want it cleared with the folder", saved.LastInstallRoot)
	}
	if _, err := os.Stat(game); err != nil {
		t.Fatalf("a file in the removed folder was touched: %v", err)
	}
}

func TestRemoveLibraryFolderUnknown(t *testing.T) {
	s, base, _ := foldersFixture(t)
	if _, err := s.RemoveLibraryFolder(context.Background(), filepath.Join(base, "nothing")); !errors.Is(err, ErrExtraLibraryPathUnknown) {
		t.Fatalf("err = %v, want ErrExtraLibraryPathUnknown", err)
	}
	if _, err := s.RemoveLibraryFolder(context.Background(), "relative"); !errors.Is(err, ErrLibraryPathRelative) {
		t.Fatalf("err = %v, want ErrLibraryPathRelative", err)
	}
}

func TestLibraryFolders(t *testing.T) {
	s, base, _ := foldersFixture(t)
	gone := filepath.Join(base, "gone")
	live := filepath.Join(base, "live")
	for _, dir := range []string{gone, live} {
		if _, err := s.AddLibraryFolder(context.Background(), dir); err != nil {
			t.Fatal(err)
		}
	}
	games := s.GetSettings().GamesPath

	t.Run("unavailable volume is listed", func(t *testing.T) {
		s.storageInfo = func(path string) (platform.StorageInfo, error) {
			if path == gone {
				return platform.StorageInfo{}, fmt.Errorf("disk space for %s: %w", path, fs.ErrNotExist)
			}
			return platform.StorageInfo{FreeBytes: 40, TotalBytes: 100}, nil
		}
		got, err := s.LibraryFolders(context.Background())
		if err != nil {
			t.Fatalf("LibraryFolders: %v", err)
		}
		want := []LibraryFolder{
			{Path: games, Primary: true, FreeBytes: 40, TotalBytes: 100, Available: true},
			{Path: gone},
			{Path: live, FreeBytes: 40, TotalBytes: 100, Available: true},
		}
		if !slices.Equal(got, want) {
			t.Fatalf("folders = %+v, want %+v", got, want)
		}
	})

	t.Run("other failure is an error", func(t *testing.T) {
		boom := errors.New("access denied")
		s.storageInfo = func(string) (platform.StorageInfo, error) { return platform.StorageInfo{}, boom }
		got, err := s.LibraryFolders(context.Background())
		if !errors.Is(err, boom) {
			t.Fatalf("err = %v, want %v", err, boom)
		}
		if got != nil {
			t.Fatalf("folders = %+v, want none with an error", got)
		}
	})

	t.Run("cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := s.LibraryFolders(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	})

	t.Run("sizes above int64 are clamped", func(t *testing.T) {
		s.storageInfo = func(string) (platform.StorageInfo, error) {
			return platform.StorageInfo{FreeBytes: ^uint64(0), TotalBytes: 1}, nil
		}
		got, err := s.LibraryFolders(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if got[0].FreeBytes <= 0 {
			t.Fatalf("FreeBytes = %d, want a positive clamp", got[0].FreeBytes)
		}
	})
}

func TestLibraryFoldersWithoutALibrary(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "settings.json"))
	got, err := s.LibraryFolders(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("folders = %#v, want an empty list", got)
	}
}

func TestConcurrentAddLibraryFolderKeepsEveryFolder(t *testing.T) {
	s, base, _ := foldersFixture(t)
	const n = 6
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = s.AddLibraryFolder(context.Background(), filepath.Join(base, fmt.Sprintf("disk%d", i)))
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("add %d: %v", i, err)
		}
	}
	if got := s.GetSettings().ExtraLibraryPaths; len(got) != n {
		t.Fatalf("extras = %q, want all %d folders", got, n)
	}
}
