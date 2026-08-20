package install

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestFindExecutablesPrefersGame(t *testing.T) {
	root := t.TempDir()
	mkFile(t, filepath.Join(root, "Game.exe"), 2<<20)
	mkFile(t, filepath.Join(root, "unins000.exe"), 4<<20)
	mkFile(t, filepath.Join(root, "UnityCrashHandler64.exe"), 3<<20)
	mkFile(t, filepath.Join(root, "vc_redist.x64.exe"), 8<<20)
	mkFile(t, filepath.Join(root, "_CommonRedist", "DXSETUP.exe"), 6<<20)
	mkFile(t, filepath.Join(root, "UE4PrereqSetup_x64.exe"), 5<<20)

	got := FindExecutables(root, "Game")
	if len(got) != 1 {
		t.Fatalf("candidates = %+v, want only Game.exe", got)
	}
	if filepath.Base(got[0].Path) != "Game.exe" {
		t.Fatalf("top = %s, want Game.exe", got[0].Path)
	}
	for _, c := range got {
		lower := strings.ToLower(filepath.Base(c.Path))
		for _, bad := range []string{"unins", "crash", "redist", "dxsetup", "prereq"} {
			if strings.Contains(lower, bad) {
				t.Fatalf("excluded exe surfaced: %s", c.Path)
			}
		}
	}
}

func TestFindExecutablesTitleBoost(t *testing.T) {
	root := t.TempDir()
	mkFile(t, filepath.Join(root, "Witcher3.exe"), 1<<20)
	mkFile(t, filepath.Join(root, "tool.exe"), 1<<20)

	got := FindExecutables(root, "The Witcher 3: Wild Hunt")
	if len(got) != 2 {
		t.Fatalf("candidates = %+v", got)
	}
	if filepath.Base(got[0].Path) != "Witcher3.exe" {
		t.Fatalf("top = %s, want Witcher3.exe", got[0].Path)
	}
	if got[0].Score <= got[1].Score {
		t.Fatalf("scores = %v, %v", got[0].Score, got[1].Score)
	}
}

func TestFindExecutablesPrefersShallowAndBinDirs(t *testing.T) {
	root := t.TempDir()
	mkFile(t, filepath.Join(root, "Binaries", "Win64", "Shooter.exe"), 1<<20)
	mkFile(t, filepath.Join(root, "third_party", "tools", "misc", "thing.exe"), 1<<20)

	got := FindExecutables(root, "Shooter")
	if len(got) != 2 || filepath.Base(got[0].Path) != "Shooter.exe" {
		t.Fatalf("candidates = %+v", got)
	}
}

func TestHighConfidence(t *testing.T) {
	if HighConfidence(nil) {
		t.Fatal("empty candidates must not be high confidence")
	}
	if !HighConfidence([]Candidate{{Path: "a", Score: 90}}) {
		t.Fatal("single strong candidate must be high confidence")
	}
	if HighConfidence([]Candidate{{Path: "a", Score: 40}}) {
		t.Fatal("weak candidate must not be high confidence")
	}
	if HighConfidence([]Candidate{{Path: "a", Score: 90}, {Path: "b", Score: 80}}) {
		t.Fatal("close runner-up must not be high confidence")
	}
	if !HighConfidence([]Candidate{{Path: "a", Score: 90}, {Path: "b", Score: 50}}) {
		t.Fatal("clear winner must be high confidence")
	}
}

func TestHighConfidenceOnScanResults(t *testing.T) {
	strong := t.TempDir()
	mkFile(t, filepath.Join(strong, "Game.exe"), 2<<20)
	if !HighConfidence(FindExecutables(strong, "Game")) {
		t.Fatalf("expected high confidence, got %+v", FindExecutables(strong, "Game"))
	}

	weak := t.TempDir()
	mkFile(t, filepath.Join(weak, "alpha.exe"), 2<<20)
	mkFile(t, filepath.Join(weak, "beta.exe"), 2<<20)
	if HighConfidence(FindExecutables(weak, "Something Else")) {
		t.Fatalf("expected low confidence, got %+v", FindExecutables(weak, "Something Else"))
	}
}

func TestExcludedExe(t *testing.T) {
	excluded := []string{
		"unins000", "uninstall", "UnityCrashHandler64", "CrashReportClient",
		"vc_redist.x64", "vcredist_x86", "DXSETUP", "dxwebsetup", "oalinst",
		"dotNetFx45_Full_setup", "setup", "Install", "Updater", "cleanup",
		"UE4PrereqSetup_x64", "launcher-helper", "activation",
	}
	for _, name := range excluded {
		if !excludedExe(name) {
			t.Fatalf("%s should be excluded", name)
		}
	}
	allowed := []string{"Game", "Witcher3", "eldenring", "GTA5", "Cyberpunk2077", "bin_win64"}
	for _, name := range allowed {
		if excludedExe(name) {
			t.Fatalf("%s should not be excluded", name)
		}
	}
}
