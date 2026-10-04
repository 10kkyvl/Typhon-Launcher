//go:build windows

package install

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"typhon/internal/installguard"
	"typhon/internal/settings"
)

type guardRecorder struct {
	mu   sync.Mutex
	opts []installguard.Options
}

func (g *guardRecorder) add(opts installguard.Options) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.opts = append(g.opts, opts)
}

func (g *guardRecorder) all() []installguard.Options {
	g.mu.Lock()
	defer g.mu.Unlock()
	return slices.Clone(g.opts)
}

func recordGuards(t *testing.T) *guardRecorder {
	t.Helper()
	rec := &guardRecorder{}
	restore := startGuard
	startGuard = func(_ context.Context, _ int, opts installguard.Options) func() {
		rec.add(opts)
		return func() {}
	}
	t.Cleanup(func() { startGuard = restore })
	return rec
}

func wantGuards(t *testing.T, rec *guardRecorder, want ...installguard.Options) {
	t.Helper()
	if got := rec.all(); !slices.Equal(got, want) {
		t.Fatalf("guards started with %+v, want %+v", got, want)
	}
}

type inlineWorker struct{ done chan error }

func (w *inlineWorker) wait() (int, error) {
	code := 0
	if err := <-w.done; err != nil {
		code = 1
	}
	return code, nil
}

func (*inlineWorker) close() {}

func (*inlineWorker) terminate() error { return errors.New("inline worker cannot be terminated") }

func startInlineWorker(t *testing.T) func(runSpec) (workerHandle, error) {
	t.Helper()
	var wg sync.WaitGroup
	t.Cleanup(wg.Wait)
	return func(launch runSpec) (workerHandle, error) {
		done := make(chan error, 1)
		wg.Add(1)
		go func() {
			defer wg.Done()
			done <- RunWorker(ParseWorkerArgs(launch.Args[1:]))
		}()
		return &inlineWorker{done: done}, nil
	}
}

func TestProcessRunnerHandsVerifyRepackToGuard(t *testing.T) {
	ps := powershellPath(t)
	for _, verify := range []bool{false, true} {
		t.Run(fmt.Sprintf("verify=%v", verify), func(t *testing.T) {
			rec := recordGuards(t)
			spec := runSpec{
				Path: ps, Dir: t.TempDir(), Hidden: true,
				Options: installOptions{VerifyRepack: verify},
				Args:    []string{"-NoProfile", "-NonInteractive", "-Command", "exit 0"},
			}
			if _, err := (processRunner{}).run(context.Background(), spec); err != nil {
				t.Fatal(err)
			}
			wantGuards(t, rec, installguard.Options{HideProgress: true, VerifyRepack: verify})
		})
	}
}

func TestDiscoveryRunHandsVerifyRepackToGuard(t *testing.T) {
	ps := powershellPath(t)
	for _, verify := range []bool{false, true} {
		t.Run(fmt.Sprintf("verify=%v", verify), func(t *testing.T) {
			rec := recordGuards(t)
			dir := t.TempDir()
			in := discoverySpec{
				Engine: EngineInno, InstallerPath: ps, WorkingDir: dir,
				Destination: filepath.Join(dir, "Game"), InfPath: filepath.Join(dir, "probe.inf"),
				Options: installOptions{SkipExtras: true, VerifyRepack: verify},
			}
			if _, err := attemptDiscovery(context.Background(), in); err != nil {
				t.Fatal(err)
			}
			wantGuards(t, rec, installguard.Options{HideProgress: true, VerifyRepack: verify})
		})
	}
}

func TestVerifyRepackSettingReachesTheGuard(t *testing.T) {
	ps := powershellPath(t)
	for _, tc := range []struct {
		name     string
		elevated bool
	}{
		{"launcher runs the installer", false},
		{"elevated worker runs the installer", true},
	} {
		for _, verify := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/verify=%v", tc.name, verify), func(t *testing.T) {
				rec := recordGuards(t)
				dir := t.TempDir()
				opts := installOptionsFrom(settings.Settings{InstallVerifyRepack: verify, InstallSkipExtras: true})
				item := Installation{ID: "chain", Engine: EngineInno, InstallerPath: ps, Destination: filepath.Join(dir, "Game"), WorkingDir: dir}
				spec, err := silentSpec(item, ps, filepath.Join(dir, "install.log"), opts)
				if err != nil {
					t.Fatal(err)
				}
				spec.StatePath = filepath.Join(dir, "state.json")
				spec.CancelPath = filepath.Join(dir, "cancel")
				spec.InfPath = filepath.Join(dir, "probe.inf")
				if tc.elevated {
					withWorkerSeams(t, startInlineWorker(t))
					_, err = runElevated(context.Background(), spec)
				} else {
					_, err = (processRunner{}).run(context.Background(), spec)
				}
				if err != nil {
					t.Fatal(err)
				}
				guard := installguard.Options{HideProgress: true, VerifyRepack: verify}
				wantGuards(t, rec, guard, guard)
			})
		}
	}
}
