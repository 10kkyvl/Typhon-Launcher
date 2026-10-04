package install

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"typhon/internal/uierr"
)

func fileDigest(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func countWorkerInstalls(t *testing.T) *int {
	t.Helper()
	calls := new(int)
	previous := workerInstall
	workerInstall = func(context.Context, workerSpec, []string) (int, error) {
		*calls++
		return 0, nil
	}
	t.Cleanup(func() { workerInstall = previous })
	return calls
}

func TestRunWorkerRejectsUnverifiedSpec(t *testing.T) {
	cases := []struct {
		name   string
		setup  func(t *testing.T, dir, specPath string) string
		reason error
	}{
		{
			name: "spec replaced after the launcher wrote it",
			setup: func(t *testing.T, dir, specPath string) string {
				digest, err := writeWorkerSpecDigest(specPath, workerSpec{ID: "w1", Run: "r1", InstallerPath: filepath.Join(dir, "setup.exe"), StatePath: filepath.Join(dir, "state.json")})
				if err != nil {
					t.Fatalf("writeWorkerSpecDigest: %v", err)
				}
				forged := workerSpec{ID: "w1", Run: "r1", InstallerPath: filepath.Join(dir, "evil.exe"), StatePath: filepath.Join(dir, "attacker-state.json")}
				if err := writeWorkerSpec(specPath, forged); err != nil {
					t.Fatalf("write forged spec: %v", err)
				}
				return digest
			},
			reason: errWorkerSpecHashMismatch,
		},
		{
			name: "digest argument is absent",
			setup: func(t *testing.T, dir, specPath string) string {
				if err := writeWorkerSpec(specPath, workerSpec{ID: "w2", InstallerPath: filepath.Join(dir, "setup.exe"), StatePath: filepath.Join(dir, "state.json")}); err != nil {
					t.Fatalf("writeWorkerSpec: %v", err)
				}
				return ""
			},
			reason: errWorkerSpecHashMissing,
		},
		{
			name: "digest is not hex",
			setup: func(t *testing.T, dir, specPath string) string {
				if err := writeWorkerSpec(specPath, workerSpec{ID: "w3", InstallerPath: filepath.Join(dir, "setup.exe"), StatePath: filepath.Join(dir, "state.json")}); err != nil {
					t.Fatalf("writeWorkerSpec: %v", err)
				}
				return strings.Repeat("z", sha256.Size*2)
			},
			reason: errWorkerSpecHashInvalid,
		},
		{
			name: "digest has the wrong length",
			setup: func(t *testing.T, dir, specPath string) string {
				if err := writeWorkerSpec(specPath, workerSpec{ID: "w4", InstallerPath: filepath.Join(dir, "setup.exe"), StatePath: filepath.Join(dir, "state.json")}); err != nil {
					t.Fatalf("writeWorkerSpec: %v", err)
				}
				return "abcd"
			},
			reason: errWorkerSpecHashInvalid,
		},
		{
			name: "digest belongs to other bytes",
			setup: func(t *testing.T, dir, specPath string) string {
				if err := writeWorkerSpec(specPath, workerSpec{ID: "w5", InstallerPath: filepath.Join(dir, "setup.exe"), StatePath: filepath.Join(dir, "state.json")}); err != nil {
					t.Fatalf("writeWorkerSpec: %v", err)
				}
				sum := sha256.Sum256([]byte("other"))
				return hex.EncodeToString(sum[:])
			},
			reason: errWorkerSpecHashMismatch,
		},
		{
			name: "spec file exceeds the size limit",
			setup: func(t *testing.T, dir, specPath string) string {
				big := []byte(`{"id":"w6","installerPath":"` + strings.Repeat("a", maxWorkerSpecBytes) + `"}`)
				if err := os.WriteFile(specPath, big, 0o600); err != nil {
					t.Fatalf("write big spec: %v", err)
				}
				return fileDigest(t, specPath)
			},
			reason: errWorkerSpecTooLarge,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			specPath := filepath.Join(dir, "spec.json")
			digest := tc.setup(t, dir, specPath)
			calls := countWorkerInstalls(t)

			err := RunWorker(specPath, digest)

			if !errors.Is(err, ErrWorkerSpecRejected) {
				t.Fatalf("RunWorker = %v, want ErrWorkerSpecRejected", err)
			}
			if !errors.Is(err, tc.reason) {
				t.Fatalf("RunWorker = %v, want reason %v", err, tc.reason)
			}
			if got := uierr.Code(err); got != "install.worker_spec_rejected" {
				t.Fatalf("uierr.Code = %q, want install.worker_spec_rejected", got)
			}
			if *calls != 0 {
				t.Fatalf("workerInstall called %d times, the installer must not start", *calls)
			}
			for _, name := range []string{"state.json", "attacker-state.json"} {
				if _, statErr := os.Stat(filepath.Join(dir, name)); !errors.Is(statErr, fs.ErrNotExist) {
					t.Fatalf("%s: stat = %v, a rejected worker must not write to a path taken from the spec", name, statErr)
				}
			}
		})
	}
}

func TestRunWorkerRunsSpecWithMatchingDigest(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "spec.json")
	statePath := filepath.Join(dir, "state.json")
	installer := filepath.Join(dir, "setup.exe")
	digest, err := writeWorkerSpecDigest(specPath, workerSpec{ID: "ok", Run: "run-1", InstallerPath: installer, StatePath: statePath})
	if err != nil {
		t.Fatalf("writeWorkerSpecDigest: %v", err)
	}
	for _, tc := range []struct {
		name   string
		digest string
	}{
		{name: "lower case", digest: digest},
		{name: "upper case", digest: strings.ToUpper(digest)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got workerSpec
			calls := 0
			previous := workerInstall
			workerInstall = func(_ context.Context, spec workerSpec, _ []string) (int, error) {
				calls++
				got = spec
				return 0, nil
			}
			t.Cleanup(func() { workerInstall = previous })

			if err := RunWorker(specPath, tc.digest); err != nil {
				t.Fatalf("RunWorker: %v", err)
			}
			if calls != 1 || got.InstallerPath != installer {
				t.Fatalf("workerInstall calls = %d, spec = %+v, want one run of %s", calls, got, installer)
			}
			state, found, err := readWorkerState(statePath)
			if err != nil || !found || !state.Done || state.Error != "" || state.Run != "run-1" {
				t.Fatalf("state = %+v found = %v err = %v, want a done state of run-1", state, found, err)
			}
		})
	}
}

func TestParseWorkerArgs(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantPath   string
		wantDigest string
	}{
		{name: "spec and digest", args: []string{"spec.json", "--spec-sha256", "ab12"}, wantPath: "spec.json", wantDigest: "ab12"},
		{name: "no digest flag", args: []string{"spec.json"}, wantPath: "spec.json"},
		{name: "flag without value", args: []string{"spec.json", "--spec-sha256"}, wantPath: "spec.json"},
		{name: "empty", args: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path, digest := ParseWorkerArgs(tc.args)
			if path != tc.wantPath || digest != tc.wantDigest {
				t.Fatalf("ParseWorkerArgs(%v) = (%q, %q), want (%q, %q)", tc.args, path, digest, tc.wantPath, tc.wantDigest)
			}
		})
	}
}

func TestRunElevatedPassesDigestOfTheWrittenSpec(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	spec := runSpec{Path: `C:\fake\installer.exe`, ID: "dg1", StatePath: statePath, CancelPath: filepath.Join(dir, "cancel")}

	withWorkerSeams(t, func(launch runSpec) (workerHandle, error) {
		if len(launch.Args) != 4 || launch.Args[0] != installWorkerFlag {
			t.Fatalf("launch args = %v, want [%s <spec> %s <sha256>]", launch.Args, installWorkerFlag, workerSpecHashFlag)
		}
		path, digest := ParseWorkerArgs(launch.Args[1:])
		ws, err := readVerifiedWorkerSpec(path, digest)
		if err != nil {
			t.Fatalf("worker cannot verify the spec the launcher wrote: %v", err)
		}
		if err := writeWorkerState(statePath, workerState{Run: ws.Run, Done: true}); err != nil {
			return nil, err
		}
		return quickExitProcess(t), nil
	})

	if _, err := runElevated(context.Background(), spec); err != nil {
		t.Fatalf("runElevated: %v", err)
	}
}

func TestRunElevatedReportsRejectedWorker(t *testing.T) {
	dir := t.TempDir()
	spec := runSpec{Path: `C:\fake\installer.exe`, ID: "rj1", StatePath: filepath.Join(dir, "state.json"), CancelPath: filepath.Join(dir, "cancel")}

	withWorkerSeams(t, func(runSpec) (workerHandle, error) {
		if runtime.GOOS == "windows" {
			cmdExe := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
			return startStandInProcess(t, cmdExe, "/C", "exit", strconv.Itoa(WorkerSpecRejectedExit)), nil
		}
		return startStandInProcess(t, "/bin/sh", "-c", "exit "+strconv.Itoa(WorkerSpecRejectedExit)), nil
	})

	_, err := runElevated(context.Background(), spec)
	if !errors.Is(err, ErrWorkerSpecRejected) {
		t.Fatalf("runElevated = %v, want ErrWorkerSpecRejected", err)
	}
	if errors.Is(err, errWorkerNotFinished) || errors.Is(err, errInstallerNotConfirmedStopped) {
		t.Fatalf("runElevated = %v, a rejected spec must not collapse into the not-finished errors", err)
	}
}
