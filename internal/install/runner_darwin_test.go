//go:build darwin && !devmock

package install

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"typhon/internal/wine"
)

func failingDetect() (wine.Runtime, error) { return wine.Runtime{}, wine.ErrNotInstalled }

func testBottle(games, dest string) wine.Bottle {
	return wine.Bottle{Key: dest, Name: "Typhon-Demo-test", Path: filepath.Join(games, ".bottle"), Drive: "t", Games: games}
}

func TestWineRunnerFailsWithoutCrossOver(t *testing.T) {
	games := t.TempDir()
	r := wineRunner{gamesPath: func() string { return games }, detect: failingDetect}

	_, err := r.run(context.Background(), runSpec{Path: filepath.Join(games, "Demo", "setup.exe"), Destination: filepath.Join(games, "Demo")})
	if err == nil {
		t.Fatal("run without CrossOver: want error")
	}
}

func TestWineRunnerNeedsDestination(t *testing.T) {
	games := t.TempDir()
	r := wineRunner{gamesPath: func() string { return games }, detect: failingDetect}

	if _, err := r.run(context.Background(), runSpec{}); err == nil {
		t.Fatal("run without a destination: want error")
	}
}

func TestWineRunnerInstallerArgs(t *testing.T) {
	games := t.TempDir()
	dest := filepath.Join(games, "Demo")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	installer := filepath.Join(dest, "setup.exe")
	if err := os.WriteFile(installer, []byte("MZ"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var recorded wine.Cmd
	r := wineRunner{
		gamesPath: func() string { return games },
		detect:    failingDetect,
		runCmd: func(_ context.Context, _ wine.Bottle, c wine.Cmd) (int, error) {
			recorded = c
			return 0, nil
		},
	}

	code, err := r.runPrepared(context.Background(), runSpec{
		Path: installer, Destination: dest, Dir: dest, Args: []string{"/VERYSILENT"}, LogPath: "/tmp/x.log",
	}, testBottle(games, dest))
	if err != nil {
		t.Fatalf("runPrepared: %v", err)
	}
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if recorded.Path != `T:\Demo\setup.exe` {
		t.Fatalf("Path = %q, want %q", recorded.Path, `T:\Demo\setup.exe`)
	}
	if recorded.WorkDir != `T:\Demo` {
		t.Fatalf("WorkDir = %q, want %q", recorded.WorkDir, `T:\Demo`)
	}
	if !recorded.WaitChildren {
		t.Fatal("WaitChildren = false: установщик распаковывает себя и работает уже оттуда")
	}
	if strings.Join(recorded.Args, " ") != "/VERYSILENT" {
		t.Fatalf("Args = %v", recorded.Args)
	}
	if recorded.Log != "/tmp/x.log" {
		t.Fatalf("Log = %q", recorded.Log)
	}
}

func TestWineRunnerRejectsInstallerOutsideGames(t *testing.T) {
	games := t.TempDir()
	dest := filepath.Join(games, "Demo")
	r := wineRunner{gamesPath: func() string { return games }, detect: failingDetect}

	_, err := r.runPrepared(context.Background(), runSpec{
		Path: filepath.Join(t.TempDir(), "setup.exe"), Destination: dest,
	}, testBottle(games, dest))
	if err == nil {
		t.Fatal("installer outside the games folder: want error")
	}
}
