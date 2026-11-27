//go:build devmock && !windows

package selfupdate

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestDevmockUpdateModeFollowsTheMockSource(t *testing.T) {
	for _, tc := range []struct {
		name     string
		url      string
		portable bool
		want     bool
	}{
		{name: "portable, mock source", url: "http://127.0.0.1:8099", portable: true, want: false},
		{name: "portable, blank mock source", url: "  ", portable: true, want: true},
		{name: "portable, no mock source", url: "", portable: true, want: true},
		{name: "installed, mock source", url: "http://127.0.0.1:8099", portable: false, want: false},
		{name: "installed, no mock source", url: "", portable: false, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(manifestURLEnv, tc.url)
			if got, err := updatesPortable(tc.portable); err != nil || got != tc.want {
				t.Fatalf("updatesPortable(%v) with %s=%q = %v, %v; want %v", tc.portable, manifestURLEnv, tc.url, got, err, tc.want)
			}
		})
	}
}

// The test app runs in portable mode for its own single-instance id and data
// folder; pointed at tools/devrelease it must still find, queue and hand the
// worker the plain launcher binary the release source publishes.
func TestPortableDevmockLauncherTakesTheMockReleaseAllTheWay(t *testing.T) {
	priv, pub := testKeyPair(t)
	m := Manifest{
		Version:     "1.2.3",
		PublishedAt: time.Now(),
		Artifacts: []Artifact{{
			OS: runtime.GOOS, Arch: runtime.GOARCH, Kind: KindInstaller, Name: "typhon-devmock",
			URL: "http://127.0.0.1:8099/launcher/download/1.2.3/typhon-devmock", Size: 1024, SHA256: strings.Repeat("0123456789abcdef", 4),
		}},
	}
	ps := newPollServer(t)
	ps.serve(signManifest(t, priv, m))
	t.Setenv(manifestURLEnv, ps.srv.URL)
	client, err := newClientWithKey(ps.srv.URL, pub)
	if err != nil {
		t.Fatal(err)
	}

	portable, err := updatesPortable(true)
	if err != nil {
		t.Fatalf("updatesPortable: %v", err)
	}
	dir := t.TempDir()
	s := &Service{dir: dir, notes: mustNotesStore(t, dir), store: mustStore(t, dir), client: client,
		currentVersion: "1.0.0", portable: portable}
	t.Cleanup(func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Errorf("ServiceShutdown: %v", err)
		}
	})

	status, err := s.CheckForUpdate(context.Background())
	if err != nil {
		t.Fatalf("CheckForUpdate: %v", err)
	}
	if status.State != StateAvailable || status.AvailableVersion != "1.2.3" {
		t.Fatalf("status = %+v, want 1.2.3 available", status)
	}
	if s.pendingArtifact == nil || s.pendingArtifact.Kind != KindInstaller || s.pendingArtifact.Name != "typhon-devmock" {
		t.Fatalf("pendingArtifact = %+v, want the plain launcher binary", s.pendingArtifact)
	}

	readyPath := filepath.Join(dir, "selfupdate", "1.2.3", "typhon-devmock")
	writeTestFile(t, readyPath, []byte("new launcher"))
	s.readyPath = readyPath
	s.readyArtifact = s.pendingArtifact
	s.status.State = StateReady

	var specPath string
	restore := startWorker
	startWorker = func(_, spec string) error { specPath = spec; return nil }
	t.Cleanup(func() { startWorker = restore })

	if err := s.ApplyUpdate("en"); err != nil {
		t.Fatalf("ApplyUpdate: %v", err)
	}
	spec, err := readUpdateSpec(specPath)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Portable || spec.InstallerPath != readyPath || spec.Version != "1.2.3" {
		t.Fatalf("spec = %+v, want a plain binary swap to 1.2.3 from %s", spec, readyPath)
	}
}

// The relaunched launcher must come back as the same isolated test instance:
// data folder, portable id and every mock switch reach it only through the
// environment the worker passes on.
func TestWorkerAndRelaunchKeepTheLauncherEnvironment(t *testing.T) {
	data := filepath.Join(t.TempDir(), "data")
	vars := map[string]string{
		"TYPHON_PORTABLE_DIR":           filepath.Join(data, "config"),
		"TYPHON_TEST_DATA_DIR":          filepath.Join(data, "config"),
		"TYPHON_DEVMOCK_STATE_DIR":      filepath.Join(data, "devmock"),
		"TYPHON_DEVMOCK_GAME_SECONDS":   "3600",
		"TYPHON_DEVMOCK_MANIFEST_URL":   "http://127.0.0.1:8099",
		"TYPHON_DEVMOCK_RELEASE_PUBKEY": "cHVia2V5",
	}
	for k, v := range vars {
		t.Setenv(k, v)
	}

	for name, start := range map[string]func(t *testing.T, script, out string) error{
		"worker": func(_ *testing.T, script, out string) error { return startUpdateWorker(script, out) },
		"relaunch": func(t *testing.T, script, out string) error {
			t.Setenv("TYPHON_ENV_DUMP", out)
			return relaunch(script)
		},
	} {
		t.Run(name, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "env.txt")
			script := filepath.Join(t.TempDir(), "dump.sh")
			writeTestFile(t, script, []byte("#!/bin/sh\ndest=\"${TYPHON_ENV_DUMP:-$2}\"\nenv > \"$dest.tmp\" && mv \"$dest.tmp\" \"$dest\"\n"))
			//nolint:gosec // G302: the stand-in launcher has to be executable to be started at all
			if err := os.Chmod(script, 0o755); err != nil {
				t.Fatalf("chmod script: %v", err)
			}
			if err := start(t, script, out); err != nil {
				t.Fatalf("start: %v", err)
			}
			got := string(waitForFile(t, out, 10*time.Second))
			for k, v := range vars {
				if !strings.Contains(got, "\n"+k+"="+v+"\n") && !strings.HasPrefix(got, k+"="+v+"\n") {
					t.Errorf("%s=%s did not reach the started process; its environment:\n%s", k, v, got)
				}
			}
		})
	}
}
