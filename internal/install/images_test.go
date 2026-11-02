package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"typhon/internal/download"
	"typhon/internal/emulators"
	"typhon/internal/library"
	"typhon/internal/settings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type imageTree map[string]string

// plantImages writes a tree: a value of "<n>" is a file of n bytes, anything
// else is the text of the file.
func plantImages(t *testing.T, root string, tree imageTree) {
	t.Helper()
	for name, body := range tree {
		path := filepath.Join(root, filepath.FromSlash(name))
		if size, ok := sizedBody(body); ok {
			mkFile(t, path, size)
			continue
		}
		mkText(t, path, body)
	}
}

func sizedBody(body string) (int, bool) {
	switch body {
	case "big":
		return 4096, true
	case "half":
		return 2048, true
	case "small":
		return 256, true
	}
	return 0, false
}

const twoDiscCue = "FILE \"Game (Disc 1).bin\" BINARY\n  TRACK 01 MODE2/2352\n"

func TestFindImages(t *testing.T) {
	cases := []struct {
		name     string
		platform string
		tree     imageTree
		want     string
		err      error
	}{
		{name: "single iso", platform: emulators.PS2, tree: imageTree{"game.iso": "big"}, want: "game.iso"},
		{name: "cue beats its bin", platform: emulators.PS1,
			tree: imageTree{"game.cue": `FILE "game.bin" BINARY`, "game.bin": "big"}, want: "game.cue"},
		{name: "m3u with discs", platform: emulators.PS1, tree: imageTree{
			"game.m3u":          "Game (Disc 1).cue\nGame (Disc 2).cue\n",
			"Game (Disc 1).cue": twoDiscCue,
			"Game (Disc 1).bin": "big",
			"Game (Disc 2).cue": `FILE "Game (Disc 2).bin" BINARY`,
			"Game (Disc 2).bin": "big",
		}, want: "game.m3u"},
		{name: "chd", platform: emulators.PS2, tree: imageTree{"game.chd": "big", "readme.txt": "hi"}, want: "game.chd"},
		{name: "pbp in a subfolder", platform: emulators.PSP,
			tree: imageTree{"PSP/GAME/TITLE/EBOOT.PBP": "big", "PSP/GAME/TITLE/ICON0.PNG": "small"}, want: "PSP/GAME/TITLE/EBOOT.PBP"},
		{name: "folder with readme and nfo", platform: emulators.PS2,
			tree: imageTree{"game.iso": "big", "readme.txt": "text", "game.nfo": "text"}, want: "game.iso"},
		{name: "discs without m3u take the first cue", platform: emulators.PS1, tree: imageTree{
			"Game (Disc 2).cue": `FILE "Game (Disc 2).bin" BINARY`,
			"Game (Disc 2).bin": "big",
			"Game (Disc 1).cue": twoDiscCue,
			"Game (Disc 1).bin": "big",
		}, want: "Game (Disc 1).cue"},
		{name: "iso discs take the first", platform: emulators.PS2,
			tree: imageTree{"Game CD2.iso": "big", "Game CD1.iso": "big"}, want: "Game CD1.iso"},
		{name: "discs in folders of their own", platform: emulators.PS1, tree: imageTree{
			"Disc 2/game.cue": `FILE "game.bin" BINARY`, "Disc 2/game.bin": "big",
			"Disc 1/game.cue": `FILE "game.bin" BINARY`, "Disc 1/game.bin": "big",
		}, want: "Disc 1/game.cue"},
		{name: "a small sibling loses to the largest", platform: emulators.PS2,
			tree: imageTree{"game.iso": "big", "bonus.iso": "small"}, want: "game.iso"},
		{name: "a bin alone is an image", platform: emulators.PS1, tree: imageTree{"game.bin": "big"}, want: "game.bin"},
		{name: "a chd beats a small iso", platform: emulators.PS2,
			tree: imageTree{"game.chd": "big", "extra.iso": "small"}, want: "game.chd"},
		{name: "uppercase extension", platform: emulators.PS2, tree: imageTree{"GAME.ISO": "big"}, want: "GAME.ISO"},
		{name: "two games", platform: emulators.PS2,
			tree: imageTree{"Alpha.iso": "big", "Beta.iso": "big"}, err: errImageAmbiguous},
		{name: "two games of close size", platform: emulators.PS2,
			tree: imageTree{"Alpha.iso": "big", "Beta.iso": "half"}, err: errImageAmbiguous},
		{name: "two games in different formats", platform: emulators.PS1,
			tree: imageTree{"Alpha.cue": `FILE "Alpha.bin" BINARY`, "Alpha.bin": "big", "Beta.chd": "big"}, err: errImageAmbiguous},
		{name: "a cue without its track is no image", platform: emulators.PS1,
			tree: imageTree{"game.cue": `FILE "lost.bin" BINARY`, "other.bin": "big"}, err: errImageNotFound},
		{name: "only documents", platform: emulators.PS2, tree: imageTree{"readme.txt": "a", "game.nfo": "b"}, err: errImageNotFound},
		{name: "nothing", platform: emulators.PS2, tree: imageTree{}, err: errImageNotFound},
		{name: "extension of another console", platform: emulators.PS2, tree: imageTree{"EBOOT.PBP": "big"}, err: errImageNotFound},
		{name: "deeper than the limit", platform: emulators.PS2, tree: imageTree{"a/b/c/d/e/f/g/game.iso": "big"}, err: errImageNotFound},
		{name: "unknown console", platform: "ps9", tree: imageTree{"game.iso": "big"}, err: emulators.ErrUnknownPlatform},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			plantImages(t, dir, c.tree)
			got, err := FindImages(t.Context(), dir, c.platform)
			if c.err != nil {
				if !errors.Is(err, c.err) {
					t.Fatalf("FindImages = %q, %v, want %v", got, err, c.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("FindImages: %v", err)
			}
			if want := filepath.Join(dir, filepath.FromSlash(c.want)); !samePath(got, want) {
				t.Fatalf("FindImages = %q, want %q", got, want)
			}
		})
	}
}

func TestFindImagesStopsOnACancelledContext(t *testing.T) {
	dir := t.TempDir()
	plantImages(t, dir, imageTree{"game.iso": "big"})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := FindImages(ctx, dir, emulators.PS2); !errors.Is(err, context.Canceled) {
		t.Fatalf("FindImages = %v", err)
	}
}

func TestFindImagesDoesNotTakeALink(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	plantImages(t, outside, imageTree{"game.iso": "big"})
	plantLink(t, filepath.Join(dir, "linked"), outside)
	if _, err := FindImages(t.Context(), dir, emulators.PS2); !errors.Is(err, errImageNotFound) {
		t.Fatalf("FindImages followed a link: %v", err)
	}
}

func TestCopyImageSourceStopsOnACancelledContext(t *testing.T) {
	src := filepath.Join(t.TempDir(), "game.iso")
	mkFile(t, src, 4096)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	partial := filepath.Join(t.TempDir(), "Game.partial")
	if err := copyImageSource(ctx, src, partial, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("copyImageSource = %v", err)
	}
}

func TestFindImagesTellsAnUnreadableListFromNoImage(t *testing.T) {
	cases := []struct {
		name string
		tree imageTree
	}{
		{"broken cue beside a stranger", imageTree{"game.cue": "FILE \"game.bin BINARY", "game.bin": "big", "stranger.iso": "big"}},
		{"cue naming nothing beside a stranger", imageTree{"game.cue": "REM nothing", "stranger.iso": "big"}},
		{"playlist with too long a line", imageTree{"game.m3u": strings.Repeat("x", 1<<17), "stranger.iso": "big"}},
		{"playlist with a broken sheet", imageTree{"game.m3u": "d1.cue", "d1.cue": "FILE \"d1.bin", "stranger.iso": "big"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			plantImages(t, dir, c.tree)
			got, err := FindImages(t.Context(), dir, emulators.PS1)
			if !errors.Is(err, errImageUnreadable) || errors.Is(err, errImageNotFound) {
				t.Fatalf("FindImages = %q, %v", got, err)
			}
		})
	}
}

func TestFindImagesReadsASheetWithAByteOrderMark(t *testing.T) {
	dir := t.TempDir()
	plantImages(t, dir, imageTree{"game.cue": "\ufeffFILE \"game.bin\" BINARY\n", "game.bin": "big"})
	got, err := FindImages(t.Context(), dir, emulators.PS1)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "game.cue"); !samePath(got, want) {
		t.Fatalf("FindImages = %q, want %q", got, want)
	}
}

func setImageSeam[T any](t *testing.T, seam *T, value T) {
	t.Helper()
	previous := *seam
	*seam = value
	t.Cleanup(func() { *seam = previous })
}

func TestImageCopyWithAnotherHashKeepsTheSource(t *testing.T) {
	calls := 0
	genuine := hashImageFile
	setImageSeam(t, &hashImageFile, func(ctx context.Context, path string, buf []byte) (string, int64, error) {
		calls++
		hash, size, err := genuine(ctx, path, buf)
		if calls == 2 {
			hash += "0"
		}
		return hash, size, err
	})
	r := newImageRig(t)
	r.s.sameVolume = differentVolumes
	root := t.TempDir()
	plantImages(t, root, imageTree{"Game.iso": "big"})
	r.downloads.add("d1", "Game.iso", root)
	item, err := r.s.Start("d1", StartOptions{Platform: emulators.PS2, Mode: ModeMove})
	if err != nil {
		t.Fatal(err)
	}
	failed := r.s.waitStatus(t, item.ID, StatusFailed)
	r.s.waitJobDone(t, item.ID)
	if !strings.Contains(failed.Error, "install.copy_verify_failed") {
		t.Fatalf("error = %q", failed.Error)
	}
	if _, err := os.Stat(filepath.Join(root, "Game.iso")); err != nil {
		t.Fatalf("the source went: %v", err)
	}
	if pathExists(failed.Destination + partialSuffix) {
		t.Fatal("the bad copy stayed")
	}
	if len(r.lib.GetPlayableGames()) != 0 {
		t.Fatal("a card was created")
	}
}

func TestImageFileThatRefusesToRenameIsCopied(t *testing.T) {
	refused := 0
	setImageSeam(t, &renameImageFile, func(string, string) error {
		refused++
		return os.ErrPermission
	})
	r := newImageRig(t)
	root := t.TempDir()
	plantImages(t, root, imageTree{"Game.iso": "big"})
	source := filepath.Join(root, "Game.iso")
	r.downloads.add("d1", "Game.iso", root)
	item, err := r.s.Start("d1", StartOptions{Platform: emulators.PS2, Mode: ModeMove})
	if err != nil {
		t.Fatal(err)
	}
	done := r.s.waitStatus(t, item.ID, StatusCompleted)
	r.s.waitJobDone(t, item.ID)
	if refused != 1 {
		t.Fatalf("renames = %d", refused)
	}
	info, err := os.Stat(filepath.Join(done.Destination, "Game.iso"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 4096 {
		t.Fatalf("size = %d", info.Size())
	}
	if _, err := os.Stat(source); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the source stayed after a move: %v", err)
	}
}

func TestInspectImage(t *testing.T) {
	t.Run("a single image stays alone", func(t *testing.T) {
		shared := t.TempDir()
		plantImages(t, shared, imageTree{"game.iso": "big", "neighbour.iso": "big"})
		path := filepath.Join(shared, "game.iso")
		plan, err := InspectImage(t.Context(), path, emulators.PS2)
		if err != nil {
			t.Fatal(err)
		}
		if plan.Type != TypeImage || !samePath(plan.ContentRoot, path) || !samePath(plan.ImagePath, path) || plan.EstimatedSize != 4096 {
			t.Fatalf("plan = %+v", plan)
		}
	})
	t.Run("an installer beside the image is not guessed", func(t *testing.T) {
		dir := t.TempDir()
		plantImages(t, dir, imageTree{"game.iso": "big", "setup.exe": "small", "game.bin": "small"})
		plan, err := InspectImage(t.Context(), dir, emulators.PS2)
		if err != nil {
			t.Fatal(err)
		}
		if plan.Type != TypeImage || plan.InstallerPath != "" || plan.Engine != "" {
			t.Fatalf("plan = %+v", plan)
		}
	})
	t.Run("a single unknown file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "game.txt")
		mkText(t, path, "x")
		if _, err := InspectImage(t.Context(), path, emulators.PS2); !errors.Is(err, errImageNotFound) {
			t.Fatalf("InspectImage = %v", err)
		}
	})
	t.Run("a cue alone has no tracks", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "game.cue")
		mkText(t, path, `FILE "game.bin" BINARY`)
		if _, err := InspectImage(t.Context(), path, emulators.PS1); !errors.Is(err, errImageNotFound) {
			t.Fatalf("InspectImage = %v", err)
		}
	})
	t.Run("an empty folder", func(t *testing.T) {
		if _, err := InspectImage(t.Context(), t.TempDir(), emulators.PS2); !errors.Is(err, errEmptySource) {
			t.Fatalf("InspectImage = %v", err)
		}
	})
	t.Run("a missing path", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "gone")
		if _, err := InspectImage(t.Context(), path, emulators.PS2); !errors.Is(err, errNoSource) {
			t.Fatalf("InspectImage = %v", err)
		}
	})
	t.Run("an unknown console", func(t *testing.T) {
		if _, err := InspectImage(t.Context(), t.TempDir(), "ps9"); !errors.Is(err, emulators.ErrUnknownPlatform) {
			t.Fatalf("InspectImage = %v", err)
		}
	})
}

type imageRig struct {
	s         *Service
	downloads *fakeDownloads
	lib       *library.Service
	games     string
	prepared  *int
}

func newImageRig(t *testing.T) imageRig {
	t.Helper()
	s := mustServiceAt(t, t.TempDir())
	s.settings = newTestSettings(t)
	cfg := s.settings.GetSettings()
	cfg.VerifyAfterInstall = true
	cfg.InstallCleanupPolicy = settings.CleanupKeep
	if err := s.settings.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}
	lib, err := library.NewServiceAt(filepath.Join(t.TempDir(), "library.json"))
	if err != nil {
		t.Fatal(err)
	}
	downloads := newFakeDownloads()
	s.downloads = downloads
	s.library = lib
	prepared := 0
	s.prepareRuntime = func(context.Context, string, string) error {
		prepared++
		return nil
	}
	s.sameVolume = func(string, string) bool { return true }
	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})
	return imageRig{s: s, downloads: downloads, lib: lib, games: s.settings.GetSettings().GamesPath, prepared: &prepared}
}

func (r imageRig) setOrigin(id string, origin download.Origin) {
	r.downloads.mu.Lock()
	defer r.downloads.mu.Unlock()
	d := r.downloads.items[id]
	d.Origin = origin
	r.downloads.items[id] = d
}

func (r imageRig) card(t *testing.T, id string) library.Game {
	t.Helper()
	game, err := r.lib.Find(id)
	if err != nil {
		t.Fatalf("find card: %v", err)
	}
	return game
}

func TestImageInstall(t *testing.T) {
	cases := []struct {
		name     string
		platform string
		mode     string
		tree     imageTree
		single   bool
		archive  bool
		cross    bool
		image    string
		files    int
		gone     bool
	}{
		{name: "single iso copied", platform: emulators.PS2, mode: ModeCopy, tree: imageTree{"Game.iso": "big"}, single: true, image: "Game.iso"},
		{name: "single iso moved", platform: emulators.PS2, mode: ModeMove, tree: imageTree{"Game.iso": "big"}, single: true, image: "Game.iso", gone: true},
		{name: "single iso moved across volumes", platform: emulators.PS2, mode: ModeMove, tree: imageTree{"Game.iso": "big"}, single: true, cross: true, image: "Game.iso", gone: true},
		{name: "cue and bin copied", platform: emulators.PS1, mode: ModeCopy,
			tree: imageTree{"Game/g.cue": `FILE "g.bin" BINARY`, "Game/g.bin": "big", "Game/readme.txt": "x"}, image: "g.cue", files: 1},
		{name: "cue and bin moved", platform: emulators.PS1, mode: ModeMove,
			tree: imageTree{"Game/g.cue": `FILE "g.bin" BINARY`, "Game/g.bin": "big", "Game/readme.txt": "x"}, image: "g.cue", files: 1, gone: true},
		{name: "m3u with discs", platform: emulators.PS1, mode: ModeCopy, tree: imageTree{
			"Game/g.m3u": "d1.cue\nd2.cue\n", "Game/d1.cue": `FILE "d1.bin" BINARY`, "Game/d1.bin": "big",
			"Game/d2.cue": `FILE "d2.bin" BINARY`, "Game/d2.bin": "big",
		}, image: "g.m3u", files: 4},
		{name: "chd", platform: emulators.PS2, mode: ModeCopy, tree: imageTree{"Game/g.chd": "big", "Game/g.nfo": "x"}, image: "g.chd"},
		{name: "pbp in a subfolder", platform: emulators.PSP, mode: ModeCopy,
			tree: imageTree{"Game/PSP/GAME/X/EBOOT.PBP": "big", "Game/other.txt": "x"}, image: "EBOOT.PBP"},
		{name: "zip archive", platform: emulators.PS2, tree: imageTree{}, single: true, archive: true, image: "game.iso"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newImageRig(t)
			if c.cross {
				r.s.sameVolume = differentVolumes
			}
			root := t.TempDir()
			plantImages(t, root, c.tree)
			name := "Game"
			if c.single {
				name = "Game.iso"
			}
			if c.archive {
				name = "Game.zip"
				writeZip(t, filepath.Join(root, name), []zipEntry{
					{name: "inner/game.iso", data: make([]byte, 4096)},
					{name: "readme.txt", data: []byte("x")},
				})
			}
			source := filepath.Join(root, name)
			r.downloads.add("d1", name, root)
			r.setOrigin("d1", download.Origin{ReleaseID: "r1", SourceID: "s1", GameID: "pc-game", Version: "1.0", Purpose: download.PurposeRelease})

			item, err := r.s.Start("d1", StartOptions{Platform: c.platform, Title: "Title: One", Cover: "https://example.com/c.jpg", Mode: c.mode})
			if err != nil {
				t.Fatal(err)
			}
			done := r.s.waitStatus(t, item.ID, StatusCompleted)
			r.s.waitJobDone(t, item.ID)

			want := filepath.Join(r.games, "Consoles", c.platform, "Title One")
			if !samePath(done.Destination, want) {
				t.Fatalf("destination = %q, want %q", done.Destination, want)
			}
			game := r.card(t, done.GameID)
			if image := filepath.Join(want, filepath.FromSlash(c.image)); !samePath(game.Executable, image) {
				t.Fatalf("image = %q, want %q", game.Executable, image)
			}
			if game.Emulation == nil || game.Emulation.Platform != c.platform || len(game.Emulation.Files) != c.files {
				t.Fatalf("emulation = %+v", game.Emulation)
			}
			if game.Title != "Title: One" || game.Cover != "https://example.com/c.jpg" || !game.Owned ||
				game.InstallType != library.InstallTypeImage || game.ReleaseID != "r1" || game.SourceID != "s1" ||
				game.SourceDownloadID != "d1" || game.Version != "1.0" || !samePath(game.InstallDir, want) {
				t.Fatalf("card = %+v", game)
			}
			if game.CanonicalGameID != "" {
				t.Fatalf("the card took the id of the PC game: %q", game.CanonicalGameID)
			}
			if game.Emulation.CatalogGameID != "pc-game" {
				t.Fatalf("catalog link = %q", game.Emulation.CatalogGameID)
			}
			if game.SizeBytes <= 0 {
				t.Fatalf("size = %d", game.SizeBytes)
			}
			if *r.prepared != 0 {
				t.Fatal("an image got a game runtime")
			}
			if len(r.lib.GetInstalledGames()) != 0 {
				t.Fatal("an image is listed among installations")
			}
			_, statErr := os.Stat(source)
			if c.gone != errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("source after install: %v, want gone=%v", statErr, c.gone)
			}
			if _, err := os.Stat(done.Destination + partialSuffix); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("partial left: %v", err)
			}
		})
	}
}

func TestImageInstallLeavesTheRegularStepsOut(t *testing.T) {
	s, downloads, registrar := newTestService(t)
	cfg := s.settings.GetSettings()
	cfg.DesktopShortcuts = true
	if err := s.settings.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}
	fake := &fakeRunner{}
	s.runner = fake
	root := t.TempDir()
	plantImages(t, root, imageTree{"Game/game.iso": "big", "Game/setup.exe": "big", "Game/game.exe": "big"})
	downloads.add("d1", "Game", root)
	item, err := s.Start("d1", StartOptions{Platform: emulators.PS2, Mode: ModeCopy})
	if err != nil {
		t.Fatal(err)
	}
	done := s.waitStatus(t, item.ID, StatusCompleted)
	if len(done.Candidates) != 0 || !strings.HasSuffix(done.Executable, "game.iso") {
		t.Fatalf("job = %+v", done)
	}
	if len(registrar.images) != 1 || len(registrar.games) != 0 {
		t.Fatalf("registered images %d, games %d", len(registrar.images), len(registrar.games))
	}
	if got := registrar.shortcutsCreated(); len(got) != 0 {
		t.Fatalf("shortcuts = %v", got)
	}
	if len(fake.calls()) != 0 {
		t.Fatalf("something was run: %+v", fake.calls())
	}
}

func TestImageStartRefuses(t *testing.T) {
	cases := []struct {
		name string
		opts StartOptions
		tree imageTree
		want error
	}{
		{name: "unknown console", opts: StartOptions{Platform: "ps9"}, want: emulators.ErrUnknownPlatform},
		{name: "console of another case", opts: StartOptions{Platform: "PS2"}, want: emulators.ErrUnknownPlatform},
		{name: "image type without a console", opts: StartOptions{Type: TypeImage}, want: errImageOptions},
		{name: "title without a console", opts: StartOptions{Title: "Game"}, want: errImageOptions},
		{name: "cover without a console", opts: StartOptions{Cover: "https://example.com/c.jpg"}, want: errImageOptions},
		{name: "another type for an image", opts: StartOptions{Platform: emulators.PS2, Type: TypePortable}, want: errImageOptions},
		{name: "installer for an image", opts: StartOptions{Platform: emulators.PS2, InstallerPath: `C:\setup.exe`}, want: errImageOptions},
		{name: "image without a card", opts: StartOptions{Platform: emulators.PS2, SkipRegister: true}, want: errImageOptions},
		{name: "local file as a cover", opts: StartOptions{Platform: emulators.PS2, Cover: `file:///C:/x.png`}, want: errImageOptions},
		{name: "script as a cover", opts: StartOptions{Platform: emulators.PS2, Cover: "javascript:alert(1)"}, want: errImageOptions},
		{name: "control character in the title", opts: StartOptions{Platform: emulators.PS2, Title: "a\x07b"}, want: errImageOptions},
		{name: "too long a title", opts: StartOptions{Platform: emulators.PS2, Title: strings.Repeat("a", 201)}, want: errImageOptions},
		{name: "unknown mode", opts: StartOptions{Platform: emulators.PS2, Mode: "link"}, want: errUnknownMode},
		{name: "folder outside Consoles", opts: StartOptions{Platform: emulators.PS2, Destination: filepath.Join(os.TempDir(), "Games", "Game")}, want: errImageDestination},
		{name: "two games", opts: StartOptions{Platform: emulators.PS2}, tree: imageTree{"Game/a.iso": "big", "Game/b.iso": "big"}, want: errImageAmbiguous},
		{name: "no image", opts: StartOptions{Platform: emulators.PS2}, tree: imageTree{"Game/game.exe": "big", "Game/data.pak": "big"}, want: errImageNotFound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newImageRig(t)
			root := t.TempDir()
			tree := c.tree
			if tree == nil {
				tree = imageTree{"Game/game.iso": "big"}
			}
			plantImages(t, root, tree)
			r.downloads.add("d1", "Game", root)
			if _, err := r.s.Start("d1", c.opts); !errors.Is(err, c.want) {
				t.Fatalf("Start = %v, want %v", err, c.want)
			}
			if len(r.s.List()) != 0 {
				t.Fatalf("a job was created: %+v", r.s.List())
			}
		})
	}
}

func TestImageStartNeedsAGamesFolder(t *testing.T) {
	r := newImageRig(t)
	root := t.TempDir()
	plantImages(t, root, imageTree{"Game/game.iso": "big"})
	r.downloads.add("d1", "Game", root)
	r.s.settings = nil
	if _, err := r.s.Start("d1", StartOptions{Platform: emulators.PS2}); !errors.Is(err, errNoDestination) {
		t.Fatalf("Start = %v", err)
	}
}

func TestImageNamesAreUnique(t *testing.T) {
	r := newImageRig(t)
	root := t.TempDir()
	plantImages(t, root, imageTree{"A/a.iso": "big", "B/b.iso": "big"})
	r.downloads.add("d1", "A", root)
	r.downloads.add("d2", "B", root)
	first, err := r.s.Start("d1", StartOptions{Platform: emulators.PS2, Title: "Game", Mode: ModeCopy})
	if err != nil {
		t.Fatal(err)
	}
	r.s.waitStatus(t, first.ID, StatusCompleted)
	second, err := r.s.Start("d2", StartOptions{Platform: emulators.PS2, Title: "Game", Mode: ModeCopy})
	if err != nil {
		t.Fatal(err)
	}
	done := r.s.waitStatus(t, second.ID, StatusCompleted)
	if want := filepath.Join(r.games, "Consoles", emulators.PS2, "Game (2)"); !samePath(done.Destination, want) {
		t.Fatalf("destination = %q, want %q", done.Destination, want)
	}
}

func TestImageReinstallOfTheSameReleaseIsRefusedBeforeCopying(t *testing.T) {
	r := newImageRig(t)
	root := t.TempDir()
	plantImages(t, root, imageTree{"A/a.iso": "big", "B/a.iso": "big"})
	r.downloads.add("d1", "A", root)
	r.downloads.add("d2", "B", root)
	origin := download.Origin{ReleaseID: "r1", SourceID: "s1"}
	r.setOrigin("d1", origin)
	r.setOrigin("d2", origin)
	first, err := r.s.Start("d1", StartOptions{Platform: emulators.PS2, Mode: ModeCopy})
	if err != nil {
		t.Fatal(err)
	}
	r.s.waitStatus(t, first.ID, StatusCompleted)
	if _, err := r.s.Start("d2", StartOptions{Platform: emulators.PS2, Mode: ModeCopy}); !errors.Is(err, library.ErrAlreadyAdded) {
		t.Fatalf("second install = %v", err)
	}
	if _, err := r.s.Start("d1", StartOptions{Platform: emulators.PS2, Mode: ModeCopy}); !errors.Is(err, library.ErrAlreadyAdded) {
		t.Fatalf("same download = %v", err)
	}
}

func TestImageArchiveWithoutAnImageLeavesNothing(t *testing.T) {
	cases := []struct {
		name    string
		entries []zipEntry
		want    error
	}{
		{"no image", []zipEntry{{name: "game.exe", data: make([]byte, 64)}}, errImageNotFound},
		{"two games", []zipEntry{{name: "a.iso", data: make([]byte, 64)}, {name: "b.iso", data: make([]byte, 64)}}, errImageAmbiguous},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newImageRig(t)
			root := t.TempDir()
			writeZip(t, filepath.Join(root, "Game.zip"), c.entries)
			r.downloads.add("d1", "Game.zip", root)
			item, err := r.s.Start("d1", StartOptions{Platform: emulators.PS2})
			if err != nil {
				t.Fatal(err)
			}
			failed := r.s.waitStatus(t, item.ID, StatusFailed)
			r.s.waitJobDone(t, item.ID)
			if !strings.Contains(failed.Error, c.want.Error()) {
				t.Fatalf("error = %q, want %q", failed.Error, c.want)
			}
			for _, path := range []string{failed.Destination, failed.Destination + partialSuffix} {
				if entries, err := os.ReadDir(path); err == nil && len(entries) > 0 {
					t.Fatalf("%s holds %d entries", path, len(entries))
				}
			}
			if len(r.lib.GetPlayableGames()) != 0 {
				t.Fatal("a card was created")
			}
		})
	}
}

func TestImageRegistrationFailureIsRetried(t *testing.T) {
	s, downloads, registrar := newTestService(t)
	root := t.TempDir()
	plantImages(t, root, imageTree{"Game/game.iso": "big"})
	downloads.add("d1", "Game", root)
	failure := errors.New("disk full")
	registrar.mu.Lock()
	registrar.imageErr = failure
	registrar.mu.Unlock()
	item, err := s.Start("d1", StartOptions{Platform: emulators.PS2, Mode: ModeMove})
	if err != nil {
		t.Fatal(err)
	}
	failed := s.waitStatus(t, item.ID, StatusFailed)
	s.waitJobDone(t, item.ID)
	if !strings.Contains(failed.Error, failure.Error()) {
		t.Fatalf("error = %q", failed.Error)
	}
	registrar.mu.Lock()
	registrar.imageErr = nil
	registrar.mu.Unlock()
	if err := s.Retry(item.ID); err != nil {
		t.Fatal(err)
	}
	s.waitStatus(t, item.ID, StatusCompleted)
	if len(registrar.images) != 1 {
		t.Fatalf("images = %d", len(registrar.images))
	}
}

func TestImageRetryInspectsTheConsoleImageAgain(t *testing.T) {
	s, downloads, registrar := newTestService(t)
	root := t.TempDir()
	plantImages(t, root, imageTree{"Game.iso": "big"})
	source := filepath.Join(root, "Game.iso")
	registrar.put(library.Game{ID: "manual", Title: "Manual", Executable: source, InstallDir: root,
		Emulation: &library.Emulation{Platform: emulators.PS2}})
	downloads.add("d1", "Game.iso", root)
	item, err := s.Start("d1", StartOptions{Platform: emulators.PS2, Title: "Named", Cover: "https://example.com/c.jpg", Mode: ModeMove})
	if err != nil {
		t.Fatal(err)
	}
	s.waitStatus(t, item.ID, StatusFailed)
	s.waitJobDone(t, item.ID)
	if err := registrar.RemoveGame("manual"); err != nil {
		t.Fatal(err)
	}
	if err := s.Retry(item.ID); err != nil {
		t.Fatal(err)
	}
	s.waitStatus(t, item.ID, StatusCompleted)
	got := registrar.images[0]
	if got.Title != "Named" || got.Cover == "" || got.Platform != emulators.PS2 || !got.Owned {
		t.Fatalf("card = %+v", got)
	}
}

// crashedImage leaves on disk what a killed launcher leaves: a record in work and
// the destination in the given state, then starts a new service over them.
func crashedImage(t *testing.T, fill func(dest string), mode string) (imageRig, Installation) {
	t.Helper()
	first := mustServiceAt(t, t.TempDir())
	root := t.TempDir()
	plantImages(t, root, imageTree{"Game/game.iso": "big"})
	cfg := newTestSettings(t)
	dest := filepath.Join(cfg.GetSettings().GamesPath, "Consoles", emulators.PS2, "Game")
	fill(dest)
	item := Installation{
		ID: "job", DownloadID: "d1", Name: "Game", Type: TypeImage, Status: StatusVerifying, Mode: mode,
		SourcePath: filepath.Join(root, "Game"), ContentRoot: filepath.Join(root, "Game"), Destination: dest,
		OwnedDestination: dest, Owned: true, Platform: emulators.PS2, Cover: "https://example.com/c.jpg",
	}
	if err := first.store.save([]Installation{item}); err != nil {
		t.Fatal(err)
	}
	s, err := newServiceAt(first.store.dir, cfg)
	if err != nil {
		t.Fatal(err)
	}
	s.prepareRuntime = func(context.Context, string, string) error {
		t.Error("an image got a game runtime")
		return nil
	}
	lib, err := library.NewServiceAt(filepath.Join(t.TempDir(), "library.json"))
	if err != nil {
		t.Fatal(err)
	}
	downloads := newFakeDownloads()
	downloads.add("d1", "Game", root)
	s.downloads, s.library = downloads, lib
	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})
	return imageRig{s: s, downloads: downloads, lib: lib, games: cfg.GetSettings().GamesPath}, item
}

func TestImageCrashAfterCommitIsFinishedOnStartup(t *testing.T) {
	r, item := crashedImage(t, func(dest string) { mkFile(t, filepath.Join(dest, "game.iso"), 4096) }, ModeMove)
	done := r.s.waitStatus(t, item.ID, StatusCompleted)
	game := r.card(t, done.GameID)
	if game.Emulation == nil || game.Cover == "" || !game.Owned {
		t.Fatalf("card = %+v", game)
	}
}

func TestImageCrashDuringCopyIsInterruptedAndRetried(t *testing.T) {
	r, item := crashedImage(t, func(dest string) { mkFile(t, filepath.Join(dest+partialSuffix, "game.iso"), 16) }, ModeCopy)
	interrupted := r.s.waitStatus(t, item.ID, StatusInterrupted)
	waitFor(t, "partial swept", func() bool { return !pathExists(interrupted.Destination + partialSuffix) })
	if err := r.s.Retry(item.ID); err != nil {
		t.Fatal(err)
	}
	done := r.s.waitStatus(t, item.ID, StatusCompleted)
	info, err := os.Stat(filepath.Join(done.Destination, "game.iso"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 4096 {
		t.Fatalf("size = %d, the partial copy survived", info.Size())
	}
}

// A crash after the card was written and before the job closed must not turn the
// next start into "already added".
func TestImageCrashAfterRegistrationDoesNotFailTheJob(t *testing.T) {
	first, item := crashedImage(t, func(dest string) { mkFile(t, filepath.Join(dest, "game.iso"), 4096) }, ModeMove)
	done := first.s.waitStatus(t, item.ID, StatusCompleted)
	first.s.waitJobDone(t, item.ID)
	again, err := first.lib.RegisterEmulated(t.Context(), library.InstalledImage{
		Title: "Game", Platform: emulators.PS2, Image: done.Executable, InstallDir: done.Destination, Owned: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != done.GameID || len(first.lib.GetPlayableGames()) != 1 {
		t.Fatalf("a second card: %+v", first.lib.GetPlayableGames())
	}
}

func ownedCard(t *testing.T, s *Service) library.Game {
	t.Helper()
	dir := filepath.Join(s.config().GamesPath, "Consoles", emulators.PS2, "Game")
	mkFile(t, filepath.Join(dir, "game.iso"), 4096)
	mkFile(t, filepath.Join(dir, "disc2.iso"), 4096)
	return library.Game{
		ID: "emu", Title: "Game", Executable: filepath.Join(dir, "game.iso"), InstallDir: dir, Owned: true,
		InstallType: library.InstallTypeImage, Emulation: &library.Emulation{Platform: emulators.PS2},
	}
}

func TestRemovingAnInstalledImageDeletesItsFolder(t *testing.T) {
	for _, keep := range []bool{false, true} {
		s, _, registrar := newTestService(t)
		game := ownedCard(t, s)
		beside := filepath.Join(filepath.Dir(game.InstallDir), "Other", "other.iso")
		mkFile(t, beside, 64)
		released := false
		s.releaseRuntime = func(string) error {
			released = true
			return nil
		}
		registrar.put(game)

		info, err := s.InspectRemoval(game.ID)
		if err != nil {
			t.Fatal(err)
		}
		if info.Method != RemovalFiles || !info.Owned {
			t.Fatalf("plan = %+v", info)
		}
		if err := s.RemoveGame(game.ID, RemoveOptions{DeleteFiles: true, KeepInLibrary: keep}); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(game.InstallDir); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("folder stayed: %v", err)
		}
		if _, err := os.Stat(beside); err != nil {
			t.Fatalf("the neighbour was touched: %v", err)
		}
		if released {
			t.Fatal("an image has no game runtime to release")
		}
	}
}

func TestRemovingAnImageRefusesFoldersItDoesNotOwn(t *testing.T) {
	cases := []struct {
		name  string
		build func(t *testing.T, s *Service, r *fakeRegistrar, game *library.Game)
		want  error
	}{
		{name: "added by hand", want: errUnsafeRemoval,
			build: func(_ *testing.T, _ *Service, _ *fakeRegistrar, game *library.Game) {
				game.Owned, game.InstallType = false, ""
			}},
		{name: "owned without the install type", want: errUnsafeRemoval,
			build: func(_ *testing.T, _ *Service, _ *fakeRegistrar, game *library.Game) { game.InstallType = "" }},
		{name: "image outside its folder", want: errUnsafeRemoval,
			build: func(t *testing.T, _ *Service, _ *fakeRegistrar, game *library.Game) {
				game.Executable = filepath.Join(t.TempDir(), "game.iso")
			}},
		{name: "track outside its folder", want: errUnsafeRemoval,
			build: func(t *testing.T, _ *Service, _ *fakeRegistrar, game *library.Game) {
				game.Emulation.Files = []string{filepath.Join(t.TempDir(), "track.bin")}
			}},
		{name: "the downloads folder", want: errUnsafeRemoval,
			build: func(t *testing.T, s *Service, _ *fakeRegistrar, game *library.Game) {
				game.InstallDir = s.config().DownloadsPath
				game.Executable = filepath.Join(game.InstallDir, "game.iso")
				mkFile(t, game.Executable, 64)
			}},
		{name: "folder holding the downloads folder", want: errUnsafeRemoval,
			build: func(t *testing.T, s *Service, _ *fakeRegistrar, game *library.Game) {
				svc, err := settings.NewServiceAt(filepath.Join(t.TempDir(), "settings.json"))
				if err != nil {
					t.Fatal(err)
				}
				if _, err := svc.SetupLibrary(game.InstallDir); err != nil {
					t.Fatal(err)
				}
				s.settings = svc
			}},
		{name: "folder with the image of another card", want: emulators.ErrImageInside,
			build: func(_ *testing.T, _ *Service, r *fakeRegistrar, game *library.Game) {
				r.put(library.Game{ID: "other", Title: "Other", Executable: filepath.Join(game.InstallDir, "disc2.iso"),
					InstallDir: game.InstallDir, Emulation: &library.Emulation{Platform: emulators.PS2}})
			}},
		{name: "folder of another game", want: errUnsafeRemoval,
			build: func(_ *testing.T, _ *Service, r *fakeRegistrar, game *library.Game) {
				r.put(library.Game{ID: "pc", Title: "Other", InstallDir: filepath.Join(game.InstallDir, "pc")})
			}},
		{name: "a link instead of the folder", want: errUnsafeRemoval,
			build: func(t *testing.T, _ *Service, _ *fakeRegistrar, game *library.Game) {
				target := t.TempDir()
				mkFile(t, filepath.Join(target, "game.iso"), 64)
				if err := os.RemoveAll(game.InstallDir); err != nil {
					t.Fatal(err)
				}
				plantLink(t, game.InstallDir, target)
			}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, _, registrar := newTestService(t)
			game := ownedCard(t, s)
			kept := game.Executable
			c.build(t, s, registrar, &game)
			registrar.put(game)
			if err := s.RemoveGame(game.ID, RemoveOptions{DeleteFiles: true}); !errors.Is(err, c.want) {
				t.Fatalf("RemoveGame = %v, want %v", err, c.want)
			}
			if _, err := os.Stat(kept); err != nil && c.name != "a link instead of the folder" {
				t.Fatalf("files were touched: %v", err)
			}
		})
	}
}

func TestDownloadGuardCoversTheFolderOfAnInstalledImage(t *testing.T) {
	s, _, registrar := newTestService(t)
	game := ownedCard(t, s)
	registrar.put(game)
	inside := download.Download{ID: "d", Name: "disc2.iso", Destination: game.InstallDir, Status: download.StatusCompleted}.WithRoot("disc2.iso")
	if err := s.CheckDownloadDeletable(inside); !errors.Is(err, errDownloadIsInstall) {
		t.Fatalf("CheckDownloadDeletable = %v", err)
	}
	if !s.insideGame(filepath.Join(game.InstallDir, "nested")) {
		t.Fatal("a new install may land inside an installed image")
	}
}

func TestPlainInstallIsNotAffected(t *testing.T) {
	t.Run("a single iso is still refused", func(t *testing.T) {
		s, downloads, _ := newTestService(t)
		root := t.TempDir()
		plantImages(t, root, imageTree{"game.iso": "big"})
		downloads.add("d1", "game.iso", root)
		if _, err := s.Start("d1", StartOptions{Destination: filepath.Join(t.TempDir(), "Game")}); !errors.Is(err, errUnrecognizedSource) {
			t.Fatalf("Start = %v", err)
		}
	})
	t.Run("a folder with an image stays unknown", func(t *testing.T) {
		dir := t.TempDir()
		plantImages(t, dir, imageTree{"game.iso": "big"})
		plan, err := Inspect(t.Context(), dir)
		if err != nil {
			t.Fatal(err)
		}
		if plan.Type != TypeUnknown || !plan.RequiresUserInteraction || plan.Platform != "" || plan.ImagePath != "" {
			t.Fatalf("plan = %+v", plan)
		}
	})
	t.Run("a bin still counts as installer data", func(t *testing.T) {
		dir := t.TempDir()
		plantImages(t, dir, imageTree{"game.exe": "small", "data.bin": "big"})
		plan, err := Inspect(t.Context(), dir)
		if err != nil {
			t.Fatal(err)
		}
		if plan.Type != TypeExeInstaller {
			t.Fatalf("plan = %+v", plan)
		}
	})
	t.Run("a portable game goes the old way", func(t *testing.T) {
		s, downloads, registrar := newTestService(t)
		s.sameVolume = differentVolumes
		root := t.TempDir()
		source := portableSource(t, root, "Game")
		downloads.add("d1", "Game", root)
		dest := filepath.Join(t.TempDir(), "Game")
		item, err := s.Start("d1", StartOptions{Destination: dest, Mode: ModeCopy})
		if err != nil {
			t.Fatal(err)
		}
		done := s.waitStatus(t, item.ID, StatusCompleted)
		if done.Type != TypePortable || done.Platform != "" || done.Cover != "" || !samePath(done.Destination, dest) {
			t.Fatalf("job = %+v", done)
		}
		if len(registrar.images) != 0 || len(registrar.games) != 1 {
			t.Fatalf("images %d, games %d", len(registrar.images), len(registrar.games))
		}
		if !strings.HasSuffix(registrar.games[0].Executable, "Game.exe") {
			t.Fatalf("game = %+v", registrar.games[0])
		}
		if _, err := os.Stat(source); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("an archive goes the old way", func(t *testing.T) {
		s, downloads, registrar := newTestService(t)
		root := t.TempDir()
		writeZip(t, filepath.Join(root, "Game.zip"), []zipEntry{{name: "Game.exe", data: append([]byte("MZ"), make([]byte, 1<<16)...)}})
		downloads.add("d1", "Game.zip", root)
		item, err := s.Start("d1", StartOptions{Destination: filepath.Join(t.TempDir(), "Game")})
		if err != nil {
			t.Fatal(err)
		}
		done := s.waitStatus(t, item.ID, StatusCompleted)
		if done.Type != TypeArchiveZip || done.Mode != "" || len(registrar.images) != 0 || len(registrar.games) != 1 {
			t.Fatalf("job = %+v", done)
		}
	})
	t.Run("a card of an image does not catch a game of the same catalog id", func(t *testing.T) {
		lib, err := library.NewServiceAt(filepath.Join(t.TempDir(), "library.json"))
		if err != nil {
			t.Fatal(err)
		}
		dir := t.TempDir()
		iso := filepath.Join(dir, "game.iso")
		exe := filepath.Join(t.TempDir(), "game.exe")
		mkFile(t, iso, 64)
		mkFile(t, exe, 64)
		image, err := lib.RegisterEmulated(t.Context(), library.InstalledImage{Title: "Game", Platform: emulators.PS2, Image: iso, InstallDir: dir})
		if err != nil {
			t.Fatal(err)
		}
		game, err := lib.RegisterInstalled(t.Context(), library.InstalledGame{Title: "Game", Executable: exe, InstallDir: filepath.Dir(exe), CanonicalGameID: "c1"})
		if err != nil {
			t.Fatal(err)
		}
		if game.ID == image.ID || game.Emulation != nil {
			t.Fatalf("the install sat on the image card: %+v", game)
		}
	})
}
