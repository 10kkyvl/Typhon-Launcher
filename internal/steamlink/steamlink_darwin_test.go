//go:build darwin && !devmock

package steamlink

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestSupportedOnMac(t *testing.T) {
	if !Supported() {
		t.Fatal("Supported() = false on macOS")
	}
}

func TestRootIsApplicationSupportSteam(t *testing.T) {
	home := t.TempDir()
	if _, err := rootIn(home); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("no Steam folder: err = %v, want ErrNotInstalled", err)
	}
	want := filepath.Join(home, "Library", "Application Support", "Steam")
	if err := os.MkdirAll(want, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := rootIn(home)
	if err != nil || got != want {
		t.Fatalf("rootIn = %q, %v; want %q", got, err, want)
	}
}

func TestRootRefusesAFileInPlaceOfTheFolder(t *testing.T) {
	home := t.TempDir()
	support := filepath.Join(home, "Library", "Application Support")
	if err := os.MkdirAll(support, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(support, "Steam"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := rootIn(home); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("err = %v, want ErrNotInstalled", err)
	}
}

func kinfo(pid int32, name string, uid uint32, stat int8) unix.KinfoProc {
	var k unix.KinfoProc
	k.Proc.P_pid = pid
	k.Proc.P_stat = stat
	k.Eproc.Ucred.Uid = uid
	copy(k.Proc.P_comm[:], name)
	return k
}

func TestSteamAmongCountsOnlyALiveClientOfThisUser(t *testing.T) {
	const me, other = 501, 502
	const run = 2 // SRUN
	cases := []struct {
		name  string
		procs []unix.KinfoProc
		want  bool
	}{
		{"none", []unix.KinfoProc{kinfo(10, "Finder", me, run)}, false},
		{"own client", []unix.KinfoProc{kinfo(10, "Finder", me, run), kinfo(11, steamProcess, me, run)}, true},
		{"helper only", []unix.KinfoProc{kinfo(11, "Steam Helper", me, run)}, false},
		{"another user", []unix.KinfoProc{kinfo(11, steamProcess, other, run)}, false},
		{"zombie", []unix.KinfoProc{kinfo(11, steamProcess, me, sZomb)}, false},
		{"similar name", []unix.KinfoProc{kinfo(11, steamProcess+"2", me, run)}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := steamAmong(tc.procs, me); got != tc.want {
				t.Fatalf("steamAmong = %v, want %v", got, tc.want)
			}
		})
	}
}

// The real process table names a process after its executable, which is how
// the client is found; a copy of sleep under the client's name stands in.
func TestSteamRunningSeesTheClientInTheProcessTable(t *testing.T) {
	fake := filepath.Join(t.TempDir(), steamProcess)
	copyExecutable(t, "/bin/sleep", fake)
	cmd := exec.Command(fake, "30") //nolint:gosec // G204: the test's own copy of /bin/sleep
	if err := cmd.Start(); err != nil {
		t.Fatalf("start stand-in: %v", err)
	}
	t.Cleanup(func() {
		if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			t.Errorf("stop stand-in: %v", err)
		}
		var exit *exec.ExitError
		if err := cmd.Wait(); err != nil && !errors.As(err, &exit) {
			t.Errorf("wait for stand-in: %v", err)
		}
	})
	deadline := time.After(5 * time.Second)
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		running, err := steamRunning(context.Background())
		if err != nil {
			t.Fatalf("steamRunning: %v", err)
		}
		if running {
			return
		}
		select {
		case <-deadline:
			t.Fatal("steamRunning = false while a steam_osx process runs")
		case <-tick.C:
		}
	}
}

func TestSteamRunningHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := steamRunning(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func copyExecutable(t *testing.T, from, to string) {
	t.Helper()
	data, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // G306: the copy is a program the test runs, it needs its execute bit
	if err := os.WriteFile(to, data, 0o700); err != nil {
		t.Fatal(err)
	}
}
