package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func useFloor(t *testing.T, floor toolVersion) {
	t.Helper()
	prev := unrarFloor
	unrarFloor = floor
	t.Cleanup(func() { unrarFloor = prev })
}

func useToolSet(t *testing.T, set toolSet) {
	t.Helper()
	prev := lookupArchiveTools
	lookupArchiveTools = func(context.Context) toolSet { return set }
	t.Cleanup(func() { lookupArchiveTools = prev })
}

func TestNewUnrar(t *testing.T) {
	errRun := errors.New("cannot start")
	windows, unix := toolVersion{major: 7, minor: 13}, toolVersion{}
	cases := []struct {
		name       string
		floor      toolVersion
		banner     string
		err        error
		refused    bool
		wantSwitch bool
	}{
		{name: "windows 7.13", floor: windows, banner: unrarBanner713, wantSwitch: true},
		{name: "windows 7.14", floor: windows, banner: "UNRAR 7.14 x64 freeware", wantSwitch: true},
		{name: "windows 8.0", floor: windows, banner: "UNRAR 8.0 x64 freeware", wantSwitch: true},
		{name: "windows 7.12 has the first traversal bug fixed but not the second", floor: windows, banner: "UNRAR 7.12 x64 freeware", refused: true},
		{name: "windows 7.00", floor: windows, banner: "UNRAR 7.00 x64 freeware", refused: true},
		{name: "windows 6.24", floor: windows, banner: "UNRAR 6.24 x64 freeware", refused: true},
		{name: "windows unparseable", floor: windows, banner: "usage", refused: true},
		{name: "windows empty", floor: windows, refused: true},
		{name: "windows unrar-free", floor: windows, banner: "unrar-free 0.3.1", refused: true},
		{name: "windows integer overflow", floor: windows, banner: "UNRAR 99999999999999999999.1 freeware", refused: true},
		{name: "windows cannot be probed", floor: windows, err: errRun, refused: true},
		{name: "windows probe timed out", floor: windows, err: context.DeadlineExceeded, refused: true},
		{name: "unix 7.13", floor: unix, banner: unrarBanner713, wantSwitch: true},
		{name: "unix 7.00 is the first with -ol-", floor: unix, banner: "UNRAR 7.00 freeware", wantSwitch: true},
		{name: "unix 6.24 runs without -ol-", floor: unix, banner: "UNRAR 6.24 x64 freeware"},
		{name: "unix 5.50 runs without -ol-", floor: unix, banner: "UNRAR 5.50 freeware"},
		{name: "unix lower case and blank lines", floor: unix, banner: "\r\n\r\nunrar 7.13 x64", wantSwitch: true},
		{name: "unix unrar-free runs without -ol-", floor: unix, banner: "unrar-free 0.3.1"},
		{name: "unix unparseable runs without -ol-", floor: unix, banner: "usage"},
		{name: "unix cannot be probed", floor: unix, err: errRun},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			useFloor(t, tc.floor)
			useBanner(t, tc.banner, tc.err)
			tool, err := newUnrar(context.Background(), filepath.Join("tools", "unrar"))
			if tc.refused {
				if !errors.Is(err, errToolVersion) {
					t.Fatalf("err = %v, want errToolVersion", err)
				}
				if tc.err != nil && !errors.Is(err, tc.err) {
					t.Fatalf("err = %v, want the probe failure kept as the cause", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("newUnrar: %v", err)
			}
			if got := slices.Contains(tool.args("a.rar", "out"), "-ol-"); got != tc.wantSwitch {
				t.Fatalf("args = %q, -ol- present = %v, want %v", tool.args("a.rar", "out"), got, tc.wantSwitch)
			}
		})
	}
}

func TestNewSevenZip(t *testing.T) {
	cases := []struct {
		name    string
		banner  string
		err     error
		refused bool
	}{
		{name: "25.01 as printed on Windows", banner: sevenZipBanner2501},
		{name: "25.01 for Linux and macOS", banner: "\n7-Zip (z) 25.01 (arm64) : Copyright (c) 1999-2025 Igor Pavlov : 2025-08-03\n"},
		{name: "25.01 alpha marker", banner: "7-Zip (a) 25.01 (x64) : Copyright (c) 1999-2025 Igor Pavlov"},
		{name: "26.00", banner: "7-Zip 26.00 (x64) : Copyright (c) 1999-2026 Igor Pavlov"},
		{name: "25.00 predates the symlink hardening", banner: "7-Zip 25.00 (x64) : Copyright (c) 1999-2025 Igor Pavlov : 2025-07-05", refused: true},
		{name: "24.09", banner: "7-Zip 24.09 (x64) : Copyright (c) 1999-2024 Igor Pavlov : 2024-11-29", refused: true},
		{name: "16.04 with the bit width marker", banner: "7-Zip [64] 16.04 : Copyright (c) 1999-2016 Igor Pavlov : 2016-10-04", refused: true},
		{name: "9.20", banner: "7-Zip 9.20  Copyright (c) 1999-2010 Igor Pavlov  2010-11-18", refused: true},
		{name: "p7zip", banner: "p7zip Version 16.02 (locale=en_US.UTF-8,Utf16=on,HugeFiles=on,64 bits,8 CPUs x64)", refused: true},
		{name: "unparseable", banner: "usage", refused: true},
		{name: "empty", refused: true},
		{name: "cannot be probed", err: errors.New("cannot start"), refused: true},
		{name: "probe timed out", err: context.DeadlineExceeded, refused: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			useBanner(t, tc.banner, tc.err)
			tool, err := newSevenZip(context.Background(), filepath.Join("tools", "7z"))
			if tc.refused {
				if !errors.Is(err, errToolVersion) {
					t.Fatalf("err = %v, want errToolVersion", err)
				}
				if tc.err != nil && !errors.Is(err, tc.err) {
					t.Fatalf("err = %v, want the probe failure kept as the cause", err)
				}
				return
			}
			if err != nil || tool.name != "7-Zip" {
				t.Fatalf("newSevenZip = %v, %v", tool.name, err)
			}
		})
	}
}

func TestBannerProbeKillsAToolThatHangs(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(fakeToolEnv, "1")
	prev := toolBannerTimeout
	toolBannerTimeout = 200 * time.Millisecond
	t.Cleanup(func() { toolBannerTimeout = prev })
	done := make(chan error, 1)
	go func() {
		_, err := runBanner(context.Background(), exe, "-test.run=^TestFakeArchiveTool$", "--", "hang", "", "a", "b")
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err = %v, want context.DeadlineExceeded", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the probe did not return after its timeout: the tool was left running")
	}
}

func TestRefusedToolsDecideTheErrorWhenNothingIsLeft(t *testing.T) {
	refused := []error{refusal("UnRAR", "UnRAR.exe", errors.New("версия 7.12 ниже 7.13"))}
	t.Run("only refused tools", func(t *testing.T) {
		useToolSet(t, toolSet{refused: refused})
		err := ExtractArchive(context.Background(), brokenRar(t), filepath.Join(t.TempDir(), "out"), nil)
		if !errors.Is(err, errArchiveToolOutdated) || !errors.Is(err, errToolVersion) || errors.Is(err, errArchiveToolMissing) {
			t.Fatalf("err = %v, want errArchiveToolOutdated that names the refused tool", err)
		}
	})
	t.Run("no tools at all", func(t *testing.T) {
		useToolSet(t, toolSet{})
		err := ExtractArchive(context.Background(), brokenRar(t), filepath.Join(t.TempDir(), "out"), nil)
		if !errors.Is(err, errArchiveToolMissing) || errors.Is(err, errArchiveToolOutdated) {
			t.Fatalf("err = %v, want errArchiveToolMissing", err)
		}
	})
	t.Run("a refused tool does not stop a usable one", func(t *testing.T) {
		useToolSet(t, toolSet{tools: []archiveTool{fakeTool(t, "ok")}, refused: refused})
		dest := filepath.Join(t.TempDir(), "out")
		if err := ExtractArchive(context.Background(), brokenRar(t), dest, nil); err != nil {
			t.Fatalf("ExtractArchive: %v", err)
		}
	})
	t.Run("a refusal stays in the final error when the usable tool fails too", func(t *testing.T) {
		useToolSet(t, toolSet{tools: []archiveTool{fakeTool(t, "fail")}, refused: refused})
		err := ExtractArchive(context.Background(), brokenRar(t), filepath.Join(t.TempDir(), "out"), nil)
		if !errors.Is(err, errArchiveToolFailed) || !errors.Is(err, errToolVersion) {
			t.Fatalf("err = %v, want errArchiveToolFailed that still carries the refusal", err)
		}
	})
}
