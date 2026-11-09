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

type replayBroker struct {
	dir, contentRoot, stateDir, gamesRoot string
	pin                                   brokerPin
	outcome                               chan BrokerOutcome
	err                                   chan error
}

// startReplayBroker runs a real RunBroker against a pinned directory and
// returns the channels its exit lands on.
func startReplayBroker(t *testing.T) *replayBroker {
	t.Helper()
	t.Setenv(devmockInstallSecondsEnv, "0")
	root := t.TempDir()
	b := &replayBroker{
		dir: filepath.Join(root, "broker"), contentRoot: filepath.Join(root, "download"),
		stateDir: filepath.Join(root, "state"), gamesRoot: filepath.Join(root, "Games"),
		outcome: make(chan BrokerOutcome, 1), err: make(chan error, 1),
	}
	for _, d := range []string{b.dir, b.contentRoot, b.stateDir, b.gamesRoot} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	b.pin = brokerPin{DownloadID: "d1", ContentRoot: b.contentRoot, LibraryRoot: b.gamesRoot, StateDir: b.stateDir}
	if err := writeBrokerPin(brokerPinPath(b.dir), b.pin); err != nil {
		t.Fatalf("write pin: %v", err)
	}
	if err := touchBrokerAlive(brokerAlivePath(b.dir)); err != nil {
		t.Fatalf("touch alive: %v", err)
	}
	shortBrokerTiming(t, time.Hour, time.Hour)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		outcome, err := RunBroker(b.dir, brokerTestPublic)
		b.outcome <- outcome
		b.err <- err
	}()
	t.Cleanup(func() {
		if err := writeWorkerFile(brokerAbortPath(b.dir), []byte{}); err != nil && !errors.Is(err, os.ErrExist) {
			t.Logf("write abort: %v", err)
		}
		wg.Wait()
	})
	return b
}

func (b *replayBroker) signedSpec(t *testing.T, name, run string) workerSpec {
	t.Helper()
	installer := filepath.Join(b.contentRoot, name+"-setup.exe")
	//nolint:forbidigo // test fixture installer
	if err := os.WriteFile(installer, []byte("fixture "+name), 0o600); err != nil {
		t.Fatal(err)
	}
	return workerSpec{
		ID: "replay-" + name, Run: run, InstallerPath: installer, Engine: EngineInno,
		Destination: filepath.Join(b.gamesRoot, name), WorkingDir: b.contentRoot,
		StatePath:  filepath.Join(b.stateDir, "state.json"),
		CancelPath: filepath.Join(b.stateDir, "cancel"),
		InfPath:    filepath.Join(b.stateDir, "discover.ini"),
		LogPath:    filepath.Join(b.stateDir, "install.log"),
	}
}

func (b *replayBroker) waitExit(t *testing.T) (BrokerOutcome, error) {
	t.Helper()
	select {
	case outcome := <-b.outcome:
		return outcome, <-b.err
	case <-time.After(10 * time.Second):
		t.Fatal("the broker did not exit")
		return "", nil
	}
}

// waitForRunDone waits until the broker reports the given run finished in
// the worker state file; the spec itself is removed before the run starts.
func waitForRunDone(t *testing.T, statePath, run string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		state, found, err := readWorkerState(statePath)
		if err != nil {
			t.Fatalf("read worker state: %v", err)
		}
		if found && state.Run == run && state.Done {
			if state.Error != "" {
				t.Fatalf("run %s finished with an error: %s", run, state.Error)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("run %s never finished", run)
}

// A spec that was already served must not run a second time even when it is
// written again byte for byte: the signature is valid, so only the Run token
// tells a replay from a new job, and a replay ends the elevated process.
func TestBrokerRefusesAReplayedSignedSpec(t *testing.T) {
	b := startReplayBroker(t)
	spec := b.signedSpec(t, "FooGame", "run-1")
	if err := writeSignedBrokerSpec(context.Background(), b.dir, spec, brokerTestPrivate); err != nil {
		t.Fatalf("write first spec: %v", err)
	}
	waitForRunDone(t, spec.StatePath, "run-1")
	if _, err := os.Stat(filepath.Join(b.gamesRoot, "FooGame", "FooGame.exe")); err != nil {
		t.Fatalf("first install did not land: %v", err)
	}

	// Same Run token again: a replay, not a second installer of the chain.
	if err := writeSignedBrokerSpec(context.Background(), b.dir, spec, brokerTestPrivate); err != nil {
		t.Fatalf("write replayed spec: %v", err)
	}
	outcome, err := b.waitExit(t)
	if !errors.Is(err, errBrokerOutsidePin) {
		t.Fatalf("broker exit error = %v, want %v", err, errBrokerOutsidePin)
	}
	if outcome != BrokerAborted {
		t.Fatalf("outcome = %q, want %q", outcome, BrokerAborted)
	}
}

// Without a Run token nothing distinguishes a replay, so such a spec is
// refused outright even though its signature verifies.
func TestBrokerRefusesASignedSpecWithoutARunToken(t *testing.T) {
	b := startReplayBroker(t)
	spec := b.signedSpec(t, "BarGame", "")
	if err := writeSignedBrokerSpec(context.Background(), b.dir, spec, brokerTestPrivate); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	outcome, err := b.waitExit(t)
	if !errors.Is(err, errBrokerOutsidePin) {
		t.Fatalf("broker exit error = %v, want %v", err, errBrokerOutsidePin)
	}
	if outcome != BrokerAborted {
		t.Fatalf("outcome = %q, want %q", outcome, BrokerAborted)
	}
	if _, err := os.Stat(filepath.Join(b.gamesRoot, "BarGame")); err == nil {
		t.Fatal("the unnamed spec was installed")
	}
}
