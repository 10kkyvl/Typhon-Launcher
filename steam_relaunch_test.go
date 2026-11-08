package main

import (
	"errors"
	"slices"
	"testing"
)

const bundleExe = "/Applications/Typhon.app/Contents/MacOS/typhon"

var fromSteam = []string{"HOME=/Users/u", "SteamClientLaunch=1", "SteamAppId=3205976898"}

var lastEnv []string

func relaunchWith(goos string, environ []string, exe string, runErr error) (steamRelaunch, *[][]string) {
	var calls [][]string
	return steamRelaunch{
		goos:    goos,
		environ: environ,
		exe:     func() (string, error) { return exe, nil },
		run: func(env []string, name string, args ...string) error {
			lastEnv = env
			calls = append(calls, append([]string{name}, args...))
			return runErr
		},
	}, &calls
}

func TestSteamRelaunchHandsAPlayRequestToLaunchServices(t *testing.T) {
	r, calls := relaunchWith("darwin", fromSteam, bundleExe, nil)
	args := []string{bundleExe, "--play", "abc", "--profile", "mods"}

	took, err := r.handOver(args)
	if err != nil || !took {
		t.Fatalf("handOver = %v, %v; want the request handed over", took, err)
	}
	want := []string{"/usr/bin/open", "-n", "-a", "/Applications/Typhon.app", "--args", "--play", "abc", "--profile", "mods"}
	if len(*calls) != 1 || !slices.Equal((*calls)[0], want) {
		t.Fatalf("ran %v, want %v", *calls, want)
	}
	// open hands its environment to the app: the copy must not look started
	// by Steam, or it would hand the request over again, and again.
	if want := []string{"HOME=/Users/u", steamHandOffMark}; !slices.Equal(lastEnv, want) {
		t.Fatalf("env of the copy = %v, want %v", lastEnv, want)
	}
	copyOf, copyCalls := relaunchWith("darwin", lastEnv, bundleExe, nil)
	if took, err := copyOf.handOver(args); took || err != nil || len(*copyCalls) != 0 {
		t.Fatalf("the copy handed over again: %v, %v, %v", took, err, *copyCalls)
	}
}

// Even with Steam's markers back in its environment, a copy never starts
// another copy.
func TestSteamRelaunchNeverChains(t *testing.T) {
	r, calls := relaunchWith("darwin", append(slices.Clone(fromSteam), steamHandOffMark), bundleExe, nil)
	if took, err := r.handOver([]string{bundleExe, "--play", "abc"}); took || err != nil || len(*calls) != 0 {
		t.Fatalf("handOver = %v, %v, ran %v; want nothing", took, err, *calls)
	}
}

func TestSteamRelaunchKeepsTheRequestWhenOpenFails(t *testing.T) {
	r, _ := relaunchWith("darwin", fromSteam, bundleExe, errors.New("boom"))
	took, err := r.handOver([]string{bundleExe, "--play", "abc"})
	if took || err == nil {
		t.Fatalf("handOver = %v, %v; want the request kept and the failure reported", took, err)
	}
}

func TestSteamRelaunchLeavesOtherStartsAlone(t *testing.T) {
	cases := []struct {
		name    string
		goos    string
		environ []string
		exe     string
		args    []string
	}{
		{"windows", "windows", fromSteam, bundleExe, []string{bundleExe, "--play", "abc"}},
		{"not from steam", "darwin", []string{"HOME=/Users/u"}, bundleExe, []string{bundleExe, "--play", "abc"}},
		{"no play request", "darwin", fromSteam, bundleExe, []string{bundleExe}},
		{"bad play request", "darwin", fromSteam, bundleExe, []string{bundleExe, "--play"}},
		{"not in a bundle", "darwin", fromSteam, "/Users/u/bin/typhon", []string{"/Users/u/bin/typhon", "--play", "abc"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, calls := relaunchWith(tc.goos, tc.environ, tc.exe, nil)
			took, err := r.handOver(tc.args)
			if took || err != nil || len(*calls) != 0 {
				t.Fatalf("handOver = %v, %v, ran %v; want nothing", took, err, *calls)
			}
		})
	}
}
