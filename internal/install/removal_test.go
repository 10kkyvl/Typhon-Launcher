package install

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"typhon/internal/download"
	"typhon/internal/library"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func (f *fakeDownloads) deletedIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.deleted...)
}

func (f *fakeDownloads) setSeeding(id string, seeding bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d := f.items[id]
	d.Seeding = seeding
	f.items[id] = d
}

func gameDir(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	mkFile(t, filepath.Join(dir, name+".exe"), 1024)
	mkFile(t, filepath.Join(dir, "data", "content.pak"), 2048)
	return dir
}

func missing(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return true
	}
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return false
}

func TestRemoveGameDeletesOwnedDir(t *testing.T) {
	s, _, registrar := newTestService(t)
	dir := gameDir(t, "Game")
	registrar.put(library.Game{
		ID:          "g1",
		Title:       "Game",
		InstallDir:  dir,
		Executable:  filepath.Join(dir, "Game.exe"),
		Owned:       true,
		InstallType: string(TypePortable),
	})
	s.mu.Lock()
	s.items = append(s.items, &Installation{ID: "i1", GameID: "g1", Status: StatusCompleted, Destination: dir})
	s.mu.Unlock()

	if err := s.RemoveGame("g1", RemoveOptions{DeleteFiles: true}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if !missing(t, dir) {
		t.Fatal("install dir still present")
	}
	if _, err := registrar.Find("g1"); !errors.Is(err, errFakeNoGame) {
		t.Fatalf("library record kept: %v", err)
	}
	if items := s.List(); len(items) != 0 {
		t.Fatalf("installations kept: %+v", items)
	}
	if pending, err := s.removals.load(); err != nil || len(pending) != 0 {
		t.Fatalf("pending removals = %v, err = %v", pending, err)
	}
}

func TestRemoveGameKeepsRecordInLibrary(t *testing.T) {
	s, downloads, registrar := newTestService(t)
	dir := gameDir(t, "Freed")
	registrar.put(library.Game{
		ID:          "g1",
		Title:       "Freed",
		InstallDir:  dir,
		Executable:  filepath.Join(dir, "Freed.exe"),
		Owned:       true,
		InstallType: string(TypePortable),
	})

	if err := s.RemoveGame("g1", RemoveOptions{DeleteFiles: true, KeepInLibrary: true}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if !missing(t, dir) {
		t.Fatal("install dir still present")
	}
	game, err := registrar.Find("g1")
	if err != nil {
		t.Fatalf("library record dropped: %v", err)
	}
	if !game.Uninstalled {
		t.Fatalf("record not marked as uninstalled: %+v", game)
	}
	if len(registrar.removed) != 0 {
		t.Fatalf("record removed from library: %v", registrar.removed)
	}
	if items := s.List(); len(items) != 0 {
		t.Fatalf("installations kept: %+v", items)
	}
	if ids := downloads.deletedIDs(); len(ids) != 0 {
		t.Fatalf("downloads deleted: %v", ids)
	}
}

func TestRemoveGameRejectsKeepWithNothingToDelete(t *testing.T) {
	s, _, registrar := newTestService(t)
	dir := gameDir(t, "Foreign")
	registrar.put(library.Game{ID: "g1", Title: "Foreign", InstallDir: dir, Source: library.SourceDiscovered})

	if err := s.RemoveGame("g1", RemoveOptions{KeepInLibrary: true}); !errors.Is(err, errNothingToRemove) {
		t.Fatalf("remove: %v", err)
	}
	game, err := registrar.Find("g1")
	if err != nil {
		t.Fatalf("library record dropped: %v", err)
	}
	if game.Uninstalled {
		t.Fatal("record marked as uninstalled without removing anything")
	}
	if missing(t, dir) {
		t.Fatal("files deleted")
	}
}

func TestRemoveGameKeepsFilesWithoutOwnership(t *testing.T) {
	s, _, registrar := newTestService(t)
	dir := gameDir(t, "Foreign")
	registrar.put(library.Game{ID: "g1", Title: "Foreign", InstallDir: dir, Source: library.SourceDiscovered})

	info, err := s.InspectRemoval("g1")
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if info.Method != RemovalRecord {
		t.Fatalf("method = %s", info.Method)
	}
	if err := s.RemoveGame("g1", RemoveOptions{DeleteFiles: true}); !errors.Is(err, errUnsafeRemoval) {
		t.Fatalf("remove with files: %v", err)
	}
	if err := s.RemoveGame("g1", RemoveOptions{}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if missing(t, dir) {
		t.Fatal("files of a foreign install were deleted")
	}
	if _, err := registrar.Find("g1"); !errors.Is(err, errFakeNoGame) {
		t.Fatalf("library record kept: %v", err)
	}
}

func TestRemoveGameTrustsMarker(t *testing.T) {
	s, _, registrar := newTestService(t)
	dir := gameDir(t, "Marked")
	if err := library.WriteMarker(dir, library.Marker{GameID: "g1", Title: "Marked", InstalledAt: time.Now()}); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	registrar.put(library.Game{ID: "g1", Title: "Marked", InstallDir: dir})

	info, err := s.InspectRemoval("g1")
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if info.Method != RemovalFiles || !info.Owned {
		t.Fatalf("method = %s, owned = %v", info.Method, info.Owned)
	}
	if err := s.RemoveGame("g1", RemoveOptions{DeleteFiles: true}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if !missing(t, dir) {
		t.Fatal("marked install dir still present")
	}
}

func TestRemoveGameRefusesRunning(t *testing.T) {
	s, _, registrar := newTestService(t)
	dir := gameDir(t, "Running")
	registrar.put(library.Game{ID: "g1", InstallDir: dir, Owned: true})
	registrar.setRunning("g1", true)

	if err := s.RemoveGame("g1", RemoveOptions{DeleteFiles: true}); !errors.Is(err, errGameRunning) {
		t.Fatalf("remove: %v", err)
	}
	if missing(t, dir) {
		t.Fatal("files deleted while the game was running")
	}
}

func TestRemoveGameRefusesBusy(t *testing.T) {
	s, _, registrar := newTestService(t)
	dir := gameDir(t, "Busy")
	registrar.put(library.Game{ID: "g1", InstallDir: dir, Owned: true})
	s.SetBusyCheck(func(id string) bool { return id == "g1" })

	if err := s.RemoveGame("g1", RemoveOptions{DeleteFiles: true}); !errors.Is(err, errGameBusy) {
		t.Fatalf("remove: %v", err)
	}
	if missing(t, dir) {
		t.Fatal("files deleted while an update was running")
	}
}

func TestRemoveGameRefusesActiveInstallInSameDir(t *testing.T) {
	s, _, registrar := newTestService(t)
	dir := gameDir(t, "Installing")
	registrar.put(library.Game{ID: "g1", InstallDir: dir, Owned: true})
	s.mu.Lock()
	s.items = append(s.items, &Installation{ID: "i1", Status: StatusExtracting, Destination: dir})
	s.mu.Unlock()

	if err := s.RemoveGame("g1", RemoveOptions{DeleteFiles: true}); !errors.Is(err, errGameBusy) {
		t.Fatalf("remove: %v", err)
	}
	if missing(t, dir) {
		t.Fatal("files deleted while an install was writing into them")
	}
}

func TestRemoveGameRefusesSeedingIntoInstallDir(t *testing.T) {
	s, downloads, registrar := newTestService(t)
	dir := gameDir(t, "Seeded")
	downloads.add("d1", "Seeded", filepath.Dir(dir))
	downloads.setSeeding("d1", true)
	registrar.put(library.Game{ID: "g1", InstallDir: dir, Owned: true, SourceDownloadID: "d1"})

	if err := s.RemoveGame("g1", RemoveOptions{DeleteFiles: true}); !errors.Is(err, errRemoveSeeding) {
		t.Fatalf("remove: %v", err)
	}
	if missing(t, dir) {
		t.Fatal("files deleted while seeding from them")
	}
}

func TestRemoveGameDeletesDownloadOnRequest(t *testing.T) {
	s, downloads, registrar := newTestService(t)
	dir := gameDir(t, "WithDownload")
	downloads.add("d1", "WithDownload", t.TempDir())
	registrar.put(library.Game{ID: "g1", InstallDir: dir, Owned: true, SourceDownloadID: "d1"})

	if err := s.RemoveGame("g1", RemoveOptions{DeleteFiles: true, DeleteDownload: true}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if got := downloads.deletedIDs(); len(got) != 1 || got[0] != "d1" {
		t.Fatalf("deleted downloads = %v", got)
	}
}

func TestRemoveGameKeepsDownloadWithoutRequest(t *testing.T) {
	s, downloads, registrar := newTestService(t)
	dir := gameDir(t, "KeepDownload")
	downloads.add("d1", "KeepDownload", t.TempDir())
	registrar.put(library.Game{ID: "g1", InstallDir: dir, Owned: true, SourceDownloadID: "d1"})

	if err := s.RemoveGame("g1", RemoveOptions{DeleteFiles: true}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if got := downloads.deletedIDs(); len(got) != 0 {
		t.Fatalf("deleted downloads = %v", got)
	}
}

func TestRemoveGameRunsUninstaller(t *testing.T) {
	s, _, registrar := newTestService(t)
	runner := &fakeRunner{}
	s.runner = runner

	dir := gameDir(t, "External")
	uninstaller := filepath.Join(dir, "unins000.exe")
	mkFile(t, uninstaller, 512)
	registrar.put(library.Game{
		ID:          "g1",
		InstallDir:  dir,
		InstallType: string(TypeExeInstaller),
		Uninstall:   library.Uninstall{Command: `"` + uninstaller + `" /SILENT`},
	})

	info, err := s.InspectRemoval("g1")
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if info.Method != RemovalInstaller {
		t.Fatalf("method = %s", info.Method)
	}
	if err := s.RemoveGame("g1", RemoveOptions{}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	calls := runner.calls()
	if len(calls) != 1 {
		t.Fatalf("runner calls = %+v", calls)
	}
	if calls[0].Path != uninstaller {
		t.Fatalf("path = %s", calls[0].Path)
	}
	if len(calls[0].Args) != 1 || calls[0].Args[0] != "/SILENT" {
		t.Fatalf("args = %v", calls[0].Args)
	}
	if _, err := registrar.Find("g1"); !errors.Is(err, errFakeNoGame) {
		t.Fatalf("library record kept: %v", err)
	}
}

func TestRemoveGameKeepsRecordWhenUninstallerFails(t *testing.T) {
	table := []struct {
		name string
		code int
		want error
	}{
		{name: "cancelled", code: msiCancelled, want: errUninstallCancelled},
		{name: "failed", code: 5, want: errUninstallFailed},
	}
	for _, tc := range table {
		t.Run(tc.name, func(t *testing.T) {
			s, _, registrar := newTestService(t)
			s.runner = &fakeRunner{code: tc.code}
			dir := gameDir(t, "External")
			uninstaller := filepath.Join(dir, "unins000.exe")
			mkFile(t, uninstaller, 512)
			registrar.put(library.Game{
				ID:         "g1",
				InstallDir: dir,
				Owned:      true,
				Uninstall:  library.Uninstall{Command: uninstaller},
			})

			if err := s.RemoveGame("g1", RemoveOptions{DeleteFiles: true}); !errors.Is(err, tc.want) {
				t.Fatalf("remove: %v", err)
			}
			if _, err := registrar.Find("g1"); err != nil {
				t.Fatalf("library record dropped: %v", err)
			}
			if missing(t, dir) {
				t.Fatal("files deleted after a failed uninstall")
			}
		})
	}
}

func TestSweepRemovalsFinishesPending(t *testing.T) {
	dir := t.TempDir()
	staged := filepath.Join(t.TempDir(), "Game"+removingSuffix)
	mkFile(t, filepath.Join(staged, "leftover.bin"), 64)
	kept := filepath.Join(t.TempDir(), "Library")
	mkFile(t, filepath.Join(kept, "game.exe"), 64)

	first := mustServiceAt(t, dir)
	if err := first.removals.add(staged); err != nil {
		t.Fatalf("add staged: %v", err)
	}
	if err := first.removals.add(kept); err != nil {
		t.Fatalf("add kept: %v", err)
	}

	second := mustServiceAt(t, dir)
	second.settings = newTestSettings(t)
	if err := second.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("startup: %v", err)
	}
	if err := second.ServiceShutdown(); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	if !missing(t, staged) {
		t.Fatal("staged directory survived the sweep")
	}
	if missing(t, kept) {
		t.Fatal("sweep deleted a path outside the removal staging")
	}
}

func TestStageForRemovalRecordsIntentBeforeRename(t *testing.T) {
	s := mustServiceAt(t, t.TempDir())
	dir := gameDir(t, "Staged")

	staged, err := s.stageForRemoval(dir)
	if err != nil {
		t.Fatalf("stage: %v", err)
	}
	if !missing(t, dir) {
		t.Fatal("source directory still in place")
	}
	pending, err := s.removals.load()
	if err != nil {
		t.Fatalf("load pending: %v", err)
	}
	if len(pending) != 1 || pending[0] != staged {
		t.Fatalf("pending = %v, staged = %s", pending, staged)
	}
}

func TestRemovableDirRejectsUnsafeTargets(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(root, "link")
	target := filepath.Join(root, "target")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	symlinks := true
	if err := os.Symlink(target, link); err != nil {
		symlinks = false
	}
	file := filepath.Join(root, "file.txt")
	mkFile(t, file, 16)

	table := []struct {
		name string
		path string
		skip bool
	}{
		{name: "empty", path: ""},
		{name: "relative", path: "Games/Title"},
		{name: "volume root", path: filepath.VolumeName(root) + string(filepath.Separator)},
		{name: "symlink", path: link, skip: !symlinks},
		{name: "file", path: file},
	}
	for _, tc := range table {
		if tc.skip {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			if err := removableDir(tc.path); err == nil {
				t.Fatalf("removableDir(%q) accepted", tc.path)
			}
		})
	}
	if err := removableDir(target); err != nil {
		t.Fatalf("removableDir(%q): %v", target, err)
	}
}

func TestSplitCommand(t *testing.T) {
	table := []struct {
		name string
		in   string
		path string
		args []string
		fail bool
	}{
		{name: "quoted", in: `"C:\Games\unins000.exe" /SILENT /NORESTART`, path: `C:\Games\unins000.exe`, args: []string{"/SILENT", "/NORESTART"}},
		{name: "unquoted with spaces", in: `C:\Program Files\Game\unins000.exe /S`, path: `C:\Program Files\Game\unins000.exe`, args: []string{"/S"}},
		{name: "msiexec", in: `MsiExec.exe /I{2C3F1A0B-0000-0000-0000-000000000001}`, path: "MsiExec.exe", args: []string{"/I{2C3F1A0B-0000-0000-0000-000000000001}"}},
		{name: "quoted arg", in: `"unins.exe" "/dir=C:\Program Files\Game"`, path: "unins.exe", args: []string{`/dir=C:\Program Files\Game`}},
		{name: "empty", in: "   ", fail: true},
		{name: "unbalanced quote", in: `"C:\Games\unins000.exe /S`, fail: true},
	}
	for _, tc := range table {
		t.Run(tc.name, func(t *testing.T) {
			path, args, err := splitCommand(tc.in)
			if tc.fail {
				if err == nil {
					t.Fatalf("splitCommand(%q) = %q %v", tc.in, path, args)
				}
				return
			}
			if err != nil {
				t.Fatalf("splitCommand(%q): %v", tc.in, err)
			}
			if path != tc.path {
				t.Fatalf("path = %q, want %q", path, tc.path)
			}
			if len(args) != len(tc.args) {
				t.Fatalf("args = %v, want %v", args, tc.args)
			}
			for i := range args {
				if args[i] != tc.args[i] {
					t.Fatalf("args = %v, want %v", args, tc.args)
				}
			}
		})
	}
}

func TestUninstallSpecPrefersProductCode(t *testing.T) {
	if runtime.GOOS != "windows" {
		return
	}
	spec, err := uninstallSpec(library.Uninstall{
		ProductCode: "{2C3F1A0B-0000-0000-0000-000000000001}",
		Command:     `"C:\Games\unins000.exe"`,
	})
	if err != nil {
		t.Fatalf("uninstallSpec: %v", err)
	}
	if !filepath.IsAbs(spec.Path) {
		t.Fatalf("path = %q, want absolute", spec.Path)
	}
	if len(spec.Args) < 2 || spec.Args[0] != "/x" || spec.Args[1] != "{2C3F1A0B-0000-0000-0000-000000000001}" {
		t.Fatalf("args = %v", spec.Args)
	}
	if !slices.Contains(spec.Args, "/qn") || !spec.Background {
		t.Fatalf("msi removal must be quiet: args = %v background = %v", spec.Args, spec.Background)
	}
}

func TestPickUninstall(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "Game")
	before := map[string]uninstallEntry{
		"HKLM\\Old": {Key: "HKLM\\Old", DisplayName: "Old", Command: "old.exe"},
	}
	table := []struct {
		name  string
		after map[string]uninstallEntry
		want  string
		ok    bool
	}{
		{
			name: "by install location",
			after: map[string]uninstallEntry{
				"HKLM\\Old":    before["HKLM\\Old"],
				"HKLM\\Game":   {Key: "HKLM\\Game", DisplayName: "Something", Command: "unins.exe", InstallLocation: dest},
				"HKLM\\Redist": {Key: "HKLM\\Redist", DisplayName: "VC++ Runtime", Command: "vc.exe"},
			},
			want: "HKLM\\Game",
			ok:   true,
		},
		{
			name: "by name",
			after: map[string]uninstallEntry{
				"HKLM\\Old":    before["HKLM\\Old"],
				"HKLM\\Title":  {Key: "HKLM\\Title", DisplayName: "My Game", Command: "unins.exe"},
				"HKLM\\Redist": {Key: "HKLM\\Redist", DisplayName: "VC++ Runtime", Command: "vc.exe"},
			},
			want: "HKLM\\Title",
			ok:   true,
		},
		{
			name: "system components ignored",
			after: map[string]uninstallEntry{
				"HKLM\\Old":    before["HKLM\\Old"],
				"HKLM\\Hidden": {Key: "HKLM\\Hidden", DisplayName: "Update", Command: "u.exe", SystemComponent: true},
				"HKLM\\Single": {Key: "HKLM\\Single", DisplayName: "Whatever", Command: "unins.exe"},
			},
			want: "HKLM\\Single",
			ok:   true,
		},
		{
			name:  "nothing new",
			after: before,
		},
		{
			name: "ambiguous",
			after: map[string]uninstallEntry{
				"HKLM\\A": {Key: "HKLM\\A", DisplayName: "Alpha", Command: "a.exe"},
				"HKLM\\B": {Key: "HKLM\\B", DisplayName: "Beta", Command: "b.exe"},
			},
		},
		{
			name: "no command",
			after: map[string]uninstallEntry{
				"HKLM\\C": {Key: "HKLM\\C", DisplayName: "My Game"},
			},
		},
	}
	for _, tc := range table {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := pickUninstall(before, tc.after, dest, "My Game")
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v (%+v)", ok, tc.ok, got)
			}
			if ok && got.Key != tc.want {
				t.Fatalf("key = %q, want %q", got.Key, tc.want)
			}
		})
	}
}

func TestDownloadRootRespectsLayout(t *testing.T) {
	d := download.Download{Destination: filepath.Join("C:", "Torrents"), Name: "Game"}
	if got := downloadRoot(d); got != filepath.Join("C:", "Torrents", "Game") {
		t.Fatalf("root = %q", got)
	}
	if got := downloadRoot(download.Download{Destination: "", Name: "Game"}); got != "" {
		t.Fatalf("root = %q", got)
	}
}

func fakeUninstaller(t *testing.T, marker string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "unins000.exe")
	mkInstaller(t, path, marker)
	return path
}

func TestUninstallSpecUsesQuietCommandForUnknownEngine(t *testing.T) {
	if runtime.GOOS != "windows" {
		return
	}
	path := fakeUninstaller(t, "nothing recognizable here")
	spec, err := uninstallSpec(library.Uninstall{
		Command:      `"` + path + `"`,
		QuietCommand: `"` + path + `" /quiet /norestart`,
	})
	if err != nil {
		t.Fatalf("uninstallSpec: %v", err)
	}
	if !samePath(spec.Path, path) {
		t.Fatalf("path = %q, want %q", spec.Path, path)
	}
	if !slices.Equal(spec.Args, []string{"/quiet", "/norestart"}) {
		t.Fatalf("args = %v", spec.Args)
	}
	if !spec.Background {
		t.Fatal("quiet command must run as background process")
	}
}

// Запись GOG в реестре: QuietUninstallString там есть, но это /SILENT без _?=,
// то есть окно прогресса и возврат управления до конца удаления.
func TestUninstallSpecOverridesVendorQuietCommand(t *testing.T) {
	if runtime.GOOS != "windows" {
		return
	}
	path := fakeUninstaller(t, "Inno Setup Setup Data (5.6.2) (u)")
	spec, err := uninstallSpec(library.Uninstall{
		Command:      `"` + path + `"`,
		QuietCommand: `"` + path + `" /SILENT`,
	})
	if err != nil {
		t.Fatalf("uninstallSpec: %v", err)
	}
	want := []string{"/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART", "_?=" + filepath.Dir(path)}
	if !slices.Equal(spec.Args, want) {
		t.Fatalf("args = %v, want %v", spec.Args, want)
	}
	if !spec.Background {
		t.Fatal("silent uninstaller must run as background process")
	}
}

func TestUninstallSpecAddsSilentFlagsByEngine(t *testing.T) {
	if runtime.GOOS != "windows" {
		return
	}
	cases := []struct {
		name   string
		marker string
		want   []string
	}{
		{"inno", "Inno Setup Setup Data (5.6.2) (u)", []string{"/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART"}},
		{"nsis", "NullsoftInst", []string{"/S"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := fakeUninstaller(t, tc.marker)
			spec, err := uninstallSpec(library.Uninstall{Command: `"` + path + `"`})
			if err != nil {
				t.Fatalf("uninstallSpec: %v", err)
			}
			if !spec.Background {
				t.Fatal("silent uninstaller must run as background process")
			}
			// Установщик может задать вопрос, который ключами тишины не убрать:
			// скрытое окно оставило бы удаление висеть без видимой причины.
			if spec.Hidden {
				t.Fatal("uninstaller window must stay visible")
			}
			want := append(append([]string(nil), tc.want...), "_?="+filepath.Dir(path))
			if !slices.Equal(spec.Args, want) {
				t.Fatalf("args = %v, want %v", spec.Args, want)
			}
		})
	}
}

func TestUninstallSpecKeepsUnknownUninstallerInteractive(t *testing.T) {
	if runtime.GOOS != "windows" {
		return
	}
	path := fakeUninstaller(t, "nothing recognizable here")
	spec, err := uninstallSpec(library.Uninstall{Command: `"` + path + `" /keep`})
	if err != nil {
		t.Fatalf("uninstallSpec: %v", err)
	}
	if spec.Background {
		t.Fatal("unknown uninstaller must stay interactive")
	}
	if !slices.Equal(spec.Args, []string{"/keep"}) {
		t.Fatalf("args = %v", spec.Args)
	}
}

func TestRemovalPlanFallsBackWhenUninstallerGone(t *testing.T) {
	if runtime.GOOS != "windows" {
		return
	}
	s, _, registrar := newTestService(t)
	dir := t.TempDir()
	mkFile(t, filepath.Join(dir, "Game.exe"), 2048)
	registrar.putLocked(library.Game{
		ID:         "g1",
		Title:      "Game",
		InstallDir: dir,
		Owned:      true,
		Uninstall:  library.Uninstall{Command: `"` + filepath.Join(dir, "unins000.exe") + `"`},
	})

	info, err := s.InspectRemoval("g1")
	if err != nil {
		t.Fatalf("inspect removal: %v", err)
	}
	if info.Method != RemovalFiles {
		t.Fatalf("method = %s, want %s", info.Method, RemovalFiles)
	}
	if info.QuietUninstall {
		t.Fatal("missing uninstaller cannot be quiet")
	}
	if err := s.RemoveGame("g1", RemoveOptions{DeleteFiles: true}); err != nil {
		t.Fatalf("remove game: %v", err)
	}
	if exists(dir) {
		t.Fatalf("%s left behind", dir)
	}
	if _, err := registrar.Find("g1"); !errors.Is(err, errFakeNoGame) {
		t.Fatalf("record still in library: %v", err)
	}
}
