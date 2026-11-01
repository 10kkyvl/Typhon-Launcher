package install

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/download"
	"typhon/internal/emulators"
	"typhon/internal/library"
)

func emulatedRecord(t *testing.T, name string) library.Game {
	t.Helper()
	dir := t.TempDir()
	rom := filepath.Join(dir, name)
	if err := os.WriteFile(rom, []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	return library.Game{
		ID: "emu", Title: "Emulated", Executable: rom, InstallDir: dir,
		Emulation: &library.Emulation{Platform: emulators.PS2},
	}
}

// The folder of an image belongs to the user. A marker left there by another
// game must not turn removing a card into deleting the folder.
func TestRemovingAnEmulatedGameNeverTouchesFiles(t *testing.T) {
	for _, keep := range []bool{false, true} {
		s, _, registrar := newTestService(t)
		game := emulatedRecord(t, "game.iso")
		other := filepath.Join(game.InstallDir, "save.dat")
		if err := os.WriteFile(other, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := library.WriteMarker(game.InstallDir, library.Marker{
			GameID: "someone", InstallType: "portable", Owned: true,
			Uninstall: library.Uninstall{Command: `"` + other + `"`},
		}); err != nil {
			t.Fatal(err)
		}
		registrar.put(game)

		info, err := s.InspectRemoval(game.ID)
		if err != nil {
			t.Fatal(err)
		}
		if info.Method != RemovalRecord || info.Owned {
			t.Fatalf("plan offers to touch files: %+v", info)
		}
		if err := s.RemoveGame(game.ID, RemoveOptions{DeleteFiles: true, KeepInLibrary: keep}); !errors.Is(err, errUnsafeRemoval) {
			t.Fatalf("delete files: %v", err)
		}
		if err := s.RemoveGame(game.ID, RemoveOptions{KeepInLibrary: true}); !errors.Is(err, errNothingToRemove) {
			t.Fatalf("keep in library: %v", err)
		}
		if err := s.RemoveGame(game.ID, RemoveOptions{}); err != nil {
			t.Fatalf("remove the record: %v", err)
		}
		for _, path := range []string{game.Executable, other} {
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("removal touched %s: %v", path, err)
			}
		}
	}
}

func TestImageFolderIsNotAGameRoot(t *testing.T) {
	s, _, registrar := newTestService(t)
	game := emulatedRecord(t, "game.iso")
	registrar.put(game)
	if s.insideGame(filepath.Join(game.InstallDir, "New Game")) {
		t.Fatal("an image in the games folder blocks every install next to it")
	}
}

func TestDownloadGuardFollowsTheImageNotItsFolder(t *testing.T) {
	game := emulatedRecord(t, "game.iso")
	base := game.InstallDir
	beside := download.Download{ID: "d1", Name: "Other", Destination: base, Status: download.StatusCompleted}.WithRoot("Other")
	holder := download.Download{ID: "d2", Name: "game.iso", Destination: base, Status: download.StatusCompleted}.WithRoot("game.iso")
	whole := download.Download{ID: "d3", Name: filepath.Base(base), Destination: filepath.Dir(base), Status: download.StatusCompleted}.WithRoot(filepath.Base(base))
	cases := []struct {
		name string
		d    download.Download
		want error
	}{
		{"a download beside the image", beside, nil},
		{"the image itself is the download", holder, errDownloadIsInstall},
		{"the download holds the image", whole, errDownloadIsInstall},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, _, registrar := newTestService(t)
			registrar.put(game)
			if err := s.CheckDownloadDeletable(c.d); !errors.Is(err, c.want) {
				t.Fatalf("CheckDownloadDeletable = %v, want %v", err, c.want)
			}
		})
	}
}

func TestExecutableCandidatesAreRefusedForAnImage(t *testing.T) {
	s, _, registrar := newTestService(t)
	game := emulatedRecord(t, "game.iso")
	registrar.put(game)
	if _, err := s.ExecutableCandidates(game.ID); !errors.Is(err, emulators.ErrUnsupported) {
		t.Fatalf("ExecutableCandidates = %v", err)
	}
}

func TestFolderHoldingAnImageIsNotDeleted(t *testing.T) {
	s, _, registrar := newTestService(t)
	dir := gameDir(t, "Owned")
	track := filepath.Join(dir, "disc", "track.bin")
	if err := os.MkdirAll(filepath.Dir(track), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(track, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	registrar.put(library.Game{ID: "pc", InstallDir: dir, Owned: true})
	registrar.put(library.Game{
		ID: "emu", Title: "Emulated", Executable: filepath.Join(t.TempDir(), "game.cue"), InstallDir: t.TempDir(),
		Emulation: &library.Emulation{Platform: emulators.PS1, Files: []string{track}},
	})
	if err := s.RemoveGame("pc", RemoveOptions{DeleteFiles: true}); !errors.Is(err, emulators.ErrImageInside) {
		t.Fatalf("RemoveGame = %v", err)
	}
	if _, err := os.Stat(track); err != nil {
		t.Fatalf("the track is gone: %v", err)
	}
}

func TestCheckHoldsNoImage(t *testing.T) {
	game := emulatedRecord(t, "game.iso")
	cases := []struct {
		name string
		dir  string
		want error
	}{
		{"source holds the image", game.InstallDir, emulators.ErrImageInside},
		{"unrelated source", t.TempDir(), nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, _, registrar := newTestService(t)
			registrar.put(game)
			if err := s.checkHoldsNoImage(c.dir); !errors.Is(err, c.want) {
				t.Fatalf("checkHoldsNoImage = %v", err)
			}
		})
	}
}

func TestMoveInstallLeavesAnImageInTheSourceAlone(t *testing.T) {
	s, downloads, registrar := newTestService(t)
	root := t.TempDir()
	portableSource(t, root, "Game")
	image := filepath.Join(root, "Game", "game.iso")
	if err := os.WriteFile(image, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	registrar.put(library.Game{ID: "emu", Title: "Emulated", Executable: image, InstallDir: filepath.Dir(image),
		Emulation: &library.Emulation{Platform: emulators.PS2}})
	downloads.add("d1", "Game", root)
	item, err := s.Start("d1", StartOptions{Destination: filepath.Join(t.TempDir(), "Game"), Mode: ModeMove})
	if err != nil {
		t.Fatal(err)
	}
	s.waitStatus(t, item.ID, StatusFailed)
	s.waitJobDone(t, item.ID)
	if _, err := os.Stat(image); err != nil {
		t.Fatalf("the move took the image: %v", err)
	}
}
