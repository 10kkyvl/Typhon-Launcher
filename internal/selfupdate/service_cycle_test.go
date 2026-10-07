package selfupdate

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type releaseStand struct {
	t            *testing.T
	priv         ed25519.PrivateKey
	pub          ed25519.PublicKey
	srv          *httptest.Server
	content      []byte
	mu           sync.Mutex
	body         []byte
	manifestHits atomic.Int32
	artifactHits atomic.Int32
}

func newReleaseStand(t *testing.T, version string, content []byte) *releaseStand {
	t.Helper()
	priv, pub := testKeyPair(t)
	st := &releaseStand{t: t, priv: priv, pub: pub, content: content}
	st.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == ManifestPath:
			st.manifestHits.Add(1)
			st.mu.Lock()
			body := st.body
			st.mu.Unlock()
			if _, err := w.Write(body); err != nil {
				t.Logf("write manifest: %v", err)
			}
		case strings.HasPrefix(r.URL.Path, "/artifact/"):
			st.artifactHits.Add(1)
			if _, err := w.Write(st.content); err != nil {
				t.Logf("write artifact: %v", err)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(st.srv.Close)
	st.publish(version)
	return st
}

func (st *releaseStand) publish(version string) {
	st.t.Helper()
	m := releaseManifest(version)
	art := &m.Artifacts[0]
	art.URL = st.srv.URL + "/artifact/" + art.Name
	art.Size = int64(len(st.content))
	art.SHA256 = hashOf(st.content)
	signed := signManifest(st.t, st.priv, m)
	st.mu.Lock()
	st.body = signed
	st.mu.Unlock()
}

func (st *releaseStand) artifactName() string {
	return releaseManifest("0.0.1").Artifacts[0].Name
}

func standService(t *testing.T, dir string, st *releaseStand, current string) *Service {
	t.Helper()
	client, err := newClientWithKey(st.srv.URL, st.pub)
	if err != nil {
		t.Fatalf("newClientWithKey: %v", err)
	}
	return &Service{dir: dir, notes: mustNotesStore(t, dir), store: mustStore(t, dir), client: client, currentVersion: current}
}

func restartedService(t *testing.T, dir, current string) *Service {
	t.Helper()
	s := &Service{dir: dir, notes: mustNotesStore(t, dir), store: mustStore(t, dir), client: mustQuietClient(t), currentVersion: current}
	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	t.Cleanup(func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Errorf("ServiceShutdown: %v", err)
		}
	})
	return s
}

type workerStarts struct {
	workers []string
	specs   []string
}

func captureWorkerStarts(t *testing.T) *workerStarts {
	t.Helper()
	w := &workerStarts{}
	restore := startWorker
	startWorker = func(worker, spec string) error {
		w.workers = append(w.workers, worker)
		w.specs = append(w.specs, spec)
		return nil
	}
	t.Cleanup(func() { startWorker = restore })
	return w
}

func TestUpdateCycleAcrossRestarts(t *testing.T) {
	tests := []struct {
		name      string
		installed bool
	}{
		{"worker installs the update", true},
		{"worker fails to install the update", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := bytes.Repeat([]byte("typhon installer "), 300)
			stand := newReleaseStand(t, "1.2.3", content)
			dir := t.TempDir()
			started := captureWorkerStarts(t)
			installerPath, err := ArtifactPath(dir, "1.2.3", stand.artifactName())
			if err != nil {
				t.Fatalf("ArtifactPath: %v", err)
			}

			s1 := standService(t, dir, stand, "1.0.0")
			status, err := s1.CheckForUpdate(context.Background())
			if err != nil {
				t.Fatalf("CheckForUpdate() error = %v", err)
			}
			if status.State != StateAvailable || status.AvailableVersion != "1.2.3" || status.CheckedAt.IsZero() {
				t.Fatalf("status after the check = %+v, want 1.2.3 available", status)
			}
			status, err = s1.DownloadUpdate(context.Background())
			if err != nil {
				t.Fatalf("DownloadUpdate() error = %v", err)
			}
			if status.State != StateReady {
				t.Fatalf("status after the download = %+v, want ready", status)
			}
			if got, err := os.ReadFile(installerPath); err != nil || !bytes.Equal(got, content) {
				t.Fatalf("installer on disk: %d bytes, %v", len(got), err)
			}

			s2 := restartedService(t, dir, "1.0.0")
			if got := s2.GetStatus(); got.State != StateReady || got.AvailableVersion != "1.2.3" {
				t.Fatalf("status after a restart = %+v, want the downloaded update still ready", got)
			}
			if got := s2.GetOutcome(); got != (Outcome{}) {
				t.Fatalf("outcome before any install = %+v", got)
			}

			if err := s2.ApplyUpdate("en"); err != nil {
				t.Fatalf("ApplyUpdate() error = %v", err)
			}
			if len(started.specs) != 1 {
				t.Fatalf("worker started %d times, want 1", len(started.specs))
			}
			spec, err := readUpdateSpec(started.specs[0])
			if err != nil {
				t.Fatalf("readUpdateSpec: %v", err)
			}
			if spec.InstallerPath != installerPath || spec.Version != "1.2.3" || spec.ParentPID != os.Getpid() {
				t.Fatalf("worker spec = %+v, want the verified installer of 1.2.3 and this process as the parent", spec)
			}
			if got := s2.GetStatus().State; got != StateApplying {
				t.Fatalf("State after ApplyUpdate = %v, want applying", got)
			}
			if err := s2.ServiceShutdown(); err != nil {
				t.Fatalf("ServiceShutdown: %v", err)
			}

			outcome := Outcome{Version: "1.2.3", OK: tt.installed, FinishedAt: time.Now()}
			current := "1.0.0"
			if tt.installed {
				current = "1.2.3"
			} else {
				outcome.Error = "selfupdate: installer finished but left the launcher binary unchanged"
			}
			outcomePath, err := OutcomePath(dir)
			if err != nil {
				t.Fatalf("OutcomePath: %v", err)
			}
			if err := writeOutcome(outcomePath, outcome); err != nil {
				t.Fatalf("writeOutcome: %v", err)
			}
			if err := os.Remove(started.specs[0]); err != nil {
				t.Fatalf("remove spec: %v", err)
			}

			s3 := restartedService(t, dir, current)
			got := s3.GetOutcome()
			if got.Version != "1.2.3" || got.OK != tt.installed || got.Error != outcome.Error {
				t.Fatalf("GetOutcome() = %+v, want %+v", got, outcome)
			}
			if _, err := os.Stat(outcomePath); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("outcome file survived the start and would be shown again: %v", err)
			}
			workerDir, err := WorkerDir(dir)
			if err != nil {
				t.Fatalf("WorkerDir: %v", err)
			}
			if _, err := os.Stat(workerDir); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("worker copy survived the restart: %v", err)
			}
			saved, err := mustStore(t, dir).Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}

			if tt.installed {
				if status := s3.GetStatus(); status.State != StateIdle || status.AvailableVersion != "" {
					t.Fatalf("status after the update = %+v, want idle: the new build must not be offered to itself", status)
				}
				if _, err := os.Stat(installerPath); !errors.Is(err, fs.ErrNotExist) {
					t.Fatalf("installer of the installed version stayed in the cache: %v", err)
				}
				if saved.Artifact != nil || saved.ReadyPath != "" || saved.AvailableVersion != "" {
					t.Fatalf("saved state still carries the installed update: %+v", saved)
				}
				return
			}

			if status := s3.GetStatus(); status.State != StateReady || status.AvailableVersion != "1.2.3" {
				t.Fatalf("status after a failed install = %+v, want the update still ready for another try", status)
			}
			if got, err := os.ReadFile(installerPath); err != nil || !bytes.Equal(got, content) {
				t.Fatalf("installer after a failed install: %d bytes, %v; it must stay for a retry", len(got), err)
			}
			if saved.Artifact == nil || saved.ReadyPath != installerPath {
				t.Fatalf("saved state = %+v, want the ready installer recorded", saved)
			}
			if err := s3.ApplyUpdate("en"); err != nil {
				t.Fatalf("retry ApplyUpdate() error = %v", err)
			}
			if len(started.specs) != 2 {
				t.Fatalf("worker started %d times, want a second start for the retry", len(started.specs))
			}
			if hits := stand.artifactHits.Load(); hits != 1 {
				t.Fatalf("installer was fetched %d times, want exactly 1: the retry must reuse the verified file", hits)
			}
		})
	}
}

func TestCheckNeverOffersAnInstalledOrOlderVersion(t *testing.T) {
	tests := []struct {
		name      string
		current   string
		published string
		offered   bool
	}{
		{"same version", "1.0.0", "1.0.0", false},
		{"older patch", "1.0.1", "1.0.0", false},
		{"older minor", "1.2.0", "1.1.9", false},
		{"older major", "2.0.0", "1.99.99", false},
		{"older but longer number", "0.10.0", "0.9.0", false},
		{"newer patch", "1.0.0", "1.0.1", true},
		{"newer but longer number", "0.9.0", "0.10.0", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stand := newReleaseStand(t, tt.published, []byte("installer"))
			dir := t.TempDir()
			s := standService(t, dir, stand, tt.current)

			status, err := s.CheckForUpdate(context.Background())
			if err != nil {
				t.Fatalf("CheckForUpdate() error = %v", err)
			}
			saved, err := mustStore(t, dir).Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			_, dlErr := s.DownloadUpdate(context.Background())

			if tt.offered {
				if status.State != StateAvailable || status.AvailableVersion != tt.published || saved.AvailableVersion != tt.published {
					t.Fatalf("status = %+v, saved = %+v, want %s offered", status, saved, tt.published)
				}
				if dlErr != nil {
					t.Fatalf("DownloadUpdate() error = %v", dlErr)
				}
				return
			}
			if status.State != StateIdle || status.AvailableVersion != "" || status.CheckedAt.IsZero() {
				t.Fatalf("status = %+v, want an idle status that still records the check", status)
			}
			if saved.AvailableVersion != "" || saved.Artifact != nil {
				t.Fatalf("saved state = %+v, a version that is not newer must not be recorded as an update", saved)
			}
			if !errors.Is(dlErr, errNoUpdateChecked) {
				t.Fatalf("DownloadUpdate() error = %v, want errNoUpdateChecked: there is nothing to download", dlErr)
			}
			if hits := stand.artifactHits.Load(); hits != 0 {
				t.Fatalf("artifact was fetched %d times for a version that is not an update", hits)
			}
		})
	}
}

func TestWithdrawnReleaseIsNoLongerOffered(t *testing.T) {
	stand := newReleaseStand(t, "1.2.3", []byte("installer of the release that gets pulled"))
	dir := t.TempDir()
	s := standService(t, dir, stand, "1.0.0")

	if _, err := s.CheckForUpdate(context.Background()); err != nil {
		t.Fatalf("CheckForUpdate() error = %v", err)
	}
	if status, err := s.DownloadUpdate(context.Background()); err != nil || status.State != StateReady {
		t.Fatalf("DownloadUpdate() = %+v, %v; want ready", status, err)
	}
	installerPath, err := ArtifactPath(dir, "1.2.3", stand.artifactName())
	if err != nil {
		t.Fatalf("ArtifactPath: %v", err)
	}

	stand.publish("1.0.0")
	status, err := s.CheckForUpdate(context.Background())
	if err != nil {
		t.Fatalf("CheckForUpdate() after the withdrawal error = %v", err)
	}
	if status.State != StateIdle || status.AvailableVersion != "" {
		t.Fatalf("status = %+v, want idle once 1.2.3 is no longer published", status)
	}
	if err := s.ApplyUpdate("en"); !errors.Is(err, ErrNotReady) {
		t.Fatalf("ApplyUpdate() error = %v, want ErrNotReady: the pulled release must not be installable", err)
	}
	if _, err := s.DownloadUpdate(context.Background()); !errors.Is(err, errNoUpdateChecked) {
		t.Fatalf("DownloadUpdate() error = %v, want errNoUpdateChecked", err)
	}
	saved, err := mustStore(t, dir).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if saved.Artifact != nil || saved.ReadyPath != "" || saved.AvailableVersion != "" {
		t.Fatalf("saved state = %+v, want no trace of the pulled release", saved)
	}

	restarted := restartedService(t, dir, "1.0.0")
	if got := restarted.GetStatus(); got.State != StateIdle {
		t.Fatalf("status after a restart = %+v, want idle", got)
	}
	if _, err := os.Stat(installerPath); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("installer of the pulled release stayed in the cache: %v", err)
	}
}

func TestCheckWithAReadOnlyStoreOffersNothing(t *testing.T) {
	stand := newReleaseStand(t, "1.2.3", []byte("installer"))
	dir := t.TempDir()
	path := stateFilePath(t, dir)
	writeTestFile(t, path, []byte("{not valid json"))

	s := standService(t, dir, stand, "1.0.0")
	s.status = Status{State: StateIdle, CurrentVersion: "1.0.0"}
	if _, err := s.store.Load(); err == nil {
		t.Fatal("store.Load() on corrupt json returned nil error")
	}

	if _, err := s.CheckForUpdate(context.Background()); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("CheckForUpdate() error = %v, want ErrReadOnly", err)
	}
	if s.pendingArtifact != nil || s.pendingVersion != "" {
		t.Fatalf("pending update %q recorded although the check could not be saved", s.pendingVersion)
	}
	if got := s.GetStatus(); got.State != StateIdle || got.AvailableVersion != "" {
		t.Fatalf("status = %+v, want it untouched by a check that could not be saved", got)
	}
	if _, err := s.DownloadUpdate(context.Background()); !errors.Is(err, errNoUpdateChecked) {
		t.Fatalf("DownloadUpdate() error = %v, want errNoUpdateChecked", err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "{not valid json" {
		t.Fatalf("state file = %q, %v; want it left as it was", got, err)
	}
}

func TestCheckWhileBusyDoesNotAskTheServer(t *testing.T) {
	stand := newReleaseStand(t, "1.2.3", []byte("installer"))
	s := standService(t, t.TempDir(), stand, "1.0.0")
	s.busy = true

	if _, err := s.CheckForUpdate(context.Background()); !errors.Is(err, ErrBusy) {
		t.Fatalf("CheckForUpdate() error = %v, want ErrBusy", err)
	}
	if hits := stand.manifestHits.Load(); hits != 0 {
		t.Fatalf("server saw %d manifest requests while a download or install was running", hits)
	}
}

func TestServiceStartupReportsWhatTheWorkerLeftBehind(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name       string
		raw        []byte
		outcome    *Outcome
		wantFailed bool
		wantEmpty  bool
	}{
		{name: "no record", wantEmpty: true},
		{name: "successful install", outcome: &Outcome{Version: "1.2.3", OK: true, FinishedAt: now}},
		{name: "failed install", outcome: &Outcome{Version: "1.2.3", Error: "installer exploded", FinishedAt: now}, wantFailed: true},
		{name: "record older than a day", outcome: &Outcome{Version: "1.2.3", OK: true, FinishedAt: now.Add(-2 * outcomeMaxAge)}, wantEmpty: true},
		{name: "truncated record", raw: []byte(`{"version":1,"data":{"version":"1.2.3","ok":tr`), wantFailed: true},
		{name: "empty record", raw: []byte{}, wantFailed: true},
		{name: "garbage record", raw: []byte("not json at all"), wantFailed: true},
		{name: "record from a newer format", raw: []byte(`{"version":9,"data":{"ok":true}}`), wantFailed: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			outcomePath, err := OutcomePath(dir)
			if err != nil {
				t.Fatalf("OutcomePath: %v", err)
			}
			switch {
			case tt.outcome != nil:
				if err := writeOutcome(outcomePath, *tt.outcome); err != nil {
					t.Fatalf("writeOutcome: %v", err)
				}
			case tt.raw != nil:
				writeTestFile(t, outcomePath, tt.raw)
			}

			s := restartedService(t, dir, "1.0.0")
			got := s.GetOutcome()

			switch {
			case tt.wantEmpty:
				if got != (Outcome{}) {
					t.Fatalf("GetOutcome() = %+v, want nothing to report", got)
				}
			case tt.wantFailed:
				if got.OK || got.Error == "" {
					t.Fatalf("GetOutcome() = %+v, an unreadable or failed record must reach the user as a failure with a reason", got)
				}
			default:
				if !got.OK || got.Version != "1.2.3" || got.Error != "" {
					t.Fatalf("GetOutcome() = %+v, want the successful install", got)
				}
			}
			if _, err := os.Stat(outcomePath); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("outcome file survived the start: %v", err)
			}
		})
	}
}

func TestApplyUpdateRollsBackEveryFailureAndCanBeRetried(t *testing.T) {
	workerDir := func(dir string) string {
		p, err := WorkerDir(dir)
		if err != nil {
			t.Fatalf("WorkerDir: %v", err)
		}
		return p
	}
	specFile := func(dir string) string {
		p, err := SpecPath(dir)
		if err != nil {
			t.Fatalf("SpecPath: %v", err)
		}
		return p
	}

	tests := []struct {
		name  string
		block func(t *testing.T, dir string) (unblock func())
	}{
		{
			name: "worker copy cannot be staged",
			block: func(t *testing.T, dir string) func() {
				p := workerDir(dir)
				writeTestFile(t, p, []byte("a file where the worker directory belongs"))
				return func() {
					if err := os.Remove(p); err != nil {
						t.Fatalf("unblock: %v", err)
					}
				}
			},
		},
		{
			name: "update spec cannot be written",
			block: func(t *testing.T, dir string) func() {
				p := specFile(dir)
				writeTestFile(t, filepath.Join(p, "occupant"), []byte("x"))
				return func() {
					if err := os.RemoveAll(p); err != nil {
						t.Fatalf("unblock: %v", err)
					}
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			readyPath := filepath.Join(dir, "selfupdate", "1.2.3", "setup.exe")
			writeTestFile(t, readyPath, []byte("installer"))
			s := &Service{dir: dir, notes: mustNotesStore(t, dir), store: mustStore(t, dir), currentVersion: "1.0.0", readyPath: readyPath}
			s.status = Status{State: StateReady, CurrentVersion: "1.0.0", AvailableVersion: "1.2.3"}
			started := captureWorkerStarts(t)

			unblock := tt.block(t, dir)
			if err := s.ApplyUpdate("en"); err == nil {
				t.Fatal("ApplyUpdate() error = nil, want the failure reported")
			}
			if len(started.specs) != 0 {
				t.Fatalf("worker was started %d times although the launcher would stay open", len(started.specs))
			}
			if s.busy {
				t.Fatal("busy stayed set, the next click would bounce off ErrBusy")
			}
			if got := s.GetStatus(); got.State != StateReady || got.AvailableVersion != "1.2.3" {
				t.Fatalf("status = %+v, want the update still ready", got)
			}

			unblock()
			if err := s.ApplyUpdate("en"); err != nil {
				t.Fatalf("retry ApplyUpdate() error = %v", err)
			}
			if len(started.specs) != 1 {
				t.Fatalf("worker started %d times after the retry, want 1", len(started.specs))
			}
			if got := s.GetStatus().State; got != StateApplying {
				t.Fatalf("State after the retry = %v, want applying", got)
			}
		})
	}
}
