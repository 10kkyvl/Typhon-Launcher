//go:build darwin && !devmock

package install

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
	"typhon/internal/wine"
)

func TestLiveInnoOptionalActions(t *testing.T) {
	compilerInstaller := os.Getenv("TYPHON_INNO_COMPILER_INSTALLER")
	if os.Getenv("TYPHON_WINE_LIVE") == "" || compilerInstaller == "" {
		t.Skip("set TYPHON_WINE_LIVE and TYPHON_INNO_COMPILER_INSTALLER")
	}
	rt, err := wine.Detect()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	dest := filepath.Join(root, "Fixture")
	if err := os.MkdirAll(dest, 0700); err != nil {
		t.Fatal(err)
	}
	manager := wine.NewManager(rt)
	b, err := manager.Ensure(dest, root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := manager.Kill(b); err != nil {
			t.Error(err)
		}
		if e := manager.Remove(dest); e != nil {
			t.Error(e)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	win := func(p string) string {
		v, e := b.ToWindows(p)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	// The compiler installer is copied into the fixture's mapped drive.
	//nolint:gosec // G703: explicit opt-in local fixture path; writes stay in the isolated test directory.
	data, err := os.ReadFile(compilerInstaller)
	if err != nil {
		t.Fatal(err)
	}
	installer := filepath.Join(root, "inno.exe")
	//nolint:gosec // G703: copy the explicitly selected compiler fixture into the isolated test directory.
	if err := os.WriteFile(installer, data, 0700); err != nil {
		t.Fatal(err)
	}
	compiler := filepath.Join(root, "Compiler")
	run := func(path string, args ...string) {
		t.Helper()
		// Fixture setup also uses the bridge so inherited Wine service pipes
		// cannot masquerade as an installer failure after the loader exits.
		code, e := (wineRunner{detect: func() (wine.Runtime, error) { return rt, nil }}).doRun(ctx, b, wine.Cmd{Path: win(path), Args: args, InstallerGuard: true, HideProgress: true})
		if code != 0 || e != nil {
			t.Fatalf("%s: %d %v", path, code, e)
		}
	}
	run(installer, "/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART", "/DIR="+win(compiler))
	script := `[Setup]
AppName=Typhon Guard Fixture
AppVersion=1
DefaultDirName={tmp}\TyphonFixture
Uninstallable=no
CreateAppDir=no
OutputDir=.
OutputBaseFilename=fixture-setup
[Run]
Filename: "{cmd}"; Parameters: "/c ping -n 5 127.0.0.1 >nul & echo verified>""{src}\verified"""; Description: "Verify game files"; Flags: postinstall runhidden
Filename: "{cmd}"; Parameters: "/c echo opened>""{src}\website"""; Description: "Visit FitGirl website"; Flags: postinstall runhidden
Filename: "{cmd}"; Parameters: "/c echo applied>""{src}\redirect"""; Description: "Apply redirection to official FitGirl site"; Flags: postinstall runhidden
`
	scriptPath := filepath.Join(root, "fixture.iss")
	if err := os.WriteFile(scriptPath, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	run(filepath.Join(compiler, "ISCC.exe"), win(scriptPath))
	log := filepath.Join(root, "setup.log")
	code, err := (wineRunner{detect: func() (wine.Runtime, error) { return rt, nil }}).runPrepared(ctx, runSpec{Path: filepath.Join(root, "fixture-setup.exe"), Engine: EngineInno, Dir: root, Args: []string{"/VERYSILENT", "/SUPPRESSMSGBOXES", "/LOG=" + log}, LogPath: log, Hidden: true}, b)
	if data, e := os.ReadFile(log); e == nil {
		t.Log(string(data))
	}
	if code != 0 || err != nil {
		t.Fatalf("installer: %d %v", code, err)
	}
	if _, err := os.Stat(filepath.Join(root, "verified")); err != nil {
		t.Fatal("required verification did not run", err)
	}
	for _, name := range []string{"website", "redirect"} {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			t.Errorf("optional action ran: %s", name)
			if data, e := os.ReadFile(wineInstallerLog(log)); e == nil {
				t.Log(string(data))
			}
		}
	}
}
