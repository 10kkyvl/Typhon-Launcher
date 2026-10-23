package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// exeCase is a cache that holds a rebuilt executable the state record calls
// ready, and a launcher folder with the build it will replace.
type exeCase struct {
	configDir string
	exePath   string
	target    string
	oldBytes  []byte
	newBytes  []byte
}

func newExeCase(t *testing.T) *exeCase {
	t.Helper()
	c := &exeCase{
		configDir: t.TempDir(),
		oldBytes:  testBytes(200, 8<<10),
		newBytes:  testBytes(201, 9<<10),
	}
	var err error
	c.exePath, err = ArtifactPath(c.configDir, "1.3.0", portableExeName)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, c.exePath, c.newBytes)
	c.target = filepath.Join(t.TempDir(), "typhon.exe")
	writeTestFile(t, c.target, c.oldBytes)
	c.record(t, Artifact{
		OS: "windows", Arch: "amd64", Kind: KindExe, Name: portableExeName,
		Size: int64(len(c.newBytes)), SHA256: hashOf(c.newBytes),
	}, c.exePath)
	return c
}

func (c *exeCase) record(t *testing.T, art Artifact, readyPath string) {
	t.Helper()
	err := mustStore(t, c.configDir).Save(stored{AvailableVersion: "1.3.0", Artifact: &art, ReadyPath: readyPath})
	if err != nil {
		t.Fatalf("save the ready record: %v", err)
	}
}

func (c *exeCase) targetBytes(t *testing.T) []byte {
	t.Helper()
	got, err := os.ReadFile(c.target)
	if err != nil {
		t.Fatalf("read the launcher: %v", err)
	}
	return got
}

type registryCalls struct {
	set      []string
	reverted int
	setErr   error
}

func fakeRegistry(t *testing.T) *registryCalls {
	t.Helper()
	calls := &registryCalls{}
	restore := updateDisplayVersion
	updateDisplayVersion = func(ver string) (func() error, error) {
		if calls.setErr != nil {
			return nil, calls.setErr
		}
		calls.set = append(calls.set, ver)
		return func() error { calls.reverted++; return nil }, nil
	}
	t.Cleanup(func() { updateDisplayVersion = restore })
	return calls
}

func TestApplyReadyExe(t *testing.T) {
	tests := []struct {
		name         string
		portable     bool
		prepare      func(t *testing.T, c *exeCase)
		ctx          func() context.Context
		target       func(c *exeCase) string
		registryErr  error
		wantErr      error
		wantAnyErr   bool
		wantSwapped  bool
		wantRegistry []string
		wantReverted int
	}{
		{
			name: "installed launcher is replaced and the uninstall entry follows", wantSwapped: true,
			wantRegistry: []string{"1.3.0"},
		},
		{
			name: "portable launcher is replaced without touching the registry", portable: true, wantSwapped: true,
		},
		{
			name: "a rebuilt file damaged after it was checked is refused",
			prepare: func(t *testing.T, c *exeCase) {
				damaged := append([]byte(nil), c.newBytes...)
				damaged[len(damaged)/2] ^= 0xff
				writeTestFile(t, c.exePath, damaged)
			},
			wantErr: ErrHashMismatch,
		},
		{
			name: "a truncated rebuilt file is refused",
			prepare: func(t *testing.T, c *exeCase) {
				writeTestFile(t, c.exePath, c.newBytes[:len(c.newBytes)-1])
			},
			wantErr: ErrSizeMismatch,
		},
		{
			name: "no record of a ready update",
			prepare: func(t *testing.T, c *exeCase) {
				if err := os.Remove(filepath.Join(c.configDir, "selfupdate", "state.json")); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: ErrNotReady,
		},
		{
			name: "the record names an installer",
			prepare: func(t *testing.T, c *exeCase) {
				c.record(t, Artifact{OS: "windows", Arch: "amd64", Kind: KindInstaller, Name: portableExeName, Size: int64(len(c.newBytes)), SHA256: hashOf(c.newBytes)}, c.exePath)
			},
			wantErr: ErrNotReady,
		},
		{
			name: "the record names another file",
			prepare: func(t *testing.T, c *exeCase) {
				c.record(t, Artifact{OS: "windows", Arch: "amd64", Kind: KindExe, Name: portableExeName, Size: int64(len(c.newBytes)), SHA256: hashOf(c.newBytes)}, filepath.Join(filepath.Dir(c.exePath), "other.exe"))
			},
			wantErr: ErrNotReady,
		},
		{
			name: "a ready file outside the cache is refused",
			prepare: func(t *testing.T, c *exeCase) {
				outside := filepath.Join(t.TempDir(), portableExeName)
				writeTestFile(t, outside, c.newBytes)
				c.exePath = outside
				c.record(t, Artifact{OS: "windows", Arch: "amd64", Kind: KindExe, Name: portableExeName, Size: int64(len(c.newBytes)), SHA256: hashOf(c.newBytes)}, outside)
			},
			wantErr: errNotCached,
		},
		{
			name: "a ready file that is a directory is refused",
			prepare: func(t *testing.T, c *exeCase) {
				if err := os.Remove(c.exePath); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(c.exePath, 0o700); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: errNotCached,
		},
		{
			name: "a missing ready file is refused",
			prepare: func(t *testing.T, c *exeCase) {
				if err := os.Remove(c.exePath); err != nil {
					t.Fatal(err)
				}
			},
			wantAnyErr: true,
		},
		{
			name:        "the uninstall entry cannot be updated",
			registryErr: errors.New("registry is locked"), wantAnyErr: true,
		},
		{
			name:       "a relative launcher path undoes the uninstall entry",
			target:     func(*exeCase) string { return "typhon.exe" },
			wantAnyErr: true, wantRegistry: []string{"1.3.0"}, wantReverted: 1,
		},
		{
			name: "a cancelled context swaps nothing",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			wantErr: context.Canceled,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newExeCase(t)
			calls := fakeRegistry(t)
			calls.setErr = tt.registryErr
			if tt.prepare != nil {
				tt.prepare(t, c)
			}
			ctx := context.Background()
			if tt.ctx != nil {
				ctx = tt.ctx()
			}
			target := c.target
			if tt.target != nil {
				target = tt.target(c)
			}

			err := applyReadyExe(ctx, c.configDir, c.exePath, target, "1.3.0", tt.portable)

			failing := tt.wantErr != nil || tt.wantAnyErr
			switch {
			case !failing && err != nil:
				t.Fatalf("applyReadyExe() error = %v", err)
			case failing && err == nil:
				t.Fatal("applyReadyExe() error = nil")
			case tt.wantErr != nil && !errors.Is(err, tt.wantErr):
				t.Fatalf("applyReadyExe() error = %v, want %v", err, tt.wantErr)
			}
			got := c.targetBytes(t)
			if tt.wantSwapped && string(got) != string(c.newBytes) {
				t.Fatal("the launcher was not replaced by the rebuilt executable")
			}
			if !tt.wantSwapped && string(got) != string(c.oldBytes) {
				t.Fatal("a failed apply changed the launcher")
			}
			if len(calls.set) != len(tt.wantRegistry) || (len(tt.wantRegistry) > 0 && calls.set[0] != tt.wantRegistry[0]) {
				t.Fatalf("uninstall entry set to %v, want %v", calls.set, tt.wantRegistry)
			}
			if calls.reverted != tt.wantReverted {
				t.Fatalf("uninstall entry reverted %d times, want %d", calls.reverted, tt.wantReverted)
			}
			if left := tempNames(t, filepath.Dir(c.target)); len(left) != 0 {
				t.Fatalf("temp files left beside the launcher: %v", left)
			}
		})
	}
}

func TestRunWorkerSwapsAReadyExecutable(t *testing.T) {
	shortWorkerTimeouts(t)
	configDir := testConfigDir(t)
	c := &exeCase{configDir: configDir, oldBytes: testBytes(210, 8<<10), newBytes: testBytes(211, 9<<10)}
	var err error
	c.exePath, err = ArtifactPath(configDir, "1.3.0", portableExeName)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, c.exePath, c.newBytes)
	c.target = filepath.Join(t.TempDir(), "typhon.exe")
	writeTestFile(t, c.target, c.oldBytes)
	art := Artifact{OS: "windows", Arch: "amd64", Kind: KindExe, Name: portableExeName, Size: int64(len(c.newBytes)), SHA256: hashOf(c.newBytes)}
	c.record(t, art, c.exePath)

	var order []string
	fakes := &fakePrimitives{aliveUntil: 1}
	installFakePrimitives(t, fakes)
	restoreSave, restoreSupported := saveCurrent, rollbackSupported
	saveCurrent = func(_ context.Context, _, exe, ver string) error {
		order = append(order, "snapshot "+ver)
		if got, err := os.ReadFile(exe); err != nil || string(got) != string(c.oldBytes) {
			t.Errorf("the snapshot ran when the launcher was already replaced (err=%v)", err)
		}
		return nil
	}
	rollbackSupported = func() error { return nil }
	restoreInstaller := applyInstaller
	applyInstaller = func(context.Context, string, string, string) error {
		t.Error("an installer was run for a rebuilt executable")
		return nil
	}
	t.Cleanup(func() {
		saveCurrent, rollbackSupported, applyInstaller = restoreSave, restoreSupported, restoreInstaller
	})
	calls := fakeRegistry(t)

	specPath := writeWorkerSpec(t, configDir, updateSpec{
		Kind: specKindExe, InstallerPath: c.exePath, InstallDir: filepath.Dir(c.target), ParentPID: 4242,
		RelaunchPath: c.target, Version: "1.3.0", FromVersion: "1.2.0", Language: "en",
	})
	if err := runWorker(specPath, quietReporter); err != nil {
		t.Fatalf("runWorker() error = %v", err)
	}

	if string(c.targetBytes(t)) != string(c.newBytes) {
		t.Fatal("the launcher was not replaced")
	}
	if len(order) != 1 || order[0] != "snapshot 1.2.0" {
		t.Fatalf("snapshot calls = %v, want one for the version being left", order)
	}
	if len(calls.set) != 1 || calls.set[0] != "1.3.0" {
		t.Fatalf("uninstall entry = %v, want 1.3.0", calls.set)
	}
	if got := readWorkerOutcome(t, configDir); !got.OK || got.Version != "1.3.0" || got.Error != "" {
		t.Fatalf("outcome = %+v, want a success for 1.3.0", got)
	}
	if len(fakes.relaunchCalls) != 1 || fakes.relaunchCalls[0] != c.target {
		t.Fatalf("relaunch calls = %v", fakes.relaunchCalls)
	}
}

func TestRunWorkerLeavesTheLauncherWhenTheReadyExecutableIsWrong(t *testing.T) {
	shortWorkerTimeouts(t)
	configDir := testConfigDir(t)
	c := &exeCase{configDir: configDir, oldBytes: testBytes(220, 8<<10), newBytes: testBytes(221, 9<<10)}
	var err error
	c.exePath, err = ArtifactPath(configDir, "1.3.0", portableExeName)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, c.exePath, append(append([]byte(nil), c.newBytes...), 0))
	c.target = filepath.Join(t.TempDir(), "typhon.exe")
	writeTestFile(t, c.target, c.oldBytes)
	c.record(t, Artifact{OS: "windows", Arch: "amd64", Kind: KindExe, Name: portableExeName, Size: int64(len(c.newBytes)), SHA256: hashOf(c.newBytes)}, c.exePath)

	fakes := &fakePrimitives{}
	installFakePrimitives(t, fakes)
	calls := fakeRegistry(t)
	specPath := writeWorkerSpec(t, configDir, updateSpec{
		Kind: specKindExe, InstallerPath: c.exePath, InstallDir: filepath.Dir(c.target), ParentPID: 4242,
		RelaunchPath: c.target, Version: "1.3.0", FromVersion: "1.2.0", Portable: true,
	})

	err = runWorker(specPath, quietReporter)
	if !errors.Is(err, ErrSizeMismatch) {
		t.Fatalf("runWorker() error = %v, want ErrSizeMismatch", err)
	}

	if string(c.targetBytes(t)) != string(c.oldBytes) {
		t.Fatal("the launcher changed although the rebuilt executable failed its check")
	}
	if got := readWorkerOutcome(t, configDir); got.OK || got.Error == "" {
		t.Fatalf("outcome = %+v, want a recorded failure", got)
	}
	if len(fakes.relaunchCalls) != 1 || len(calls.set) != 0 {
		t.Fatalf("relaunch %v, registry %v: the old launcher must come back and the entry stay as it was", fakes.relaunchCalls, calls.set)
	}
}

// A worker that dies after it wrote the replacement beside the launcher and
// before it renamed leaves the old launcher, a temp file and an intact ready
// record. The next start has to clean up and still be able to apply.
func TestInterruptedSwapLeavesALauncherThatStartsAndAnUpdateThatApplies(t *testing.T) {
	c := newExeCase(t)
	fakeRegistry(t)
	torn := filepath.Join(filepath.Dir(c.target), ".tmp-torn")
	writeTestFile(t, torn, c.newBytes[:len(c.newBytes)/2])
	ageFile(t, torn, 2*swapLeftoverAge)

	recoverSwap(c.target)

	if _, err := os.Stat(torn); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the torn replacement survived the start: %v", err)
	}
	if string(c.targetBytes(t)) != string(c.oldBytes) {
		t.Fatal("recovery changed the launcher")
	}
	s := &Service{dir: c.configDir, notes: mustNotesStore(t, c.configDir), store: mustStore(t, c.configDir), client: mustQuietClient(t), currentVersion: "1.2.0"}
	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup() error = %v", err)
	}
	t.Cleanup(func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Errorf("ServiceShutdown: %v", err)
		}
	})
	if got := s.GetStatus(); got.State != StateReady {
		t.Fatalf("state after the restart = %v, want the rebuilt executable still ready", got.State)
	}

	if err := applyReadyExe(context.Background(), c.configDir, c.exePath, c.target, "1.3.0", false); err != nil {
		t.Fatalf("applyReadyExe() after the restart: %v", err)
	}
	if string(c.targetBytes(t)) != string(c.newBytes) {
		t.Fatal("the launcher was not replaced after the restart")
	}
}

func TestServiceStartupVetsAReadyExecutableLikeAnInstaller(t *testing.T) {
	tests := []struct {
		name      string
		damage    func(t *testing.T, c *exeCase)
		wantReady bool
	}{
		{"intact file stays ready", func(*testing.T, *exeCase) {}, true},
		{"file with a flipped byte is dropped", func(t *testing.T, c *exeCase) {
			damaged := append([]byte(nil), c.newBytes...)
			damaged[3] ^= 1
			writeTestFile(t, c.exePath, damaged)
		}, false},
		{"truncated file is dropped", func(t *testing.T, c *exeCase) {
			writeTestFile(t, c.exePath, c.newBytes[:10])
		}, false},
		{"missing file is dropped", func(t *testing.T, c *exeCase) {
			if err := os.Remove(c.exePath); err != nil {
				t.Fatal(err)
			}
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newExeCase(t)
			tt.damage(t, c)

			s := restartedService(t, c.configDir, "1.2.0")

			state := s.GetStatus().State
			if tt.wantReady && (state != StateReady || s.readyArtifact == nil || s.readyArtifact.Kind != KindExe) {
				t.Fatalf("state = %v, ready = %+v, want a ready rebuilt executable", state, s.readyArtifact)
			}
			if !tt.wantReady {
				if state == StateReady || s.readyPath != "" {
					t.Fatalf("state = %v, ready path %q: a damaged rebuilt executable must not stay ready", state, s.readyPath)
				}
				saved, err := s.store.Load()
				if err != nil {
					t.Fatal(err)
				}
				if saved.Artifact != nil || saved.ReadyPath != "" {
					t.Fatalf("the record still points at %q", saved.ReadyPath)
				}
			}
		})
	}
}

func TestApplyUpdateStartsTheWorkerForARebuiltExecutable(t *testing.T) {
	tests := []struct {
		name     string
		portable bool
		kind     Kind
		wantKind string
		wantErr  error
	}{
		{"installed launcher", false, KindExe, specKindExe, nil},
		{"portable launcher", true, KindExe, specKindExe, nil},
		{"installed launcher with an installer", false, KindInstaller, "", nil},
		{"portable launcher with an installer is still refused", true, KindInstaller, "", ErrNotReady},
		{"installed launcher with a zip is still refused", false, KindPortable, "", ErrNotReady},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			readyPath := filepath.Join(dir, "selfupdate", "1.3.0", portableExeName)
			writeTestFile(t, readyPath, []byte("rebuilt"))
			s := &Service{dir: dir, notes: mustNotesStore(t, dir), store: mustStore(t, dir), currentVersion: "1.2.0", portable: tt.portable,
				readyPath: readyPath, readyArtifact: &Artifact{Kind: tt.kind}}
			s.status = Status{State: StateReady, CurrentVersion: "1.2.0", AvailableVersion: "1.3.0"}
			var specPath string
			restore := startWorker
			startWorker = func(_, spec string) error { specPath = spec; return nil }
			t.Cleanup(func() { startWorker = restore })

			err := s.ApplyUpdate("en")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ApplyUpdate() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			spec, err := readUpdateSpec(specPath)
			if err != nil {
				t.Fatalf("readUpdateSpec: %v", err)
			}
			if spec.Kind != tt.wantKind || spec.InstallerPath != readyPath || spec.Version != "1.3.0" || spec.Portable != tt.portable {
				t.Fatalf("spec = %+v, want kind %q for %s", spec, tt.wantKind, readyPath)
			}
		})
	}
}

func TestRollbackDropsAReadyRebuiltExecutable(t *testing.T) {
	tests := []struct {
		name      string
		art       *Artifact
		wantReady bool
	}{
		{"a rebuilt executable is dropped", &Artifact{Kind: KindExe, Name: portableExeName, Size: 1, SHA256: hashOf([]byte("x"))}, false},
		{"an installer is kept", &Artifact{Kind: KindInstaller, Name: "setup.exe", Size: 1, SHA256: hashOf([]byte("x"))}, true},
		{"no record stays empty", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			st := mustStore(t, dir)
			readyPath := ""
			if tt.art != nil {
				readyPath = filepath.Join(dir, "selfupdate", "1.4.0", tt.art.Name)
			}
			if err := st.Save(stored{AvailableVersion: "1.4.0", Artifact: tt.art, ReadyPath: readyPath, CheckedAt: time.Now()}); err != nil {
				t.Fatal(err)
			}

			if err := markSkipped(dir, "1.3.0"); err != nil {
				t.Fatalf("markSkipped() error = %v", err)
			}

			got, err := st.Load()
			if err != nil {
				t.Fatal(err)
			}
			if got.SkippedVersion != "1.3.0" || got.AvailableVersion != "1.4.0" {
				t.Fatalf("record = %+v, want the skip noted and the offered version kept", got)
			}
			if ready := got.Artifact != nil && got.ReadyPath != ""; ready != tt.wantReady {
				t.Fatalf("ready = %v, want %v: the launcher a rollback returns to may not know a rebuilt executable", ready, tt.wantReady)
			}
		})
	}
}
