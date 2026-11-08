package library

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"typhon/internal/emulators"
)

func installedImage(t *testing.T) InstalledImage {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "Consoles", "ps1", "Game")
	writeFile(t, filepath.Join(dir, "game.bin"), "12345678")
	writeFile(t, filepath.Join(dir, "disc2.bin"), "1234")
	cue := writeFile(t, filepath.Join(dir, "game.cue"), `FILE "game.bin" BINARY`)
	return InstalledImage{
		Title: "Game", Cover: "https://example.com/c.jpg", CatalogGameID: "c1", Platform: emulators.PS1, Image: cue, InstallDir: dir,
		Owned: true, ReleaseID: "r1", SourceID: "s1", SourceDownloadID: "d1", Version: "1.0",
	}
}

func TestRegisterEmulated(t *testing.T) {
	s := newLibrary(t)
	input := installedImage(t)
	game, err := s.RegisterEmulated(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if game.Emulation == nil || game.Emulation.Platform != emulators.PS1 || len(game.Emulation.Files) != 1 {
		t.Fatalf("emulation = %+v", game.Emulation)
	}
	if game.CanonicalGameID != "" || !game.Owned || game.InstallType != InstallTypeImage || game.Source != SourceManaged {
		t.Fatalf("card = %+v", game)
	}
	if game.Cover != input.Cover || game.ReleaseID != "r1" || game.SourceID != "s1" || game.SourceDownloadID != "d1" || game.Version != "1.0" {
		t.Fatalf("card = %+v", game)
	}
	if game.SizeBytes < 12 || game.SizeUnknown {
		t.Fatalf("size = %d, unknown = %v: the folder is the game's", game.SizeBytes, game.SizeUnknown)
	}
	if len(s.GetInstalledGames()) != 0 {
		t.Fatal("an image is listed among installations")
	}
	reloaded, err := NewServiceAt(s.path)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := reloaded.Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Emulation == nil || !stored.Owned || stored.InstallType != InstallTypeImage {
		t.Fatalf("after reload: %+v", stored)
	}
}

func TestRegisterEmulatedRefuses(t *testing.T) {
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	cases := []struct {
		name string
		ctx  context.Context
		edit func(*InstalledImage)
		want error
	}{
		{name: "cancelled", ctx: cancelled, edit: func(*InstalledImage) {}, want: context.Canceled},
		{name: "no title", edit: func(g *InstalledImage) { g.Title = " " }, want: emulators.ErrTitleEmpty},
		{name: "no folder", edit: func(g *InstalledImage) { g.InstallDir = " " }, want: errEmptyInstallDir},
		{name: "relative folder", edit: func(g *InstalledImage) { g.InstallDir = "games" }, want: errEmptyInstallDir},
		{name: "unknown console", edit: func(g *InstalledImage) { g.Platform = "ps9" }, want: emulators.ErrUnknownPlatform},
		{name: "extension of another console", edit: func(g *InstalledImage) { g.Platform = emulators.PSP }, want: emulators.ErrImageExtension},
		{name: "no image", edit: func(g *InstalledImage) { g.Image = g.Image + ".cue" }, want: emulators.ErrImageMissing},
		{name: "image outside the folder", edit: func(g *InstalledImage) { g.InstallDir = filepath.Join(g.InstallDir, "sub") }, want: errImageOutside},
		{name: "cover is not a url", edit: func(g *InstalledImage) { g.Cover = `C:\cover.png` }, want: errCoverInvalid},
		{name: "cover of a wrong scheme", edit: func(g *InstalledImage) { g.Cover = "javascript:alert(1)" }, want: errCoverInvalid},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newLibrary(t)
			input := installedImage(t)
			c.edit(&input)
			ctx := c.ctx
			if ctx == nil {
				ctx = t.Context()
			}
			if _, err := s.RegisterEmulated(ctx, input); !errors.Is(err, c.want) {
				t.Fatalf("RegisterEmulated = %v, want %v", err, c.want)
			}
			if len(s.GetGames()) != 0 {
				t.Fatal("a card stayed in memory")
			}
		})
	}
}

func TestRegisterEmulatedRollsBackWhenSaveFails(t *testing.T) {
	s := newLibrary(t)
	first, err := s.RegisterEmulated(t.Context(), installedImage(t))
	if err != nil {
		t.Fatal(err)
	}
	blockEmulated(t, s)
	second := installedImage(t)
	second.ReleaseID, second.SourceDownloadID = "r2", "d2"
	if _, err := s.RegisterEmulated(t.Context(), second); err == nil {
		t.Fatal("the failed save was swallowed")
	}
	if games := s.GetGames(); len(games) != 1 || games[0].ID != first.ID {
		t.Fatalf("memory left ahead of the disk: %+v", games)
	}
}

func TestRegisterEmulatedRollsBackAnUpdateWhenSaveFails(t *testing.T) {
	s := newLibrary(t)
	input := installedImage(t)
	game, err := s.RegisterEmulated(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	blockEmulated(t, s)
	input.Title = "Renamed"
	if _, err := s.RegisterEmulated(t.Context(), input); err == nil {
		t.Fatal("the failed save was swallowed")
	}
	stored, err := s.Find(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Title != "Game" {
		t.Fatalf("title = %q", stored.Title)
	}
}

func TestRegisterEmulatedDuplicates(t *testing.T) {
	t.Run("the same folder is the same install", func(t *testing.T) {
		s := newLibrary(t)
		input := installedImage(t)
		first, err := s.RegisterEmulated(t.Context(), input)
		if err != nil {
			t.Fatal(err)
		}
		second, err := s.RegisterEmulated(t.Context(), input)
		if err != nil {
			t.Fatal(err)
		}
		if second.ID != first.ID || len(s.GetGames()) != 1 {
			t.Fatalf("games = %+v", s.GetGames())
		}
	})
	t.Run("the same release in another folder", func(t *testing.T) {
		s := newLibrary(t)
		if _, err := s.RegisterEmulated(t.Context(), installedImage(t)); err != nil {
			t.Fatal(err)
		}
		if _, err := s.RegisterEmulated(t.Context(), installedImage(t)); !errors.Is(err, ErrAlreadyAdded) {
			t.Fatalf("RegisterEmulated = %v", err)
		}
	})
	t.Run("an image already added by hand", func(t *testing.T) {
		s := newLibrary(t)
		input := installedImage(t)
		if _, err := s.AddEmulatedGame(t.Context(), input.Image, input.Platform, "Manual"); err != nil {
			t.Fatal(err)
		}
		if _, err := s.RegisterEmulated(t.Context(), input); !errors.Is(err, ErrAlreadyAdded) {
			t.Fatalf("RegisterEmulated = %v", err)
		}
	})
	t.Run("a card left without files comes back", func(t *testing.T) {
		s := newLibrary(t)
		first, err := s.RegisterEmulated(t.Context(), installedImage(t))
		if err != nil {
			t.Fatal(err)
		}
		if err := s.MarkUninstalled(first.ID); err != nil {
			t.Fatal(err)
		}
		second, err := s.RegisterEmulated(t.Context(), installedImage(t))
		if err != nil {
			t.Fatal(err)
		}
		if second.ID != first.ID || second.Uninstalled || !second.Owned || second.ReleaseID != "r1" || len(s.GetGames()) != 1 {
			t.Fatalf("games = %+v", s.GetGames())
		}
	})
}

func TestFootprintOfAnInstalledImageIsItsFolder(t *testing.T) {
	s := newLibrary(t)
	input := installedImage(t)
	game, err := s.RegisterEmulated(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if got := game.Footprint(); len(got) != 3 || got[0] != input.InstallDir {
		t.Fatalf("footprint = %v", got)
	}
	manual := emulatedGame(t, s, "game.iso")
	if got := manual.Footprint(); len(got) != 1 || got[0] != manual.Executable {
		t.Fatalf("footprint of a manual image = %v", got)
	}
}

func TestRebindingOutOfItsFolderDropsTheOwnership(t *testing.T) {
	s := newLibrary(t)
	input := installedImage(t)
	game, err := s.RegisterEmulated(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(input.InstallDir, "disc2.bin")
	bound, err := s.rebindImage(game.ID, inside, emulators.PS1)
	if err != nil {
		t.Fatal(err)
	}
	if !bound.Owned || bound.InstallDir != input.InstallDir {
		t.Fatalf("inside its folder the card stays the owner: %+v", bound)
	}
	outside := writeFile(t, filepath.Join(t.TempDir(), "Downloads", "other.bin"), "x")
	moved, err := s.rebindImage(game.ID, outside, emulators.PS1)
	if err != nil {
		t.Fatal(err)
	}
	if moved.Owned || moved.InstallType != "" || moved.ReleaseID != "" || moved.InstallDir != filepath.Dir(outside) {
		t.Fatalf("the card would delete the folder it was moved to: %+v", moved)
	}
}

func TestCatalogLinkStaysLocal(t *testing.T) {
	s := newLibrary(t)
	game, err := s.RegisterEmulated(t.Context(), installedImage(t))
	if err != nil {
		t.Fatal(err)
	}
	if game.Emulation.CatalogGameID != "c1" || game.CanonicalGameID != "" {
		t.Fatalf("card = %+v", game)
	}
	encoded, err := json.Marshal(s.SyncSnapshot())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "c1") || strings.Contains(string(encoded), "catalogGameId") {
		t.Fatalf("the link left for sync: %s", encoded)
	}
	stored, err := os.ReadFile(emulatedPathFor(s.path))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stored), "catalogGameId") {
		t.Fatalf("the link is not in the image file: %s", stored)
	}
	native, err := os.ReadFile(s.path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if strings.Contains(string(native), "catalogGameId") {
		t.Fatalf("the link reached library.json: %s", native)
	}
}

func TestRebindingKeepsTheCatalogLinkWhileOwned(t *testing.T) {
	s := newLibrary(t)
	input := installedImage(t)
	game, err := s.RegisterEmulated(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := s.rebindImage(game.ID, filepath.Join(input.InstallDir, "disc2.bin"), emulators.PS1)
	if err != nil {
		t.Fatal(err)
	}
	if bound.Emulation.CatalogGameID != "c1" {
		t.Fatalf("link lost inside the folder: %+v", bound.Emulation)
	}
	moved, err := s.rebindImage(game.ID, writeFile(t, filepath.Join(t.TempDir(), "other.bin"), "x"), emulators.PS1)
	if err != nil {
		t.Fatal(err)
	}
	if moved.Emulation.CatalogGameID != "" {
		t.Fatalf("link survived the loss of ownership: %+v", moved.Emulation)
	}
}

func TestSheetWithAByteOrderMarkIsRead(t *testing.T) {
	names, err := cueFiles(strings.NewReader("\ufeffFILE \"game.bin\" BINARY\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "game.bin" {
		t.Fatalf("names = %v", names)
	}
}

func TestUnreadableListKeepsAManualCardWithUnknownSize(t *testing.T) {
	s := newLibrary(t)
	cue := writeFile(t, filepath.Join(t.TempDir(), "game.cue"), "FILE \"broken")
	game, err := s.AddEmulatedGame(t.Context(), cue, emulators.PS1, "Game")
	if err != nil {
		t.Fatal(err)
	}
	if !game.SizeUnknown {
		t.Fatalf("card = %+v", game)
	}
	if _, _, err := ImageMembers(cue, t.Context().Err); !errors.Is(err, ErrImageList) {
		t.Fatalf("ImageMembers = %v", err)
	}
}
