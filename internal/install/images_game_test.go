package install

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/download"
	"typhon/internal/emulators"
	"typhon/internal/emulators/sfotest"
	"typhon/internal/library"
)

func sfoBody(title, category string) string {
	return string(sfotest.Bytes(map[string]string{"TITLE_ID": title, "CATEGORY": category}))
}

func TestFindImagesOfUnpackedGames(t *testing.T) {
	disc := sfoBody("BLES00001", "DG")
	update := sfoBody("BLES00001", "GD")
	other := sfoBody("BLUS99999", "HG")
	vita := sfoBody("PCSE00001", "gd")
	cases := []struct {
		name     string
		platform string
		tree     imageTree
		want     string
		err      error
	}{
		{name: "ps3 disc", platform: emulators.PS3, tree: imageTree{
			"PS3_DISC.SFB": "small", "PS3_GAME/PARAM.SFO": disc, "PS3_GAME/USRDIR/EBOOT.BIN": "big",
		}, want: "PS3_GAME/USRDIR/EBOOT.BIN"},
		{name: "ps3 title folder", platform: emulators.PS3, tree: imageTree{
			"BLES00001/PARAM.SFO": disc, "BLES00001/USRDIR/EBOOT.BIN": "big",
		}, want: "BLES00001/USRDIR/EBOOT.BIN"},
		{name: "ps3 another case", platform: emulators.PS3, tree: imageTree{
			"G/param.sfo": disc, "G/usrdir/eboot.bin": "big",
		}, want: "G/usrdir/eboot.bin"},
		{name: "an update next to the game is not a second game", platform: emulators.PS3, tree: imageTree{
			"PS3_GAME/PARAM.SFO": disc, "PS3_GAME/USRDIR/EBOOT.BIN": "big",
			"BLES00001_UPDATE/PARAM.SFO": update, "BLES00001_UPDATE/USRDIR/EBOOT.BIN": "big",
		}, want: "PS3_GAME/USRDIR/EBOOT.BIN"},
		{name: "the update folder of a disc is not read", platform: emulators.PS3, tree: imageTree{
			"PS3_GAME/PARAM.SFO": disc, "PS3_GAME/USRDIR/EBOOT.BIN": "big",
			"PS3_UPDATE/PARAM.SFO": "broken", "PS3_UPDATE/USRDIR/EBOOT.BIN": "big",
		}, want: "PS3_GAME/USRDIR/EBOOT.BIN"},
		{name: "an eboot inside the game is the game's", platform: emulators.PS3, tree: imageTree{
			"PS3_GAME/PARAM.SFO": disc, "PS3_GAME/USRDIR/EBOOT.BIN": "small",
			"PS3_GAME/USRDIR/extra/PARAM.SFO": other, "PS3_GAME/USRDIR/extra/USRDIR/EBOOT.BIN": "big",
		}, want: "PS3_GAME/USRDIR/EBOOT.BIN"},
		{name: "a copy of the disc gives way to it", platform: emulators.PS3, tree: imageTree{
			"PS3_GAME/PARAM.SFO": disc, "PS3_GAME/USRDIR/EBOOT.BIN": "small",
			"BLES00001/PARAM.SFO": disc, "BLES00001/USRDIR/EBOOT.BIN": "big",
		}, want: "PS3_GAME/USRDIR/EBOOT.BIN"},
		{name: "a stray bin is nothing", platform: emulators.PS3, tree: imageTree{
			"PS3_GAME/PARAM.SFO": disc, "PS3_GAME/USRDIR/EBOOT.BIN": "small", "PS3_GAME/USRDIR/data.bin": "big",
		}, want: "PS3_GAME/USRDIR/EBOOT.BIN"},
		{name: "ps4", platform: emulators.PS4, tree: imageTree{
			"CUSA00001/eboot.bin": "big", "CUSA00001/sce_sys/param.sfo": sfoBody("CUSA00001", "gd"),
		}, want: "CUSA00001/eboot.bin"},
		{name: "ps4 update beside the game", platform: emulators.PS4, tree: imageTree{
			"CUSA00001/eboot.bin": "big", "CUSA00001/sce_sys/param.sfo": sfoBody("CUSA00001", "gd"),
			"CUSA00001-UPDATE/eboot.bin": "big", "CUSA00001-UPDATE/sce_sys/param.sfo": sfoBody("CUSA00001", "gp"),
		}, want: "CUSA00001/eboot.bin"},
		{name: "vita folder", platform: emulators.Vita, tree: imageTree{
			"PCSE00001/eboot.bin": "big", "PCSE00001/sce_sys/param.sfo": vita,
		}, want: "PCSE00001/eboot.bin"},

		{name: "two discs are two games", platform: emulators.PS3, tree: imageTree{
			"A/PS3_GAME/PARAM.SFO": disc, "A/PS3_GAME/USRDIR/EBOOT.BIN": "big",
			"B/PS3_GAME/PARAM.SFO": other, "B/PS3_GAME/USRDIR/EBOOT.BIN": "small",
		}, err: errImageAmbiguous},
		{name: "a disc and a folder of another game", platform: emulators.PS3, tree: imageTree{
			"PS3_GAME/PARAM.SFO": disc, "PS3_GAME/USRDIR/EBOOT.BIN": "big",
			"BLUS99999/PARAM.SFO": other, "BLUS99999/USRDIR/EBOOT.BIN": "small",
		}, err: errImageAmbiguous},
		{name: "two ps4 games, one small", platform: emulators.PS4, tree: imageTree{
			"A/eboot.bin": "big", "A/sce_sys/param.sfo": sfoBody("CUSA00001", "gd"),
			"B/eboot.bin": "small", "B/sce_sys/param.sfo": sfoBody("CUSA00002", "gd"),
		}, err: errImageAmbiguous},
		{name: "only an update", platform: emulators.PS3, tree: imageTree{
			"BLES00001/PARAM.SFO": update, "BLES00001/USRDIR/EBOOT.BIN": "big",
		}, err: errImageUpdateOnly},
		{name: "a game stays one with an update beside a package", platform: emulators.PS4, tree: imageTree{
			"U/eboot.bin": "big", "U/sce_sys/param.sfo": sfoBody("CUSA00001", "gp"), "game.pkg": "big",
		}, err: errImageUpdateOnly},
		{name: "no sibling", platform: emulators.PS3, tree: imageTree{"G/USRDIR/EBOOT.BIN": "big"}, err: errImageNotFound},
		{name: "ps4 no sibling", platform: emulators.PS4, tree: imageTree{"G/eboot.bin": "big"}, err: errImageNotFound},
		{name: "eboot outside USRDIR", platform: emulators.PS3, tree: imageTree{
			"G/PARAM.SFO": disc, "G/EBOOT.BIN": "big",
		}, err: errImageNotFound},
		{name: "any other bin", platform: emulators.PS4, tree: imageTree{
			"G/game.bin": "big", "G/sce_sys/param.sfo": sfoBody("CUSA00001", "gd"),
		}, err: errImageNotFound},
		{name: "an empty file", platform: emulators.PS4, tree: imageTree{
			"G/eboot.bin": "", "G/sce_sys/param.sfo": sfoBody("CUSA00001", "gd"),
		}, err: errImageNotFound},
		{name: "nothing", platform: emulators.Vita, tree: imageTree{}, err: errImageNotFound},
		{name: "ps3 iso only", platform: emulators.PS3, tree: imageTree{"game.iso": "big"}, err: emulators.ErrImageManualOnly},
		{name: "ps3 pkg only", platform: emulators.PS3, tree: imageTree{"game.pkg": "big", "a.nfo": "x"}, err: emulators.ErrImageManualOnly},
		{name: "ps4 pkg only", platform: emulators.PS4, tree: imageTree{"game.PKG": "big"}, err: emulators.ErrImageManualOnly},
		{name: "vita pkg only", platform: emulators.Vita, tree: imageTree{"game.pkg": "big"}, err: emulators.ErrImageManualOnly},
		{name: "broken param.sfo is not a missing game", platform: emulators.PS3, tree: imageTree{
			"G/PARAM.SFO": "broken", "G/USRDIR/EBOOT.BIN": "big",
		}, err: emulators.ErrSFOInvalid},
		{name: "vita folder with a bad id", platform: emulators.Vita, tree: imageTree{
			"G/eboot.bin": "big", "G/sce_sys/param.sfo": sfoBody("bad", "gd"),
		}, err: emulators.ErrTitleIDInvalid},
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

func vitaPackage(t *testing.T, path, title, category string) string {
	t.Helper()
	writeZip(t, path, []zipEntry{
		{name: "eboot.bin", data: make([]byte, 64)},
		{name: "sce_sys/param.sfo", data: sfotest.Bytes(map[string]string{"TITLE_ID": title, "CATEGORY": category})},
	})
	return path
}

func TestFindImagesOfVitaPackages(t *testing.T) {
	t.Run("one package", func(t *testing.T) {
		dir := t.TempDir()
		image := vitaPackage(t, filepath.Join(dir, "game.vpk"), "PCSE00001", "gd")
		got, err := FindImages(t.Context(), dir, emulators.Vita)
		if err != nil || !samePath(got, image) {
			t.Fatalf("FindImages = %q, %v", got, err)
		}
	})
	t.Run("two packages, however small one is", func(t *testing.T) {
		dir := t.TempDir()
		vitaPackage(t, filepath.Join(dir, "a.vpk"), "PCSE00001", "gd")
		if err := os.Mkdir(filepath.Join(dir, "sub"), 0o700); err != nil {
			t.Fatal(err)
		}
		vitaPackage(t, filepath.Join(dir, "sub", "b.vpk"), "PCSE00002", "gd")
		if _, err := FindImages(t.Context(), dir, emulators.Vita); !errors.Is(err, errImageAmbiguous) {
			t.Fatalf("FindImages = %v", err)
		}
	})
	t.Run("a package beside its update", func(t *testing.T) {
		dir := t.TempDir()
		image := vitaPackage(t, filepath.Join(dir, "game.vpk"), "PCSE00001", "gd")
		vitaPackage(t, filepath.Join(dir, "patch.vpk"), "PCSE00001", "gp")
		got, err := FindImages(t.Context(), dir, emulators.Vita)
		if err != nil || !samePath(got, image) {
			t.Fatalf("FindImages = %q, %v", got, err)
		}
	})
	t.Run("a package beside its add-on", func(t *testing.T) {
		dir := t.TempDir()
		image := vitaPackage(t, filepath.Join(dir, "game.vpk"), "PCSE00001", "gd")
		vitaPackage(t, filepath.Join(dir, "dlc.vpk"), "PCSE00001", "ac")
		got, err := FindImages(t.Context(), dir, emulators.Vita)
		if err != nil || !samePath(got, image) {
			t.Fatalf("FindImages = %q, %v", got, err)
		}
	})
	t.Run("only an add-on", func(t *testing.T) {
		dir := t.TempDir()
		vitaPackage(t, filepath.Join(dir, "dlc.vpk"), "PCSE00001", "ac")
		if _, err := FindImages(t.Context(), dir, emulators.Vita); !errors.Is(err, errImageAddonOnly) {
			t.Fatalf("FindImages = %v", err)
		}
	})
	t.Run("a package with a path that climbs out", func(t *testing.T) {
		dir := t.TempDir()
		writeZip(t, filepath.Join(dir, "game.vpk"), []zipEntry{
			{name: "sce_sys/param.sfo", data: sfotest.Bytes(map[string]string{"TITLE_ID": "PCSE00001"})},
			{name: "../evil.dll", data: []byte("x")},
		})
		if _, err := FindImages(t.Context(), dir, emulators.Vita); !errors.Is(err, emulators.ErrPackageInvalid) {
			t.Fatalf("FindImages = %v", err)
		}
	})
}

func TestFindImagesOfOlderConsolesStaysTheSame(t *testing.T) {
	dir := t.TempDir()
	plantImages(t, dir, imageTree{"game.iso": "big", "readme.pkg": "small"})
	for _, id := range []string{emulators.PS2, emulators.PSP} {
		got, err := FindImages(t.Context(), dir, id)
		if err != nil || !samePath(got, filepath.Join(dir, "game.iso")) {
			t.Fatalf("%s: FindImages = %q, %v", id, got, err)
		}
	}
}

func TestFindImagesDoesNotTakeALinkedGame(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	plantImages(t, outside, imageTree{"eboot.bin": "big", "sce_sys/param.sfo": sfoBody("CUSA00001", "gd")})
	plantLink(t, filepath.Join(dir, "linked"), outside)
	if _, err := FindImages(t.Context(), dir, emulators.PS4); !errors.Is(err, errImageNotFound) {
		t.Fatalf("FindImages followed a link: %v", err)
	}
}

func TestFindImagesOfGamesStopsOnACancelledContext(t *testing.T) {
	dir := t.TempDir()
	plantImages(t, dir, imageTree{"eboot.bin": "big", "sce_sys/param.sfo": sfoBody("CUSA00001", "gd")})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := FindImages(ctx, dir, emulators.PS4); !errors.Is(err, context.Canceled) {
		t.Fatalf("FindImages = %v", err)
	}
}

func TestInspectImageOfNextGenConsoles(t *testing.T) {
	t.Run("an iso alone", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "game.iso")
		mkFile(t, path, 64)
		for _, id := range []string{emulators.PS3, emulators.PS4, emulators.Vita} {
			if plan, err := InspectImage(t.Context(), path, id); !errors.Is(err, emulators.ErrImageManualOnly) || plan.CanAutoInstall {
				t.Fatalf("%s: %+v, %v", id, plan, err)
			}
		}
	})
	t.Run("a folder with a package only", func(t *testing.T) {
		dir := t.TempDir()
		plantImages(t, dir, imageTree{"game.pkg": "big", "other.bin": "small"})
		if _, err := InspectImage(t.Context(), dir, emulators.PS4); !errors.Is(err, emulators.ErrImageManualOnly) {
			t.Fatalf("InspectImage = %v", err)
		}
	})
	t.Run("an archive beside a package still gets its chance", func(t *testing.T) {
		dir := t.TempDir()
		writeZip(t, filepath.Join(dir, "game.zip"), []zipEntry{{name: "x", data: make([]byte, 8192)}})
		plantImages(t, dir, imageTree{"a.pkg": "x"})
		plan, err := InspectImage(t.Context(), dir, emulators.PS4)
		if err != nil || plan.ArchivePath == "" {
			t.Fatalf("plan = %+v, %v", plan, err)
		}
	})
	for name, category := range map[string]string{"update": "gp", "add-on": "ac"} {
		t.Run("an archive beside an "+name+" still gets its chance", func(t *testing.T) {
			dir := t.TempDir()
			filler := make([]byte, 1<<16)
			if _, err := rand.Read(filler); err != nil {
				t.Fatal(err)
			}
			writeZip(t, filepath.Join(dir, "game.zip"), []zipEntry{{name: "x", data: filler}})
			vitaPackage(t, filepath.Join(dir, "extra.vpk"), "PCSE00001", category)
			plan, err := InspectImage(t.Context(), dir, emulators.Vita)
			if err != nil || plan.ArchivePath == "" {
				t.Fatalf("plan = %+v, %v", plan, err)
			}
		})
	}
	t.Run("an eboot alone is not a game", func(t *testing.T) {
		dir := t.TempDir()
		plantImages(t, dir, imageTree{"eboot.bin": "big", "sce_sys/param.sfo": sfoBody("CUSA00001", "gd")})
		path := filepath.Join(dir, "eboot.bin")
		if _, err := InspectImage(t.Context(), path, emulators.PS4); !errors.Is(err, errImageNotFound) {
			t.Fatalf("InspectImage = %v", err)
		}
	})
	t.Run("a single package", func(t *testing.T) {
		path := vitaPackage(t, filepath.Join(t.TempDir(), "game.vpk"), "PCSE00001", "gd")
		plan, err := InspectImage(t.Context(), path, emulators.Vita)
		if err != nil || !plan.CanAutoInstall || !samePath(plan.ImagePath, path) {
			t.Fatalf("plan = %+v, %v", plan, err)
		}
	})
	t.Run("a single update package", func(t *testing.T) {
		path := vitaPackage(t, filepath.Join(t.TempDir(), "patch.vpk"), "PCSE00001", "gp")
		if plan, err := InspectImage(t.Context(), path, emulators.Vita); !errors.Is(err, errImageUpdateOnly) || plan.CanAutoInstall {
			t.Fatalf("plan = %+v, %v", plan, err)
		}
	})
	t.Run("a single add-on package", func(t *testing.T) {
		path := vitaPackage(t, filepath.Join(t.TempDir(), "dlc.vpk"), "PCSE00001", "ac")
		if plan, err := InspectImage(t.Context(), path, emulators.Vita); !errors.Is(err, errImageAddonOnly) || plan.CanAutoInstall {
			t.Fatalf("plan = %+v, %v", plan, err)
		}
	})
	t.Run("a broken package", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "game.vpk")
		mkFile(t, path, 64)
		if _, err := InspectImage(t.Context(), path, emulators.Vita); !errors.Is(err, emulators.ErrPackageInvalid) {
			t.Fatalf("InspectImage = %v", err)
		}
	})
}

func TestDiscInstallKeepsTheWholeGameInItsFolder(t *testing.T) {
	r := newImageRig(t)
	root := t.TempDir()
	plantImages(t, root, imageTree{
		"Game/PS3_DISC.SFB": "small", "Game/PS3_GAME/PARAM.SFO": sfoBody("BLES00001", "DG"),
		"Game/PS3_GAME/USRDIR/EBOOT.BIN": "big", "Game/PS3_GAME/USRDIR/data.dat": "big",
	})
	r.downloads.add("d1", "Game", root)
	r.setOrigin("d1", download.Origin{ReleaseID: "r1", SourceID: "s1", Purpose: download.PurposeRelease})

	item, err := r.s.Start("d1", StartOptions{Platform: emulators.PS3, Title: "Disc Game", Mode: ModeCopy})
	if err != nil {
		t.Fatal(err)
	}
	done := r.s.waitStatus(t, item.ID, StatusCompleted)
	r.s.waitJobDone(t, item.ID)

	want := filepath.Join(r.games, "Consoles", emulators.PS3, "Disc Game")
	game := r.card(t, done.GameID)
	if !samePath(game.InstallDir, want) || !game.Owned {
		t.Fatalf("card = %+v", game)
	}
	if image := filepath.Join(want, "PS3_GAME", "USRDIR", "EBOOT.BIN"); !samePath(game.Executable, image) {
		t.Fatalf("image = %q, want %q", game.Executable, image)
	}
	if game.SizeBytes < 8192 {
		t.Fatalf("size = %d: the folder of the game was not measured", game.SizeBytes)
	}
}

func unpackedCard(t *testing.T, s *Service, name string) library.Game {
	t.Helper()
	dir := filepath.Join(s.config().GamesPath, "Consoles", emulators.PS3, name)
	plantImages(t, dir, imageTree{
		"PS3_DISC.SFB": "small", "PS3_GAME/PARAM.SFO": sfoBody("BLES00001", "DG"), "PS3_GAME/USRDIR/EBOOT.BIN": "big",
	})
	return library.Game{
		ID: "emu", Title: name, Executable: filepath.Join(dir, "PS3_GAME", "USRDIR", "EBOOT.BIN"), InstallDir: dir, Owned: true,
		InstallType: library.InstallTypeImage, Emulation: &library.Emulation{Platform: emulators.PS3},
	}
}

// The file on the card lies three folders deep: removal must take the folder of
// the card and nothing above or below it.
func TestRemovingAnUnpackedGameDeletesItsWholeFolder(t *testing.T) {
	s, _, registrar := newTestService(t)
	game := unpackedCard(t, s, "Game")
	beside := filepath.Join(filepath.Dir(game.InstallDir), "Other", "PS3_GAME", "USRDIR", "EBOOT.BIN")
	mkFile(t, beside, 64)
	registrar.put(game)

	if err := s.RemoveGame(game.ID, RemoveOptions{DeleteFiles: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(game.InstallDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("folder stayed: %v", err)
	}
	if _, err := os.Stat(beside); err != nil {
		t.Fatalf("the neighbour was touched: %v", err)
	}
}

func TestRemovingAnUnpackedGameRefusesTheWrongLevel(t *testing.T) {
	cases := []struct {
		name string
		edit func(game *library.Game)
	}{
		{"the folder of the game file", func(game *library.Game) { game.InstallDir = filepath.Dir(game.Executable) }},
		{"the folder with the proof", func(game *library.Game) { game.InstallDir = filepath.Dir(filepath.Dir(game.Executable)) }},
		{"the folder of all games of the console", func(game *library.Game) { game.InstallDir = filepath.Dir(game.InstallDir) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, _, registrar := newTestService(t)
			game := unpackedCard(t, s, "Game")
			kept := game.Executable
			c.edit(&game)
			registrar.put(game)
			if err := s.RemoveGame(game.ID, RemoveOptions{DeleteFiles: true}); !errors.Is(err, errUnsafeRemoval) {
				t.Fatalf("RemoveGame = %v", err)
			}
			if _, err := os.Stat(kept); err != nil {
				t.Fatalf("files were touched: %v", err)
			}
		})
	}
}

// A folder named after the console can be the game folder of a record that only
// looks right: USRDIR as the card's folder would delete half of a game.
func TestRemovingAGameStandingOnItsOwnUSRDIRIsRefused(t *testing.T) {
	s, _, registrar := newTestService(t)
	consoleDir := filepath.Join(s.config().GamesPath, "Consoles", emulators.PS3)
	plantImages(t, consoleDir, imageTree{"PARAM.SFO": sfoBody("BLES00001", "DG"), "USRDIR/EBOOT.BIN": "big"})
	game := library.Game{
		ID: "emu", Title: "Game", Executable: filepath.Join(consoleDir, "USRDIR", "EBOOT.BIN"), InstallDir: filepath.Join(consoleDir, "USRDIR"),
		Owned: true, InstallType: library.InstallTypeImage, Emulation: &library.Emulation{Platform: emulators.PS3},
	}
	registrar.put(game)
	if err := s.RemoveGame(game.ID, RemoveOptions{DeleteFiles: true}); !errors.Is(err, errUnsafeRemoval) {
		t.Fatalf("RemoveGame = %v", err)
	}
	if _, err := os.Stat(game.Executable); err != nil {
		t.Fatalf("files were touched: %v", err)
	}
}

func TestNewRefusalsAreNotLoggedAsFaults(t *testing.T) {
	for _, err := range []error{
		errImageUpdateOnly, errImageAddonOnly, emulators.ErrImageManualOnly, emulators.ErrSFOMissing,
		emulators.ErrSFOInvalid, emulators.ErrTitleIDInvalid, emulators.ErrPackageInvalid,
	} {
		if !expectedImageRefusal(err) {
			t.Errorf("%v counts as a fault of the launcher", err)
		}
	}
}
