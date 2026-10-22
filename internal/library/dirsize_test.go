package library

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDirSizeReportsFailures(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "game.exe")
	if err := os.WriteFile(file, make([]byte, 1024), 0o644); err != nil {
		t.Fatal(err)
	}

	size, err := dirSizeCtx(t.Context(), root)
	if err != nil {
		t.Fatalf("dirSize: %v", err)
	}
	if size != 1024 {
		t.Fatalf("size = %d, want 1024", size)
	}

	cases := []struct{ name, dir string }{
		{name: "empty path", dir: ""},
		{name: "missing dir", dir: filepath.Join(root, "gone")},
		{name: "file on the path", dir: filepath.Join(file, "sub")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			size, err := dirSizeCtx(t.Context(), tc.dir)
			if err == nil {
				t.Fatalf("dirSize = %d, want error", size)
			}
			if size != 0 {
				t.Fatalf("size = %d, want 0 alongside the error", size)
			}
		})
	}
}

func TestRegisterInstalledMarksUnknownSize(t *testing.T) {
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	exe := tempGameExe(t)

	game, err := s.RegisterInstalled(t.Context(), InstalledGame{
		Executable: exe,
		Title:      "Game",
		InstallDir: filepath.Join(exe, "sub"),
	})
	if err != nil {
		t.Fatalf("RegisterInstalled: %v", err)
	}
	if !game.SizeUnknown {
		t.Fatal("unmeasurable install dir must be marked as unknown size, not as 0 bytes")
	}
	if game.SizeBytes != 0 {
		t.Fatalf("sizeBytes = %d, want 0", game.SizeBytes)
	}
}

func TestCancelledMeasureIsUnknownNotZero(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	exe := tempGameExe(t)

	added, err := s.AddGame(ctx, exe, "Added")
	if err != nil || !added.SizeUnknown || added.SizeBytes != 0 {
		t.Fatalf("AddGame = %+v, %v; want SizeUnknown", added, err)
	}
	exe2 := tempGameExe(t)
	reg, err := s.RegisterInstalled(ctx, InstalledGame{Title: "Reg", Executable: exe2, InstallDir: filepath.Dir(exe2)})
	if err != nil || !reg.SizeUnknown || reg.SizeBytes != 0 {
		t.Fatalf("RegisterInstalled = %+v, %v; want SizeUnknown", reg, err)
	}
	if _, err := s.RegisterInstalled(t.Context(), InstalledGame{Title: "Reg", Executable: exe2, InstallDir: filepath.Dir(exe2)}); err != nil {
		t.Fatal(err)
	}
	updated, err := s.ApplyInstalledUpdate(ctx, InstalledUpdate{ID: reg.ID, Version: "2"})
	if err != nil || !updated.SizeUnknown || updated.SizeBytes != 0 {
		t.Fatalf("ApplyInstalledUpdate = %+v, %v; want SizeUnknown", updated, err)
	}
}

func TestMeasureRunsOutsideLibraryMutex(t *testing.T) {
	old := measureInstall
	t.Cleanup(func() { measureInstall = old })
	entered := make(chan struct{})
	release := make(chan struct{})
	measureInstall = func(ctx context.Context, _, _ string) (int64, bool) {
		close(entered)
		select {
		case <-release:
		case <-ctx.Done():
		}
		return 1, false
	}
	s := mustServiceAt(t, filepath.Join(t.TempDir(), "library.json"))
	exe := tempGameExe(t)
	done := make(chan error, 1)
	go func() {
		_, err := s.AddGame(t.Context(), exe, "Slow")
		done <- err
	}()
	<-entered
	listed := make(chan struct{})
	go func() {
		s.GetGames()
		close(listed)
	}()
	<-listed
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
