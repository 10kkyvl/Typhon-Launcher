//go:build devmock && !windows

package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// inProcessWorkerHandle stands in for the real elevated worker process: the
// launcher itself cannot be built inside a unit test, so this runs RunWorker
// in a goroutine against the very spec file runElevated already wrote, and
// wait() reports the same (code, no-wait-error) shape a reaped subprocess
// would — a non-nil RunWorker error means the worker "process" exited
// non-zero, matching devmockProc.wait()'s mapping of *exec.ExitError.
type inProcessWorkerHandle struct {
	done chan error
}

func (h *inProcessWorkerHandle) wait() (int, error) {
	code := 0
	if err := <-h.done; err != nil {
		code = 1
	}
	return code, nil
}

func (*inProcessWorkerHandle) close() {}

// startInProcessWorker builds the startElevatedWorker seam for the two tests
// below and registers a cleanup that blocks until RunWorker has actually
// returned. runElevated can pick up a completed install through the
// state-file poll before ever consuming the exited channel, which would
// otherwise leave RunWorker's internal watchWorkerCancel goroutine (and the
// package-level pollInterval vars it reads) running past the end of the test
// — a real, race-detector-visible hazard against the next test in the
// process, not just a hypothetical one.
func startInProcessWorker(t *testing.T) func(runSpec) (workerHandle, error) {
	t.Helper()
	var wg sync.WaitGroup
	t.Cleanup(wg.Wait)
	return func(launchSpec runSpec) (workerHandle, error) {
		specFile, err := workerSpecArg(launchSpec.Args)
		if err != nil {
			return nil, err
		}
		done := make(chan error, 1)
		wg.Add(1)
		go func() {
			defer wg.Done()
			done <- RunWorker(specFile)
		}()
		return &inProcessWorkerHandle{done: done}, nil
	}
}

// TestElevatedWorkerEndToEndInProcess proves the two halves of the worker
// protocol agree on the files without ever building the launcher binary:
// runElevated writes the spec, the faked launcher runs RunWorker in-process
// against that exact spec file, and RunWorker's mock install (a spec without
// StatePath, so it never recurses into runElevated) leaves the state file
// runElevated is polling.
func TestElevatedWorkerEndToEndInProcess(t *testing.T) {
	t.Setenv(devmockInstallSecondsEnv, "0")

	dir := t.TempDir()
	installerPath := filepath.Join(dir, "download", "FooGame-setup.exe")
	dest := filepath.Join(dir, "Games", "FooGame")
	spec := runSpec{
		Path: installerPath, InstallerPath: installerPath, ID: "e2e1",
		Engine: EngineInno, Destination: dest, Dir: filepath.Dir(installerPath),
		StatePath:  filepath.Join(dir, "state.json"),
		CancelPath: filepath.Join(dir, "cancel"),
		InfPath:    filepath.Join(dir, "discover.ini"),
	}

	withWorkerSeams(t, startInProcessWorker(t))

	code, err := runElevated(context.Background(), spec)
	if err != nil {
		t.Fatalf("runElevated: %v", err)
	}
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	if _, statErr := os.Stat(filepath.Join(dest, "FooGame.exe")); statErr != nil {
		t.Fatalf("stat installed exe: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(dest, devmockMarkerName)); statErr != nil {
		t.Fatalf("stat devmock marker: %v", statErr)
	}

	state, found, err := readWorkerState(spec.StatePath)
	if err != nil {
		t.Fatalf("readWorkerState: %v", err)
	}
	if !found || !state.Done || state.Error != "" {
		t.Fatalf("final worker state = %+v, want done without error", state)
	}
}

// TestElevatedWorkerEndToEndInProcessCancel proves cancellation crosses the
// same file boundary: the cancel marker written by runElevated is the one
// watchWorkerCancel (running inside the in-process RunWorker goroutine)
// actually observes, and the resulting state is marked Cancelled, not merely
// failed.
func TestElevatedWorkerEndToEndInProcessCancel(t *testing.T) {
	t.Setenv(devmockInstallSecondsEnv, "5")

	dir := t.TempDir()
	installerPath := filepath.Join(dir, "download", "FooGame-setup.exe")
	dest := filepath.Join(dir, "Games", "FooGame")
	spec := runSpec{
		Path: installerPath, InstallerPath: installerPath, ID: "e2e2",
		Engine: EngineInno, Destination: dest, Dir: filepath.Dir(installerPath),
		StatePath:  filepath.Join(dir, "state.json"),
		CancelPath: filepath.Join(dir, "cancel"),
		InfPath:    filepath.Join(dir, "discover.ini"),
	}

	restorePoll := workerCancelPollInterval
	workerCancelPollInterval = 10 * time.Millisecond
	t.Cleanup(func() { workerCancelPollInterval = restorePoll })

	withWorkerSeams(t, startInProcessWorker(t))

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-time.After(60 * time.Millisecond)
		cancel()
	}()

	_, err := runElevated(ctx, spec)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runElevated error = %v, want errors.Is(err, context.Canceled)", err)
	}

	if exists(dest) {
		t.Fatal("destination created despite cancellation before the install delay elapsed")
	}

	state, found, stateErr := readWorkerState(spec.StatePath)
	if stateErr != nil {
		t.Fatalf("readWorkerState: %v", stateErr)
	}
	if !found || !state.Done || !state.Cancelled {
		t.Fatalf("final worker state = %+v, want done and cancelled", state)
	}
}
