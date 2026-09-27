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

// A synthetic Win32 installer, never a game or the user's release state.
func TestLiveInstallerGuardFixture(t *testing.T) {
	fixture := os.Getenv("TYPHON_GUARD_FIXTURE_EXE")
	if os.Getenv("TYPHON_WINE_LIVE") == "" || fixture == "" {
		t.Skip("set TYPHON_WINE_LIVE and TYPHON_GUARD_FIXTURE_EXE")
	}
	rt, err := wine.Detect()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	dest := filepath.Join(root, "GuardFixture")
	if err = os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // G703: explicit opt-in local fixture path; writes stay in the isolated test directory.
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(root, "fixture.exe")
	//nolint:gosec // G703: explicit opt-in local fixture path; writes stay in the isolated test directory.
	if err = os.WriteFile(exe, data, 0o755); err != nil {
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
		if err := manager.Remove(dest); err != nil {
			t.Error(err)
		}
	})
	t.Logf("isolated bottle: %s", b.Name)
	path, err := b.ToWindows(exe)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	for _, external := range []bool{false, true} {
		report := filepath.Join(root, "fixture-report.txt")
		winReport, err := b.ToWindows(report)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(report); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		pattern := "^(TestGuardControls|TestRunJobCancelsDescendantAfterLoaderExit|TestGuardVerifier|TestRunJobRejectsOversizedProbe|TestRunJobWaitsForNormalDescendantCompletion)$"
		if external {
			pattern = "^TestGuardControls$"
		}
		args := []string{"-test.v", "-test.run=" + pattern, "-guard.report=" + winReport}
		if fixture32 := os.Getenv("TYPHON_GUARD_X86_FIXTURE_EXE"); fixture32 != "" {
			//nolint:gosec // G703: explicit opt-in local fixture path; writes stay in the isolated test directory.
			data, err := os.ReadFile(fixture32)
			if err != nil {
				t.Fatal(err)
			}
			x86 := filepath.Join(root, "fixture32.exe")
			//nolint:gosec // G703: explicit opt-in local fixture path; writes stay in the isolated test directory.
			if err = os.WriteFile(x86, data, 0700); err != nil {
				t.Fatal(err)
			}
			win, err := b.ToWindows(x86)
			if err != nil {
				t.Fatal(err)
			}
			args = append(args, "-job.x86="+win)
		}

		if external {
			args = append(args, "-guard.external=true")
		}
		log := filepath.Join(root, "fixture.wine.log")
		code, err := (wineRunner{detect: func() (wine.Runtime, error) { return rt, nil }}).doRun(ctx, b, wine.Cmd{Path: path, Args: args, Log: log, DebugMessages: "-all,err+all", WaitChildren: true, InstallerGuard: external, HideProgress: true})
		if err != nil || code != 0 {
			if reportData, e := os.ReadFile(report); e == nil {
				t.Log(string(reportData))
			}
			if data, e := os.ReadFile(log); e == nil {
				if len(data) > 6000 {
					data = data[len(data)-6000:]
				}
				t.Log(string(data))
			}
			t.Fatalf("external=%v: code %d, error %v", external, code, err)
		}
	}
}
