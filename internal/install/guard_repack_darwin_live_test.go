//go:build darwin && !devmock

package install

import (
	"context"
	//nolint:gosec // G401/G501: verify the repack-provided MD5 file list, not a security signature.
	"crypto/md5"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"typhon/internal/wine"
)

// Explicit opt-in, isolated destination/bottle, existing installer bytes only.
func TestLiveRepackGuardCancellation(t *testing.T) {
	runRepackLive(t, false)
}

func TestLiveRepackFullInstall(t *testing.T) {
	if os.Getenv("TYPHON_GUARD_FULL_INSTALL") != "1" {
		t.Skip("set TYPHON_GUARD_FULL_INSTALL=1")
	}
	runRepackLive(t, true)
}

func runRepackLive(t *testing.T, full bool) {
	installer := os.Getenv("TYPHON_GUARD_REAL_INSTALLER")
	if os.Getenv("TYPHON_WINE_LIVE") == "" || installer == "" {
		t.Skip("set TYPHON_WINE_LIVE and TYPHON_GUARD_REAL_INSTALLER")
	}
	rt, err := wine.Detect()
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp("", "typhon-repack-guard-")
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "Source")
	dest := filepath.Join(root, "Target")
	for _, p := range []string{source, dest} {
		if err = os.MkdirAll(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(filepath.Dir(installer))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if err = os.Symlink(filepath.Join(filepath.Dir(installer), entry.Name()), filepath.Join(source, entry.Name())); err != nil {
			t.Fatal(err)
		}
	}
	manager := wine.NewManager(rt)
	b, err := manager.Ensure(dest, root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := manager.Kill(b); err != nil {
			t.Logf("bottle cleanup stop returned: %v", err)
		}
		if err := manager.Remove(dest); err != nil {
			t.Error(err)
			return
		}
		if err := os.RemoveAll(root); err != nil {
			t.Error(err)
		}
	})
	t.Logf("isolated bottle %s, root %s", b.Name, root)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	path, err := b.ToWindows(filepath.Join(source, filepath.Base(installer)))
	if err != nil {
		t.Fatal(err)
	}
	winDest, err := b.ToWindows(dest)
	if err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(root, "setup.log")
	winLog, err := b.ToWindows(log)
	if err != nil {
		t.Fatal(err)
	}
	inf := filepath.Join(root, "probe.inf")
	winInf, err := b.ToWindows(inf)
	if err != nil {
		t.Fatal(err)
	}
	if full {
		duration := 10 * time.Minute
		if os.Getenv("TYPHON_GUARD_VISIBLE_INSTALL") == "1" {
			duration = 90 * time.Second
		}
		fullCtx, fullCancel := context.WithTimeout(ctx, duration)
		defer fullCancel()
		opts := installOptions{SkipExtras: true, SkipShortcuts: true}
		nativeInstaller := filepath.Join(source, filepath.Base(installer))
		runner := wineRunner{detect: func() (wine.Runtime, error) { return rt, nil }}
		t.Log("starting component discovery")
		outcome, e := discoverWithBottle(fullCtx, discoverySpec{Engine: EngineInno, InstallerPath: nativeInstaller, Destination: dest, WorkingDir: source, InfPath: inf, Options: opts}, b, runner.doRun)
		if e != nil {
			t.Fatal(e)
		}
		t.Logf("discovery complete, components=%v reason=%s", outcome.components, outcome.reason)
		plan, e := silentArgs(EngineInno, nativeInstaller, dest, log, opts)
		if e != nil {
			t.Fatal(e)
		}
		plan = planWithComponents(plan, outcome.components)
		hidden := true
		if os.Getenv("TYPHON_GUARD_VISIBLE_INSTALL") == "1" {
			hidden = false
			for i, a := range plan.Args {
				if a == "/VERYSILENT" {
					plan.Args[i] = "/SILENT"
				}
			}
		}
		code, e := runner.runPrepared(fullCtx, runSpec{Path: nativeInstaller, Engine: EngineInno, Destination: dest, Dir: source, Args: plan.Args, LogPath: log, Hidden: hidden}, b)
		if e != nil || code != 0 {
			for _, name := range []string{log, wineInstallerLog(log)} {
				if data, readErr := os.ReadFile(name); readErr == nil {
					if len(data) > 12000 {
						data = data[len(data)-12000:]
					}
					t.Logf("%s: %s", name, data)
				}
			}
			t.Fatalf("main installation: code=%d error=%v", code, e)
		}
		files, e := FindExecutables(fullCtx, dest, "9-Bit Armies")
		if e != nil {
			t.Fatal(e)
		}
		if len(files) == 0 {
			t.Fatal("no game executable installed")
		}
		t.Logf("installed game executables: %v", files)
		manifest := filepath.Join(dest, "_Redist", "fitgirl.md5")
		data, e := os.ReadFile(manifest)
		if e != nil {
			t.Fatal(e)
		}
		checked := 0
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, ";") {
				continue
			}
			fields := strings.SplitN(line, " ", 2)
			if len(fields) != 2 {
				t.Fatalf("invalid checksum line %q", line)
			}
			name := strings.ReplaceAll(strings.TrimLeft(fields[1], " *"), `\`, "/")
			path := filepath.Join(filepath.Dir(manifest), name)
			rel, e := filepath.Rel(dest, path)
			if e != nil || rel == ".." || strings.HasPrefix(rel, "../") {
				t.Fatal("checksum path outside fixture")
			}
			//nolint:gosec // G703: explicit opt-in local fixture path; writes stay in the isolated test directory.
			f, e := os.Open(path)
			if e != nil {
				t.Fatal(e)
			}
			//nolint:gosec // G401/G501: verify the repack-provided MD5 file list, not a security signature.
			h := md5.New()
			_, e = io.Copy(h, f)
			//nolint:errcheck // read-only in-memory/file reader cleanup cannot affect the result.
			if err := f.Close(); err != nil {
				t.Error(err)
			}
			if e != nil {
				t.Fatal(e)
			}
			if hex.EncodeToString(h.Sum(nil)) != strings.ToLower(fields[0]) {
				t.Fatalf("checksum mismatch: %s", rel)
			}
			checked++
		}
		if checked == 0 {
			t.Fatal("empty checksum manifest")
		}
		t.Logf("verified %d installed files against repack MD5", checked)

		return
	}
	// Let the real repack build its UI/music controls, then exercise cancellation.
	timer := time.AfterFunc(20*time.Second, cancel)
	defer timer.Stop()
	started := time.Now()
	code, err := (wineRunner{detect: func() (wine.Runtime, error) { return rt, nil }}).doRun(ctx, b, wine.Cmd{Path: path, Args: []string{"/SP-", "/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART", "/DIR=" + winDest, "/SAVEINF=" + winInf, "/LOG=" + winLog}, InstallerGuard: true, HideProgress: true, Limit32BitAddressSpace: isFitGirlInstaller(EngineInno, installer), WaitChildren: true, DLLOverrides: wineInstallerDLLOverrides(EngineInno, installer)})
	t.Logf("run duration=%s code=%d error=%v", time.Since(started), code, err)
	if data, e := os.ReadFile(log); e == nil {
		lines := strings.Split(string(data), "\n")
		if len(lines) > 25 {
			lines = lines[len(lines)-25:]
		}
		t.Log(strings.Join(lines, "\n"))
	}
	if !errors.Is(err, context.Canceled) || errors.Is(err, wine.ErrTreeNotStopped) {
		t.Fatalf("cancellation unconfirmed: code=%d error=%v", code, err)
	}
	if time.Since(started) > 26*time.Second {
		t.Fatal("cancellation took too long")
	}
}
