package install

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"typhon/internal/download"
	"typhon/internal/platform"
)

func (r *rig) patchDownload(id string, edit func(*download.Download)) {
	r.t.Helper()
	r.downloads.mu.Lock()
	defer r.downloads.mu.Unlock()
	d, ok := r.downloads.items[id]
	if !ok {
		r.t.Fatalf("download %s is not registered", id)
	}
	edit(&d)
	r.downloads.items[id] = d
}

func (r *rig) freeSpace(free uint64, err error) {
	r.s.freeSpace = func(string) (platform.StorageInfo, error) {
		return platform.StorageInfo{FreeBytes: free}, err
	}
}

// Каждый отказ Start обязан прийти в интерфейс со своим кодом и не оставить за
// собой ни записи, ни файлов: пользователь видит причину, а не «не удалось».
func TestStartRefusalsCarryTheirCodes(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, r *rig) (dest string, opts StartOptions)
		code  string
	}{
		{"download is still running", func(t *testing.T, r *rig) (string, StartOptions) {
			dest := archiveSource(t, r, "game.zip", func(path string) { writeZip(t, path, gameZipEntries()) })
			r.patchDownload("d1", func(d *download.Download) { d.Status = download.StatusDownloading })
			return dest, StartOptions{Destination: dest}
		}, "install.not_completed"},
		{"destination is blank", func(t *testing.T, r *rig) (string, StartOptions) {
			archiveSource(t, r, "game.zip", func(path string) { writeZip(t, path, gameZipEntries()) })
			return "", StartOptions{Destination: "   "}
		}, "install.no_destination"},
		{"destination is relative", func(t *testing.T, r *rig) (string, StartOptions) {
			archiveSource(t, r, "game.zip", func(path string) { writeZip(t, path, gameZipEntries()) })
			return "", StartOptions{Destination: filepath.Join("relative", "Game")}
		}, "install.relative_destination"},
		{"destination holds files", func(t *testing.T, r *rig) (string, StartOptions) {
			dest := archiveSource(t, r, "game.zip", func(path string) { writeZip(t, path, gameZipEntries()) })
			mkFile(t, filepath.Join(dest, "keep.txt"), 8)
			return dest, StartOptions{Destination: dest}
		}, "install.dest_not_empty"},
		{"nothing recognisable in the download", func(t *testing.T, r *rig) (string, StartOptions) {
			root := t.TempDir()
			mkText(t, filepath.Join(root, "Game", "readme.txt"), "nothing useful")
			r.download("d1", "Game", root)
			return "", StartOptions{Destination: filepath.Join(t.TempDir(), "Game")}
		}, "install.unknown_type"},
		{"hand-picked installer does not exist", func(t *testing.T, r *rig) (string, StartOptions) {
			dest := silentSource(t, r, rgInnoMarker)
			return dest, StartOptions{Destination: dest, InstallerPath: filepath.Join(t.TempDir(), "missing.exe")}
		}, "install.no_executable"},
		{"hand-picked installer is a directory", func(t *testing.T, r *rig) (string, StartOptions) {
			dest := silentSource(t, r, rgInnoMarker)
			return dest, StartOptions{Destination: dest, InstallerPath: t.TempDir()}
		}, "install.no_executable"},
		{"installer type forced on a download without one", func(t *testing.T, r *rig) (string, StartOptions) {
			root := t.TempDir()
			portableSource(t, root, "Game")
			r.download("d1", "Game", root)
			return "", StartOptions{Destination: filepath.Join(t.TempDir(), "Game"), Type: TypeExeInstaller}
		}, "install.no_executable"},
		{"zip that is not a zip", func(t *testing.T, r *rig) (string, StartOptions) {
			dest := archiveSource(t, r, "game.zip", func(path string) { mkText(t, path, strings.Repeat("not a zip ", 64)) })
			return dest, StartOptions{Destination: dest}
		}, "install.unsupported_archive"},
		{"7z that is not a 7z", func(t *testing.T, r *rig) (string, StartOptions) {
			dest := archiveSource(t, r, "game.7z", func(path string) { mkText(t, path, strings.Repeat("not a 7z ", 64)) })
			return dest, StartOptions{Destination: dest}
		}, "install.unsupported_archive"},
		{"rar that is not a rar", func(t *testing.T, r *rig) (string, StartOptions) {
			neverConsultTools(t)
			dest := archiveSource(t, r, "game.rar", func(path string) { mkText(t, path, strings.Repeat("not a rar ", 64)) })
			return dest, StartOptions{Destination: dest}
		}, "install.unsupported_archive"},
		{"rar with encrypted headers", func(t *testing.T, r *rig) (string, StartOptions) {
			neverConsultTools(t)
			dest := archiveSource(t, r, "game.rar", func(path string) {
				writeRar(t, path, buildStoredRar([]rarEntry{{name: "Game/a.bin", data: []byte("data")}}, rarOptions{encryptedHeaders: true}))
			})
			return dest, StartOptions{Destination: dest}
		}, "install.archive_encrypted"},
		{"rar whose next volume never arrived", func(t *testing.T, r *rig) (string, StartOptions) {
			neverConsultTools(t)
			dest := archiveSource(t, r, "game.rar", func(path string) {
				writeRar(t, path, buildStoredRar([]rarEntry{{name: "Game/a.bin", data: []byte("data")}}, rarOptions{multiVolume: true}))
			})
			return dest, StartOptions{Destination: dest}
		}, "install.archive_incomplete"},
		{"not enough room for the extracted game", func(t *testing.T, r *rig) (string, StartOptions) {
			dest := archiveSource(t, r, "game.zip", func(path string) { writeZip(t, path, gameZipEntries()) })
			r.freeSpace(1024, nil)
			return dest, StartOptions{Destination: dest}
		}, "install.not_enough_space"},
		{"free space cannot be measured", func(t *testing.T, r *rig) (string, StartOptions) {
			dest := archiveSource(t, r, "game.zip", func(path string) { writeZip(t, path, gameZipEntries()) })
			r.freeSpace(0, errors.New("volume gone"))
			return dest, StartOptions{Destination: dest}
		}, "install.free_space_unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			dest, opts := tc.setup(t, r)
			_, err := r.s.Start("d1", opts)
			if got := errCode(err); got != tc.code {
				t.Fatalf("Start error = %v (code %q), want %s", err, got, tc.code)
			}
			if got := r.s.List(); len(got) != 0 {
				t.Fatalf("a refused Start left %+v", got)
			}
			if got := r.disk(); len(got) != 0 {
				t.Fatalf("a refused Start wrote %+v", got)
			}
			if dest != "" && tc.code != "install.dest_not_empty" && (exists(dest) || exists(dest+partialSuffix)) {
				t.Fatalf("a refused Start created %s", dest)
			}
		})
	}
}

func TestStartRefusedWithoutDownloadManager(t *testing.T) {
	s := mustServiceAt(t, t.TempDir())
	if _, err := s.Start("d1", StartOptions{}); errCode(err) != "install.no_downloads" {
		t.Fatalf("Start error = %v, want install.no_downloads", err)
	}
	if _, err := s.InspectDownload("d1"); errCode(err) != "install.no_downloads" {
		t.Fatalf("InspectDownload error = %v, want install.no_downloads", err)
	}
	if err := s.DeleteDownloadData("d1"); errCode(err) != "install.no_downloads" {
		t.Fatalf("DeleteDownloadData error = %v, want install.no_downloads", err)
	}
}

func TestStartRefusesWhatIsAlreadyRunning(t *testing.T) {
	r := newRig(t)
	root := t.TempDir()
	mkInstaller(t, filepath.Join(root, "Game", "setup.exe"), rgInnoMarker)
	r.download("d1", "Game", root)
	otherRoot := t.TempDir()
	mkInstaller(t, filepath.Join(otherRoot, "Other", "setup.exe"), rgInnoMarker)
	r.download("d2", "Other", otherRoot)
	dest := filepath.Join(r.games, "Game")
	run := newRgRunner(t, rgStep{wait: true})
	r.setRunner(run)

	first, err := r.s.Start("d1", StartOptions{Destination: dest})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	run.entered(0)
	if _, err := r.s.Start("d1", StartOptions{Destination: filepath.Join(r.games, "Elsewhere")}); errCode(err) != "install.busy" {
		t.Fatalf("second Start of the same download = %v, want install.busy", err)
	}
	if _, err := r.s.Start("d2", StartOptions{Destination: dest}); errCode(err) != "install.busy" {
		t.Fatalf("Start of another download into the same folder = %v, want install.busy", err)
	}
	if got := r.s.List(); len(got) != 1 || got[0].ID != first.ID {
		t.Fatalf("records = %+v, want only the first install", got)
	}
	if err := r.s.Cancel(first.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	r.settle(first.ID)
}

func TestStartWithAHandPickedInstaller(t *testing.T) {
	r := newRig(t)
	root := t.TempDir()
	dir := cupheadSource(t, root)
	r.download("d1", "Cuphead", root)
	dest := filepath.Join(r.games, "Cuphead")
	run := newRgRunner(t, rgStep{act: func(runSpec) { mkFile(t, filepath.Join(dest, "Cuphead.exe"), 8192) }})
	r.setRunner(run)
	addon := filepath.Join(dir, cupheadAddon+".exe")

	item, err := r.s.Start("d1", StartOptions{Destination: dest, InstallerPath: addon})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !item.ManualInstaller || item.InstallerPath != addon || len(item.ExtraInstallers) != 0 {
		t.Fatalf("record = manual %v installer %q extras %v, want the hand-picked file alone", item.ManualInstaller, item.InstallerPath, item.ExtraInstallers)
	}
	got := r.settle(item.ID)
	if got.Status != StatusCompleted || got.ChainStep != 0 {
		t.Fatalf("status %s chain step %d (%q)", got.Status, got.ChainStep, got.Error)
	}
	if calls := run.calls(); len(calls) != 1 || calls[0].InstallerPath != addon {
		t.Fatalf("installer runs = %+v, want only the hand-picked installer", calls)
	}
	r.assertDurable(item.ID)
}

func TestInspectDownloadProposesWhereToInstall(t *testing.T) {
	r := newRig(t)
	root := t.TempDir()
	portableSource(t, root, "Game")
	r.download("d1", "Game", root)
	games := r.cfg.GetSettings().GamesPath
	mkFile(t, filepath.Join(games, "Game", "taken.txt"), 4)
	r.freeSpace(1<<40, nil)

	info, err := r.s.InspectDownload("d1")
	if err != nil {
		t.Fatalf("InspectDownload: %v", err)
	}
	if info.Plan.Type != TypePortable || info.Name != "Game" || info.DownloadID != "d1" {
		t.Fatalf("info = %+v", info)
	}
	if want := filepath.Join(games, "Game (2)"); info.Plan.Destination != want {
		t.Fatalf("destination = %q, want %q: the taken folder must not be proposed", info.Plan.Destination, want)
	}
	if info.RequiredBytes <= 0 || info.FreeBytes != 1<<40 {
		t.Fatalf("required %d free %d", info.RequiredBytes, info.FreeBytes)
	}
}

func TestInspectDownloadLeavesTheInstallerToChooseItsFolder(t *testing.T) {
	r := newRig(t)
	interactiveSource(t, r)
	r.freeSpace(1<<40, nil)
	info, err := r.s.InspectDownload("d1")
	if err != nil {
		t.Fatalf("InspectDownload: %v", err)
	}
	if info.Plan.Type != TypeExeInstaller || info.Plan.Silent || info.Plan.Destination != "" || !info.Plan.RequiresUserInteraction {
		t.Fatalf("plan = %+v: a wizard decides its own folder", info.Plan)
	}
}

func TestInspectDownloadRefusals(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, r *rig)
		code  string
	}{
		{"download is still running", func(t *testing.T, r *rig) {
			root := t.TempDir()
			portableSource(t, root, "Game")
			r.download("d1", "Game", root)
			r.patchDownload("d1", func(d *download.Download) { d.Status = download.StatusPaused })
		}, "install.not_completed"},
		{"free space cannot be measured", func(t *testing.T, r *rig) {
			root := t.TempDir()
			portableSource(t, root, "Game")
			r.download("d1", "Game", root)
			r.freeSpace(0, errors.New("volume gone"))
		}, "install.free_space_unknown"},
		{"download has no files", func(t *testing.T, r *rig) {
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, "Game"), 0o755); err != nil {
				t.Fatal(err)
			}
			r.download("d1", "Game", root)
		}, "install.empty_source"},
		{"download holds only part files", func(t *testing.T, r *rig) {
			root := t.TempDir()
			mkFile(t, filepath.Join(root, "Game", "game.zip.part"), 128)
			r.download("d1", "Game", root)
		}, "install.incomplete_source"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			tc.setup(t, r)
			if _, err := r.s.InspectDownload("d1"); errCode(err) != tc.code {
				t.Fatalf("InspectDownload error = %v, want %s", err, tc.code)
			}
		})
	}
}

// Записи, у которых операция недоступна, получают код «недоступно», а не
// молчаливое «ok»: интерфейс показывает кнопку по статусу, но состояние могло
// измениться между кликом и вызовом.
func TestOperationsRefusedOnTheWrongRecord(t *testing.T) {
	done := Installation{ID: "done", Name: "Done", Type: TypeArchiveZip, Status: StatusCompleted}
	waiting := Installation{ID: "wait", Name: "Wait", Type: TypeExeInstaller, Status: StatusWaitingForUser}
	cancelled := Installation{ID: "cancelled", Name: "Cancelled", Type: TypeExeInstaller, Status: StatusCancelled}
	zipFailed := Installation{ID: "zipfail", Name: "Zip", DownloadID: "d1", Type: TypeArchiveZip, Status: StatusFailed}
	cases := []struct {
		name string
		do   func(r *rig) error
		code string
	}{
		{"Cancel unknown", func(r *rig) error { return r.s.Cancel("nope") }, "install.not_found"},
		{"Cancel finished", func(r *rig) error { return r.s.Cancel("done") }, "install.unavailable"},
		{"Retry unknown", func(r *rig) error { return r.s.Retry("nope") }, "install.not_found"},
		{"Retry finished", func(r *rig) error { return r.s.Retry("done") }, "install.unavailable"},
		{"Retry waiting for the player", func(r *rig) error { return r.s.Retry("wait") }, "install.unavailable"},
		{"RetryInteractive unknown", func(r *rig) error { return r.s.RetryInteractive("nope") }, "install.not_found"},
		{"RetryInteractive on an archive", func(r *rig) error { return r.s.RetryInteractive("zipfail") }, "install.unavailable"},
		{"RetryInteractive on a cancelled install", func(r *rig) error { return r.s.RetryInteractive("cancelled") }, "install.unavailable"},
		{"Dismiss unknown", func(r *rig) error { return r.s.Dismiss("nope") }, "install.not_found"},
		{"Dismiss waiting for the player", func(r *rig) error { return r.s.Dismiss("wait") }, "install.unavailable"},
		{"Confirm unknown", func(r *rig) error { return r.s.ConfirmExecutable("nope", "x.exe") }, "install.not_found"},
		{"Confirm on a finished install", func(r *rig) error { return r.s.ConfirmExecutable("done", "x.exe") }, "install.unavailable"},
		{"Confirm a file that does not exist", func(r *rig) error {
			return r.s.ConfirmExecutable("wait", filepath.Join(r.games, "missing.exe"))
		}, "install.no_executable"},
		{"Confirm an empty path", func(r *rig) error { return r.s.ConfirmExecutable("wait", "  ") }, "install.no_executable"},
		{"Confirm a directory", func(r *rig) error { return r.s.ConfirmExecutable("wait", r.games) }, "install.no_executable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			for _, item := range []Installation{done, waiting, cancelled, zipFailed} {
				r.add(item)
			}
			before := r.s.List()
			err := tc.do(r)
			if got := errCode(err); got != tc.code {
				t.Fatalf("error = %v (code %q), want %s", err, got, tc.code)
			}
			after := r.s.List()
			if len(before) != len(after) {
				t.Fatalf("a refused operation changed the list: %d -> %d records", len(before), len(after))
			}
			for i := range before {
				if viewOf(before[i]) != viewOf(after[i]) {
					t.Fatalf("a refused operation changed %s: %+v -> %+v", before[i].ID, viewOf(before[i]), viewOf(after[i]))
				}
			}
		})
	}
}

func TestConfirmExecutableRefusesFilesOutsideTheInstallation(t *testing.T) {
	r := newRig(t)
	dest := archiveSource(t, r, "game.zip", func(path string) {
		writeZip(t, path, []zipEntry{{name: "Game/run.bat", data: []byte("@echo off")}, {name: "Game/data.bin", data: []byte("x")}})
	})
	item, err := r.s.Start("d1", StartOptions{Destination: dest})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := r.settle(item.ID); got.Status != StatusWaitingForUser {
		t.Fatalf("status = %s (%q)", got.Status, got.Error)
	}
	outside := filepath.Join(t.TempDir(), "other.exe")
	mkFile(t, outside, 4096)

	if err := r.s.ConfirmExecutable(item.ID, outside); errCode(err) != "install.outside_install" {
		t.Fatalf("Confirm of a file outside the installation = %v, want install.outside_install", err)
	}
	if got := r.get(item.ID); got.Status != StatusWaitingForUser || got.Executable == outside {
		t.Fatalf("a refused Confirm changed the record: %+v", viewOf(got))
	}
	if games := r.reg.registered(); len(games) != 0 {
		t.Fatalf("a refused Confirm registered %+v", games)
	}
	if err := r.s.ConfirmExecutable(item.ID, filepath.Join(dest, "run.bat")); err != nil {
		t.Fatalf("Confirm of a file inside: %v", err)
	}
}

func TestRetryRefusedWhileTheInstallRuns(t *testing.T) {
	r := newRig(t)
	dest := silentSource(t, r, rgInnoMarker)
	run := newRgRunner(t, rgStep{wait: true})
	r.setRunner(run)
	item, err := r.s.Start("d1", StartOptions{Destination: dest})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	run.entered(0)
	if err := r.s.Retry(item.ID); errCode(err) != "install.unavailable" {
		t.Fatalf("Retry of a running install = %v, want install.unavailable", err)
	}
	if err := r.s.Dismiss(item.ID); errCode(err) != "install.unavailable" {
		t.Fatalf("Dismiss of a running install = %v, want install.unavailable", err)
	}
	if n := len(run.calls()); n != 1 {
		t.Fatalf("installer runs = %d, want the refused Retry to start nothing", n)
	}
	if err := r.s.Cancel(item.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	r.settle(item.ID)
}

func TestRetryKeepsTheRecordWhenTheDownloadIsGone(t *testing.T) {
	r := newRig(t)
	dest := archiveSource(t, r, "game.zip", func(path string) { writeZip(t, path, gameZipEntries()) })
	r.add(Installation{
		ID: "gone", DownloadID: "d1", Name: "Game", Type: TypeArchiveZip, Status: StatusFailed, Error: "boom", Destination: dest,
	})
	r.downloads.mu.Lock()
	delete(r.downloads.items, "d1")
	r.downloads.mu.Unlock()
	before := r.get("gone")

	if err := r.s.Retry("gone"); err == nil {
		t.Fatal("Retry succeeded although the downloaded files are gone")
	}
	if after := r.get("gone"); viewOf(after) != viewOf(before) {
		t.Fatalf("a refused Retry changed the record: %+v -> %+v", viewOf(before), viewOf(after))
	}
}

func TestRetryInteractiveForgetsTheSilentFolderAndSticks(t *testing.T) {
	r := newRig(t)
	dest := silentSource(t, r, rgInnoMarker)
	r.setRunner(newRgRunner(t, rgStep{code: 1, log: wizardRefusedLog}))
	item, err := r.s.Start("d1", StartOptions{Destination: dest})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	failed := r.settle(item.ID)
	if failed.Status != StatusFailed || uiCode(failed.Error) != "install.installer_needs_interactive" {
		t.Fatalf("status %s error %q", failed.Status, failed.Error)
	}

	installed := filepath.Join(r.games, "Chosen")
	run := newRgRunner(t, rgStep{act: func(runSpec) { mkFile(t, filepath.Join(installed, "Game.exe"), 512<<10) }})
	r.setRunner(run)
	if err := r.s.RetryInteractive(item.ID); err != nil {
		t.Fatalf("RetryInteractive: %v", err)
	}
	waiting := r.settle(item.ID)
	if waiting.Status != StatusWaitingForUser || waiting.Silent || !waiting.Interactive || waiting.Destination != installed {
		t.Fatalf("after the interactive retry: %+v", viewOf(waiting))
	}
	if calls := run.calls(); len(calls) != 1 || len(calls[0].Args) != 0 || calls[0].Background || calls[0].Hidden {
		t.Fatalf("the retry must run the installer as is: %+v", calls)
	}

	r.restart()
	if again := r.get(item.ID); !again.Interactive || again.Silent {
		t.Fatalf("the choice of an interactive run did not survive a restart: %+v", viewOf(again))
	}
	if !strings.HasSuffix(filepath.ToSlash(r.get(item.ID).Destination), "/Chosen") {
		t.Fatalf("destination = %q", r.get(item.ID).Destination)
	}
}
