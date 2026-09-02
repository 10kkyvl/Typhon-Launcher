//go:build devmock && !windows

package install

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestStartElevatedRunsShellScriptAndReportsExitCode(t *testing.T) {
	cases := []struct {
		name     string
		script   string
		wantCode int
	}{
		{"exits zero", "exit 0", 0},
		{"exits non-zero", "exit 3", 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			spec := runSpec{
				Path:      "/bin/sh",
				Args:      []string{"-c", tc.script},
				StatePath: filepath.Join(dir, "state.json"),
				ID:        "t1",
			}
			handle, err := startElevated(spec)
			if err != nil {
				t.Fatalf("startElevated: %v", err)
			}
			defer handle.close()

			code, err := handle.wait()
			if err != nil {
				t.Fatalf("wait: %v", err)
			}
			if code != tc.wantCode {
				t.Fatalf("code = %d, want %d", code, tc.wantCode)
			}

			if _, statErr := os.Stat(devmockWorkerLog(dir, "t1")); statErr != nil {
				t.Fatalf("stat worker log: %v", statErr)
			}
		})
	}
}

func TestWorkerProcessAliveOwnPidIsAlive(t *testing.T) {
	alive, err := workerProcessAlive(os.Getpid())
	if err != nil {
		t.Fatalf("workerProcessAlive: %v", err)
	}
	if !alive {
		t.Fatal("workerProcessAlive(own pid) = false, want true")
	}
}

func TestWorkerProcessAliveReapedChildIsDead(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "exit 0")
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatalf("run: %v", err)
		}
	}
	alive, err := workerProcessAlive(cmd.Process.Pid)
	if err != nil {
		t.Fatalf("workerProcessAlive: %v", err)
	}
	if alive {
		t.Fatal("workerProcessAlive(reaped child) = true, want false")
	}
}
