package install

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

type entryWorld struct {
	mu       sync.Mutex
	cur      map[string]uninstallEntry
	reads    int
	failFrom int
}

func newEntryWorld(pre ...uninstallEntry) *entryWorld {
	w := &entryWorld{cur: map[string]uninstallEntry{}}
	w.add(pre...)
	return w
}

func (w *entryWorld) add(entries ...uninstallEntry) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, e := range entries {
		w.cur[e.Key] = e
	}
}

func (w *entryWorld) read() (map[string]uninstallEntry, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.reads++
	if w.failFrom > 0 && w.reads >= w.failFrom {
		return nil, errors.New("registry unavailable")
	}
	out := make(map[string]uninstallEntry, len(w.cur))
	for k, v := range w.cur {
		out[k] = v
	}
	return out, nil
}

type locationDirs struct {
	outside   string
	downloads string
	missing   string
	root      string
}

type locationCase struct {
	name  string
	pre   func(d locationDirs) []uninstallEntry
	added func(d locationDirs) []uninstallEntry
	found bool
}

func uninstaller(dir string) string {
	return `"` + filepath.Join(dir, "Uninstall.exe") + `" /x`
}

func locationCases() []locationCase {
	return []locationCase{
		{
			name: "install location of a new entry",
			added: func(d locationDirs) []uninstallEntry {
				return []uninstallEntry{{Key: "HKLM\\Game", DisplayName: "Game 1.1.7", Command: uninstaller(d.outside), InstallLocation: d.outside + string(filepath.Separator)}}
			},
			found: true,
		},
		{
			name: "no install location, uninstall string names the directory",
			added: func(d locationDirs) []uninstallEntry {
				return []uninstallEntry{{Key: "HKLM\\Game", DisplayName: "Game 1.1.7", Command: uninstaller(d.outside)}}
			},
			found: true,
		},
		{
			name: "no install location, quiet uninstall string names the directory",
			added: func(d locationDirs) []uninstallEntry {
				return []uninstallEntry{{Key: "HKLM\\Game", DisplayName: "Game 1.1.7", QuietCommand: uninstaller(d.outside)}}
			},
			found: true,
		},
		{
			name: "no install location, display icon names the directory",
			added: func(d locationDirs) []uninstallEntry {
				return []uninstallEntry{{
					Key: "HKLM\\Game", DisplayName: "Game 1.1.7", Command: "msiexec.exe /x {A}",
					Icon: `"` + filepath.Join(d.outside, "Game.exe") + `",0`,
				}}
			},
			found: true,
		},
		{
			name: "install location that does not exist",
			added: func(d locationDirs) []uninstallEntry {
				return []uninstallEntry{{Key: "HKLM\\Game", DisplayName: "Game", Command: uninstaller(d.missing), InstallLocation: d.missing}}
			},
		},
		{
			name: "install location is a drive root",
			added: func(d locationDirs) []uninstallEntry {
				return []uninstallEntry{{Key: "HKLM\\Game", DisplayName: "Game", Command: uninstaller(d.root), InstallLocation: d.root}}
			},
		},
		{
			name: "install location is the downloads folder",
			added: func(d locationDirs) []uninstallEntry {
				return []uninstallEntry{{Key: "HKLM\\Game", DisplayName: "Game", Command: uninstaller(d.downloads), InstallLocation: d.downloads}}
			},
		},
		{
			name: "install location is relative",
			added: func(d locationDirs) []uninstallEntry {
				return []uninstallEntry{{Key: "HKLM\\Game", DisplayName: "Game", Command: "uninst.exe", InstallLocation: filepath.Base(d.outside)}}
			},
		},
		{
			name: "entry that was already registered before the install",
			pre: func(d locationDirs) []uninstallEntry {
				return []uninstallEntry{{Key: "HKLM\\Game", DisplayName: "Game", Command: uninstaller(d.outside), InstallLocation: d.outside}}
			},
		},
		{
			name: "system component entry",
			added: func(d locationDirs) []uninstallEntry {
				return []uninstallEntry{{Key: "HKLM\\Game", DisplayName: "Game", Command: uninstaller(d.outside), InstallLocation: d.outside, SystemComponent: true}}
			},
		},
		{
			name: "entry without any uninstall command",
			added: func(d locationDirs) []uninstallEntry {
				return []uninstallEntry{{Key: "HKLM\\Game", DisplayName: "Game", InstallLocation: d.outside}}
			},
		},
	}
}

func locationDirsFor(t *testing.T, r *rig) locationDirs {
	t.Helper()
	downloads := r.cfg.GetSettings().DownloadsPath
	if downloads == "" {
		t.Fatal("settings have no downloads folder")
	}
	if err := os.MkdirAll(downloads, 0o755); err != nil {
		t.Fatalf("create downloads folder: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "GamersGoMakers")
	return locationDirs{
		outside:   outside,
		downloads: downloads,
		missing:   filepath.Join(t.TempDir(), "Gone"),
		root:      filepath.VolumeName(outside) + string(filepath.Separator),
	}
}

func writeOutside(t *testing.T, dir string) func(runSpec) {
	return func(runSpec) {
		mkFile(t, filepath.Join(dir, "Game.exe"), 512<<10)
		mkFile(t, filepath.Join(dir, "Uninstall.exe"), 4096)
	}
}

func TestInteractiveInstallOutsideRootsFoundThroughUninstallEntry(t *testing.T) {
	for _, tc := range locationCases() {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			interactiveSource(t, r)
			d := locationDirsFor(t, r)
			var pre, added []uninstallEntry
			if tc.pre != nil {
				pre = tc.pre(d)
			}
			if tc.added != nil {
				added = tc.added(d)
			}
			world := newEntryWorld(pre...)
			r.s.readEntries = world.read
			install := writeOutside(t, d.outside)
			r.setRunner(newRgRunner(t, rgStep{act: func(spec runSpec) {
				install(spec)
				world.add(added...)
			}}))

			item, err := r.s.Start("d1", StartOptions{})
			if err != nil {
				t.Fatalf("Start: %v", err)
			}
			got := r.settle(item.ID)
			if got.Status != StatusWaitingForUser {
				t.Fatalf("status = %s (%q), want %s", got.Status, got.Error, StatusWaitingForUser)
			}
			if !tc.found {
				if got.Destination != "" || len(got.Candidates) != 0 {
					t.Fatalf("destination %q and candidates %v found from an entry that must be ignored", got.Destination, got.Candidates)
				}
				return
			}
			if got.Destination != d.outside {
				t.Fatalf("destination = %q, want %q", got.Destination, d.outside)
			}
			want := filepath.Join(d.outside, "Game.exe")
			if len(got.Candidates) == 0 || got.Candidates[0].Path != want {
				t.Fatalf("candidates = %+v, want %s first", got.Candidates, want)
			}
			if got.Owned {
				t.Fatal("a directory known only from the registry was claimed as owned")
			}
			if got.Uninstall.Key != added[0].Key {
				t.Fatalf("uninstall = %+v, want the entry %s", got.Uninstall, added[0].Key)
			}
		})
	}
}

func TestSilentInstallOutsideRootsFoundThroughUninstallEntry(t *testing.T) {
	for _, tc := range locationCases() {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			dest := silentSource(t, r, rgInnoMarker)
			d := locationDirsFor(t, r)
			var pre, added []uninstallEntry
			if tc.pre != nil {
				pre = tc.pre(d)
			}
			if tc.added != nil {
				added = tc.added(d)
			}
			world := newEntryWorld(pre...)
			r.s.readEntries = world.read
			install := writeOutside(t, d.outside)
			r.setRunner(newRgRunner(t, rgStep{act: func(spec runSpec) {
				install(spec)
				world.add(added...)
			}}))

			item, err := r.s.Start("d1", StartOptions{Destination: dest})
			if err != nil {
				t.Fatalf("Start: %v", err)
			}
			got := r.settle(item.ID)
			if !tc.found {
				if got.Status != StatusFailed || errCode(errors.New(got.Error)) != "install.installer_no_output" {
					t.Fatalf("status = %s (%q), want a failure with install.installer_no_output", got.Status, got.Error)
				}
				return
			}
			if got.Status != StatusCompleted {
				t.Fatalf("status = %s (%q), want %s", got.Status, got.Error, StatusCompleted)
			}
			if got.Destination != d.outside {
				t.Fatalf("destination = %q, want %q", got.Destination, d.outside)
			}
			if got.Owned {
				t.Fatal("a directory known only from the registry was claimed as owned")
			}
			if exists(dest) {
				t.Fatalf("the empty directory %s asked for was left behind", dest)
			}
			if games := r.reg.registered(); len(games) != 1 || games[0].InstallDir != d.outside {
				t.Fatalf("registered = %+v, want one game in %s", games, d.outside)
			}
		})
	}
}

func TestUninstallEntryReadFailureKeepsSnapshotResult(t *testing.T) {
	t.Run("interactive install inside the roots", func(t *testing.T) {
		r := newRig(t)
		inside := interactiveSource(t, r)
		world := newEntryWorld()
		world.failFrom = 2
		r.s.readEntries = world.read
		r.setRunner(newRgRunner(t, rgStep{act: writeOutside(t, inside)}))

		item, err := r.s.Start("d1", StartOptions{})
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
		got := r.settle(item.ID)
		if got.Status != StatusWaitingForUser || got.Destination != inside {
			t.Fatalf("status = %s destination = %q (%q), want %s and %q", got.Status, got.Destination, got.Error, StatusWaitingForUser, inside)
		}
		if !got.UninstallUnknown {
			t.Fatal("unreadable uninstall entries were not marked UninstallUnknown")
		}
	})
	t.Run("interactive install outside the roots", func(t *testing.T) {
		r := newRig(t)
		interactiveSource(t, r)
		d := locationDirsFor(t, r)
		world := newEntryWorld()
		world.failFrom = 2
		r.s.readEntries = world.read
		r.setRunner(newRgRunner(t, rgStep{act: writeOutside(t, d.outside)}))

		item, err := r.s.Start("d1", StartOptions{})
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
		got := r.settle(item.ID)
		if got.Status != StatusWaitingForUser || got.Destination != "" || len(got.Candidates) != 0 {
			t.Fatalf("status = %s destination = %q candidates = %v (%q), want an empty waiting record", got.Status, got.Destination, got.Candidates, got.Error)
		}
	})
}
