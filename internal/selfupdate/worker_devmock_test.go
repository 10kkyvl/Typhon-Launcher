//go:build devmock && !windows

package selfupdate

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestWorkerProcessAliveForSelf(t *testing.T) {
	alive, err := workerProcessAlive(os.Getpid())
	if err != nil {
		t.Fatalf("workerProcessAlive(self): %v", err)
	}
	if !alive {
		t.Fatal("workerProcessAlive(self) = false, want true")
	}
}

func TestWorkerProcessAliveReapedChild(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "exit 0")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start stand-in process: %v", err)
	}
	pid := cmd.Process.Pid
	if err := cmd.Wait(); err != nil {
		t.Fatalf("wait stand-in process: %v", err)
	}

	alive, err := workerProcessAlive(pid)
	if err != nil {
		t.Fatalf("workerProcessAlive(reaped): %v", err)
	}
	if alive {
		t.Fatal("workerProcessAlive() = true for a process that already exited")
	}
}

func TestWorkerProcessAlivePidOneIsEPERM(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: signal 0 to pid 1 would succeed instead of EPERM")
	}
	alive, err := workerProcessAlive(1)
	if err != nil {
		t.Fatalf("workerProcessAlive(1): %v", err)
	}
	if !alive {
		t.Fatal("workerProcessAlive(1) = false, want true: EPERM means the process exists")
	}
}

func TestWorkerProcessAliveZeroPID(t *testing.T) {
	alive, err := workerProcessAlive(0)
	if err != nil {
		t.Fatalf("workerProcessAlive(0): %v", err)
	}
	if alive {
		t.Fatal("workerProcessAlive(0) = true, want false")
	}
}

func TestStartUpdateWorkerReturnsBeforeChildExits(t *testing.T) {
	configDir := testConfigDir(t)

	script := filepath.Join(t.TempDir(), "worker.sh")
	writeTestFile(t, script, []byte("#!/bin/sh\nsleep 1\n"))
	//nolint:gosec // G302: this stand-in worker needs the executable bit; 0600 would make it non-executable and the test could never start it
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatalf("chmod script: %v", err)
	}
	specPath := filepath.Join(t.TempDir(), "spec.json")

	start := time.Now()
	if err := startUpdateWorker(script, specPath); err != nil {
		t.Fatalf("startUpdateWorker: %v", err)
	}
	if elapsed := time.Since(start); elapsed >= 900*time.Millisecond {
		t.Fatalf("startUpdateWorker blocked for %v, want it to return before the child exits", elapsed)
	}

	workerDir, err := WorkerDir(configDir)
	if err != nil {
		t.Fatalf("WorkerDir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workerDir, workerLogName)); err != nil {
		t.Fatalf("worker log not created: %v", err)
	}
}

func waitForFile(t *testing.T, path string, timeout time.Duration) []byte {
	t.Helper()
	deadline := time.After(timeout)
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		data, err := os.ReadFile(path)
		if err == nil {
			return data
		}
		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("read %s: %v", path, err)
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %s", path)
		case <-ticker.C:
		}
	}
}

// TestRunWorkerEndToEnd exercises the real devmock primitives together: a
// detached parent the test kills, a fake artifact staged inside the cache, an
// Apply that renames it over the relaunch target, and a relaunch that actually
// executes the replaced file. Nothing here is faked, unlike worker_run_test.go.
func TestRunWorkerEndToEnd(t *testing.T) {
	shortWorkerTimeouts(t)
	configDir := testConfigDir(t)

	installDir := t.TempDir()
	target := filepath.Join(installDir, "typhon")
	writeTestFile(t, target, []byte("#!/bin/sh\nexit 0\n"))
	//nolint:gosec // G302: target stands in for the launcher binary and relaunch() executes it; 0600 would make it non-executable
	if err := os.Chmod(target, 0o755); err != nil {
		t.Fatalf("chmod target: %v", err)
	}
	before := sha256Hex(t, []byte("#!/bin/sh\nexit 0\n"))

	markerPath := filepath.Join(t.TempDir(), "relaunched.marker")
	scriptContent := []byte("#!/bin/sh\necho relaunched > \"" + markerPath + "\"\n")
	installerPath, _ := seedReadyArtifact(t, configDir, "2.0.0", "typhon-devmock", scriptContent)

	parent := exec.Command("sleep", "30")
	if err := parent.Start(); err != nil {
		t.Fatalf("start parent stand-in: %v", err)
	}
	parentPID := parent.Process.Pid
	t.Cleanup(func() {
		if err := parent.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			t.Logf("kill parent stand-in during cleanup: %v", err)
		}
		if err := parent.Wait(); err != nil {
			t.Logf("wait parent stand-in during cleanup: %v", err)
		}
	})

	spec := updateSpec{
		InstallerPath: installerPath,
		InstallDir:    installDir,
		ParentPID:     parentPID,
		RelaunchPath:  target,
		Version:       "2.0.0",
	}
	specPath, err := SpecPath(configDir)
	if err != nil {
		t.Fatalf("SpecPath: %v", err)
	}
	if err := writeUpdateSpec(specPath, spec); err != nil {
		t.Fatalf("writeUpdateSpec: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- RunWorker(specPath) }()

	if err := parent.Process.Kill(); err != nil {
		t.Fatalf("kill parent stand-in: %v", err)
	}
	if err := parent.Wait(); err != nil {
		t.Logf("wait parent stand-in: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunWorker: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("RunWorker did not finish in time")
	}

	outcome := readWorkerOutcome(t, configDir)
	if !outcome.OK || outcome.Version != "2.0.0" || outcome.Error != "" {
		t.Fatalf("outcome = %+v, want ok for 2.0.0", outcome)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(got) != string(scriptContent) {
		t.Fatalf("target content = %q, want the new artifact bytes", got)
	}
	if after := sha256Hex(t, got); after == before {
		t.Fatal("target digest did not change")
	}

	marker := waitForFile(t, markerPath, 5*time.Second)
	if string(marker) != "relaunched\n" {
		t.Fatalf("marker content = %q, want %q", marker, "relaunched\n")
	}
}
