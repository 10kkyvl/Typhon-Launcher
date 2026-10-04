package install

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"typhon/internal/uierr"
)

func TestRunWorkerRefusesAnInstallerItCannotPin(t *testing.T) {
	cases := []struct {
		name   string
		digest func(t *testing.T, installer string) string
		tamper func(t *testing.T, installer string)
		reason error
		code   string
	}{
		{
			name:   "installer rewritten with content of the same size",
			digest: fileDigest,
			tamper: func(t *testing.T, installer string) { mkText(t, installer, "installer fixturE") },
			reason: errInstallerChanged,
			code:   "install.installer_changed",
		},
		{
			name:   "installer removed",
			digest: fileDigest,
			tamper: func(t *testing.T, installer string) {
				if err := os.Remove(installer); err != nil {
					t.Fatalf("remove installer: %v", err)
				}
			},
			reason: fs.ErrNotExist,
		},
		{
			name:   "spec without the installer digest",
			digest: func(*testing.T, string) string { return "" },
			tamper: func(*testing.T, string) {},
			reason: errInstallerHashMissing,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			installer := installerFixture(t, dir)
			specPath := filepath.Join(dir, "spec.json")
			statePath := filepath.Join(dir, "state.json")
			digest, err := writeWorkerSpecDigest(specPath, workerSpec{
				ID: "pin", Run: "run-pin", InstallerPath: installer, InstallerSHA256: tc.digest(t, installer), StatePath: statePath,
			})
			if err != nil {
				t.Fatalf("writeWorkerSpecDigest: %v", err)
			}
			tc.tamper(t, installer)
			calls := countWorkerInstalls(t)

			err = RunWorker(specPath, digest)

			if !errors.Is(err, tc.reason) {
				t.Fatalf("RunWorker = %v, want %v", err, tc.reason)
			}
			if tc.code != "" && uierr.Code(err) != tc.code {
				t.Fatalf("uierr.Code = %q, want %q", uierr.Code(err), tc.code)
			}
			if *calls != 0 {
				t.Fatalf("workerInstall called %d times, the installer must not start", *calls)
			}
			state, found, err := readWorkerState(statePath)
			if err != nil || !found || !state.Done || state.Error == "" || state.Run != "run-pin" {
				t.Fatalf("state = %+v found = %v err = %v, want a done state of run-pin carrying the refusal", state, found, err)
			}
		})
	}
}

func TestRunElevatedSendsTheInstallerDigest(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	installer := installerFixture(t, dir)
	spec := runSpec{Path: installer, InstallerPath: installer, ID: "dg2", StatePath: statePath, CancelPath: filepath.Join(dir, "cancel")}

	var sent string
	withWorkerSeams(t, func(launch runSpec) (workerHandle, error) {
		ws, err := readVerifiedWorkerSpec(ParseWorkerArgs(launch.Args[1:]))
		if err != nil {
			return nil, err
		}
		sent = ws.InstallerSHA256
		if err := writeWorkerState(statePath, workerState{Run: ws.Run, Done: true}); err != nil {
			return nil, err
		}
		return quickExitProcess(t), nil
	})

	if _, err := runElevated(context.Background(), spec); err != nil {
		t.Fatalf("runElevated: %v", err)
	}
	if want := fileDigest(t, installer); sent != want {
		t.Fatalf("installer digest in the spec = %q, want %q", sent, want)
	}
}

func TestRunElevatedDoesNotStartWorkerWithoutInstallerDigest(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	cases := []struct {
		name      string
		ctx       context.Context
		installer func(t *testing.T, dir string) string
		reason    error
	}{
		{name: "installer path not set", ctx: context.Background(), installer: func(*testing.T, string) string { return "" }, reason: errEmptyInstallerPath},
		{name: "installer missing", ctx: context.Background(), installer: func(_ *testing.T, dir string) string { return filepath.Join(dir, "gone.exe") }, reason: fs.ErrNotExist},
		{name: "cancelled while hashing", ctx: cancelled, installer: installerFixture, reason: context.Canceled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			installer := tc.installer(t, dir)
			spec := runSpec{Path: `C:\fake\installer.exe`, InstallerPath: installer, ID: "nd", StatePath: filepath.Join(dir, "state.json"), CancelPath: filepath.Join(dir, "cancel")}
			started := false
			withWorkerSeams(t, func(runSpec) (workerHandle, error) {
				started = true
				return quickExitProcess(t), nil
			})

			_, err := runElevated(tc.ctx, spec)

			if !errors.Is(err, tc.reason) {
				t.Fatalf("runElevated = %v, want %v", err, tc.reason)
			}
			if started {
				t.Fatal("worker started without a verified installer digest")
			}
			if _, statErr := os.Stat(workerSpecFilePath(dir, spec.ID)); !errors.Is(statErr, fs.ErrNotExist) {
				t.Fatalf("spec file stat = %v, want no spec written", statErr)
			}
		})
	}
}

func TestWorkerRefusesInstallerSwappedAfterHandoff(t *testing.T) {
	dir := t.TempDir()
	installer := installerFixture(t, dir)
	spec := runSpec{
		Path: installer, InstallerPath: installer, Engine: EngineInno, Destination: filepath.Join(dir, "Game"),
		ID: "swap", StatePath: filepath.Join(dir, "state.json"), CancelPath: filepath.Join(dir, "cancel"),
	}
	calls := countWorkerInstalls(t)
	withWorkerSeams(t, func(launch runSpec) (workerHandle, error) {
		mkText(t, installer, "payload planted by an unelevated process")
		if err := RunWorker(ParseWorkerArgs(launch.Args[1:])); !errors.Is(err, errInstallerChanged) {
			t.Errorf("RunWorker = %v, want errInstallerChanged", err)
		}
		return quickExitProcess(t), nil
	})

	_, err := runElevated(context.Background(), spec)

	if err == nil || !strings.Contains(err.Error(), "установщик изменился") {
		t.Fatalf("runElevated = %v, want the worker's refusal", err)
	}
	if *calls != 0 {
		t.Fatalf("workerInstall called %d times, the swapped installer must not start", *calls)
	}
}

func TestPinInstallerStopsOnCancelledContext(t *testing.T) {
	installer := installerFixture(t, t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	f, _, err := pinInstaller(ctx, workerSpec{InstallerPath: installer, InstallerSHA256: fileDigest(t, installer)})

	if !errors.Is(err, context.Canceled) || f != nil {
		t.Fatalf("pinInstaller = (%v, %v), want no handle and context.Canceled", f, err)
	}
}

func TestBrokerReportsChangedInstallerAsOutsidePin(t *testing.T) {
	dir, pin := brokerDirs(t)
	spec := goodSpec(pin)
	spec.Run = "changed"
	mkText(t, spec.InstallerPath, "installer fixture")
	if err := writeSignedBrokerSpec(context.Background(), dir, spec, brokerTestPrivate); err != nil {
		t.Fatalf("writeSignedBrokerSpec: %v", err)
	}
	signed, found, err := readBrokerSpec(dir, pin, brokerTestPublic)
	if err != nil || !found {
		t.Fatalf("readBrokerSpec: found = %v, err = %v", found, err)
	}
	mkText(t, spec.InstallerPath, "installer fixturE")
	calls := countWorkerInstalls(t)

	err = runSignedBrokerSpec(signed)

	if !errors.Is(err, errBrokerOutsidePin) || !errors.Is(err, errInstallerChanged) {
		t.Fatalf("runSignedBrokerSpec = %v, want errBrokerOutsidePin wrapping errInstallerChanged", err)
	}
	if *calls != 0 {
		t.Fatalf("workerInstall called %d times, the changed installer must not start", *calls)
	}
}
