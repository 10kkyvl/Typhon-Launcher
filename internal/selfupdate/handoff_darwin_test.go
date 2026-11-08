//go:build darwin && !devmock

package selfupdate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBundleHandoffSwapKeepsTheOldBundleParkedAndUndoes(t *testing.T) {
	parent := t.TempDir()
	bundle := filepath.Join(parent, "Typhon.app")
	exe := filepath.Join(bundle, "Contents", "MacOS", "typhon")
	writeTestFile(t, exe, []byte("old build"))

	plan := handoffPlan{
		Method: methodBundle, Exe: exe, Target: bundle, ArtifactPath: appZip(t, "new build"),
		Nonce: "n0nce", Version: "1.5.0", FromVersion: "1.4.0",
	}
	j := plan.journalFor(os.Getpid())

	undo, err := performSwap(t.Context(), plan, j)
	if err != nil {
		t.Fatalf("performSwap() error = %v", err)
	}
	if got := readFileString(t, exe); got != "new build" {
		t.Fatalf("bundle executable = %q, want the new build", got)
	}
	parked := filepath.Join(j.OldPath, "Contents", "MacOS", "typhon")
	if got := readFileString(t, parked); got != "old build" {
		t.Fatalf("parked executable = %q, want the old build", got)
	}
	info, err := os.Stat(exe)
	if err != nil || info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("the new executable is not runnable: %v %v", info, err)
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("parent holds %d entries, want the bundle and the parked one", len(entries))
	}

	// Recovery works from the journal alone.
	if err := undoSwap(j); err != nil {
		t.Fatalf("undoSwap() error = %v", err)
	}
	if got := readFileString(t, exe); got != "old build" {
		t.Fatalf("after undo executable = %q, want the old build", got)
	}
	if exists(t, j.OldPath) {
		t.Fatal("the parked bundle was left after undo")
	}
	if err := undo(); err != nil {
		t.Fatalf("a repeated undo must be harmless: %v", err)
	}
}

func TestBundleHandoffRecoveryFinishesWhenTheNewVersionRuns(t *testing.T) {
	parent := t.TempDir()
	bundle := filepath.Join(parent, "Typhon.app")
	exe := filepath.Join(bundle, "Contents", "MacOS", "typhon")
	writeTestFile(t, exe, []byte("old build"))
	cfg := t.TempDir()
	plan := handoffPlan{Method: methodBundle, Exe: exe, Target: bundle, ArtifactPath: appZip(t, "new build"), Nonce: "n0nce", Version: "1.5.0", FromVersion: "1.4.0"}
	j := plan.journalFor(1 << 22)
	if _, err := performSwap(t.Context(), plan, j); err != nil {
		t.Fatal(err)
	}
	j.Stage = stageSwapped
	if err := saveJournal(cfg, j); err != nil {
		t.Fatal(err)
	}
	prevExe, prevAlive := currentExecutable, parentAlive
	currentExecutable = func() (string, error) { return exe, nil }
	parentAlive = func(int) (bool, error) { return false, nil }
	t.Cleanup(func() { currentExecutable, parentAlive = prevExe, prevAlive })

	if err := RecoverHandoff(cfg, "1.5.0"); err != nil {
		t.Fatalf("RecoverHandoff() error = %v", err)
	}
	if got := readFileString(t, exe); got != "new build" {
		t.Fatalf("executable = %q, want the new build kept", got)
	}
	if exists(t, j.OldPath) {
		t.Fatal("the parked bundle was not removed")
	}
}
