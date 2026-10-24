package wrappers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func (f *fixture) moveGame(t *testing.T, to string) {
	t.Helper()
	if err := os.Rename(f.dir, to); err != nil {
		t.Fatal(err)
	}
	f.dir = to
	f.games.game.InstallDir = to
	f.games.game.Executable = filepath.Join(to, "game.exe")
}

func (f *fixture) backupExists(name string) bool {
	_, err := os.Stat(filepath.Join(f.cfg, "wrappers", "backup", "g1", name))
	return err == nil
}

func TestRelocatedGameTakesWrapperAlong(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, makeTgz(t, dxvkEntries()), dxvkRelease, machineX86, []string{"d3d9.dll"})
	f.write(t, "d3d9.dll", "ORIGINAL")
	if err := f.svc.Enable(ctx, "g1", WrapperDXVK, "default"); err != nil {
		t.Fatal(err)
	}
	f.moveGame(t, f.dir+"-moved")
	info, err := f.svc.Inspect(ctx, "g1")
	if err != nil || info.Active == nil || info.Problem != nil {
		t.Fatalf("info = %+v, err = %v", info, err)
	}
	if err := f.svc.Disable(ctx, "g1"); err != nil {
		t.Fatal(err)
	}
	if f.read(t, "d3d9.dll") != "ORIGINAL" {
		t.Fatal("original not restored in the new directory")
	}
	if f.backupExists("d3d9.dll") || len(f.svc.entries) != 0 {
		t.Fatal("entry or backup left behind")
	}
}

func TestUnavailableGameDirKeepsBackup(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, makeTgz(t, dxvkEntries()), dxvkRelease, machineX86, []string{"d3d9.dll"})
	f.write(t, "d3d9.dll", "ORIGINAL")
	if err := f.svc.Enable(ctx, "g1", WrapperDXVK, "default"); err != nil {
		t.Fatal(err)
	}
	away := f.dir + "-away"
	if err := os.Rename(f.dir, away); err != nil {
		t.Fatal(err)
	}
	f.setPhase(t, phaseRemoving)
	svc := f.reopen(t)
	if err := svc.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
		t.Fatalf("an absent disk must not stop the launcher: %v", err)
	}
	info, err := svc.Inspect(ctx, "g1")
	if err != nil || info.Problem == nil || info.Problem.Code != "wrappers.game_dir_missing" {
		t.Fatalf("info = %+v, err = %v", info, err)
	}
	if err := svc.Disable(ctx, "g1"); !errors.Is(err, ErrGameDirMissing) {
		t.Fatalf("Disable err = %v", err)
	}
	if !f.backupExists("d3d9.dll") || len(svc.entries) != 1 {
		t.Fatal("backup or entry lost while the directory was absent")
	}
	if err := os.Rename(away, f.dir); err != nil {
		t.Fatal(err)
	}
	if err := svc.Disable(ctx, "g1"); err != nil {
		t.Fatal(err)
	}
	if f.read(t, "d3d9.dll") != "ORIGINAL" || f.backupExists("d3d9.dll") {
		t.Fatal("original not restored after the disk came back")
	}
}

func TestGameRemovedFromLibraryReleasesEntry(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, makeTgz(t, dxvkEntries()), dxvkRelease, machineX86, []string{"d3d9.dll"})
	f.write(t, "d3d9.dll", "ORIGINAL")
	if err := f.svc.Enable(ctx, "g1", WrapperDXVK, "default"); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(f.dir); err != nil {
		t.Fatal(err)
	}
	f.games.game.ID = "someone-else"
	if err := f.svc.Disable(ctx, "g1"); err != nil {
		t.Fatal(err)
	}
	if f.backupExists("d3d9.dll") || len(f.svc.entries) != 0 {
		t.Fatal("entry or backup left behind")
	}
}

func TestWrapperConfIsAlwaysRemoved(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		orig string
	}{
		{"no original conf", ""},
		{"with original conf", "USER CONF"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t, makeZip(t, dgvoodooEntries()), dgvoodooRelease, machineX86, []string{"DDRAW.dll"})
			if c.orig != "" {
				f.write(t, confName, c.orig)
			}
			if err := f.svc.Enable(ctx, "g1", WrapperDgVoodoo, "default"); err != nil {
				t.Fatal(err)
			}
			f.write(t, confName, "EDITED IN dgVoodooCpl")
			if err := f.svc.Disable(ctx, "g1"); err != nil {
				t.Fatalf("an edited wrapper config must not block Disable: %v", err)
			}
			if c.orig == "" && f.exists(confName) {
				t.Fatal("wrapper conf left behind")
			}
			if c.orig != "" && f.read(t, confName) != c.orig {
				t.Fatal("original conf not restored")
			}
			if f.exists("ddraw.dll") || len(f.svc.entries) != 0 {
				t.Fatal("wrapper not fully removed")
			}
		})
	}
}

func TestChangedDllRefusesThenKeepChanged(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, makeTgz(t, dxvkEntries()), dxvkRelease, machineX86, []string{"d3d11.dll"})
	f.write(t, "dxgi.dll", "ORIG-DXGI")
	if err := f.svc.Enable(ctx, "g1", WrapperDXVK, "default"); err != nil {
		t.Fatal(err)
	}
	f.write(t, "d3d11.dll", "UPDATED BY THE GAME")
	err := f.svc.Disable(ctx, "g1")
	if !errors.Is(err, ErrTampered) {
		t.Fatalf("Disable err = %v", err)
	}
	info, err := f.svc.Inspect(ctx, "g1")
	want := &Problem{Code: "wrappers.tampered", File: "d3d11.dll"}
	if err != nil || !reflect.DeepEqual(info.Problem, want) || info.Active == nil {
		t.Fatalf("info = %+v, err = %v", info, err)
	}
	if f.read(t, "dxgi.dll") != "DXVK-dxgi" || !f.exists("d3d10core.dll") {
		t.Fatal("a refused Disable must change nothing")
	}
	if err := f.svc.DisableKeepChanged(ctx, "g1"); err != nil {
		t.Fatal(err)
	}
	if f.read(t, "d3d11.dll") != "UPDATED BY THE GAME" {
		t.Fatal("changed file was touched")
	}
	if f.read(t, "dxgi.dll") != "ORIG-DXGI" || f.exists("d3d10core.dll") {
		t.Fatal("untouched files not returned to their original state")
	}
	if f.backupExists("dxgi.dll") || len(f.svc.entries) != 0 {
		t.Fatal("entry or backup left behind")
	}
}

func TestFailedRollbackStaysVisible(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, makeTgz(t, dxvkEntries()), dxvkRelease, machineX86, []string{"d3d11.dll"})
	f.svc.emit = func(_ string, data any) {
		if p, ok := data.(Progress); ok && p.Stage == "apply" && p.Done == 1 {
			if err := os.WriteFile(filepath.Join(f.dir, "d3d11.dll"), []byte("USER"), 0o600); err != nil {
				t.Error(err)
			}
		}
	}
	err := f.svc.Enable(ctx, "g1", WrapperDXVK, "default")
	if !errors.Is(err, ErrTampered) {
		t.Fatalf("Enable err = %v", err)
	}
	info, err := f.svc.Inspect(ctx, "g1")
	want := &Problem{Code: "wrappers.tampered", File: "d3d11.dll"}
	if err != nil || info.Active != nil || !reflect.DeepEqual(info.Problem, want) {
		t.Fatalf("info = %+v, err = %v", info, err)
	}
	if err := f.svc.Enable(ctx, "g1", WrapperDXVK, "default"); !errors.Is(err, ErrNeedsRepair) {
		t.Fatalf("second Enable err = %v", err)
	}
	if err := f.svc.DisableKeepChanged(ctx, "g1"); err != nil {
		t.Fatal(err)
	}
	if f.read(t, "d3d11.dll") != "USER" || f.exists("d3d10core.dll") || len(f.svc.entries) != 0 {
		t.Fatal("cleanup after the failed rollback is wrong")
	}
}

func TestInterruptedPhaseWithoutRecordedProblemIsReported(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, makeTgz(t, dxvkEntries()), dxvkRelease, machineX86, []string{"d3d9.dll"})
	if err := f.svc.Enable(ctx, "g1", WrapperDXVK, "default"); err != nil {
		t.Fatal(err)
	}
	f.setPhase(t, phaseRemoving)
	svc := f.reopen(t)
	info, err := svc.Inspect(ctx, "g1")
	if err != nil || info.Active != nil || info.Problem == nil || info.Problem.Code != "wrappers.recovery_failed" {
		t.Fatalf("info = %+v, err = %v", info, err)
	}
	if err := svc.Enable(ctx, "g1", WrapperDXVK, "default"); !errors.Is(err, ErrNeedsRepair) {
		t.Fatalf("Enable err = %v", err)
	}
	if err := svc.Disable(ctx, "g1"); err != nil || f.exists("d3d9.dll") {
		t.Fatalf("Disable err = %v", err)
	}
}

func TestStartupSweepsOwnTempLeftovers(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, makeTgz(t, dxvkEntries()), dxvkRelease, machineX86, []string{"d3d9.dll"})
	if err := f.svc.Enable(ctx, "g1", WrapperDXVK, "default"); err != nil {
		t.Fatal(err)
	}
	f.write(t, ".typhon-wrap-123.tmp", "half written")
	f.write(t, "other.tmp", "not ours")
	svc := f.reopen(t)
	if err := svc.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	if f.exists(".typhon-wrap-123.tmp") || !f.exists("other.tmp") {
		t.Fatal("only this package's temp files must be swept")
	}
}

func TestPlacingLeavesNoTempFiles(t *testing.T) {
	f := newFixture(t, makeTgz(t, dxvkEntries()), dxvkRelease, machineX86, []string{"d3d11.dll"})
	if err := f.svc.Enable(context.Background(), "g1", WrapperDXVK, "default"); err != nil {
		t.Fatal(err)
	}
	left, err := filepath.Glob(filepath.Join(f.dir, ".typhon-wrap-*"))
	if err != nil || len(left) != 0 {
		t.Fatalf("leftovers %v, err %v", left, err)
	}
}

func TestStateWithBadLocationIsRejected(t *testing.T) {
	cases := map[string]string{
		"relative install dir": `{"version":1,"entries":{"g1":{"installDir":"rel","relDir":".","files":[]}}}`,
		"escaping rel dir":     `{"version":1,"entries":{"g1":{"installDir":"` + filepath.ToSlash(os.TempDir()) + `","relDir":"../x","files":[]}}}`,
		"path in file name":    `{"version":1,"entries":{"g1":{"installDir":"` + filepath.ToSlash(os.TempDir()) + `","relDir":".","files":[{"name":"../evil.dll"}]}}}`,
		"bad game id":          `{"version":1,"entries":{"../g":{"installDir":"` + filepath.ToSlash(os.TempDir()) + `","relDir":".","files":[]}}}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := seedState(t, body)
			_, err := newService(cfg, &fakeGames{}, map[string]release{}, newDownloader(allowedURL), true)
			if !errors.Is(err, ErrBadState) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}
