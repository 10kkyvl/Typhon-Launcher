package selfupdate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"typhon/internal/selfupdate/delta"
)

func testBytes(seed uint64, n int) []byte {
	out := make([]byte, 0, n+sha256.Size)
	var block [16]byte
	binary.LittleEndian.PutUint64(block[:8], seed)
	for counter := uint64(0); len(out) < n; counter++ {
		binary.LittleEndian.PutUint64(block[8:], counter)
		sum := sha256.Sum256(block[:])
		out = append(out, sum[:]...)
	}
	return out[:n]
}

// deltaStand is a release server with one installer, one patch and the two
// executables the patch sits between.
type deltaStand struct {
	t         *testing.T
	srv       *httptest.Server
	oldExe    []byte
	newExe    []byte
	patch     []byte
	installer []byte

	patchStatus   int
	patchHits     atomic.Int32
	installerHits atomic.Int32
}

func newDeltaStand(t *testing.T) *deltaStand {
	t.Helper()
	oldExe := testBytes(100, 256<<10)
	newExe := append(append([]byte(nil), oldExe[:100<<10]...), testBytes(101, 4<<10)...)
	newExe = append(newExe, oldExe[100<<10:]...)
	var patch bytes.Buffer
	if err := delta.Encode(&patch, oldExe, newExe); err != nil {
		t.Fatalf("delta.Encode: %v", err)
	}
	st := &deltaStand{t: t, oldExe: oldExe, newExe: newExe, patch: patch.Bytes(), installer: testBytes(102, 600<<10)}
	st.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/patch":
			st.patchHits.Add(1)
			if st.patchStatus != 0 {
				w.WriteHeader(st.patchStatus)
				return
			}
			if _, err := w.Write(st.patch); err != nil {
				t.Logf("write patch: %v", err)
			}
		case "/installer":
			st.installerHits.Add(1)
			if _, err := w.Write(st.installer); err != nil {
				t.Logf("write installer: %v", err)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(st.srv.Close)
	return st
}

func (st *deltaStand) artifact() Artifact {
	return Artifact{
		OS: "windows", Arch: "amd64", Kind: KindInstaller, Name: "typhon-amd64-installer.exe",
		URL: st.srv.URL + "/installer", Size: int64(len(st.installer)), SHA256: hashOf(st.installer),
		ExeSHA256: hashOf(st.newExe), ExeSize: int64(len(st.newExe)),
		Deltas: []Delta{{
			From: "1.2.0", FromSHA256: hashOf(st.oldExe), Name: "typhon-1.2.0-to-1.3.0.delta",
			URL: st.srv.URL + "/patch", Size: int64(len(st.patch)), SHA256: hashOf(st.patch),
		}},
	}
}

// runningLauncher makes currentExecutable point at a file with content in a
// folder of its own and returns that folder.
func runningLauncher(t *testing.T, content []byte) string {
	t.Helper()
	dir := t.TempDir()
	exe := filepath.Join(dir, "typhon.exe")
	writeTestFile(t, exe, content)
	restore := currentExecutable
	currentExecutable = func() (string, error) { return exe, nil }
	t.Cleanup(func() { currentExecutable = restore })
	return dir
}

func downloadingService(t *testing.T, st *deltaStand, art Artifact, current string) *Service {
	t.Helper()
	dir := t.TempDir()
	client, err := NewClient(st.srv.URL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	s := &Service{
		dir: dir, notes: mustNotesStore(t, dir), store: mustStore(t, dir), client: client, currentVersion: current,
		pendingArtifact: &art, pendingVersion: "1.3.0",
		status: Status{State: StateAvailable, CurrentVersion: current, AvailableVersion: "1.3.0"},
	}
	return s
}

type logCapture struct {
	buf bytes.Buffer
}

func captureLogs(t *testing.T) *logCapture {
	t.Helper()
	c := &logCapture{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&c.buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return c
}

func (c *logCapture) count(substr string) int { return strings.Count(c.buf.String(), substr) }

func TestDownloadUpdateTakesThePatchWhenItFits(t *testing.T) {
	st := newDeltaStand(t)
	launcherDir := runningLauncher(t, st.oldExe)
	art := st.artifact()
	s := downloadingService(t, st, art, "1.2.0")

	status, err := s.DownloadUpdate(context.Background())
	if err != nil {
		t.Fatalf("DownloadUpdate() error = %v", err)
	}

	if status.State != StateReady || !status.Delta {
		t.Fatalf("status = %+v, want a ready update that says it came from a patch", status)
	}
	if want := int64(len(st.patch)); status.TotalBytes != want || status.DownloadedBytes != want {
		t.Fatalf("TotalBytes/DownloadedBytes = %d/%d, want the patch size %d", status.TotalBytes, status.DownloadedBytes, want)
	}
	if st.installerHits.Load() != 0 {
		t.Fatalf("the full installer was requested %d times next to a working patch", st.installerHits.Load())
	}
	exePath, err := ArtifactPath(s.dir, "1.3.0", portableExeName)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(exePath); err != nil || !bytes.Equal(got, st.newExe) {
		t.Fatalf("rebuilt executable: err=%v, equal=%v", err, bytes.Equal(got, st.newExe))
	}
	patchPath, err := ArtifactPath(s.dir, "1.3.0", art.Deltas[0].Name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(patchPath); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the patch stayed in the cache after it was applied: %v", err)
	}

	saved, err := s.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if saved.ReadyPath != exePath || saved.Artifact == nil || saved.Artifact.Kind != KindExe ||
		saved.Artifact.SHA256 != art.ExeSHA256 || saved.Artifact.Size != art.ExeSize || len(saved.Artifact.Deltas) != 0 {
		t.Fatalf("saved record = %+v / %+v, want the rebuilt executable as the ready file", saved.ReadyPath, saved.Artifact)
	}
	if s.readyArtifact == nil || s.readyArtifact.Kind != KindExe || s.readyPath != exePath {
		t.Fatalf("service ready state = %v / %q", s.readyArtifact, s.readyPath)
	}
	if left := tempNames(t, launcherDir); len(left) != 0 {
		t.Fatalf("the writability probe left %v beside the launcher", left)
	}
	if got, err := os.ReadFile(filepath.Join(launcherDir, "typhon.exe")); err != nil || !bytes.Equal(got, st.oldExe) {
		t.Fatal("downloading touched the running launcher; only the worker may replace it")
	}
}

func TestDownloadUpdateFallsBackToTheFullFile(t *testing.T) {
	tests := []struct {
		name string
		// setup returns the artifact to offer and the version the launcher runs.
		setup func(t *testing.T, st *deltaStand) (Artifact, string)
		// reason is what the single log line must carry.
		reason string
		// patchRequested says whether the patch was asked for before giving up.
		patchRequested bool
	}{
		{
			name: "manifest carries no patches",
			setup: func(t *testing.T, st *deltaStand) (Artifact, string) {
				runningLauncher(t, st.oldExe)
				a := st.artifact()
				a.Deltas, a.ExeSHA256, a.ExeSize = nil, "", 0
				return a, "1.2.0"
			},
		},
		{
			name: "no patch from the running version",
			setup: func(t *testing.T, st *deltaStand) (Artifact, string) {
				runningLauncher(t, st.oldExe)
				return st.artifact(), "1.1.0"
			},
			reason: "no patch from the running version",
		},
		{
			name: "the running executable was changed",
			setup: func(t *testing.T, st *deltaStand) (Artifact, string) {
				runningLauncher(t, append(append([]byte(nil), st.oldExe...), 0))
				return st.artifact(), "1.2.0"
			},
			reason: "not the build the patch starts from",
		},
		{
			name: "the launcher folder takes no new files",
			setup: func(t *testing.T, st *deltaStand) (Artifact, string) {
				dir := runningLauncher(t, st.oldExe)
				if err := os.RemoveAll(dir); err != nil {
					t.Fatal(err)
				}
				return st.artifact(), "1.2.0"
			},
			reason: "does not accept new files",
		},
		{
			name: "the patch is not smaller than the installer",
			setup: func(t *testing.T, st *deltaStand) (Artifact, string) {
				runningLauncher(t, st.oldExe)
				a := st.artifact()
				a.Deltas[0].Size = a.Size
				return a, "1.2.0"
			},
			reason: "not smaller than the full download",
		},
		{
			name: "the patch is gone from the server",
			setup: func(t *testing.T, st *deltaStand) (Artifact, string) {
				runningLauncher(t, st.oldExe)
				st.patchStatus = http.StatusNotFound
				return st.artifact(), "1.2.0"
			},
			patchRequested: true,
		},
		{
			name: "the patch bytes do not match the manifest",
			setup: func(t *testing.T, st *deltaStand) (Artifact, string) {
				runningLauncher(t, st.oldExe)
				a := st.artifact()
				a.Deltas[0].SHA256 = hashOf([]byte("another patch"))
				return a, "1.2.0"
			},
			patchRequested: true,
		},
		{
			name: "the patch rebuilds something else than the manifest promises",
			setup: func(t *testing.T, st *deltaStand) (Artifact, string) {
				runningLauncher(t, st.oldExe)
				a := st.artifact()
				a.ExeSHA256 = hashOf([]byte("not the new launcher"))
				return a, "1.2.0"
			},
			patchRequested: true,
		},
		{
			name: "the manifest promises a different executable size",
			setup: func(t *testing.T, st *deltaStand) (Artifact, string) {
				runningLauncher(t, st.oldExe)
				a := st.artifact()
				a.ExeSize--
				return a, "1.2.0"
			},
			patchRequested: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := newDeltaStand(t)
			art, current := tt.setup(t, st)
			logs := captureLogs(t)
			s := downloadingService(t, st, art, current)

			status, err := s.DownloadUpdate(context.Background())
			if err != nil {
				t.Fatalf("DownloadUpdate() error = %v: a patch that does not work must not fail the update", err)
			}

			if status.State != StateReady || status.Delta {
				t.Fatalf("status = %+v, want a ready update from the full file", status)
			}
			if want := art.Size; status.TotalBytes != want || status.DownloadedBytes != want {
				t.Fatalf("TotalBytes/DownloadedBytes = %d/%d, want the installer size %d", status.TotalBytes, status.DownloadedBytes, want)
			}
			if st.installerHits.Load() != 1 {
				t.Fatalf("installer requested %d times, want once", st.installerHits.Load())
			}
			if got := st.patchHits.Load() > 0; got != tt.patchRequested {
				t.Fatalf("patch requested = %v, want %v", got, tt.patchRequested)
			}
			if s.readyArtifact == nil || s.readyArtifact.Kind != KindInstaller || len(s.readyArtifact.Deltas) != 0 {
				t.Fatalf("ready artifact = %+v, want the installer without the patch list", s.readyArtifact)
			}
			if want := filepath.Join(s.dir, "selfupdate", "1.3.0", art.Name); s.readyPath != want {
				t.Fatalf("ready path = %q, want %q", s.readyPath, want)
			}
			exePath := filepath.Join(s.dir, "selfupdate", "1.3.0", portableExeName)
			if _, err := os.Stat(exePath); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("a rebuilt executable that failed its check was left in the cache: %v", err)
			}
			if tt.reason == "" {
				if n := logs.count("downloading the full file instead of a patch"); n != 0 {
					t.Fatalf("an ordinary release without patches logged %d warnings", n)
				}
				return
			}
			if n := logs.count("downloading the full file instead of a patch"); n != 1 || !strings.Contains(logs.buf.String(), tt.reason) {
				t.Fatalf("log has %d reason lines, want exactly one containing %q:\n%s", n, tt.reason, logs.buf.String())
			}
		})
	}
}

func TestDownloadUpdateFallbackIsLoggedOncePerWorkedOutPatch(t *testing.T) {
	st := newDeltaStand(t)
	runningLauncher(t, st.oldExe)
	art := st.artifact()
	art.ExeSHA256 = hashOf([]byte("not the new launcher"))
	logs := captureLogs(t)
	s := downloadingService(t, st, art, "1.2.0")

	if _, err := s.DownloadUpdate(context.Background()); err != nil {
		t.Fatalf("DownloadUpdate() error = %v", err)
	}
	if n := logs.count("the patch did not work out"); n != 1 {
		t.Fatalf("log has %d lines about the failed patch, want one:\n%s", n, logs.buf.String())
	}
}

func TestDownloadUpdateCancelledStaysCancelled(t *testing.T) {
	st := newDeltaStand(t)
	runningLauncher(t, st.oldExe)
	s := downloadingService(t, st, st.artifact(), "1.2.0")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := s.DownloadUpdate(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("DownloadUpdate() error = %v, want context.Canceled", err)
	}
	if st.installerHits.Load() != 0 {
		t.Fatal("a cancelled download went on to fetch the full installer")
	}
	if got := s.GetStatus(); got.State != StateAvailable || got.Delta || got.TotalBytes != 0 {
		t.Fatalf("status after a cancelled download = %+v, want available with nothing counted", got)
	}
}

func TestPlanDeltaReadsTheExecutableOnce(t *testing.T) {
	st := newDeltaStand(t)
	dir := runningLauncher(t, st.oldExe)
	art := st.artifact()

	plan, err := planDelta(context.Background(), art, "1.2.0")
	if err != nil {
		t.Fatalf("planDelta() error = %v", err)
	}
	if !bytes.Equal(plan.base, st.oldExe) || plan.delta.From != "1.2.0" {
		t.Fatalf("plan = %d base bytes from %q", len(plan.base), plan.delta.From)
	}
	if left := tempNames(t, dir); len(left) != 0 {
		t.Fatalf("the writability probe left %v", left)
	}
	if _, err := planDelta(context.Background(), art, "1.2.0.0"); err != nil {
		t.Fatalf("the same version written another way: %v", err)
	}
}

func TestPlanDeltaReasons(t *testing.T) {
	st := newDeltaStand(t)
	runningLauncher(t, st.oldExe)
	tests := []struct {
		name    string
		art     func(Artifact) Artifact
		current string
		ctx     func() context.Context
		want    error
	}{
		{"no patches", func(a Artifact) Artifact { a.Deltas = nil; return a }, "1.2.0", nil, errNoDelta},
		{"no executable hash", func(a Artifact) Artifact { a.ExeSHA256 = ""; return a }, "1.2.0", nil, errNoDelta},
		{"another version", func(a Artifact) Artifact { return a }, "1.0.0", nil, errDeltaNoSource},
		{"not smaller", func(a Artifact) Artifact { a.Size = a.Deltas[0].Size; return a }, "1.2.0", nil, errDeltaNotSmaller},
		{"other hash", func(a Artifact) Artifact { a.Deltas[0].FromSHA256 = hashOf([]byte("x")); return a }, "1.2.0", nil, errDeltaSourceChanged},
		{"cancelled", func(a Artifact) Artifact { return a }, "1.2.0", func() context.Context {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return ctx
		}, context.Canceled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := tt.art(st.artifact())
			ctx := context.Background()
			if tt.ctx != nil {
				ctx = tt.ctx()
			}
			if _, err := planDelta(ctx, a, tt.current); !errors.Is(err, tt.want) {
				t.Fatalf("planDelta() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestReadyMatches(t *testing.T) {
	art := deltaArtifact(nil)
	exe := &Artifact{OS: art.OS, Arch: art.Arch, Kind: KindExe, Name: portableExeName, Size: art.ExeSize, SHA256: art.ExeSHA256}
	stripped := art
	stripped.Deltas = nil

	tests := []struct {
		name  string
		ready *Artifact
		art   Artifact
		want  bool
	}{
		{"nothing is ready", nil, art, false},
		{"the same installer without its patch list", &stripped, art, true},
		{"another installer", &Artifact{OS: art.OS, Arch: art.Arch, Kind: KindInstaller, Name: art.Name, URL: art.URL, Size: art.Size, SHA256: strings.Repeat("99", 32)}, art, false},
		{"a rebuilt executable for the same release", exe, art, true},
		{"a rebuilt executable of another release", exe, deltaArtifact(func(a *Artifact) { a.ExeSHA256 = strings.Repeat("77", 32) }), false},
		{"a rebuilt executable of a manifest without patches", exe, deltaArtifact(func(a *Artifact) { a.ExeSHA256, a.ExeSize, a.Deltas = "", 0, nil }), false},
		{"a rebuilt executable of another size", exe, deltaArtifact(func(a *Artifact) { a.ExeSize++ }), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := readyMatches(tt.ready, tt.art); got != tt.want {
				t.Fatalf("readyMatches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildCheckStatusKeepsAReadyRebuiltExe(t *testing.T) {
	s := &Service{currentVersion: "1.2.0"}
	m := Manifest{Version: "1.3.0", Artifacts: []Artifact{deltaArtifact(nil)}}
	art := m.Artifacts[0]
	exe := &Artifact{OS: art.OS, Arch: art.Arch, Kind: KindExe, Name: portableExeName, Size: art.ExeSize, SHA256: art.ExeSHA256}
	current := Status{State: StateReady, CurrentVersion: "1.2.0", AvailableVersion: "1.3.0", TotalBytes: 4096, DownloadedBytes: 4096, Delta: true}

	status, st, err := s.buildCheckStatus(current, "ready.exe", exe, m, art, nil, true, nil)
	if err != nil {
		t.Fatalf("buildCheckStatus() error = %v", err)
	}
	if status.State != StateReady || !status.Delta || status.TotalBytes != 4096 || status.DownloadedBytes != 4096 {
		t.Fatalf("status = %+v, want the ready patch update to survive a re-check with its own sizes", status)
	}
	if st.Artifact != exe || st.ReadyPath != "ready.exe" {
		t.Fatalf("stored = %+v, want the ready record carried over", st)
	}
}
