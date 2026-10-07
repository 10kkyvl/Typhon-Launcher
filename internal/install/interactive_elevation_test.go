package install

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"typhon/internal/uierr"
)

type elevatingRunner struct{}

func (elevatingRunner) run(ctx context.Context, spec runSpec) (int, error) {
	return runElevated(ctx, spec)
}

type workerCapture struct {
	mu    sync.Mutex
	specs []workerSpec
}

func (c *workerCapture) all() []workerSpec {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]workerSpec(nil), c.specs...)
}

func (c *workerCapture) launcher(t *testing.T, installed string) func(runSpec) (workerHandle, error) {
	t.Helper()
	return func(rs runSpec) (workerHandle, error) {
		specFile, digest := ParseWorkerArgs(rs.Args[1:])
		ws, err := readVerifiedWorkerSpec(specFile, digest)
		if err != nil {
			t.Errorf("worker spec rejected: %v", err)
			return quickExitProcess(t), nil
		}
		c.mu.Lock()
		c.specs = append(c.specs, ws)
		c.mu.Unlock()
		mkFile(t, filepath.Join(installed, "MyGame.exe"), 512<<10)
		if err := writeWorkerState(ws.StatePath, workerState{Run: ws.Run, Done: true}); err != nil {
			t.Errorf("write worker state: %v", err)
		}
		return quickExitProcess(t), nil
	}
}

func TestInteractiveInstallThatNeedsElevationRunsThroughTheWorker(t *testing.T) {
	s, downloads, _ := newTestService(t)
	root := t.TempDir()
	mkFile(t, filepath.Join(root, "Game", "setup.exe"), 4096)
	downloads.add("d1", "Game", root)

	programs := t.TempDir()
	s.roots = []string{programs}
	s.runner = elevatingRunner{}
	capture := &workerCapture{}
	withWorkerSeams(t, capture.launcher(t, filepath.Join(programs, "MyGame")))

	item, err := s.Start("d1", StartOptions{})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	s.waitStatus(t, item.ID, StatusWaitingForUser)

	got := capture.all()
	if len(got) != 1 {
		t.Fatalf("worker specs = %d, want 1", len(got))
	}
	ws := got[0]
	if !ws.Interactive {
		t.Fatalf("worker spec is not interactive: %+v", ws)
	}
	if ws.Hidden || ws.Background {
		t.Fatalf("interactive worker spec hides or backgrounds the installer: %+v", ws)
	}
	if ws.ID != item.ID || ws.InstallerPath == "" {
		t.Fatalf("worker spec lost the job identity: %+v", ws)
	}
	if ws.StatePath != s.workerStatePath(item.ID) || ws.CancelPath != s.workerCancelPath(item.ID) {
		t.Fatalf("worker spec paths = state %q cancel %q, want %q and %q",
			ws.StatePath, ws.CancelPath, s.workerStatePath(item.ID), s.workerCancelPath(item.ID))
	}
}

func TestMainRunSpecInteractive(t *testing.T) {
	root := t.TempDir()
	installer := filepath.Join(root, "setup.exe")
	cases := []struct {
		name   string
		engine Engine
	}{
		{name: "unknown engine", engine: EngineUnknown},
		{name: "inno", engine: EngineInno},
		{name: "nsis", engine: EngineNsis},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ws := workerSpec{
				Interactive: true, Engine: tc.engine, InstallerPath: installer, WorkingDir: root,
				Options: installOptions{SkipShortcuts: true, SkipExtras: true, VerifyRepack: true},
				Hidden:  true, Background: true,
			}
			rs, err := mainRunSpec(ws, []string{"core"})
			if err != nil {
				t.Fatalf("mainRunSpec: %v", err)
			}
			if rs.Path != installer || len(rs.Args) != 0 || rs.CmdLine != "" || rs.Tail != "" {
				t.Fatalf("interactive run carries silent launch parameters: %+v", rs)
			}
			if rs.Hidden || rs.Background {
				t.Fatalf("interactive run is hidden or backgrounded: %+v", rs)
			}
			if rs.Dir != root || rs.InstallerPath != installer {
				t.Fatalf("interactive run lost its paths: %+v", rs)
			}
		})
	}
}

func TestInteractiveRunSkipsComponentDiscovery(t *testing.T) {
	opts := installOptions{SkipExtras: true, SkipShortcuts: true}
	cases := []struct {
		name string
		in   discoverySpec
		want bool
	}{
		{name: "silent inno discovers", in: discoverySpec{Engine: EngineInno, Options: opts}, want: true},
		{name: "interactive inno does not", in: discoverySpec{Engine: EngineInno, Options: opts, Interactive: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldDiscoverComponents(tc.in); got != tc.want {
				t.Fatalf("shouldDiscoverComponents = %v, want %v", got, tc.want)
			}
		})
	}
	ws := workerSpec{Engine: EngineInno, Options: opts, Interactive: true}
	if !ws.discovery().Interactive {
		t.Fatal("worker spec does not hand Interactive to discovery")
	}
}

func TestSignedBrokerSpecCoversInteractive(t *testing.T) {
	for _, signed := range []bool{false, true} {
		dir, pin := brokerDirs(t)
		spec := goodSpec(pin)
		spec.Run = "fresh"
		spec.Interactive = signed
		mkText(t, spec.InstallerPath, "fixture")
		if err := writeSignedBrokerSpec(context.Background(), dir, spec, brokerTestPrivate); err != nil {
			t.Fatal(err)
		}
		read, found, err := readBrokerSpec(dir, pin, brokerTestPublic)
		if err != nil || !found {
			t.Fatalf("signed=%v: valid signature rejected: %v", signed, err)
		}
		read.Interactive = !signed
		if err := writeWorkerSpec(brokerSpecPath(dir), read); err != nil {
			t.Fatal(err)
		}
		if _, _, err := readBrokerSpec(dir, pin, brokerTestPublic); err == nil {
			t.Fatalf("signed=%v: broker accepted a spec with Interactive flipped after signing", signed)
		}
	}
}

func TestWorkerSpecHashCoversInteractive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "spec.json")
	digest, err := writeWorkerSpecDigest(path, workerSpec{ID: "i1", Interactive: false})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	flipped := strings.Replace(string(data), `"interactive": false`, `"interactive": true`, 1)
	if flipped == string(data) {
		t.Fatalf("spec file has no interactive field to flip: %s", data)
	}
	if err := writeWorkerFile(path, []byte(flipped)); err != nil {
		t.Fatal(err)
	}
	if _, err := readVerifiedWorkerSpec(path, digest); err == nil {
		t.Fatal("worker accepted a spec with Interactive flipped after the hash was taken")
	}
}

func TestWorkerStatePathMissingIsCoded(t *testing.T) {
	_, err := runElevated(context.Background(), runSpec{Path: "setup.exe", InstallerPath: "setup.exe", ID: "x"})
	if got := uierr.Code(err); got != "install.worker_state_missing" {
		t.Fatalf("runElevated without a state path = %v (code %q), want install.worker_state_missing", err, got)
	}
}

func TestServiceStartupInterruptsInteractiveRunWhoseWorkerSurvived(t *testing.T) {
	for _, tc := range []struct {
		name  string
		alive bool
	}{
		{name: "worker finished while the launcher was down"},
		{name: "worker still running", alive: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := mustServiceAt(t, t.TempDir())
			s.settings = newTestSettings(t)
			s.downloads = newFakeDownloads()
			s.library = &fakeRegistrar{}
			runner := &fakeRunner{}
			s.runner = runner

			installer := filepath.Join(t.TempDir(), "setup.exe")
			mkText(t, installer, "installer")
			const id = "manual1"
			item := Installation{
				ID: id, DownloadID: "d1", Name: "Game", Type: TypeExeInstaller,
				Status: StatusInstalling, Engine: EngineUnknown, Interactive: true,
				StartedAt: time.Now(), InstallerPath: installer,
			}
			if err := s.store.save([]Installation{item}); err != nil {
				t.Fatalf("seed store: %v", err)
			}
			state := workerState{PID: os.Getpid(), Done: !tc.alive}
			if err := writeWorkerState(s.workerStatePath(id), state); err != nil {
				t.Fatalf("write worker state: %v", err)
			}

			if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
				t.Fatalf("startup: %v", err)
			}
			t.Cleanup(func() {
				if err := s.ServiceShutdown(); err != nil {
					t.Errorf("shutdown: %v", err)
				}
			})

			got, ok := s.snapshot(id)
			if !ok {
				t.Fatal("installation vanished")
			}
			if got.Status != StatusInterrupted {
				t.Fatalf("status = %q, want %q: the snapshots an interactive run needs to finish are gone with the old launcher", got.Status, StatusInterrupted)
			}
			if n := len(runner.calls()); n != 0 {
				t.Fatalf("runner calls = %d, want 0", n)
			}
		})
	}
}
