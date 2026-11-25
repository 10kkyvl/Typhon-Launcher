package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const brokenJournal = "{not json"

func writeBrokenJournal(t *testing.T, cfg string) string {
	t.Helper()
	path, err := handoffJournalPath(cfg)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, path, []byte(brokenJournal))
	return path
}

// setAside lists what an unparseable journal was renamed to.
func setAside(t *testing.T, journal string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(journal))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), filepath.Base(journal)+".") {
			out = append(out, filepath.Join(filepath.Dir(journal), e.Name()))
		}
	}
	return out
}

// A journal that cannot be parsed blocked every update on every start. A
// start that runs from the launcher's own name sets it aside, renamed and
// kept with its bytes, sweeps nothing, and updates work again.
func TestUnparseableJournalIsSetAsideAtAnOrdinaryStart(t *testing.T) {
	r := newHandoffRig(t, func(HandoffSpec, *fakeSuccessor) {})
	r.readyForHandoff(t)
	parked := filepath.Join(r.disk.install, oldPrefix+"zz.exe")
	writeTestFile(t, parked, []byte(oldBody))
	journal := writeBrokenJournal(t, r.disk.cfg)

	if err := RecoverHandoff(r.disk.cfg, "1.4.0"); err != nil {
		t.Fatalf("RecoverHandoff() = %v, want the journal set aside", err)
	}
	if exists(t, journal) {
		t.Fatal("the unparseable journal is still in place")
	}
	aside := setAside(t, journal)
	if len(aside) != 1 || !strings.Contains(filepath.Base(aside[0]), "unreadable") {
		t.Fatalf("set aside as %v, want one file named as unreadable", aside)
	}
	if got := readFileString(t, aside[0]); got != brokenJournal {
		t.Fatalf("the set-aside journal holds %q, want its bytes kept", got)
	}
	if !exists(t, parked) {
		t.Fatal("a parked launcher was swept on the strength of an unreadable journal")
	}
	if _, err := r.svc.prepareHandoff(); err != nil {
		t.Fatalf("prepareHandoff() after the journal was set aside = %v", err)
	}
}

// A start from a parked name is not the launcher's ordinary path: the
// journal may describe the very file it runs from, so it stays and refuses.
func TestUnparseableJournalStaysForAStartFromAParkedName(t *testing.T) {
	d := newHandoffDisk(t)
	parked := filepath.Join(d.install, oldPrefix+"zz.exe")
	writeTestFile(t, parked, []byte(oldBody))
	currentExecutable = func() (string, error) { return parked, nil }
	journal := writeBrokenJournal(t, d.cfg)

	if err := RecoverHandoff(d.cfg, "1.4.0"); err == nil {
		t.Fatal("RecoverHandoff() = nil for a start from a parked name")
	}
	if got := readFileString(t, journal); got != brokenJournal {
		t.Fatalf("journal = %q, want it left in place", got)
	}
	if len(setAside(t, journal)) != 0 {
		t.Fatal("the journal was set aside by a start from a parked name")
	}
}

// A journal that cannot be read at all (here: a directory in its place) is
// not a broken one: nothing is renamed and updates stay refused.
func TestJournalThatCannotBeReadIsNotSetAside(t *testing.T) {
	r := newHandoffRig(t, func(HandoffSpec, *fakeSuccessor) {})
	r.readyForHandoff(t)
	journal, err := handoffJournalPath(r.disk.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(journal, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := RecoverHandoff(r.disk.cfg, "1.4.0"); err == nil {
		t.Fatal("RecoverHandoff() = nil for a journal it could not read")
	}
	if !exists(t, journal) || len(setAside(t, journal)) != 0 {
		t.Fatal("an unreadable journal was moved")
	}
	if err := r.svc.BeginHandoff(1); !errors.Is(err, ErrHandoffPending) {
		t.Fatalf("BeginHandoff() = %v, want ErrHandoffPending", err)
	}
}

// U4: a journal that turns unparseable while this launcher runs refuses
// the update; only a start sets it aside.
func TestBeginHandoffRefusesWhileTheJournalCannotBeParsed(t *testing.T) {
	r := newHandoffRig(t, func(HandoffSpec, *fakeSuccessor) { t.Error("the successor started") })
	r.readyForHandoff(t)
	journal := writeBrokenJournal(t, r.disk.cfg)
	if err := r.svc.BeginHandoff(1); !errors.Is(err, ErrHandoffPending) {
		t.Fatalf("BeginHandoff() = %v, want ErrHandoffPending", err)
	}
	if err := r.svc.ApplyUpdate("en"); !errors.Is(err, ErrHandoffPending) {
		t.Fatalf("ApplyUpdate() = %v, want ErrHandoffPending", err)
	}
	if got := readFileString(t, journal); got != brokenJournal {
		t.Fatalf("journal = %q, want it untouched", got)
	}
	if got := readFileString(t, r.disk.exe); got != oldBody {
		t.Fatalf("launcher = %q, want it untouched", got)
	}
}

// U5: a journal the successor marked done and could not remove settles
// nothing any more: it does not hold the next update back.
func TestBeginHandoffIsNotRefusedByADoneJournal(t *testing.T) {
	r := newHandoffRig(t, func(HandoffSpec, *fakeSuccessor) {})
	r.readyForHandoff(t)
	j := r.plan.journalFor(1 << 22)
	j.Stage = stageDone
	if err := saveJournal(r.disk.cfg, j); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.prepareHandoff(); err != nil {
		t.Fatalf("prepareHandoff() with a done journal = %v, want the update allowed", err)
	}
}

// U6: the installer fails and putting the parked launcher back fails too:
// the swap returns the undo it still owes, and that undo, retried once the
// name is free, puts the old launcher back.
func TestSwapInstallerReturnsTheUndoItOwes(t *testing.T) {
	d := newHandoffDisk(t)
	plan := d.plan(t, "setup", sha256Hex(t, []byte("setup")))
	plan.Method = methodInstaller
	plan.InstallDir = d.install
	j := plan.journalFor(os.Getpid())
	prevInstaller, prevRename := applyInstaller, handoffRename
	t.Cleanup(func() { applyInstaller, handoffRename = prevInstaller, prevRename })
	applyInstaller = func(context.Context, string, string, string) error { return errors.New("installer crashed") }
	locked := true
	handoffRename = func(from, to string) error {
		if from == j.OldPath && locked {
			return errors.New("access denied")
		}
		return prevRename(from, to)
	}

	undo, err := performSwap(t.Context(), plan, j)
	if err == nil {
		t.Fatal("performSwap() = nil for a failed installer")
	}
	if undo == nil {
		t.Fatal("performSwap() returned no undo although its own failed and the launcher is parked")
	}
	if exists(t, d.exe) || readFileString(t, j.OldPath) != oldBody {
		t.Fatal("setup: the old launcher should be parked with the name empty")
	}
	locked = false
	if err := undo(); err != nil {
		t.Fatalf("the owed undo = %v", err)
	}
	if got := readFileString(t, d.exe); got != oldBody {
		t.Fatalf("launcher = %q, want the old build back", got)
	}
}

// The successor removes a journal it has settled; one it cannot parse is
// set aside like at a start, not deleted.
func TestSuccessorSetsAsideAnUnparseableJournal(t *testing.T) {
	r := newSuccessorRig(t, false)
	journal := writeBrokenJournal(t, r.svc.dir)
	r.oldAlive.Store(false)
	if err := r.complete(t); err != nil {
		t.Fatalf("CompleteHandoff() = %v", err)
	}
	if exists(t, journal) {
		t.Fatal("the unparseable journal is still in place")
	}
	aside := setAside(t, journal)
	if len(aside) != 1 || readFileString(t, aside[0]) != brokenJournal {
		t.Fatalf("set aside as %v, want the journal's bytes kept", aside)
	}
}
