//go:build windows

package shortcut

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type linkInfo struct {
	target, args, desc, id string
}

//nolint:gosec // G103: addresses of live COM objects and locals for the duration of each call. G115: HRESULT is the 32-bit value in r0.
func inspectLink(t *testing.T, path string) linkInfo {
	t.Helper()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := coInitialize(); err != nil {
		t.Fatal(err)
	}
	defer windows.CoUninitialize()
	sl, err := createShellLink()
	if err != nil {
		t.Fatal(err)
	}
	defer sl.release()
	pf, err := sl.queryPersistFile()
	if err != nil {
		t.Fatal(err)
	}
	defer pf.release()
	if err := pf.load(path); err != nil {
		t.Fatal(err)
	}
	var info linkInfo
	if info.target, err = sl.getPath(); err != nil {
		t.Fatal(err)
	}
	if info.args, err = sl.getArguments(); err != nil {
		t.Fatal(err)
	}
	buf := make([]uint16, 1024)
	if hr := int32(callMethod(sl.vtbl.GetDescription, uintptr(unsafe.Pointer(sl)), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))); hr < 0 {
		t.Fatalf("GetDescription: hresult 0x%08X", uint32(hr))
	}
	info.desc = windows.UTF16ToString(buf)
	ps, err := sl.queryPropertyStore()
	if err != nil {
		t.Fatal(err)
	}
	defer ps.release()
	if info.id, err = ps.getString(&pkeyAppUserModelID); err != nil {
		t.Fatal(err)
	}
	return info
}

func selfExe(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return exe
}

func makeLink(t *testing.T, dir, name, target, args string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := Create(path, Link{Target: target, Args: args, WorkDir: dir, Description: "desc " + name}); err != nil {
		t.Fatal(err)
	}
	return path
}

func snapshot(t *testing.T, path string) ([]byte, time.Time) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return data, st.ModTime()
}

func TestRefreshLauncherLinksSetsIDKeepsRest(t *testing.T) {
	exe := selfExe(t)
	dir := t.TempDir()
	path := makeLink(t, dir, "Typhon.lnk", exe, "--minimized")

	if err := RefreshLauncherLinks(context.Background(), []LinkSpot{{Dir: dir, Name: "Typhon.lnk"}}, exe, LauncherAppID); err != nil {
		t.Fatal(err)
	}
	got := inspectLink(t, path)
	same, err := sameFile(got.target, exe)
	if err != nil {
		t.Fatal(err)
	}
	if got.id != LauncherAppID || got.args != "--minimized" || got.desc != "desc Typhon.lnk" || !same {
		t.Fatalf("link = %+v", got)
	}
}

func TestRefreshLauncherLinksScansWholeFolder(t *testing.T) {
	exe := selfExe(t)
	dir := t.TempDir()
	pinned := makeLink(t, dir, "Anything.lnk", exe, "")
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RefreshLauncherLinks(context.Background(), []LinkSpot{{Dir: dir}}, exe, LauncherAppID); err != nil {
		t.Fatal(err)
	}
	if got := inspectLink(t, pinned); got.id != LauncherAppID {
		t.Fatalf("id = %q", got.id)
	}
}

func TestRefreshLauncherLinksSkipsOtherTargetAndGameLinks(t *testing.T) {
	exe := selfExe(t)
	dir := t.TempDir()
	other := filepath.Join(dir, "other.exe")
	if err := os.WriteFile(other, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	foreign := makeLink(t, dir, "Foreign.lnk", other, "")
	game := makeLink(t, dir, "Game.lnk", exe, "--play abc")
	beforeForeign, mtimeForeign := snapshot(t, foreign)
	beforeGame, mtimeGame := snapshot(t, game)

	if err := RefreshLauncherLinks(context.Background(), []LinkSpot{{Dir: dir}}, exe, LauncherAppID); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path   string
		data   []byte
		mtime  time.Time
		reason string
	}{{foreign, beforeForeign, mtimeForeign, "other target"}, {game, beforeGame, mtimeGame, "--play link"}} {
		data, mtime := snapshot(t, tc.path)
		if !bytes.Equal(data, tc.data) || !mtime.Equal(tc.mtime) {
			t.Fatalf("%s was rewritten", tc.reason)
		}
	}
}

func TestRefreshLauncherLinksSkipsLinkWithRightID(t *testing.T) {
	exe := selfExe(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "Typhon.lnk")
	if err := Create(path, Link{Target: exe, AppUserModelID: LauncherAppID}); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	before, mtime := snapshot(t, path)

	if err := RefreshLauncherLinks(context.Background(), []LinkSpot{{Dir: dir, Name: "Typhon.lnk"}}, exe, LauncherAppID); err != nil {
		t.Fatal(err)
	}
	after, mtimeAfter := snapshot(t, path)
	if !bytes.Equal(before, after) || !mtime.Equal(mtimeAfter) {
		t.Fatal("link with the right id was rewritten")
	}
}

func TestRefreshLauncherLinksMissingFolderAndFile(t *testing.T) {
	exe := selfExe(t)
	missing := filepath.Join(t.TempDir(), "nope")
	spots := []LinkSpot{{Dir: missing}, {Dir: missing, Name: "Typhon.lnk"}}
	if err := RefreshLauncherLinks(context.Background(), spots, exe, LauncherAppID); err != nil {
		t.Fatalf("missing folder: %v", err)
	}
}

func TestRefreshLauncherLinksCancelledContext(t *testing.T) {
	exe := selfExe(t)
	dir := t.TempDir()
	path := makeLink(t, dir, "Typhon.lnk", exe, "")
	before, _ := snapshot(t, path)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := RefreshLauncherLinks(ctx, []LinkSpot{{Dir: dir, Name: "Typhon.lnk"}}, exe, LauncherAppID)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want canceled", err)
	}
	after, _ := snapshot(t, path)
	if !bytes.Equal(before, after) {
		t.Fatal("link written after cancel")
	}
}

func TestRefreshLauncherLinksRejectsBadInput(t *testing.T) {
	if err := RefreshLauncherLinks(context.Background(), nil, "", LauncherAppID); err == nil {
		t.Fatal("empty exe accepted")
	}
	if err := RefreshLauncherLinks(context.Background(), nil, selfExe(t), "bad id"); !errors.Is(err, ErrAppIDInvalid) {
		t.Fatalf("err = %v", err)
	}
}

func TestLauncherLinkSpots(t *testing.T) {
	spots, err := LauncherLinkSpots()
	if err != nil {
		t.Fatal(err)
	}
	if len(spots) != 3 {
		t.Fatalf("spots = %v", spots)
	}
	for _, s := range spots {
		if s.Dir == "" {
			t.Fatalf("empty dir in %v", spots)
		}
	}
}

func isPlay(args string) bool { return IsPlayArgs(args) }

//nolint:gosec // G103: addresses of live COM objects and locals for the duration of each call. G115: HRESULT is the 32-bit value in r0.
func withLink(t *testing.T, path string, write bool, fn func(sl *shellLink)) {
	t.Helper()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := coInitialize(); err != nil {
		t.Fatal(err)
	}
	defer windows.CoUninitialize()
	sl, err := createShellLink()
	if err != nil {
		t.Fatal(err)
	}
	defer sl.release()
	pf, err := sl.queryPersistFile()
	if err != nil {
		t.Fatal(err)
	}
	defer pf.release()
	if err := pf.load(path); err != nil {
		t.Fatal(err)
	}
	fn(sl)
	if write {
		if err := pf.save(path); err != nil {
			t.Fatal(err)
		}
	}
}

//nolint:gosec // G103: addresses of live COM objects and locals for the duration of each call. G115: HRESULT is the 32-bit value in r0.
func iconAndHotkey(t *testing.T, path string) (string, uint16) {
	t.Helper()
	var icon string
	var hotkey uint16
	withLink(t, path, false, func(sl *shellLink) {
		buf := make([]uint16, 1024)
		var idx int32
		if hr := int32(callMethod(sl.vtbl.GetIconLocation, uintptr(unsafe.Pointer(sl)), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(unsafe.Pointer(&idx)))); hr < 0 {
			t.Fatalf("GetIconLocation: 0x%08X", uint32(hr))
		}
		icon = windows.UTF16ToString(buf)
		if hr := int32(callMethod(sl.vtbl.GetHotkey, uintptr(unsafe.Pointer(sl)), uintptr(unsafe.Pointer(&hotkey)))); hr < 0 {
			t.Fatalf("GetHotkey: 0x%08X", uint32(hr))
		}
	})
	return icon, hotkey
}

//nolint:gosec // G103: receiver pointer. G115: HRESULT is the 32-bit value in r0.
func TestSetLinkAppIDKeepsIconAndHotkey(t *testing.T) {
	exe := selfExe(t)
	dir := t.TempDir()
	iconSrc := filepath.Join(dir, "game.exe")
	if err := os.WriteFile(iconSrc, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "Game.lnk")
	if err := Create(path, Link{Target: exe, Args: "--play abc", Icon: iconSrc}); err != nil {
		t.Fatal(err)
	}
	const hotkey = 0x0641
	withLink(t, path, true, func(sl *shellLink) {
		if hr := int32(callMethod(sl.vtbl.SetHotkey, uintptr(unsafe.Pointer(sl)), hotkey)); hr < 0 {
			t.Fatalf("SetHotkey: 0x%08X", uint32(hr))
		}
	})
	iconBefore, hotkeyBefore := iconAndHotkey(t, path)
	if hotkeyBefore != hotkey || iconBefore == "" {
		t.Fatalf("setup: icon %q hotkey %#x", iconBefore, hotkeyBefore)
	}

	if err := SetLinkAppID(path, exe, "Typhon.Game.abc", isPlay); err != nil {
		t.Fatal(err)
	}
	iconAfter, hotkeyAfter := iconAndHotkey(t, path)
	if iconAfter != iconBefore || hotkeyAfter != hotkey {
		t.Fatalf("icon %q->%q hotkey %#x->%#x", iconBefore, iconAfter, hotkeyBefore, hotkeyAfter)
	}
	if got := inspectLink(t, path); got.id != "Typhon.Game.abc" || got.args != "--play abc" {
		t.Fatalf("link = %+v", got)
	}
}

func TestSetLinkAppIDSkipsRightIDAndMissingFile(t *testing.T) {
	exe := selfExe(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "Game.lnk")
	if err := Create(path, Link{Target: exe, Args: "--play abc", AppUserModelID: "Typhon.Game.abc"}); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	before, mtime := snapshot(t, path)
	if err := SetLinkAppID(path, exe, "Typhon.Game.abc", isPlay); err != nil {
		t.Fatal(err)
	}
	after, mtimeAfter := snapshot(t, path)
	if !bytes.Equal(before, after) || !mtime.Equal(mtimeAfter) {
		t.Fatal("link with the right id was rewritten")
	}

	gone := filepath.Join(dir, "Gone.lnk")
	if err := SetLinkAppID(gone, exe, "Typhon.Game.abc", isPlay); err != nil {
		t.Fatalf("missing file: %v", err)
	}
	if _, err := os.Stat(gone); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing link was created: %v", err)
	}
}

func TestSetLinkAppIDRespectsTargetAndMatch(t *testing.T) {
	exe := selfExe(t)
	dir := t.TempDir()
	other := filepath.Join(dir, "other.exe")
	if err := os.WriteFile(other, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	foreign := makeLink(t, dir, "Foreign.lnk", other, "--play abc")
	launcher := makeLink(t, dir, "Launcher.lnk", exe, "")
	fb, _ := snapshot(t, foreign)
	lb, _ := snapshot(t, launcher)
	if err := SetLinkAppID(foreign, exe, "Typhon.Game.abc", isPlay); err != nil {
		t.Fatal(err)
	}
	if err := SetLinkAppID(launcher, exe, "Typhon.Game.abc", isPlay); err != nil {
		t.Fatal(err)
	}
	fa, _ := snapshot(t, foreign)
	la, _ := snapshot(t, launcher)
	if !bytes.Equal(fb, fa) || !bytes.Equal(lb, la) {
		t.Fatal("link outside target or match was rewritten")
	}
	if err := SetLinkAppID(launcher, "", "Typhon.Game.abc", isPlay); err == nil {
		t.Fatal("empty exe accepted")
	}
	if err := SetLinkAppID(launcher, exe, "bad id", isPlay); !errors.Is(err, ErrAppIDInvalid) {
		t.Fatalf("err = %v", err)
	}
}

func shortPath(t *testing.T, path string) string {
	t.Helper()
	from, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]uint16, maxLinkPath)
	n, err := windows.GetShortPathName(from, &buf[0], maxLinkPath)
	if err != nil {
		t.Fatal(err)
	}
	return windows.UTF16ToString(buf[:n])
}

func TestSetLinkAppIDMatchesLauncherPathAliases(t *testing.T) {
	exe := selfExe(t)
	aliases := map[string]string{
		"extended": `\\?\` + exe,
		"short":    shortPath(t, exe),
	}
	for name, alias := range aliases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := makeLink(t, dir, "Game.lnk", exe, "--play abc")
			if err := SetLinkAppID(path, alias, "Typhon.Game.abc", isPlay); err != nil {
				t.Fatal(err)
			}
			if got := inspectLink(t, path); got.id != "Typhon.Game.abc" {
				t.Fatalf("alias %q: link = %+v", alias, got)
			}
		})
	}
}

func TestSetLinkAppIDSkipsMissingTarget(t *testing.T) {
	exe := selfExe(t)
	dir := t.TempDir()
	gone := filepath.Join(dir, "gone.exe")
	if err := os.WriteFile(gone, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := makeLink(t, dir, "Gone.lnk", gone, "--play abc")
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}
	before, _ := snapshot(t, path)
	if err := SetLinkAppID(path, exe, "Typhon.Game.abc", isPlay); err != nil {
		t.Fatal(err)
	}
	after, _ := snapshot(t, path)
	if !bytes.Equal(before, after) {
		t.Fatal("link to a missing target was rewritten")
	}
}

func TestPortableLauncherLeavesLinksAlone(t *testing.T) {
	exe := selfExe(t)
	dir := t.TempDir()
	path := makeLink(t, dir, "Typhon.lnk", exe, "--minimized")
	before, _ := snapshot(t, path)

	prev := portableMode
	portableMode = func() bool { return true }
	t.Cleanup(func() { portableMode = prev })

	if err := RefreshLauncherLinks(context.Background(), []LinkSpot{{Dir: dir, Name: "Typhon.lnk"}}, exe, LauncherAppID); err != nil {
		t.Fatal(err)
	}
	after, _ := snapshot(t, path)
	if !bytes.Equal(before, after) {
		t.Fatal("a portable launcher rewrote a link")
	}
	if got := inspectLink(t, path); got.id != "" {
		t.Fatalf("link id = %q, want none", got.id)
	}

	spots, err := LauncherLinkSpots()
	if err != nil || len(spots) != 0 {
		t.Fatalf("LauncherLinkSpots() = %v, %v; want none in portable mode", spots, err)
	}
}
