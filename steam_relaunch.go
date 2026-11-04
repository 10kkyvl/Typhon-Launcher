package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"typhon/internal/platform"
)

// steamRelaunch hands a game started from the Mac Steam over to a launcher
// that Steam does not own. Steam counts the program it started as the running
// game: a launcher that stayed up as that program kept the entry "running"
// long after the game was closed, so Play did nothing until the launcher quit.
// The copy started through LaunchServices forwards the request to a running
// launcher or becomes the launcher itself, exactly like a click on the app, and
// this process ends at once, as it does when it only forwards the request.
type steamRelaunch struct {
	goos    string
	environ []string
	exe     func() (string, error)
	run     func(env []string, name string, args ...string) error
}

// steamHandOffMark is set on the copy started by handOver. open passes its
// own environment to the app it starts, so the copy would see Steam's markers
// again if they ever slipped through; the mark stops a chain of copies.
const steamHandOffMark = "TYPHON_STEAM_HANDOFF=1"

func systemSteamRelaunch(goos string) steamRelaunch {
	return steamRelaunch{
		goos:    goos,
		environ: os.Environ(),
		exe:     os.Executable,
		run: func(env []string, name string, args ...string) error {
			//nolint:gosec // G204: name is /usr/bin/open, args are the launcher's own bundle and the checked play request
			cmd := exec.Command(name, args...)
			cmd.Env = env
			return cmd.Run()
		},
	}
}

// handOver reports whether another launcher took the request. A failure keeps
// the request here: the game still starts, only Steam keeps the entry open.
func (r steamRelaunch) handOver(args []string) (bool, error) {
	if r.goos != "darwin" || !platform.LaunchedBySteam(r.environ) || slices.Contains(r.environ, steamHandOffMark) {
		return false, nil
	}
	// A malformed request stays here: the regular start reports it.
	_, requested, parseErr := playTarget(args)
	if playable := requested && parseErr == nil; !playable {
		return false, nil
	}
	exe, err := r.exe()
	if err != nil {
		return false, fmt.Errorf("launcher path: %w", err)
	}
	bundle, ok := appBundle(exe)
	if !ok {
		return false, nil
	}
	// -n: this process belongs to the same bundle, and open without it would
	// only bring "the running copy", which is about to exit, to the front.
	openArgs := append([]string{"-n", "-a", bundle, "--args"}, args[1:]...)
	env := append(platform.WithoutSteamLaunchEnv(r.environ), steamHandOffMark)
	if err := r.run(env, "/usr/bin/open", openArgs...); err != nil {
		return false, fmt.Errorf("start the launcher outside Steam: %w", err)
	}
	return true, nil
}

// appBundle finds X.app of an executable at X.app/Contents/MacOS/<name>.
func appBundle(exe string) (string, bool) {
	macOS := filepath.Dir(exe)
	contents := filepath.Dir(macOS)
	bundle := filepath.Dir(contents)
	if filepath.Base(macOS) != "MacOS" || filepath.Base(contents) != "Contents" || !strings.HasSuffix(bundle, ".app") {
		return "", false
	}
	return bundle, true
}
