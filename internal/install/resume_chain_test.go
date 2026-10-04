package install

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type chainResume struct {
	s         *Service
	runner    *chainRunner
	reg       *fakeRegistrar
	installer []string
	id        string
}

func newChainResume(t *testing.T, extras, step int, state workerState, codes ...int) *chainResume {
	t.Helper()
	s := mustServiceAt(t, t.TempDir())
	s.settings = newTestSettings(t)
	reg := &fakeRegistrar{}
	s.downloads = newFakeDownloads()
	s.library = reg
	runner := &chainRunner{codes: codes}
	s.runner = runner

	src := t.TempDir()
	chain := []string{filepath.Join(src, "setup.exe")}
	for i := 1; i <= extras; i++ {
		chain = append(chain, filepath.Join(src, fmt.Sprintf("setup_addon_%d.exe", i)))
	}
	for _, p := range chain {
		mkInstaller(t, p, "Inno Setup Setup Data (5.6.2) (u)")
	}

	dest := filepath.Join(t.TempDir(), "Game")
	exe := filepath.Join(dest, "Game.exe")
	mkFile(t, exe, 4096)

	const id = "chain1"
	item := Installation{
		ID: id, DownloadID: "d1", Name: "Game", Type: TypeExeInstaller,
		Status: StatusInstalling, Destination: dest, Executable: exe,
		Engine: EngineInno, Silent: true, StartedAt: time.Now(),
		InstallerPath: chain[0], ExtraInstallers: chain[1:], ChainStep: step,
	}
	if err := s.store.save([]Installation{item}); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	state.PID = os.Getpid()
	state.Done = true
	if err := writeWorkerState(s.workerStatePath(id), state); err != nil {
		t.Fatalf("write worker state: %v", err)
	}

	restore := resumeWatchPollInterval
	resumeWatchPollInterval = 20 * time.Millisecond
	t.Cleanup(func() { resumeWatchPollInterval = restore })

	return &chainResume{s: s, runner: runner, reg: reg, installer: chain, id: id}
}

func (c *chainResume) start(t *testing.T) {
	t.Helper()
	if err := c.s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("startup: %v", err)
	}
	t.Cleanup(func() {
		if err := c.s.ServiceShutdown(); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})
}

func (c *chainResume) paths() []string {
	var out []string
	for _, spec := range c.runner.calls() {
		out = append(out, spec.InstallerPath)
	}
	return out
}

func TestResumedChainRunsRemainingInstallers(t *testing.T) {
	c := newChainResume(t, 2, 1, workerState{Code: 0})
	c.start(t)

	done := c.s.waitStatus(t, c.id, StatusCompleted)

	got := c.paths()
	want := c.installer[1:]
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("installers run after restart = %v, want %v", got, want)
	}
	if done.ChainStep != 3 {
		t.Fatalf("ChainStep = %d, want 3", done.ChainStep)
	}
	if len(c.reg.registered()) != 1 {
		t.Fatalf("registered games = %d, want 1", len(c.reg.registered()))
	}
	if done.Owned || !done.UninstallUnknown {
		t.Fatalf("Owned=%v UninstallUnknown=%v, want false/true: origin of a resumed install is not known", done.Owned, done.UninstallUnknown)
	}
}

func TestResumedChainMiddleStepRunsOnlyTail(t *testing.T) {
	c := newChainResume(t, 2, 2, workerState{Code: 0})
	c.start(t)

	c.s.waitStatus(t, c.id, StatusCompleted)

	got := c.paths()
	if len(got) != 1 || got[0] != c.installer[2] {
		t.Fatalf("installers run after restart = %v, want only %v", got, c.installer[2])
	}
}

func TestResumedChainLastStepFinalizesWithoutRunning(t *testing.T) {
	c := newChainResume(t, 2, 3, workerState{Code: 0})
	c.start(t)

	c.s.waitStatus(t, c.id, StatusCompleted)

	if got := c.paths(); len(got) != 0 {
		t.Fatalf("installers run after restart = %v, want none: the last step already finished", got)
	}
	if len(c.reg.registered()) != 1 {
		t.Fatalf("registered games = %d, want 1", len(c.reg.registered()))
	}
}

func TestResumedChainSingleInstallerFinalizes(t *testing.T) {
	c := newChainResume(t, 0, 0, workerState{Code: 0})
	c.start(t)

	c.s.waitStatus(t, c.id, StatusCompleted)

	if got := c.paths(); len(got) != 0 {
		t.Fatalf("installers run after restart = %v, want none", got)
	}
}

func TestResumedChainNeverFinalizesUnlessEveryStepSucceeded(t *testing.T) {
	cases := []struct {
		name      string
		extras    int
		step      int
		state     workerState
		codes     []int
		remove    int
		wantErr   error
		wantText  string
		wantCalls int
	}{
		{
			name: "step exited with failure code", extras: 2, step: 1,
			state: workerState{Code: 1}, wantErr: errInstallerFail,
		},
		{
			name: "worker reported an error", extras: 2, step: 1,
			state: workerState{Error: "worker boom"}, wantText: "worker boom",
		},
		{
			name: "record without a step number", extras: 2, step: 0,
			state: workerState{Code: 0}, wantErr: errChainStepUnknown, wantText: "3",
		},
		{
			name: "step number beyond the chain", extras: 2, step: 4,
			state: workerState{Code: 0}, wantErr: errChainStepUnknown,
		},
		{
			name: "next installer file is gone", extras: 2, step: 1, remove: 1,
			state: workerState{Code: 0}, wantErr: errChainInstallerMissing, wantText: "2 из 3",
		},
		{
			name: "later installer is gone", extras: 2, step: 1, remove: 2,
			state: workerState{Code: 0}, wantErr: errChainInstallerMissing, wantText: "3 из 3",
		},
		{
			name: "next installer fails", extras: 2, step: 1, codes: []int{1},
			state: workerState{Code: 0}, wantErr: errInstallerFail, wantText: "2 из 3", wantCalls: 1,
		},
		{
			name: "last installer fails", extras: 2, step: 1, codes: []int{0, 1},
			state: workerState{Code: 0}, wantErr: errInstallerFail, wantText: "3 из 3", wantCalls: 2,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newChainResume(t, tc.extras, tc.step, tc.state, tc.codes...)
			if tc.remove > 0 {
				if err := os.Remove(c.installer[tc.remove]); err != nil {
					t.Fatalf("remove installer: %v", err)
				}
			}
			c.start(t)

			failed := c.s.waitStatus(t, c.id, StatusFailed)

			if tc.wantText != "" && !strings.Contains(failed.Error, tc.wantText) {
				t.Fatalf("error = %q, want it to contain %q", failed.Error, tc.wantText)
			}
			if tc.wantErr != nil && !strings.Contains(failed.Error, tc.wantErr.Error()) {
				t.Fatalf("error = %q, want it to carry %q", failed.Error, tc.wantErr)
			}
			if got := len(c.paths()); got != tc.wantCalls {
				t.Fatalf("installers run after restart = %d (%v), want %d", got, c.paths(), tc.wantCalls)
			}
			if regs := c.reg.registered(); len(regs) != 0 {
				t.Fatalf("registered games = %d, want none: the chain did not complete", len(regs))
			}
		})
	}
}

func TestResumedChainHonoursCancelMarker(t *testing.T) {
	c := newChainResume(t, 2, 1, workerState{Code: 0})
	if err := writeWorkerCancel(c.s.workerCancelPath(c.id)); err != nil {
		t.Fatalf("write cancel marker: %v", err)
	}
	c.start(t)

	c.s.waitStatus(t, c.id, StatusCancelled)

	if got := c.paths(); len(got) != 0 {
		t.Fatalf("installers run after cancel = %v, want none", got)
	}
}

func TestSilentChainRecordsStepBeforeEachInstaller(t *testing.T) {
	s, downloads, _ := newTestService(t)
	root := t.TempDir()
	cupheadSource(t, root)
	downloads.add("d1", "Cuphead", root)

	games := t.TempDir()
	dest := filepath.Join(games, "Cuphead")
	s.roots = []string{games}

	var seen []int
	var staleState []bool
	var id string
	runner := &chainRunner{act: func(spec runSpec) {
		got, _ := s.snapshot(spec.ID)
		seen = append(seen, got.ChainStep)
		_, err := os.Stat(s.workerStatePath(spec.ID))
		staleState = append(staleState, !errors.Is(err, fs.ErrNotExist))
		mkFile(t, filepath.Join(dest, "Cuphead.exe"), 8192)
		id = spec.ID
		if err := writeWorkerState(s.workerStatePath(spec.ID), workerState{Done: true}); err != nil {
			t.Errorf("write worker state: %v", err)
		}
	}}
	s.runner = runner

	item, err := s.Start("d1", StartOptions{Destination: dest})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	s.waitStatus(t, item.ID, StatusCompleted)

	if len(seen) != 2 || seen[0] != 1 || seen[1] != 2 {
		t.Fatalf("ChainStep seen by each installer = %v, want [1 2]", seen)
	}
	if staleState[0] || staleState[1] {
		t.Fatalf("worker state left from the previous step = %v, want none", staleState)
	}
	if got, _ := s.snapshot(id); got.ChainStep != 2 {
		t.Fatalf("ChainStep after the chain = %d, want 2", got.ChainStep)
	}
}

func TestSilentSingleInstallerDoesNotRecordStep(t *testing.T) {
	s, downloads, _ := newTestService(t)
	root := t.TempDir()
	innoSource(t, root)
	downloads.add("d1", "Game", root)

	games := t.TempDir()
	dest := filepath.Join(games, "Game")
	s.roots = []string{games}
	runner := &chainRunner{act: func(runSpec) { mkFile(t, filepath.Join(dest, "Game.exe"), 8192) }}
	s.runner = runner

	item, err := s.Start("d1", StartOptions{Destination: dest})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	done := s.waitStatus(t, item.ID, StatusCompleted)
	if done.ChainStep != 0 {
		t.Fatalf("ChainStep = %d, want 0 for a single installer", done.ChainStep)
	}
}

func TestBeginChainStepRollsBackWhenPersistFails(t *testing.T) {
	s := mustServiceAt(t, t.TempDir())
	const id = "step1"
	s.mu.Lock()
	s.items = append(s.items, &Installation{ID: id, Name: "Game", Status: StatusInstalling, ChainStep: 1})
	s.mu.Unlock()
	if err := os.Mkdir(s.store.listPath(), 0o755); err != nil {
		t.Fatalf("block the store file: %v", err)
	}

	err := s.beginChainStep(id, 2)
	if err == nil || !strings.Contains(err.Error(), errPersistPrefix) {
		t.Fatalf("beginChainStep error = %v, want the persist-state wrapper", err)
	}
	if got, _ := s.snapshot(id); got.ChainStep != 1 {
		t.Fatalf("ChainStep = %d, want rolled back to 1", got.ChainStep)
	}
}

func TestBeginChainStepRefusesInactiveInstallation(t *testing.T) {
	s := mustServiceAt(t, t.TempDir())
	const id = "step2"
	s.mu.Lock()
	s.items = append(s.items, &Installation{ID: id, Name: "Game", Status: StatusCancelled})
	s.mu.Unlock()

	if err := s.beginChainStep(id, 2); !errors.Is(err, errUnavailable) {
		t.Fatalf("beginChainStep error = %v, want errUnavailable", err)
	}
	if err := s.beginChainStep("missing", 2); !errors.Is(err, errNotFound) {
		t.Fatalf("beginChainStep error = %v, want errNotFound", err)
	}
}

type blockingRunner struct {
	started chan struct{}
}

func (r *blockingRunner) run(ctx context.Context, _ runSpec) (int, error) {
	close(r.started)
	<-ctx.Done()
	return 0, ctx.Err()
}

func TestCancelDuringResumedChainStopsTheRunningInstaller(t *testing.T) {
	c := newChainResume(t, 2, 1, workerState{Code: 0})
	blocker := &blockingRunner{started: make(chan struct{})}
	c.s.runner = blocker
	c.start(t)

	select {
	case <-blocker.started:
	case <-time.After(15 * time.Second):
		t.Fatal("the resumed chain never started its next installer")
	}
	if err := c.s.Cancel(c.id); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	c.s.waitStatus(t, c.id, StatusCancelled)
	if regs := c.reg.registered(); len(regs) != 0 {
		t.Fatalf("registered games = %d, want none after cancel", len(regs))
	}
}
