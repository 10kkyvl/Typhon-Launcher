package selfupdate

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"typhon/internal/handoffgate"
	"typhon/internal/uierr"
	"typhon/internal/windowhandoff"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	oldBody = "old launcher"
	newBody = "new launcher build"
)

func readFileString(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}

func exists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Lstat(path)
	if err == nil {
		return true
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false
	}
	t.Fatalf("stat %s: %v", path, err)
	return false
}

func silenceSeams(t *testing.T) {
	t.Helper()
	prevAllow, prevYield, prevQuit, prevSettle, prevHide := allowForeground, yieldForeground, quitApplication, settleWindow, hideOldWindow
	allowForeground = func(int) error { return nil }
	yieldForeground = func(int) error { return nil }
	quitApplication = func() {}
	settleWindow = func(*application.WebviewWindow, bool) error { return nil }
	hideOldWindow = func(*application.WebviewWindow) error { return nil }
	t.Cleanup(func() {
		allowForeground, yieldForeground, quitApplication, settleWindow, hideOldWindow = prevAllow, prevYield, prevQuit, prevSettle, prevHide
	})
}

func TestHandoffSpecRoundTrip(t *testing.T) {
	dir := t.TempDir()
	ready, err := HandoffReadyPath(dir, "abc123")
	if err != nil {
		t.Fatal(err)
	}
	spec := HandoffSpec{
		Nonce: "abc123", ParentPID: 4242, PhaseStart: 1700000000123, ReadyPath: ready, ConfigDir: dir,
		Version: "1.5.0", FromVersion: "1.4.0", Background: "#112233",
		Geometry: windowhandoff.Geometry{X: 10, Y: 20, Width: 1440, Height: 900, Maximised: true, Focused: true},
	}
	arg, err := spec.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, found, err := ParseHandoffArgs([]string{"typhon", handoffFlag, arg})
	if err != nil || !found {
		t.Fatalf("ParseHandoffArgs() = %v, %v, %v", got, found, err)
	}
	if got != spec {
		t.Fatalf("ParseHandoffArgs() = %+v, want %+v", got, spec)
	}
	if r, g, b := got.BackgroundRGB(); r != 0x11 || g != 0x22 || b != 0x33 {
		t.Fatalf("BackgroundRGB() = %d %d %d", r, g, b)
	}
}

func TestHandoffArgsAbsentAndBroken(t *testing.T) {
	if _, found, err := ParseHandoffArgs([]string{"typhon", "--play", "x"}); found || err != nil {
		t.Fatalf("no flag: found=%v err=%v", found, err)
	}
	for name, args := range map[string][]string{
		"no value":  {"typhon", handoffFlag},
		"not b64":   {"typhon", handoffFlag, "***"},
		"not json":  {"typhon", handoffFlag, "aGVsbG8"},
		"empty obj": {"typhon", handoffFlag, "e30"},
	} {
		if _, found, err := ParseHandoffArgs(args); !found || err == nil {
			t.Errorf("%s: found=%v err=%v, want found with an error", name, found, err)
		}
	}
}

func TestHandoffSpecRejectsForeignReadyPath(t *testing.T) {
	dir := t.TempDir()
	spec := HandoffSpec{Nonce: "abc", ParentPID: 1, ConfigDir: dir, ReadyPath: filepath.Join(dir, "settings.json")}
	arg, err := spec.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := ParseHandoffArgs([]string{"typhon", handoffFlag, arg}); !errors.Is(err, errHandoffBadSpec) {
		t.Fatalf("ParseHandoffArgs() error = %v, want errHandoffBadSpec", err)
	}
}

// The default is the dark preset's page colour (--bg in theme/presets.go).
func TestBackgroundFallsBackToTheDefaultTheme(t *testing.T) {
	for _, bad := range []string{"", "red", "#12345", "#gggggg", "123456"} {
		r, g, b := HandoffSpec{Background: bad}.BackgroundRGB()
		if r != 14 || g != 15 || b != 17 {
			t.Errorf("BackgroundRGB(%q) = %d %d %d, want the default #0e0f11", bad, r, g, b)
		}
	}
	s := &Service{}
	s.SetHandoff(HandoffSpec{Background: "not a colour"})
	if got := s.HandoffInfo().Background; got != "#0e0f11" {
		t.Errorf("HandoffInfo().Background = %q, want the default #0e0f11", got)
	}
}

func TestSweepOldRemovesOnlyOurPrefixes(t *testing.T) {
	dir := t.TempDir()
	keep := []string{"typhon.exe", "notes.txt", "typhon-old-1.exe", "old-.typhon-old-x", ".typhon-swap-1"}
	for _, name := range keep {
		writeTestFile(t, filepath.Join(dir, name), []byte("x"))
	}
	for _, name := range []string{oldPrefix + "aa.exe", stagedPrefix + "bb.exe"} {
		writeTestFile(t, filepath.Join(dir, name), []byte("x"))
	}
	writeTestFile(t, filepath.Join(dir, oldPrefix+"cc.app", "Contents", "x"), []byte("x"))

	sweepOld(dir)

	for _, name := range keep {
		if !exists(t, filepath.Join(dir, name)) {
			t.Errorf("sweepOld removed %s, which is not ours", name)
		}
	}
	for _, name := range []string{oldPrefix + "aa.exe", stagedPrefix + "bb.exe", oldPrefix + "cc.app"} {
		if exists(t, filepath.Join(dir, name)) {
			t.Errorf("sweepOld left %s", name)
		}
	}
}

// handoffDisk is an installation directory plus a config directory.
type handoffDisk struct {
	cfg, install, exe string
}

func newHandoffDisk(t *testing.T) handoffDisk {
	t.Helper()
	d := handoffDisk{cfg: t.TempDir(), install: t.TempDir()}
	d.exe = filepath.Join(d.install, "typhon.exe")
	writeTestFile(t, d.exe, []byte(oldBody))
	if err := os.Chmod(d.exe, 0o755); err != nil {
		t.Fatal(err)
	}
	prev := currentExecutable
	currentExecutable = func() (string, error) { return d.exe, nil }
	t.Cleanup(func() { currentExecutable = prev })
	return d
}

func (d handoffDisk) plan(t *testing.T, newContent string, wantHash string) handoffPlan {
	t.Helper()
	art := filepath.Join(d.cfg, "selfupdate", "1.5.0", "typhon.exe")
	writeTestFile(t, art, []byte(newContent))
	return handoffPlan{
		Method: methodExeFile, Exe: d.exe, Target: d.exe, ArtifactPath: art,
		Art:   Artifact{Kind: KindExe, Size: int64(len(newContent)), SHA256: wantHash},
		Nonce: "n0nce", Version: "1.5.0", FromVersion: "1.4.0", Portable: true,
	}
}

func TestSwapParkedExeReplacesAndUndoRestores(t *testing.T) {
	d := newHandoffDisk(t)
	plan := d.plan(t, newBody, sha256Hex(t, []byte(newBody)))
	j := plan.journalFor(os.Getpid())

	undo, err := performSwap(t.Context(), plan, j)
	if err != nil {
		t.Fatalf("performSwap() error = %v", err)
	}
	if got := readFileString(t, d.exe); got != newBody {
		t.Fatalf("launcher = %q, want the new build", got)
	}
	if got := readFileString(t, j.OldPath); got != oldBody {
		t.Fatalf("parked = %q, want the old build", got)
	}
	if exists(t, j.StagedPath) {
		t.Fatal("the staged file was left behind")
	}

	if err := undo(); err != nil {
		t.Fatalf("undo() error = %v", err)
	}
	if got := readFileString(t, d.exe); got != oldBody {
		t.Fatalf("after undo launcher = %q, want the old build", got)
	}
	if exists(t, j.OldPath) {
		t.Fatal("the parked file was left after undo")
	}
}

func TestSwapWithAWrongHashTouchesNothing(t *testing.T) {
	d := newHandoffDisk(t)
	plan := d.plan(t, newBody, strings.Repeat("ab", 32))
	j := plan.journalFor(os.Getpid())

	if _, err := performSwap(t.Context(), plan, j); !errors.Is(err, ErrSwapHash) {
		t.Fatalf("performSwap() error = %v, want ErrSwapHash", err)
	}
	if got := readFileString(t, d.exe); got != oldBody {
		t.Fatalf("launcher = %q, want it untouched", got)
	}
	entries, err := os.ReadDir(d.install)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("install dir holds %d entries, want only the launcher", len(entries))
	}
}

func TestSwapFailingBetweenTheRenamesPutsTheOldBack(t *testing.T) {
	d := newHandoffDisk(t)
	plan := d.plan(t, newBody, sha256Hex(t, []byte(newBody)))
	j := plan.journalFor(os.Getpid())

	prev := handoffRename
	calls := 0
	handoffRename = func(from, to string) error {
		calls++
		if calls == 2 { // staged -> target
			return errors.New("antivirus holds the file")
		}
		return prev(from, to)
	}
	t.Cleanup(func() { handoffRename = prev })

	if _, err := performSwap(t.Context(), plan, j); err == nil || !strings.Contains(err.Error(), "antivirus") {
		t.Fatalf("performSwap() error = %v, want the rename failure", err)
	}
	if got := readFileString(t, d.exe); got != oldBody {
		t.Fatalf("launcher = %q, want the old build restored", got)
	}
	if exists(t, j.OldPath) || exists(t, j.StagedPath) {
		t.Fatal("parked or staged file left after a failed swap")
	}
}

func TestSwapFromAPortableZip(t *testing.T) {
	d := newHandoffDisk(t)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("typhon.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(newBody)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	plan := d.plan(t, buf.String(), sha256Hex(t, buf.Bytes()))
	plan.Method = methodExeZip
	plan.Art.Kind = KindPortable
	j := plan.journalFor(os.Getpid())

	undo, err := performSwap(t.Context(), plan, j)
	if err != nil {
		t.Fatalf("performSwap() error = %v", err)
	}
	if got := readFileString(t, d.exe); got != newBody {
		t.Fatalf("launcher = %q, want the executable from the zip", got)
	}
	if err := undo(); err != nil {
		t.Fatal(err)
	}
	if got := readFileString(t, d.exe); got != oldBody {
		t.Fatalf("after undo launcher = %q", got)
	}
}

func TestSwapInstallerParksTheRunningExeAndChecksTheResult(t *testing.T) {
	d := newHandoffDisk(t)
	plan := d.plan(t, "setup", sha256Hex(t, []byte("setup")))
	plan.Method = methodInstaller
	plan.InstallDir = d.install
	j := plan.journalFor(os.Getpid())

	prev := applyInstaller
	t.Cleanup(func() { applyInstaller = prev })

	applyInstaller = func(_ context.Context, _, _, target string) error {
		if exists(t, target) {
			t.Error("the running executable was not parked before the installer ran")
		}
		writeTestFile(t, target, []byte(newBody))
		return nil
	}
	undo, err := performSwap(t.Context(), plan, j)
	if err != nil {
		t.Fatalf("performSwap() error = %v", err)
	}
	if readFileString(t, d.exe) != newBody || readFileString(t, j.OldPath) != oldBody {
		t.Fatal("installer swap did not leave new at the launcher and old parked")
	}
	if err := undo(); err != nil {
		t.Fatal(err)
	}
	if got := readFileString(t, d.exe); got != oldBody {
		t.Fatalf("after undo launcher = %q", got)
	}

	// An installer that exits 0 without writing the launcher is a failure.
	applyInstaller = func(context.Context, string, string, string) error { return nil }
	if _, err := performSwap(t.Context(), plan, j); !errors.Is(err, ErrNotReplaced) {
		t.Fatalf("no-op installer error = %v, want ErrNotReplaced", err)
	}
	if got := readFileString(t, d.exe); got != oldBody {
		t.Fatalf("after a no-op installer launcher = %q, want the old build back", got)
	}

	applyInstaller = func(context.Context, string, string, string) error { return errors.New("installer crashed") }
	if _, err := performSwap(t.Context(), plan, j); err == nil {
		t.Fatal("a failed installer reported success")
	}
	if got := readFileString(t, d.exe); got != oldBody {
		t.Fatalf("after a failed installer launcher = %q, want the old build back", got)
	}
}

func TestUndoSwapRefusesForeignNames(t *testing.T) {
	d := newHandoffDisk(t)
	victim := filepath.Join(d.install, "settings.json")
	writeTestFile(t, victim, []byte("precious"))
	j := handoffJournal{Method: methodExeFile, Target: d.exe, OldPath: victim}
	if err := undoSwap(j); err == nil {
		t.Fatal("undoSwap() accepted a journal pointing at a file that is not a parked launcher")
	}
	if readFileString(t, d.exe) != oldBody || readFileString(t, victim) != "precious" {
		t.Fatal("undoSwap() touched files despite refusing")
	}
}

func TestRecoverHandoff(t *testing.T) {
	stages := []string{stagePrepared, stageSwapped, stageReady, stageDone}
	for _, stage := range stages {
		for _, runningNew := range []bool{true, false} {
			name := stage + "/running-old"
			if runningNew {
				name = stage + "/running-new"
			}
			t.Run(name, func(t *testing.T) {
				d := newHandoffDisk(t)
				plan := d.plan(t, newBody, sha256Hex(t, []byte(newBody)))
				j := plan.journalFor(1 << 22) // a pid that is not alive
				// The disk as a crashed old process left it: swapped.
				if _, err := performSwap(t.Context(), plan, j); err != nil {
					t.Fatal(err)
				}
				j.Stage = stage
				if err := saveJournal(d.cfg, j); err != nil {
					t.Fatal(err)
				}
				prev := parentAlive
				parentAlive = func(int) (bool, error) { return false, nil }
				t.Cleanup(func() { parentAlive = prev })

				current := "1.4.0"
				if runningNew {
					current = "1.5.0"
				}
				if err := RecoverHandoff(d.cfg, current); err != nil {
					t.Fatalf("RecoverHandoff() error = %v", err)
				}

				if exists(t, j.OldPath) {
					t.Error("the parked launcher survived recovery")
				}
				path, err := handoffJournalPath(d.cfg)
				if err != nil {
					t.Fatal(err)
				}
				if exists(t, path) {
					t.Error("the journal survived recovery")
				}
				want := oldBody
				if runningNew || stage == stageDone {
					want = newBody
				}
				if got := readFileString(t, d.exe); got != want {
					t.Errorf("launcher = %q, want %q", got, want)
				}
			})
		}
	}
}

func TestRecoverHandoffAfterAnAbortedSwapRemovesTheStagedFile(t *testing.T) {
	d := newHandoffDisk(t)
	plan := d.plan(t, newBody, sha256Hex(t, []byte(newBody)))
	j := plan.journalFor(1 << 22)
	writeTestFile(t, j.StagedPath, []byte("half"))
	if err := saveJournal(d.cfg, j); err != nil {
		t.Fatal(err)
	}
	prev := parentAlive
	parentAlive = func(int) (bool, error) { return false, nil }
	t.Cleanup(func() { parentAlive = prev })

	if err := RecoverHandoff(d.cfg, "1.4.0"); err != nil {
		t.Fatalf("RecoverHandoff() error = %v", err)
	}
	if exists(t, j.StagedPath) {
		t.Error("the staged file survived")
	}
	if got := readFileString(t, d.exe); got != oldBody {
		t.Errorf("launcher = %q, want it untouched", got)
	}
}

func TestRecoverHandoffLeavesALiveHandoffAlone(t *testing.T) {
	d := newHandoffDisk(t)
	plan := d.plan(t, newBody, sha256Hex(t, []byte(newBody)))
	j := plan.journalFor(1 << 22)
	if _, err := performSwap(t.Context(), plan, j); err != nil {
		t.Fatal(err)
	}
	j.Stage = stageSwapped
	if err := saveJournal(d.cfg, j); err != nil {
		t.Fatal(err)
	}
	prev := parentAlive
	parentAlive = func(int) (bool, error) { return true, nil }
	t.Cleanup(func() { parentAlive = prev })

	if err := RecoverHandoff(d.cfg, "1.5.0"); err != nil {
		t.Fatalf("RecoverHandoff() error = %v", err)
	}
	if !exists(t, j.OldPath) {
		t.Error("recovery removed the parked launcher while the old process was alive")
	}
}

func TestRecoverHandoffWithoutAJournalSweepsByPrefixOnly(t *testing.T) {
	d := newHandoffDisk(t)
	writeTestFile(t, filepath.Join(d.install, oldPrefix+"zz.exe"), []byte("x"))
	writeTestFile(t, filepath.Join(d.install, "user-data.bin"), []byte("x"))
	if err := RecoverHandoff(d.cfg, "1.5.0"); err != nil {
		t.Fatal(err)
	}
	if exists(t, filepath.Join(d.install, oldPrefix+"zz.exe")) {
		t.Error("a parked launcher was left")
	}
	if !exists(t, filepath.Join(d.install, "user-data.bin")) || !exists(t, d.exe) {
		t.Error("recovery removed a file that is not a parked launcher")
	}
}

func TestRecoverHandoffKeepsTheJournalWhenTheUndoFails(t *testing.T) {
	d := newHandoffDisk(t)
	plan := d.plan(t, newBody, sha256Hex(t, []byte(newBody)))
	j := plan.journalFor(1 << 22)
	if _, err := performSwap(t.Context(), plan, j); err != nil {
		t.Fatal(err)
	}
	if err := saveJournal(d.cfg, j); err != nil {
		t.Fatal(err)
	}
	prevAlive, prevRename := parentAlive, handoffRename
	parentAlive = func(int) (bool, error) { return false, nil }
	handoffRename = func(string, string) error { return errors.New("locked") }
	t.Cleanup(func() { parentAlive, handoffRename = prevAlive, prevRename })

	if err := RecoverHandoff(d.cfg, "1.4.0"); err == nil {
		t.Fatal("RecoverHandoff() = nil, want the undo failure")
	}
	path, err := handoffJournalPath(d.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !exists(t, path) {
		t.Fatal("the journal was removed although the swap was not undone")
	}
}

// fakeSuccessor stands in for the new launcher process.
type fakeSuccessor struct {
	done   chan struct{}
	killed atomic.Bool
}

func newFakeSuccessor() *fakeSuccessor { return &fakeSuccessor{done: make(chan struct{})} }

func (f *fakeSuccessor) Pid() int { return 4321 }
func (f *fakeSuccessor) Kill() error {
	if f.killed.CompareAndSwap(false, true) {
		close(f.done)
	}
	return nil
}
func (f *fakeSuccessor) Done() <-chan struct{} { return f.done }

type handoffRig struct {
	disk   handoffDisk
	svc    *Service
	plan   handoffPlan
	quit   atomic.Int32
	quitCh chan struct{}
	proc   *fakeSuccessor
	args   []string
}

// newHandoffRig wires a Service whose "successor" is behave(spec, proc). The
// service is built as a finished download leaves it: ready and idle, with the
// rig's verified artifact, in a portable install (no registry is touched).
func newHandoffRig(t *testing.T, behave func(spec HandoffSpec, proc *fakeSuccessor)) *handoffRig {
	t.Helper()
	silenceSeams(t)
	r := &handoffRig{disk: newHandoffDisk(t), proc: newFakeSuccessor(), quitCh: make(chan struct{}, 4)}
	r.plan = r.disk.plan(t, newBody, sha256Hex(t, []byte(newBody)))
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	art := r.plan.Art
	r.svc = &Service{
		dir: r.disk.cfg, store: mustStore(t, r.disk.cfg), currentVersion: "1.4.0", ctx: ctx, cancel: cancel,
		status:    Status{State: StateReady, CurrentVersion: "1.4.0", AvailableVersion: "1.5.0"},
		readyPath: r.plan.ArtifactPath, readyArtifact: &art, portable: true,
	}

	prevStart, prevQuit, prevTimeout := startHandoff, quitApplication, handoffReadyTimeout
	startHandoff = func(exe string, args []string) (handoffProc, error) {
		r.args = args
		if exe != r.disk.exe {
			t.Errorf("successor started from %q, want %q", exe, r.disk.exe)
		}
		spec, found, err := ParseHandoffArgs(append([]string{"typhon"}, args...))
		if err != nil || !found {
			t.Errorf("the successor got an unusable argument: %v", err)
		}
		if got := readFileString(t, exe); got != newBody {
			t.Errorf("the successor started before the new launcher was in place: %q", got)
		}
		behave(spec, r.proc)
		return r.proc, nil
	}
	quitApplication = func() {
		r.quit.Add(1)
		r.quitCh <- struct{}{}
	}
	handoffReadyTimeout = 300 * time.Millisecond
	t.Cleanup(func() { startHandoff, quitApplication, handoffReadyTimeout = prevStart, prevQuit, prevTimeout })
	return r
}

// waitQuit waits for the old launcher's quit, which runs on its own goroutine.
func (r *handoffRig) waitQuit(t *testing.T) {
	t.Helper()
	select {
	case <-r.quitCh:
	case <-time.After(5 * time.Second):
		t.Fatal("the old launcher did not quit")
	}
}

// readyForHandoff makes BeginHandoff see a platform that hands over an
// executable at its own name. The rig's service is already ready; a failed
// hand-over leaves it ready again.
func (r *handoffRig) readyForHandoff(t *testing.T) {
	t.Helper()
	prevMethod, prevTarget := methodFor, targetFor
	methodFor = func(Artifact, bool) handoffMethod { return methodExeFile }
	targetFor = func(exe string, _ handoffMethod) (string, error) { return exe, nil }
	t.Cleanup(func() { methodFor, targetFor = prevMethod, prevTarget })
}

func (r *handoffRig) assertRolledBack(t *testing.T) {
	t.Helper()
	if got := readFileString(t, r.disk.exe); got != oldBody {
		t.Errorf("launcher = %q, want the old build restored", got)
	}
	entries, err := os.ReadDir(r.disk.install)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("install dir holds %d entries after the rollback, want only the launcher", len(entries))
	}
	path, err := handoffJournalPath(r.disk.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if exists(t, path) {
		t.Error("the journal survived the rollback")
	}
	st := r.svc.GetStatus()
	if st.State != StateReady || st.ErrorCode != errorCodeHandoffFailed {
		t.Errorf("status = %+v, want ready with error code %q", st, errorCodeHandoffFailed)
	}
	if r.svc.busy || r.svc.HandoffInProgress() {
		t.Error("the service is still busy after the rollback")
	}
	if r.quit.Load() != 0 {
		t.Error("the old launcher quit although the hand-off failed")
	}
}

func TestHandoffHappyPath(t *testing.T) {
	r := newHandoffRig(t, func(spec HandoffSpec, _ *fakeSuccessor) {
		if spec.PhaseStart != 1234 || spec.ParentPID != os.Getpid() || spec.Version != "1.5.0" {
			t.Errorf("spec = %+v", spec)
		}
		if err := os.WriteFile(spec.ReadyPath, []byte(spec.Nonce), 0o600); err != nil { //nolint:forbidigo // test double for the successor's marker
			t.Errorf("write the marker: %v", err)
		}
	})
	r.svc.runHandoff(t.Context(), r.plan, 1234)

	r.waitQuit(t)
	if r.quit.Load() != 1 {
		t.Fatalf("quit called %d times, want once", r.quit.Load())
	}
	if got := readFileString(t, r.disk.exe); got != newBody {
		t.Fatalf("launcher = %q, want the new build", got)
	}
	j, err := loadJournal(r.disk.cfg)
	if err != nil || j.Stage != stageReady {
		t.Fatalf("journal = %+v, %v; want stage ready", j, err)
	}
	if !exists(t, j.OldPath) {
		t.Fatal("the old launcher must stay parked until the successor takes over")
	}

	// The successor's half.
	succ := &Service{dir: r.disk.cfg, store: mustStore(t, r.disk.cfg), currentVersion: "1.5.0", ctx: t.Context()}
	succ.SetHandoff(HandoffSpec{Nonce: j.Nonce, ParentPID: os.Getpid(), Version: "1.5.0", PhaseStart: 1234})
	prevAlive := parentAlive
	parentAlive = func(int) (bool, error) { return false, nil }
	t.Cleanup(func() { parentAlive = prevAlive })
	handoffgate.Arm()
	t.Cleanup(handoffgate.Open)
	if !succ.HandoffInfo().Active {
		t.Fatal("HandoffInfo().Active = false before the takeover")
	}
	tookOver := false
	if err := succ.CompleteHandoff(t.Context(), func() error { tookOver = true; return nil }); err != nil {
		t.Fatalf("CompleteHandoff() error = %v", err)
	}
	if !tookOver || handoffgate.Armed() {
		t.Error("the single instance was not taken over or the gate stayed closed")
	}
	if succ.HandoffInfo().Active {
		t.Error("HandoffInfo().Active = true after the old process is gone")
	}
	if exists(t, j.OldPath) {
		t.Error("the parked old launcher was not removed")
	}
	path, err := handoffJournalPath(r.disk.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if exists(t, path) {
		t.Error("the journal was not removed")
	}
	if got := readFileString(t, r.disk.exe); got != newBody {
		t.Errorf("launcher = %q after the hand-off", got)
	}
}

func TestHandoffRollsBackWhenTheSuccessorCrashesAtOnce(t *testing.T) {
	r := newHandoffRig(t, func(_ HandoffSpec, proc *fakeSuccessor) { close(proc.done) })
	r.svc.runHandoff(t.Context(), r.plan, 1)
	r.assertRolledBack(t)
}

func TestHandoffRollsBackWhenTheSuccessorNeverAnswers(t *testing.T) {
	r := newHandoffRig(t, func(HandoffSpec, *fakeSuccessor) {})
	r.svc.runHandoff(t.Context(), r.plan, 1)
	r.assertRolledBack(t)
	if !r.proc.killed.Load() {
		t.Error("the unresponsive successor was not killed")
	}
	if !strings.Contains(r.svc.GetStatus().Error, "in time") {
		t.Errorf("status error = %q, want the timeout", r.svc.GetStatus().Error)
	}
}

func TestHandoffRollsBackWhenTheSwapFailsMidway(t *testing.T) {
	r := newHandoffRig(t, func(HandoffSpec, *fakeSuccessor) { t.Error("the successor started after a failed swap") })
	prev := handoffRename
	calls := 0
	handoffRename = func(from, to string) error {
		calls++
		if calls == 2 {
			return errors.New("sharing violation")
		}
		return prev(from, to)
	}
	t.Cleanup(func() { handoffRename = prev })
	r.svc.runHandoff(t.Context(), r.plan, 1)
	r.assertRolledBack(t)
}

func TestHandoffRollsBackWhenTheSuccessorCannotStart(t *testing.T) {
	r := newHandoffRig(t, func(HandoffSpec, *fakeSuccessor) {})
	startHandoff = func(string, []string) (handoffProc, error) { return nil, errors.New("exec format error") }
	r.svc.runHandoff(t.Context(), r.plan, 1)
	r.assertRolledBack(t)
}

func TestBeginHandoffWithAWrongHashChangesNothing(t *testing.T) {
	r := newHandoffRig(t, func(HandoffSpec, *fakeSuccessor) { t.Error("the successor started despite a hash mismatch") })
	// The download was altered in the cache after it was verified.
	writeTestFile(t, r.plan.ArtifactPath, []byte(strings.Repeat("x", len(newBody))))
	r.readyForHandoff(t)

	err := r.svc.BeginHandoff(1)
	if !errors.Is(err, ErrHashMismatch) && !errors.Is(err, ErrSizeMismatch) {
		t.Fatalf("BeginHandoff() error = %v, want a hash mismatch", err)
	}
	if got := readFileString(t, r.disk.exe); got != oldBody {
		t.Fatalf("launcher = %q, want it untouched", got)
	}
	entries, rerr := os.ReadDir(r.disk.install)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if len(entries) != 1 {
		t.Fatalf("install dir holds %d entries, want only the launcher", len(entries))
	}
	path, jerr := handoffJournalPath(r.disk.cfg)
	if jerr != nil {
		t.Fatal(jerr)
	}
	if exists(t, path) {
		t.Fatal("a journal was written for a rejected update")
	}
	if r.svc.busy || r.svc.GetStatus().State != StateReady {
		t.Fatalf("service left busy or not ready: busy=%v status=%+v", r.svc.busy, r.svc.GetStatus())
	}
}

func TestBeginHandoffRejectsAnUnsupportedPlatformBeforeAnyChange(t *testing.T) {
	r := newHandoffRig(t, func(HandoffSpec, *fakeSuccessor) {})
	r.readyForHandoff(t)
	methodFor = func(Artifact, bool) handoffMethod { return "" }

	err := r.svc.BeginHandoff(1)
	if !errors.Is(err, ErrHandoffUnsupported) {
		t.Fatalf("BeginHandoff() error = %v, want ErrHandoffUnsupported", err)
	}
	// The update screen tells "cannot hand over here" from a real failure by
	// this code and only then falls back to the restart without a word.
	if got := uierr.Code(err); got != "selfupdate.handoff_unsupported" {
		t.Fatalf("BeginHandoff() error code = %q, want selfupdate.handoff_unsupported", got)
	}
	if r.svc.busy || r.svc.GetStatus().State != StateReady {
		t.Fatal("the worker fallback is not open after a rejected hand-off")
	}
}

func TestHandoffRenderedRevealsOnceThenWritesTheMarker(t *testing.T) {
	cfg := t.TempDir()
	ready, err := HandoffReadyPath(cfg, "abc")
	if err != nil {
		t.Fatal(err)
	}
	s := &Service{dir: cfg}
	if err := s.HandoffRendered(); err == nil {
		t.Fatal("HandoffRendered() outside a hand-off = nil")
	}

	s = &Service{dir: cfg}
	s.SetHandoff(HandoffSpec{Nonce: "abc", ReadyPath: ready, Geometry: windowhandoff.Geometry{Focused: true}})
	s.SetHandoffWindow(nil)
	if err := s.HandoffRendered(); !errors.Is(err, errNoHandoffWindow) {
		t.Fatalf("HandoffRendered() without a window = %v", err)
	}
	if exists(t, ready) {
		t.Fatal("the marker was written although the window never showed")
	}
}

func TestHandoffRenderedMarkerFollowsTheReveal(t *testing.T) {
	cfg := t.TempDir()
	ready, err := HandoffReadyPath(cfg, "abc")
	if err != nil {
		t.Fatal(err)
	}
	reveals := 0
	prev := revealWindow
	revealWindow = func(_ *application.WebviewWindow, focus bool) error {
		reveals++
		if !focus {
			t.Error("focus not passed to Reveal")
		}
		if exists(t, ready) {
			t.Error("the marker exists before the window was revealed")
		}
		return nil
	}
	t.Cleanup(func() { revealWindow = prev })

	s := &Service{dir: cfg}
	s.SetHandoff(HandoffSpec{Nonce: "abc", ReadyPath: ready, Geometry: windowhandoff.Geometry{Focused: true}})
	s.SetHandoffWindow(&application.WebviewWindow{})
	for i := 0; i < 3; i++ {
		if err := s.HandoffRendered(); err != nil {
			t.Fatalf("HandoffRendered() #%d = %v", i, err)
		}
	}
	if reveals != 1 {
		t.Fatalf("Reveal called %d times, want once", reveals)
	}
	if !exists(t, ready) {
		t.Fatal("the marker was not written")
	}
}

func TestSuccessorStartupHoldsBackStateCleanup(t *testing.T) {
	s := &Service{}
	if s.successorPending() {
		t.Fatal("a normal launcher reports a pending successor")
	}
	r := newSuccessorRig(t, false)
	if !r.svc.successorPending() {
		t.Fatal("a successor does not hold back its cleanup")
	}
	r.oldAlive.Store(false)
	if err := r.complete(t); err != nil {
		t.Fatalf("CompleteHandoff() = %v", err)
	}
	if r.svc.successorPending() {
		t.Fatal("cleanup is still held back after the old launcher left")
	}
}
