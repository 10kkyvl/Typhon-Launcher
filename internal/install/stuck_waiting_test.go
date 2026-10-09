package install

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"typhon/internal/library"
)

func TestInteractiveInstallOfAGameKnownOnlyFromItsRegistryEntry(t *testing.T) {
	r := newRig(t)
	root := t.TempDir()
	mkFile(t, filepath.Join(root, "GamersGoMakers", "setup.exe"), 4096)
	r.download("d1", "GamersGoMakers", root)
	game := filepath.Join(t.TempDir(), "GamersGoMakers")
	uninstall := filepath.Join(game, "Uninstall.exe")
	entry := uninstallEntry{
		Key:             `HKLM32\GamersGoMakers 1.1.7`,
		DisplayName:     "GamersGoMakers 1.1.7",
		Command:         uninstall,
		Icon:            uninstall,
		InstallLocation: game + string(filepath.Separator),
	}
	world := newEntryWorld()
	r.s.readEntries = world.read
	r.setRunner(newRgRunner(t, rgStep{act: func(runSpec) {
		mkFile(t, filepath.Join(game, "ggm.exe"), 600<<10)
		mkFile(t, uninstall, 100<<10)
		mkFile(t, filepath.Join(game, "3dmgame.dll"), 4096)
		mkFile(t, filepath.Join(game, "data", "ggm.xml"), 2048)
		mkFile(t, filepath.Join(game, "Redist", "dxwebsetup.exe"), 290<<10)
		mkFile(t, filepath.Join(game, "Redist", "vcredist_x86.exe"), 2<<20)
		world.add(entry)
	}}))

	item, err := r.s.Start("d1", StartOptions{})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	got := r.settle(item.ID)
	if got.Status != StatusWaitingForUser {
		t.Fatalf("status = %s (%q), want %s", got.Status, got.Error, StatusWaitingForUser)
	}
	if got.Destination != game {
		t.Fatalf("destination = %q, want %q", got.Destination, game)
	}
	want := filepath.Join(game, "ggm.exe")
	if len(got.Candidates) != 1 || got.Candidates[0].Path != want {
		t.Fatalf("candidates = %+v, want only %s", got.Candidates, want)
	}
	if got.Uninstall.Key != entry.Key || got.Owned {
		t.Fatalf("uninstall = %+v owned = %v, want the entry %s and a directory the install does not own", got.Uninstall, got.Owned, entry.Key)
	}
}

func waitingFor(id, downloadID string) Installation {
	return Installation{ID: id, DownloadID: downloadID, Name: "Game", Type: TypeExeInstaller, Status: StatusWaitingForUser}
}

func itemStatus(t *testing.T, r *rig, id string) Status {
	t.Helper()
	return r.get(id).Status
}

func TestRemoveGameCancelsTheWaitingInstallOfItsDownload(t *testing.T) {
	cases := []struct {
		name string
		opts RemoveOptions
	}{
		{name: "record only"},
		{name: "with the download data", opts: RemoveOptions{DeleteDownload: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			dir := gameDir(t, "Game")
			r.download("d1", "Game", t.TempDir())
			r.reg.put(library.Game{ID: "g1", Title: "Game", InstallDir: dir, SourceDownloadID: "d1", Source: library.SourceDiscovered})
			mine := waitingFor("mine", "d1")
			mine.GameID = "g1"
			mine.Destination = dir
			r.add(mine)
			r.add(waitingFor("foreign", "d2"))
			r.add(waitingFor("orphan", ""))

			if err := r.s.RemoveGame("g1", tc.opts); err != nil {
				t.Fatalf("RemoveGame: %v", err)
			}
			if got := itemStatus(t, r, "mine"); got != StatusCancelled {
				t.Fatalf("install of the removed game's download is %s, want %s", got, StatusCancelled)
			}
			if got := r.diskItem("mine").Status; got != StatusCancelled {
				t.Fatalf("saved install of the removed game's download is %s, want %s", got, StatusCancelled)
			}
			for _, id := range []string{"foreign", "orphan"} {
				if got := itemStatus(t, r, id); got != StatusWaitingForUser {
					t.Fatalf("install %s is %s, want it untouched", id, got)
				}
			}
			if err := r.s.Dismiss("mine"); err != nil {
				t.Fatalf("Dismiss of the cancelled install: %v", err)
			}
		})
	}
}

func TestCancelReleasesARecordThatNoWorkerOrDownloadBacks(t *testing.T) {
	r := newRig(t)
	r.add(Installation{
		ID:            "f16d9b142e1d3f52",
		DownloadID:    "d367d65370c78e18",
		Name:          "GamersGoMakers",
		Type:          TypeExeInstaller,
		Status:        StatusWaitingForUser,
		SourcePath:    filepath.Join(t.TempDir(), "GamersGoMakers 1.1.7.exe"),
		InstallerPath: filepath.Join(t.TempDir(), "GamersGoMakers 1.1.7.exe"),
		Uninstall:     library.Uninstall{Key: `HKLM32\GamersGoMakers 1.1.7`, Command: `G:\games\GamersGoMakers\Uninstall.exe`},
	})
	r.downloads.getErr["d367d65370c78e18"] = errors.New("download manager is busy")
	r.restart()
	before := r.stateFiles()

	if err := r.s.Dismiss("f16d9b142e1d3f52"); !errors.Is(err, errUnavailable) {
		t.Fatalf("Dismiss of a waiting record = %v, want %v", err, errUnavailable)
	}
	if err := r.s.Cancel("f16d9b142e1d3f52"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if got := itemStatus(t, r, "f16d9b142e1d3f52"); got != StatusCancelled {
		t.Fatalf("status = %s, want %s", got, StatusCancelled)
	}
	for _, name := range r.stateFiles() {
		if !slices.Contains(before, name) {
			t.Fatalf("Cancel wrote %s into the state directory", name)
		}
	}
	if err := r.s.Dismiss("f16d9b142e1d3f52"); err != nil {
		t.Fatalf("Dismiss after Cancel: %v", err)
	}
	if left := r.disk(); len(left) != 0 {
		t.Fatalf("installations.json still holds %+v", left)
	}
	if _, err := os.Stat(filepath.Join(r.dir, "worker-f16d9b142e1d3f52-cancel")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancel file for a worker that does not exist: %v", err)
	}
}

func TestHandleDownloadGoneCancelsOnlyTheWaitingInstallsOfThatDownload(t *testing.T) {
	cases := []struct {
		name     string
		gone     string
		wantGone Status
	}{
		{name: "download of the waiting install", gone: "d1", wantGone: StatusCancelled},
		{name: "unrelated download", gone: "d9", wantGone: StatusWaitingForUser},
		{name: "empty download id", gone: "", wantGone: StatusWaitingForUser},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			r.add(waitingFor("waiting", "d1"))
			r.add(waitingFor("detached", ""))
			r.add(Installation{ID: "done", DownloadID: "d1", Name: "Game", Status: StatusCompleted})

			if err := r.s.HandleDownloadGone(tc.gone); err != nil {
				t.Fatalf("HandleDownloadGone: %v", err)
			}
			if got := itemStatus(t, r, "waiting"); got != tc.wantGone {
				t.Fatalf("waiting install is %s, want %s", got, tc.wantGone)
			}
			if got := r.diskItem("waiting").Status; got != tc.wantGone {
				t.Fatalf("saved waiting install is %s, want %s", got, tc.wantGone)
			}
			if got := itemStatus(t, r, "detached"); got != StatusWaitingForUser {
				t.Fatalf("install without a download is %s, want it untouched", got)
			}
			if got := itemStatus(t, r, "done"); got != StatusCompleted {
				t.Fatalf("completed install is %s, want it untouched", got)
			}
			if tc.wantGone == StatusCancelled {
				if err := r.s.Dismiss("waiting"); err != nil {
					t.Fatalf("Dismiss of the released install: %v", err)
				}
			}
		})
	}
}

func TestHandleDownloadGoneReportsAnInstallItCouldNotRelease(t *testing.T) {
	r := newRig(t)
	r.add(waitingFor("waiting", "d1"))
	unblock := r.blockStore()

	err := r.s.HandleDownloadGone("d1")
	wantPersistError(t, err)
	unblock()
	if got := r.diskItem("waiting").Status; got != StatusWaitingForUser {
		t.Fatalf("saved install is %s after a failed release, want %s", got, StatusWaitingForUser)
	}
}
