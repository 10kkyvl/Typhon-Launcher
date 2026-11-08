//go:build darwin

package emulators

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func kinfo(pid, uid int, comm string, stat int8) unix.KinfoProc {
	var k unix.KinfoProc
	k.Proc.P_pid = int32(pid & 0x7FFFFFFF)
	k.Proc.P_stat = stat
	k.Eproc.Ucred.Uid = uint32(uid & 0x7FFFFFFF)
	copy(k.Proc.P_comm[:], comm)
	return k
}

func TestCopiesNamedKeepsTheUsersLiveCopies(t *testing.T) {
	table := []unix.KinfoProc{
		kinfo(10, 501, "rpcs3", 2),
		kinfo(11, 502, "rpcs3", 2),     // another user's copy
		kinfo(12, 501, "rpcs3", sZomb), // already gone
		kinfo(13, 501, "rpcs3-helper", 2),
		kinfo(14, 501, "rpcs3", 2), // the caller itself
		kinfo(15, 501, "averyveryverylon", 2),
	}
	if got := copiesNamed(table, "rpcs3", 501, 14); !slices.Equal(got, []int{10}) {
		t.Fatalf("copies = %v", got)
	}
	// The kernel keeps 16 bytes of the name.
	if got := copiesNamed(table, "averyveryverylongname", 501, 0); !slices.Equal(got, []int{15}) {
		t.Fatalf("long name = %v", got)
	}
}

// A real process under the program's name is found and closed by its name.
func TestDarwinCopiesFindAndCloseARealProcess(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "rpcs3-typhon-test")
	data, err := os.ReadFile("/bin/sleep")
	if err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // G306: the copy is a program the test runs
	if err := os.WriteFile(fake, data, 0o700); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // G204: fixed tool on the test's own copy
	if out, err := exec.Command("/usr/bin/codesign", "--force", "--sign", "-", fake).CombinedOutput(); err != nil {
		t.Fatalf("sign: %v %s", err, out)
	}
	cmd := exec.Command(fake, "30") //nolint:gosec // G204: the test's own copy of /bin/sleep
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	t.Cleanup(func() {
		if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			t.Errorf("kill: %v", err)
		}
	})
	deadline := time.After(5 * time.Second)
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for found := false; !found; {
		pids, err := RunningCopies(t.Context(), fake)
		if err != nil {
			t.Fatal(err)
		}
		found = slices.Contains(pids, cmd.Process.Pid)
		if found {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("copy %d not found in %v", cmd.Process.Pid, pids)
		case <-tick.C:
		}
	}
	if err := CloseRunning(t.Context(), fake); err != nil {
		t.Fatal(err)
	}
	select {
	case <-waited:
	case <-time.After(5 * time.Second):
		t.Fatal("the copy kept running")
	}
}
