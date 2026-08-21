package install

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectPortableDir(t *testing.T) {
	root := t.TempDir()
	mkFile(t, filepath.Join(root, "Game.exe"), 2<<20)
	mkFile(t, filepath.Join(root, "Data", "assets.pak"), 4096)

	plan, err := Inspect(context.Background(), root)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if plan.Type != TypePortable {
		t.Fatalf("type = %s, want %s", plan.Type, TypePortable)
	}
	if plan.ContentRoot != root {
		t.Fatalf("contentRoot = %s, want %s", plan.ContentRoot, root)
	}
	if !plan.CanAutoInstall || plan.RequiresUserInteraction {
		t.Fatalf("flags auto=%v interactive=%v", plan.CanAutoInstall, plan.RequiresUserInteraction)
	}
	if len(plan.Candidates) == 0 || filepath.Base(plan.Candidates[0].Path) != "Game.exe" {
		t.Fatalf("candidates = %+v", plan.Candidates)
	}
	if plan.EstimatedSize < 2<<20 {
		t.Fatalf("estimatedSize = %d", plan.EstimatedSize)
	}
}

func TestInspectExeInstaller(t *testing.T) {
	root := t.TempDir()
	mkFile(t, filepath.Join(root, "setup.exe"), 1024)
	mkFile(t, filepath.Join(root, "data1.bin"), 8192)
	mkFile(t, filepath.Join(root, "data2.bin"), 8192)

	plan, err := Inspect(context.Background(), root)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if plan.Type != TypeExeInstaller {
		t.Fatalf("type = %s, want %s", plan.Type, TypeExeInstaller)
	}
	if plan.InstallerPath != filepath.Join(root, "setup.exe") {
		t.Fatalf("installerPath = %s", plan.InstallerPath)
	}
	if plan.WorkingDir != root {
		t.Fatalf("workingDir = %s, want %s", plan.WorkingDir, root)
	}
	if !plan.RequiresUserInteraction || plan.CanAutoInstall {
		t.Fatalf("flags auto=%v interactive=%v", plan.CanAutoInstall, plan.RequiresUserInteraction)
	}
}

func TestInspectSingleExeWithData(t *testing.T) {
	root := t.TempDir()
	mkFile(t, filepath.Join(root, "Game_Release.exe"), 1024)
	mkFile(t, filepath.Join(root, "data1.cab"), 8192)

	plan, err := Inspect(context.Background(), root)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if plan.Type != TypeExeInstaller {
		t.Fatalf("type = %s, want %s", plan.Type, TypeExeInstaller)
	}
	if plan.WorkingDir != root {
		t.Fatalf("workingDir = %s, want %s", plan.WorkingDir, root)
	}
}

func TestInspectMsiInstaller(t *testing.T) {
	root := t.TempDir()
	mkFile(t, filepath.Join(root, "installer.msi"), 4096)

	plan, err := Inspect(context.Background(), root)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if plan.Type != TypeMsiInstaller {
		t.Fatalf("type = %s, want %s", plan.Type, TypeMsiInstaller)
	}
	if plan.InstallerPath != filepath.Join(root, "installer.msi") {
		t.Fatalf("installerPath = %s", plan.InstallerPath)
	}
	if plan.WorkingDir != root {
		t.Fatalf("workingDir = %s", plan.WorkingDir)
	}
	if !plan.RequiresUserInteraction {
		t.Fatal("msi must require user interaction")
	}
}

func TestInspectMsiWinsOverSetupExe(t *testing.T) {
	root := t.TempDir()
	mkFile(t, filepath.Join(root, "setup.exe"), 1024)
	mkFile(t, filepath.Join(root, "payload.msi"), 4096)

	plan, err := Inspect(context.Background(), root)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if plan.Type != TypeMsiInstaller {
		t.Fatalf("type = %s, want %s", plan.Type, TypeMsiInstaller)
	}
}

func TestInspectSingleArchive(t *testing.T) {
	root := t.TempDir()
	archive := filepath.Join(root, "game.zip")
	writeZip(t, archive, []zipEntry{
		{name: "game/Game.exe", data: make([]byte, 5000)},
		{name: "game/readme.txt", data: []byte("hello")},
	})

	plan, err := Inspect(context.Background(), root)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if plan.Type != TypeArchiveZip {
		t.Fatalf("type = %s, want %s", plan.Type, TypeArchiveZip)
	}
	if plan.ArchivePath != archive {
		t.Fatalf("archivePath = %s, want %s", plan.ArchivePath, archive)
	}
	if plan.CompressedSize <= 0 {
		t.Fatalf("compressedSize = %d", plan.CompressedSize)
	}
	if plan.EstimatedSize != 5005 {
		t.Fatalf("estimatedSize = %d, want 5005", plan.EstimatedSize)
	}
	if !plan.CanAutoInstall {
		t.Fatal("zip must be auto installable")
	}
}

func TestInspectArchiveFileDirectly(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "game.zip")
	writeZip(t, archive, []zipEntry{{name: "game.exe", data: make([]byte, 64)}})

	plan, err := Inspect(context.Background(), archive)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if plan.Type != TypeArchiveZip {
		t.Fatalf("type = %s, want %s", plan.Type, TypeArchiveZip)
	}
	if plan.ArchivePath != archive || plan.ContentRoot != dir {
		t.Fatalf("plan = %+v", plan)
	}
	if !plan.CanAutoInstall {
		t.Fatal("readable archive must be auto installable")
	}
}

func TestInspectUnreadableArchiveIsNotAutoInstallable(t *testing.T) {
	cases := []struct{ name, file string }{
		{name: "broken 7z", file: "game.7z"},
		{name: "broken zip", file: "game.zip"},
		{name: "broken rar", file: "game.rar"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			archive := filepath.Join(dir, tc.file)
			mkFile(t, archive, 64)

			plan, err := Inspect(context.Background(), archive)
			if err == nil {
				t.Fatalf("Inspect = %+v, want error", plan)
			}
			if plan.CanAutoInstall {
				t.Fatal("unreadable archive must never be marked auto installable")
			}
		})
	}
}

func TestInspectJunkOnly(t *testing.T) {
	root := t.TempDir()
	mkText(t, filepath.Join(root, "readme.txt"), "nothing here")
	mkText(t, filepath.Join(root, "release.nfo"), "scene info")

	plan, err := Inspect(context.Background(), root)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if plan.Type != TypeUnknown {
		t.Fatalf("type = %s, want %s", plan.Type, TypeUnknown)
	}
	if !plan.RequiresUserInteraction {
		t.Fatal("unknown must require user interaction")
	}
}

func TestInspectDescendsSingleSubdir(t *testing.T) {
	outer := t.TempDir()
	mkText(t, filepath.Join(outer, "info.nfo"), "scene")
	inner := filepath.Join(outer, "Some.Game.Repack")
	mkFile(t, filepath.Join(inner, "Game.exe"), 2<<20)
	mkFile(t, filepath.Join(inner, "Data", "main.pak"), 2048)

	plan, err := Inspect(context.Background(), outer)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if plan.ContentRoot != inner {
		t.Fatalf("contentRoot = %s, want %s", plan.ContentRoot, inner)
	}
	if plan.SourcePath != outer {
		t.Fatalf("sourcePath = %s, want %s", plan.SourcePath, outer)
	}
	if plan.Type != TypePortable {
		t.Fatalf("type = %s, want %s", plan.Type, TypePortable)
	}
}

func TestInspectIgnoresUninstallerAsGame(t *testing.T) {
	root := t.TempDir()
	mkFile(t, filepath.Join(root, "unins000.exe"), 1<<20)
	mkFile(t, filepath.Join(root, "Data", "blob.dat"), 1024)

	plan, err := Inspect(context.Background(), root)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if plan.Type != TypeUnknown {
		t.Fatalf("type = %s, want %s", plan.Type, TypeUnknown)
	}
	for _, c := range plan.Candidates {
		if strings.Contains(strings.ToLower(c.Path), "unins") {
			t.Fatalf("uninstaller surfaced as candidate: %s", c.Path)
		}
	}
}

func TestInspectMissingDir(t *testing.T) {
	if _, err := Inspect(context.Background(), filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("expected error for missing dir")
	}
}
