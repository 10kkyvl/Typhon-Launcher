package install

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"typhon/internal/settings"
)

func TestInstallOptionsFromVerifyRepack(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  settings.Settings
		want bool
	}{
		{"zero settings skip verification", settings.Settings{}, false},
		{"enabled in settings", settings.Settings{InstallVerifyRepack: true}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := installOptionsFrom(tc.cfg).VerifyRepack; got != tc.want {
				t.Fatalf("VerifyRepack = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSilentSpecCarriesVerifyRepack(t *testing.T) {
	root := t.TempDir()
	item := Installation{Engine: EngineInno, InstallerPath: filepath.Join(root, "setup.exe"), Destination: filepath.Join(root, "Game")}
	for _, verify := range []bool{false, true} {
		spec, err := silentSpec(item, item.InstallerPath, "", installOptions{VerifyRepack: verify})
		if err != nil {
			t.Fatal(err)
		}
		if spec.VerifyRepack != verify || spec.Options.VerifyRepack != verify {
			t.Fatalf("verify=%v: spec.VerifyRepack=%v spec.Options.VerifyRepack=%v", verify, spec.VerifyRepack, spec.Options.VerifyRepack)
		}
	}
}

func TestMainRunSpecCarriesVerifyRepack(t *testing.T) {
	root := t.TempDir()
	for _, verify := range []bool{false, true} {
		ws := workerSpec{
			Engine: EngineInno, InstallerPath: filepath.Join(root, "setup.exe"), Destination: filepath.Join(root, "Game"),
			Options: installOptions{VerifyRepack: verify}, Hidden: true,
		}
		rs, err := mainRunSpec(ws, nil)
		if err != nil {
			t.Fatal(err)
		}
		if rs.VerifyRepack != verify {
			t.Fatalf("verify=%v: runSpec.VerifyRepack=%v", verify, rs.VerifyRepack)
		}
		if rs.Options != (installOptions{}) {
			t.Fatalf("worker main run must not repeat component discovery, Options = %+v", rs.Options)
		}
	}
}

func TestRunElevatedHandsVerifyRepackToWorker(t *testing.T) {
	for _, verify := range []bool{false, true} {
		dir := t.TempDir()
		statePath := filepath.Join(dir, "state.json")
		spec := runSpec{
			Path: `C:\fake\installer.exe`, ID: "vr", StatePath: statePath, CancelPath: filepath.Join(dir, "cancel"),
			Options: installOptions{VerifyRepack: verify}, VerifyRepack: verify,
		}
		var sent bool
		withWorkerSeams(t, func(launch runSpec) (workerHandle, error) {
			ws, err := readWorkerSpec(launch.Args[1])
			if err != nil {
				return nil, err
			}
			sent = ws.Options.VerifyRepack
			if err := writeWorkerState(statePath, workerState{Run: ws.Run, Done: true}); err != nil {
				return nil, err
			}
			return quickExitProcess(t), nil
		})
		if _, err := runElevated(context.Background(), spec); err != nil {
			t.Fatalf("runElevated: %v", err)
		}
		if sent != verify {
			t.Fatalf("worker received VerifyRepack=%v, want %v", sent, verify)
		}
	}
}

func TestWorkerSpecFileKeepsVerifyRepack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "spec.json")
	if err := writeWorkerSpec(path, workerSpec{ID: "v1", Options: installOptions{VerifyRepack: true}}); err != nil {
		t.Fatal(err)
	}
	got, err := readWorkerSpec(path)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Options.VerifyRepack {
		t.Fatalf("VerifyRepack lost in worker spec file: %+v", got.Options)
	}
}

func TestSignedBrokerSpecCoversVerifyRepack(t *testing.T) {
	dir, pin := brokerDirs(t)
	spec := goodSpec(pin)
	spec.Run = "fresh"
	if err := os.WriteFile(spec.InstallerPath, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeSignedBrokerSpec(context.Background(), dir, spec, brokerTestPrivate); err != nil {
		t.Fatal(err)
	}
	signed, found, err := readBrokerSpec(dir, pin, brokerTestPublic)
	if err != nil || !found {
		t.Fatalf("valid signature rejected: %v", err)
	}
	if signed.Options.VerifyRepack {
		t.Fatal("VerifyRepack appeared out of nowhere")
	}
	signed.Options.VerifyRepack = true
	if err := writeWorkerSpec(brokerSpecPath(dir), signed); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readBrokerSpec(dir, pin, brokerTestPublic); err == nil {
		t.Fatal("broker accepted a spec with VerifyRepack flipped after signing")
	}
}
