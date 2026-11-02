package install

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"typhon/internal/download"
	"typhon/internal/emulators"
	"typhon/internal/library"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const imageCover = "https://example.com/c.jpg"

func imageOrigin() download.Origin {
	return download.Origin{ReleaseID: "r1", SourceID: "s1", Platform: emulators.PS2, Title: "Console Game", Cover: imageCover}
}

func (r *rig) imageDownload(id string, tree imageTree, origin download.Origin) download.Download {
	r.t.Helper()
	root := r.t.TempDir()
	plantImages(r.t, root, tree)
	r.download(id, "Game", root)
	r.patchDownload(id, func(d *download.Download) { d.Origin = origin })
	return r.current(id)
}

func (r *rig) current(id string) download.Download {
	r.t.Helper()
	d, err := r.downloads.Get(id)
	if err != nil {
		r.t.Fatalf("download: %v", err)
	}
	return d
}

// triedImage is also true of a download a finished move took away with it.
func (r *rig) triedImage(id string) bool {
	r.t.Helper()
	d, ok := r.downloads.record(id)
	return !ok || d.ImageAttempted
}

func (r *rig) setAutoInstall(on bool) {
	r.t.Helper()
	cfg := r.cfg.GetSettings()
	cfg.AutoInstall = on
	if err := r.cfg.SaveSettings(cfg); err != nil {
		r.t.Fatalf("save settings: %v", err)
	}
}

func (r *rig) wantOneImageCard(item Installation) {
	r.t.Helper()
	done := r.settle(item.ID)
	if done.Status != StatusCompleted {
		r.t.Fatalf("install ended as %+v", viewOf(done))
	}
	want := filepath.Join(r.cfg.GetSettings().GamesPath, "Consoles", emulators.PS2, "Console Game")
	if !samePath(done.Destination, want) {
		r.t.Fatalf("destination = %q, want %q", done.Destination, want)
	}
	r.reg.mu.Lock()
	defer r.reg.mu.Unlock()
	if len(r.reg.images) != 1 {
		r.t.Fatalf("cards = %+v", r.reg.images)
	}
	card := r.reg.images[0]
	if card.Title != "Console Game" || card.Cover != imageCover || card.Platform != emulators.PS2 || !card.Owned {
		r.t.Fatalf("card = %+v", card)
	}
}

func TestFinishedImageInstallsWhateverTheSettingsSay(t *testing.T) {
	yes, no := true, false
	cases := []struct {
		name     string
		auto     bool
		override *bool
		tree     imageTree
	}{
		{name: "setting off", tree: imageTree{"Game/game.iso": "big"}},
		{name: "setting on", auto: true, tree: imageTree{"Game/game.iso": "big"}},
		{name: "dialog said no", auto: true, override: &no, tree: imageTree{"Game/game.iso": "big"}},
		{name: "dialog said yes", override: &yes, tree: imageTree{"Game/game.iso": "big"}},
		{name: "an iso beside an installer", tree: imageTree{"Game/game.iso": "big", "Game/setup.exe": "small"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			r.setAutoInstall(tc.auto)
			origin := imageOrigin()
			origin.AutoInstall = tc.override

			r.s.HandleDownloadCompleted(r.imageDownload("d1", tc.tree, origin))

			items := r.s.List()
			if len(items) != 1 {
				t.Fatalf("records = %+v, want the image install to start by itself", items)
			}
			r.wantOneImageCard(items[0])
			if !r.triedImage("d1") {
				t.Fatal("the download does not say its image was tried")
			}
		})
	}
}

func TestRefusedImageStaysVisible(t *testing.T) {
	long := imageOrigin()
	long.Title = strings.Repeat("a", maxImageTitle+1)
	unknown := imageOrigin()
	unknown.Platform = "ps9"
	badCover := imageOrigin()
	badCover.Cover = "file:///C:/x.png"
	cases := []struct {
		name   string
		origin download.Origin
		tree   imageTree
		free   uint64
		want   error
		title  string
	}{
		{name: "unknown console", origin: unknown, tree: imageTree{"Game/game.iso": "big"}, want: emulators.ErrUnknownPlatform, title: "Game"},
		{name: "local cover", origin: badCover, tree: imageTree{"Game/game.iso": "big"}, want: errImageOptions, title: "Game"},
		{name: "too long a title", origin: long, tree: imageTree{"Game/game.iso": "big"}, want: errImageOptions, title: "Game"},
		{name: "no image", origin: imageOrigin(), tree: imageTree{"Game/game.exe": "big"}, want: errImageNotFound, title: "Console Game"},
		{name: "two games", origin: imageOrigin(), tree: imageTree{"Game/a.iso": "big", "Game/b.iso": "big"}, want: errImageAmbiguous, title: "Console Game"},
		{name: "no room", origin: imageOrigin(), tree: imageTree{"Game/game.iso": "big"}, free: 16, want: errNotEnoughSpace, title: "Console Game"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			if tc.free != 0 {
				r.freeSpace(tc.free, nil)
			}
			d := r.imageDownload("d1", tc.tree, tc.origin)

			r.s.HandleDownloadCompleted(d)
			r.s.HandleDownloadCompleted(r.current("d1"))

			items := r.s.List()
			if len(items) != 1 {
				t.Fatalf("records = %+v, want one that shows the refusal", items)
			}
			got := items[0]
			if got.Status != StatusFailed || got.Type != TypeImage || got.DownloadID != "d1" || got.Name != tc.title {
				t.Fatalf("record = %+v", got)
			}
			if !strings.Contains(got.Error, strings.TrimPrefix(tc.want.Error(), "[")) {
				t.Fatalf("error = %q, want %q", got.Error, tc.want)
			}
			if onDisk := r.diskItem(got.ID); onDisk.Status != StatusFailed {
				t.Fatalf("on disk = %+v", onDisk)
			}
			if !r.current("d1").ImageAttempted {
				t.Fatal("the refusal would repeat on every start")
			}
		})
	}
}

func TestRefusedImageIsRetriedAfterTheCauseIsGone(t *testing.T) {
	r := newRig(t)
	r.freeSpace(16, nil)
	r.s.HandleDownloadCompleted(r.imageDownload("d1", imageTree{"Game/game.iso": "big"}, imageOrigin()))
	items := r.s.List()
	if len(items) != 1 || items[0].Status != StatusFailed {
		t.Fatalf("records = %+v", items)
	}

	r.freeSpace(1<<40, nil)
	if err := r.s.Retry(items[0].ID); err != nil {
		t.Fatalf("retry: %v", err)
	}

	r.wantOneImageCard(items[0])
}

func TestRetryOfARefusedImageChecksWhatStartChecks(t *testing.T) {
	t.Run("no room yet", func(t *testing.T) {
		r := newRig(t)
		r.freeSpace(16, nil)
		r.s.HandleDownloadCompleted(r.imageDownload("d1", imageTree{"Game/game.iso": "big"}, imageOrigin()))
		item := r.s.List()[0]
		if err := r.s.Retry(item.ID); !errors.Is(err, errNotEnoughSpace) {
			t.Fatalf("retry = %v", err)
		}
		if got := r.get(item.ID); got.Status != StatusFailed || got.Destination != "" {
			t.Fatalf("record after a refused retry = %+v", viewOf(got))
		}
	})
	t.Run("card appeared in between", func(t *testing.T) {
		r := newRig(t)
		r.freeSpace(16, nil)
		r.s.HandleDownloadCompleted(r.imageDownload("d1", imageTree{"Game/game.iso": "big"}, imageOrigin()))
		item := r.s.List()[0]
		r.reg.put(library.Game{ID: "g1", Title: "Console Game", ReleaseID: "r1", SourceID: "s1", Emulation: &library.Emulation{Platform: emulators.PS2}})
		if err := r.s.Retry(item.ID); !errors.Is(err, library.ErrAlreadyAdded) {
			t.Fatalf("retry = %v", err)
		}
	})
}

func TestSecondDownloadOfAnInstalledImageIsAVisibleRefusal(t *testing.T) {
	r := newRig(t)
	r.reg.put(library.Game{ID: "g1", Title: "Console Game", ReleaseID: "r1", SourceID: "s1", Emulation: &library.Emulation{Platform: emulators.PS2}})

	r.s.HandleDownloadCompleted(r.imageDownload("d1", imageTree{"Game/game.iso": "big"}, imageOrigin()))

	items := r.s.List()
	if len(items) != 1 {
		t.Fatalf("records = %+v", items)
	}
	item := items[0]
	if item.Status != StatusFailed || !strings.Contains(item.Error, "library.already_added") {
		t.Fatalf("record = %+v", item)
	}
	want, err := sourceDir(r.current("d1"))
	if err != nil {
		t.Fatal(err)
	}
	if item.SourcePath == "" || item.SourcePath != want {
		t.Fatalf("source path = %q, want %q", item.SourcePath, want)
	}
	if !r.current("d1").ImageAttempted {
		t.Fatal("the download does not say its image was tried")
	}
}

func TestStartTakesTheConsoleFromTheDownload(t *testing.T) {
	t.Run("nothing named", func(t *testing.T) {
		r := newImageRig(t)
		root := t.TempDir()
		plantImages(t, root, imageTree{"Game/game.iso": "big", "Game/setup.exe": "big"})
		r.downloads.add("d1", "Game", root)
		r.setOrigin("d1", imageOrigin())
		item, err := r.s.Start("d1", StartOptions{Mode: ModeCopy})
		if err != nil {
			t.Fatal(err)
		}
		done := r.s.waitStatus(t, item.ID, StatusCompleted)
		if done.Type != TypeImage || done.Platform != emulators.PS2 || done.Name != "Console Game" || done.Cover != imageCover {
			t.Fatalf("job = %+v", done)
		}
	})
	t.Run("a PC install cannot be forced", func(t *testing.T) {
		for _, opts := range []StartOptions{{Type: TypePortable}, {InstallerPath: `C:\setup.exe`}} {
			r := newImageRig(t)
			root := t.TempDir()
			plantImages(t, root, imageTree{"Game/game.iso": "big"})
			r.downloads.add("d1", "Game", root)
			r.setOrigin("d1", imageOrigin())
			if _, err := r.s.Start("d1", opts); !errors.Is(err, errImageOptions) {
				t.Fatalf("Start(%+v) = %v", opts, err)
			}
		}
	})
}

func TestImageThatIsTakenCareOfIsNotAFailure(t *testing.T) {
	t.Run("install of the download is running", func(t *testing.T) {
		r := newRig(t)
		d := r.imageDownload("d1", imageTree{"Game/game.iso": "big"}, imageOrigin())
		r.add(Installation{ID: "busy", DownloadID: "d1", Name: "Console Game", Type: TypeImage, Status: StatusWaitingForUser})

		r.s.HandleDownloadCompleted(d)

		if items := r.s.List(); len(items) != 1 || items[0].ID != "busy" {
			t.Fatalf("records = %+v", items)
		}
	})
	t.Run("finished earlier and the card is gone", func(t *testing.T) {
		r := newRig(t)
		d := r.imageDownload("d1", imageTree{"Game/game.iso": "big"}, imageOrigin())
		r.s.HandleDownloadCompleted(d)
		r.settle(r.s.List()[0].ID)
		r.downloads.add("d1", "Game", t.TempDir())
		r.patchDownload("d1", func(d *download.Download) { d.Origin, d.ImageAttempted = imageOrigin(), true })

		r.s.HandleDownloadCompleted(r.current("d1"))

		if items := r.s.List(); len(items) != 1 {
			t.Fatalf("records = %+v", items)
		}
	})
}

func TestImageIsInstalledOnTheNextRun(t *testing.T) {
	t.Run("launcher died between completion and install", func(t *testing.T) {
		r := newRig(t)
		r.imageDownload("d1", imageTree{"Game/game.iso": "big"}, imageOrigin())

		r.restart()

		var items []Installation
		waitFor(t, "the image install to start on boot", func() bool {
			items = r.s.List()
			return len(items) == 1
		})
		r.wantOneImageCard(items[0])
		if !r.triedImage("d1") {
			t.Fatal("the next start would look at this download again")
		}
	})
	t.Run("completion arrived before the service started", func(t *testing.T) {
		s := mustServiceAt(t, t.TempDir())
		s.settings = newTestSettings(t)
		downloads := newFakeDownloads()
		registrar := &fakeRegistrar{}
		s.downloads, s.library = downloads, registrar
		root := t.TempDir()
		plantImages(t, root, imageTree{"Game/game.iso": "big"})
		downloads.add("d1", "Game", root)
		d := downloads.items["d1"]
		d.Origin = imageOrigin()
		downloads.items["d1"] = d

		s.HandleDownloadCompleted(d)

		if items := s.List(); len(items) != 0 {
			t.Fatalf("records before startup = %+v", items)
		}
		if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := s.ServiceShutdown(); err != nil {
				t.Errorf("shutdown: %v", err)
			}
		})
		waitFor(t, "the image install to start on boot", func() bool { return len(s.List()) == 1 })
		s.waitStatus(t, s.List()[0].ID, StatusCompleted)
	})
}

func TestNextRunDoesNotRepeatWhatTheUserAlreadySaw(t *testing.T) {
	tree := imageTree{"Game/game.iso": "big"}
	cases := []struct {
		name  string
		setup func(r *rig)
	}{
		{"tried before, record dismissed", func(r *rig) {
			r.s.HandleDownloadCompleted(r.imageDownload("d1", tree, imageOrigin()))
			item := r.s.List()[0]
			r.settle(item.ID)
			if err := r.s.Dismiss(item.ID); err != nil {
				r.t.Fatal(err)
			}
		}},
		{"refused before, record dismissed", func(r *rig) {
			r.freeSpace(16, nil)
			r.s.HandleDownloadCompleted(r.imageDownload("d1", tree, imageOrigin()))
			if err := r.s.Dismiss(r.s.List()[0].ID); err != nil {
				r.t.Fatal(err)
			}
		}},
		{"a PC download", func(r *rig) {
			r.imageDownload("d1", tree, download.Origin{ReleaseID: "r1"})
		}},
		{"an update", func(r *rig) {
			origin := imageOrigin()
			origin.Purpose = download.PurposeUpdate
			r.imageDownload("d1", tree, origin)
		}},
		{"still downloading", func(r *rig) {
			r.imageDownload("d1", tree, imageOrigin())
			r.patchDownload("d1", func(d *download.Download) { d.Status = download.StatusDownloading })
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			tc.setup(r)

			r.restart()
			r.s.resumeImageInstalls(t.Context())

			if items := r.s.List(); len(items) != 0 {
				t.Fatalf("records after a restart = %+v", items)
			}
		})
	}
}

func TestMarkThatFailedDoesNotStartASecondInstall(t *testing.T) {
	r := newRig(t)
	r.downloads.markErr = errors.New("disk full")
	r.s.HandleDownloadCompleted(r.imageDownload("d1", imageTree{"Game/game.iso": "big"}, imageOrigin()))
	first := r.s.List()[0]
	r.settle(first.ID)

	r.restart()
	r.s.resumeImageInstalls(t.Context())

	if items := r.s.List(); len(items) != 1 || items[0].ID != first.ID {
		t.Fatalf("records = %+v", items)
	}
}

func TestRefusalThatCouldNotBeSavedIsNotMarkedAsTried(t *testing.T) {
	r := newRig(t)
	r.freeSpace(16, nil)
	d := r.imageDownload("d1", imageTree{"Game/game.iso": "big"}, imageOrigin())
	unblock := r.blockStore()

	r.s.HandleDownloadCompleted(d)

	if items := r.s.List(); len(items) != 0 {
		t.Fatalf("a record the disk does not have = %+v", items)
	}
	if r.current("d1").ImageAttempted {
		t.Fatal("the attempt is marked, and nothing on disk shows the refusal")
	}
	unblock()
	r.s.HandleDownloadCompleted(r.current("d1"))
	if items := r.s.List(); len(items) != 1 || items[0].Status != StatusFailed {
		t.Fatalf("records = %+v", items)
	}
}

func TestNewServiceMakesTheManagerCheckOrigins(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	valid := imageOrigin()
	overlong := imageOrigin()
	overlong.Title = strings.Repeat("a", maxImageTitle+1)
	control := imageOrigin()
	control.Title = "a\x07b"
	script := imageOrigin()
	script.Cover = "javascript:alert(1)"
	unknown := imageOrigin()
	unknown.Platform = "PS2"
	repair := imageOrigin()
	repair.Purpose = download.PurposeRepair
	cases := []struct {
		name   string
		origin download.Origin
		want   error
	}{
		{"unknown console", unknown, emulators.ErrUnknownPlatform},
		{"script as a cover", script, errImageOptions},
		{"control character", control, errImageOptions},
		{"too long a title", overlong, errImageOptions},
		{"title without a console", download.Origin{Title: "Game"}, errImageOptions},
		{"cover without a console", download.Origin{Cover: imageCover}, errImageOptions},
		{"image as a repair", repair, errImageOptions},
		{"a valid image is let through", valid, nil},
		{"a PC download is not looked at", download.Origin{GameID: "g"}, nil},
	}
	refused := func(err error) bool {
		return errors.Is(err, errImageOptions) || errors.Is(err, emulators.ErrUnknownPlatform)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			manager := new(download.Manager)
			if _, err := NewService(nil, manager, nil); err != nil {
				t.Fatal(err)
			}
			_, started := manager.StartDownloadFrom("hash", t.TempDir(), nil, tc.origin)
			_, added := manager.AddTask(t.Context(), download.AddRequest{Origin: tc.origin})
			for name, err := range map[string]error{"start": started, "add task": added} {
				if tc.want == nil && refused(err) {
					t.Fatalf("%s refused a valid origin: %v", name, err)
				}
				if tc.want != nil && !errors.Is(err, tc.want) {
					t.Fatalf("%s = %v, want %v", name, err, tc.want)
				}
			}
		})
	}
}
