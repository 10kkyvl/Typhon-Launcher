//go:build devmock && !windows

package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"typhon/internal/selfupdate"
)

// These tests drive the launcher's own selfupdate.Service (devmock build:
// its manifest url and key come from the same variables the test app gets)
// against a running source, so each failure mode is proved by the code the
// launcher actually reports, not by what the source sends.

const liveArtifactSize = 64 << 10

// uiCodePattern is the rule the frontend reads a code by
// (frontend/src/lib/i18n/errors.ts): the first typhon:<code> in the message.
// uierr.Code is not enough here: the artifact status error carries the code
// in its text and matches ErrArtifactStatus through Is, without wrapping it.
var uiCodePattern = regexp.MustCompile(`typhon:([a-z0-9_]+(?:\.[a-z0-9_]+)*)`)

func uiCode(err error) string {
	if err == nil {
		return ""
	}
	if m := uiCodePattern.FindStringSubmatch(err.Error()); m != nil {
		return m[1]
	}
	return ""
}

type liveSource struct {
	ts       *httptest.Server
	srv      *releaseServer
	pub      ed25519.PublicKey
	mu       sync.Mutex
	requests []string
}

func (l *liveSource) ranges() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.requests...)
}

// startLiveSource serves a chain far above the test binary's own version, on
// a port known before the manifests are signed, as run() does.
func startLiveSource(t *testing.T) *liveSource {
	t.Helper()
	priv, pub, err := resolveKeyPair("")
	if err != nil {
		t.Fatalf("resolveKeyPair: %v", err)
	}
	ls := &liveSource{pub: pub}
	ls.ts = httptest.NewUnstartedServer(nil)
	addr, ok := ls.ts.Listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener address %v is not TCP", ls.ts.Listener.Addr())
	}

	versions := []string{"99.0.0", "99.0.1"}
	dir := t.TempDir()
	paths := make([]string, len(versions))
	for i := range versions {
		content := make([]byte, liveArtifactSize)
		if _, err := rand.Read(content); err != nil {
			t.Fatalf("random artifact: %v", err)
		}
		paths[i] = filepath.Join(dir, versions[i])
		if err := os.WriteFile(paths[i], content, 0o755); err != nil {
			t.Fatalf("write artifact: %v", err)
		}
	}
	releases, err := prepareReleases(versions, paths, addr.Port, sampleNotes, time.Now().UTC(), priv)
	if err != nil {
		t.Fatalf("prepareReleases: %v", err)
	}
	ls.srv = newReleaseServer(releases, 0, nil)
	mux := newMux(ls.srv)
	ls.ts.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/launcher/download/") {
			ls.mu.Lock()
			ls.requests = append(ls.requests, r.Header.Get("Range"))
			ls.mu.Unlock()
		}
		mux.ServeHTTP(w, r)
	})
	ls.ts.Start()
	t.Cleanup(ls.ts.Close)
	return ls
}

type testLauncher struct {
	svc *selfupdate.Service
	dir string
}

func launcherAgainst(t *testing.T, ls *liveSource) testLauncher {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("TYPHON_TEST_DATA_DIR", dir)
	t.Setenv("TYPHON_DEVMOCK_MANIFEST_URL", ls.ts.URL)
	t.Setenv("TYPHON_DEVMOCK_RELEASE_PUBKEY", base64.StdEncoding.EncodeToString(ls.pub))
	svc, err := selfupdate.NewService()
	if err != nil {
		t.Fatalf("selfupdate.NewService: %v", err)
	}
	t.Cleanup(func() {
		if err := svc.ServiceShutdown(); err != nil {
			t.Errorf("ServiceShutdown: %v", err)
		}
	})
	return testLauncher{svc: svc, dir: dir}
}

func (l testLauncher) partialSize(t *testing.T, version string) int64 {
	t.Helper()
	vdir, err := selfupdate.VersionDir(l.dir, version)
	if err != nil {
		t.Fatalf("VersionDir: %v", err)
	}
	info, err := os.Stat(filepath.Join(vdir, artifactName()+".partial"))
	if errors.Is(err, fs.ErrNotExist) {
		return 0
	}
	if err != nil {
		t.Fatalf("stat partial: %v", err)
	}
	return info.Size()
}

func TestManifestFailureModesEndInTheLaunchersCodes(t *testing.T) {
	for _, tc := range []struct {
		mode, code string
		sentinel   error
	}{
		{mode: "manifest-500", code: "selfupdate.manifest_status", sentinel: selfupdate.ErrManifestStatus},
		{mode: "bad-signature", code: "selfupdate.bad_signature", sentinel: selfupdate.ErrBadSignature},
		{mode: "unknown-key", code: "selfupdate.unknown_key", sentinel: selfupdate.ErrUnknownKey},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			ls := startLiveSource(t)
			l := launcherAgainst(t, ls)
			setMode(t, ls.ts.URL, tc.mode)

			status, err := l.svc.CheckForUpdate(t.Context())
			if got := uiCode(err); got != tc.code || !errors.Is(err, tc.sentinel) {
				t.Fatalf("CheckForUpdate error = %v (code %q), want %s", err, got, tc.code)
			}
			if status.State == selfupdate.StateAvailable {
				t.Fatalf("status = %+v, want no update offered", status)
			}

			setMode(t, ls.ts.URL, "")
			status, err = l.svc.CheckForUpdate(t.Context())
			if err != nil || status.State != selfupdate.StateAvailable || status.AvailableVersion != "99.0.0" {
				t.Fatalf("after fail= the check = %+v, %v; want 99.0.0 available", status, err)
			}
		})
	}
}

func TestArtifactFailureModesEndInTheLaunchersCodes(t *testing.T) {
	for _, tc := range []struct {
		mode, code string
		sentinel   error
	}{
		{mode: "hash", code: "selfupdate.hash_mismatch", sentinel: selfupdate.ErrHashMismatch},
		{mode: "size", code: "selfupdate.size_mismatch", sentinel: selfupdate.ErrSizeMismatch},
		// The client retries a 5xx twice, after 2 s and 5 s: this case takes
		// those seven seconds because the backoff is the launcher's own.
		{mode: "artifact-500", code: "selfupdate.artifact_status", sentinel: selfupdate.ErrArtifactStatus},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			ls := startLiveSource(t)
			l := launcherAgainst(t, ls)
			if _, err := l.svc.CheckForUpdate(t.Context()); err != nil {
				t.Fatalf("CheckForUpdate: %v", err)
			}
			setMode(t, ls.ts.URL, tc.mode)

			status, err := l.svc.DownloadUpdate(t.Context())
			if got := uiCode(err); got != tc.code || !errors.Is(err, tc.sentinel) {
				t.Fatalf("DownloadUpdate error = %v (code %q), want %s", err, got, tc.code)
			}
			if status.State == selfupdate.StateReady {
				t.Fatalf("status = %+v, want nothing ready", status)
			}

			setMode(t, ls.ts.URL, "")
			status, err = l.svc.DownloadUpdate(t.Context())
			if err != nil || status.State != selfupdate.StateReady {
				t.Fatalf("after fail= the download = %+v, %v; want ready", status, err)
			}
		})
	}
}

// waitPartial waits until the launcher has written exactly want bytes of the
// artifact, or fails when the download ends first.
func waitPartial(t *testing.T, l testLauncher, version string, want int64, done <-chan error) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		if got := l.partialSize(t, version); got == want {
			return
		} else if got > want {
			t.Fatalf("partial artifact = %d bytes, want the download held at %d", got, want)
		}
		select {
		case err := <-done:
			t.Fatalf("download ended before stalling at %d bytes: %v", want, err)
		case <-deadline:
			t.Fatalf("partial artifact never reached %d bytes", want)
		case <-ticker.C:
		}
	}
}

// The launcher's stall guard gives up after 60 s and resumes by Range; the
// user's Cancel ends the stalled transfer the same way here, without the
// minute. What the source must show: bytes stop at the middle while the
// connection stays open, and a resumed request continues from there.
func TestStalledDownloadHoldsMidwayAndResumesByRange(t *testing.T) {
	ls := startLiveSource(t)
	l := launcherAgainst(t, ls)
	if _, err := l.svc.CheckForUpdate(t.Context()); err != nil {
		t.Fatalf("CheckForUpdate: %v", err)
	}
	setMode(t, ls.ts.URL, "stall")

	half := int64(liveArtifactSize / 2)
	done := make(chan error, 1)
	go func() {
		_, err := l.svc.DownloadUpdate(t.Context())
		done <- err
	}()
	waitPartial(t, l, "99.0.0", half, done)

	if err := l.svc.CancelDownload(); err != nil {
		t.Fatalf("CancelDownload: %v", err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled download error = %v, want context.Canceled", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the stalled download did not end on cancel")
	}
	if got := l.partialSize(t, "99.0.0"); got != half {
		t.Fatalf("partial after cancel = %d bytes, want %d", got, half)
	}

	setMode(t, ls.ts.URL, "")
	status, err := l.svc.DownloadUpdate(t.Context())
	if err != nil || status.State != selfupdate.StateReady {
		t.Fatalf("resumed download = %+v, %v; want ready", status, err)
	}
	want := []string{"", fmt.Sprintf("bytes=%d-", half)}
	if got := ls.ranges(); len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("download requests carried Range %q, want %q", got, want)
	}
}

func TestStallEndsWhenTheModeIsCleared(t *testing.T) {
	ls := startLiveSource(t)
	l := launcherAgainst(t, ls)
	if _, err := l.svc.CheckForUpdate(t.Context()); err != nil {
		t.Fatalf("CheckForUpdate: %v", err)
	}
	setMode(t, ls.ts.URL, "stall")

	done := make(chan error, 1)
	go func() {
		_, err := l.svc.DownloadUpdate(t.Context())
		done <- err
	}()
	waitPartial(t, l, "99.0.0", liveArtifactSize/2, done)
	setMode(t, ls.ts.URL, "")

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("download after the stall ended: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the download stayed stalled after fail= cleared the mode")
	}
	if got := ls.ranges(); len(got) != 1 {
		t.Fatalf("download requests = %q, want the one held connection to carry on", got)
	}
	if got := l.svc.GetStatus(); got.State != selfupdate.StateReady {
		t.Fatalf("status = %+v, want ready", got)
	}
}
