package install

import (
	"path/filepath"
	"testing"

	"typhon/internal/download"
	"typhon/internal/settings"
)

func TestHandleDownloadCompletedStartsOnlyWhatIsSafeToStartAlone(t *testing.T) {
	yes, no := true, false
	zipSource := func(t *testing.T, r *rig) {
		archiveSource(t, r, "game.zip", func(path string) { writeZip(t, path, gameZipEntries()) })
	}
	cases := []struct {
		name     string
		setup    func(t *testing.T, r *rig)
		auto     bool
		override *bool
		purpose  download.Purpose
		free     uint64
		started  bool
	}{
		{name: "setting on, archive", setup: zipSource, auto: true, started: true},
		{name: "setting off, archive", setup: zipSource},
		{name: "dialog choice beats the setting off", setup: zipSource, override: &yes, started: true},
		{name: "dialog choice beats the setting on", setup: zipSource, auto: true, override: &no},
		{name: "update download is the updater's business", setup: zipSource, auto: true, purpose: download.PurposeUpdate},
		{name: "repair download is the repairer's business", setup: zipSource, auto: true, purpose: download.PurposeRepair},
		{name: "silent installer", setup: func(t *testing.T, r *rig) { silentSource(t, r, rgInnoMarker) }, auto: true, started: true},
		{name: "installer with a wizard needs the player", setup: func(t *testing.T, r *rig) { interactiveSource(t, r) }, auto: true},
		{name: "nothing recognisable", setup: func(t *testing.T, r *rig) {
			root := t.TempDir()
			mkText(t, filepath.Join(root, "Game", "readme.txt"), "nothing useful")
			r.download("d1", "Game", root)
		}, auto: true},
		{name: "no room for the extracted game", setup: zipSource, auto: true, free: 1024},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			tc.setup(t, r)
			cfg := r.cfg.GetSettings()
			cfg.AutoInstall = tc.auto
			if err := r.cfg.SaveSettings(cfg); err != nil {
				t.Fatalf("save settings: %v", err)
			}
			r.setRunner(newRgRunner(t, rgStep{}))
			if tc.free != 0 {
				r.freeSpace(tc.free, nil)
			}
			d, err := r.downloads.Get("d1")
			if err != nil {
				t.Fatalf("download: %v", err)
			}
			d.Origin.AutoInstall, d.Origin.Purpose = tc.override, tc.purpose

			r.s.HandleDownloadCompleted(d)

			items := r.s.List()
			if tc.started != (len(items) == 1) {
				t.Fatalf("records after auto install = %+v, want started=%v", items, tc.started)
			}
			if !tc.started {
				return
			}
			if want := filepath.Join(cfg.GamesPath, "Game"); items[0].Destination != want {
				t.Fatalf("destination = %q, want %q: auto install picks the games folder itself", items[0].Destination, want)
			}
			r.settle(items[0].ID)
		})
	}
}

func TestHandleDownloadCompletedInstallsAnArchiveIntoTheGamesFolder(t *testing.T) {
	r := newRig(t)
	archiveSource(t, r, "game.zip", func(path string) { writeZip(t, path, gameZipEntries()) })
	cfg := r.cfg.GetSettings()
	cfg.AutoInstall = true
	cfg.InstallCleanupPolicy = settings.CleanupKeep
	if err := r.cfg.SaveSettings(cfg); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	d, err := r.downloads.Get("d1")
	if err != nil {
		t.Fatalf("download: %v", err)
	}

	r.s.HandleDownloadCompleted(d)

	items := r.s.List()
	if len(items) != 1 {
		t.Fatalf("records = %+v, want the install to start by itself", items)
	}
	got := r.settle(items[0].ID)
	want := filepath.Join(cfg.GamesPath, "Game")
	if got.Status != StatusCompleted || got.Destination != want || got.Executable != filepath.Join(want, "Game.exe") {
		t.Fatalf("auto install ended as %+v, want a completed install in %s", viewOf(got), want)
	}
	r.assertDurable(items[0].ID)
}
