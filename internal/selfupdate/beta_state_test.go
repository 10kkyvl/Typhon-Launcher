package selfupdate

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"
)

// A launcher without beta support cannot parse a version with a suffix, so any
// of these keys holding one would fail its start or its checks.
var legacyVersion = regexp.MustCompile(`^[0-9]+(\.[0-9]+)*$`)

var versionKeys = map[string]bool{
	"availableVersion": true,
	"skippedVersion":   true,
	"lastSeenVersion":  true,
	"version":          true,
}

func walkVersionStrings(t *testing.T, file string, node any, found *[]string) {
	t.Helper()
	switch v := node.(type) {
	case map[string]any:
		for key, child := range v {
			if key == "beta" {
				continue
			}
			if str, ok := child.(string); ok && versionKeys[key] && str != "" && !legacyVersion.MatchString(str) {
				*found = append(*found, file+": "+key+"="+str)
			}
			walkVersionStrings(t, file, child, found)
		}
	case []any:
		for _, child := range v {
			walkVersionStrings(t, file, child, found)
		}
	}
}

func prereleaseVersionsOnDisk(t *testing.T, dir string) []string {
	t.Helper()
	var found []string
	root := os.DirFS(dir)
	err := fs.WalkDir(root, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		data, rerr := fs.ReadFile(root, path)
		if rerr != nil {
			return rerr
		}
		var doc any
		if jerr := json.Unmarshal(data, &doc); jerr != nil {
			return jerr
		}
		walkVersionStrings(t, path, doc, &found)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return found
}

func TestStoreKeepsABetaUpdateWhereOlderLaunchersDoNotLook(t *testing.T) {
	dir := t.TempDir()
	store := mustStore(t, dir)
	want := stored{
		AvailableVersion: "1.3.0-beta.2",
		Notes:            "n",
		PublishedAt:      time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		CheckedAt:        time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC),
		Artifact:         &Artifact{OS: "windows", Arch: "amd64", Kind: KindInstaller, Name: "setup.exe", URL: "https://x/setup.exe", Size: 5, SHA256: "ab"},
		ReadyPath:        filepath.Join(dir, "setup.exe"),
		SkippedVersion:   "1.2.0",
	}
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.AvailableVersion != want.AvailableVersion || got.ReadyPath != want.ReadyPath || got.SkippedVersion != "1.2.0" ||
		got.Artifact == nil || got.Artifact.Name != "setup.exe" || !got.CheckedAt.Equal(want.CheckedAt) {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
	if found := prereleaseVersionsOnDisk(t, dir); len(found) != 0 {
		t.Fatalf("a stable launcher would meet these: %v", found)
	}
	var legacy storedFile
	raw, err := os.ReadFile(filepath.Join(mustCacheDir(t, dir), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(envelope.Data, &legacy.stored); err != nil {
		t.Fatal(err)
	}
	if legacy.AvailableVersion != "" || legacy.ReadyPath != "" || legacy.Artifact != nil {
		t.Fatalf("an older launcher reads %+v from the beta state, want nothing on offer", legacy.stored)
	}
}

func TestStoreKeepsAStableUpdateWhereItWas(t *testing.T) {
	dir := t.TempDir()
	store := mustStore(t, dir)
	if err := store.Save(stored{AvailableVersion: "1.3.0", ReadyPath: "x"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(mustCacheDir(t, dir), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	data, ok := doc["data"].(map[string]any)
	if !ok {
		t.Fatalf("state = %s, want an envelope", raw)
	}
	if data["availableVersion"] != "1.3.0" || data["beta"] != nil {
		t.Fatalf("state = %s, want the stable layout unchanged", raw)
	}
}

func TestBetaRunLeavesNoPrereleaseForTheStableLauncher(t *testing.T) {
	st := newBetaStand(t, "1.3.0-beta.2", "beta")
	dir := t.TempDir()
	s := st.service(t, dir, "1.3.0-beta.1", func() (string, bool, error) { return "tok", true, nil })

	if _, err := s.CheckForUpdate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if status, err := s.DownloadUpdate(context.Background()); err != nil || status.State != StateReady {
		t.Fatalf("DownloadUpdate() = %+v, %v", status, err)
	}
	if err := s.AcknowledgeReleaseNotes(); err != nil {
		t.Fatal(err)
	}
	if err := markSkipped(dir, "1.3.0-beta.1"); err != nil {
		t.Fatal(err)
	}

	if found := prereleaseVersionsOnDisk(t, dir); len(found) != 0 {
		t.Fatalf("a stable launcher would fail on: %v", found)
	}
	reload := mustStore(t, dir)
	v, err := reload.Load()
	if err != nil {
		t.Fatal(err)
	}
	if v.AvailableVersion != "1.3.0-beta.2" || v.ReadyPath == "" || v.SkippedVersion != "" {
		t.Fatalf("beta state after a restart = %+v", v)
	}
}

func TestMarkSkippedWritesOnlyStableVersions(t *testing.T) {
	cases := []struct {
		name, from, want string
	}{
		{"stable is remembered", "1.3.0", "1.3.0"},
		{"a beta is not", "1.3.0-beta.1", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := markSkipped(dir, c.from); err != nil {
				t.Fatal(err)
			}
			v, err := mustStore(t, dir).Load()
			if err != nil {
				t.Fatal(err)
			}
			if v.SkippedVersion != c.want {
				t.Fatalf("SkippedVersion = %q, want %q", v.SkippedVersion, c.want)
			}
		})
	}
}

func TestReleaseNotesMarkerIgnoresABeta(t *testing.T) {
	cases := []struct {
		name, current, want string
	}{
		{"stable build marks its version", "1.2.0", "1.2.0"},
		{"beta build leaves the marker alone", "1.3.0-beta.1", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			s := &Service{dir: dir, notes: mustNotesStore(t, dir), store: mustStore(t, dir), currentVersion: c.current}
			if err := s.storeReleaseNotes(nil); err != nil {
				t.Fatal(err)
			}
			if err := s.AcknowledgeReleaseNotes(); err != nil {
				t.Fatal(err)
			}
			if got := s.notesState.LastSeenVersion; got != c.want {
				t.Fatalf("LastSeenVersion = %q, want %q", got, c.want)
			}
		})
	}
}

func betaRollbackService(t *testing.T, st *betaStand, f rollbackFixture) *Service {
	t.Helper()
	s := rollbackService(t, f, "1.3.0-beta.1")
	client, err := newClientWithKey(st.srv.URL, st.pub)
	if err != nil {
		t.Fatal(err)
	}
	s.client = client
	return s
}

func TestInstallStableRollsBackWhenTheSnapshotIsTheOffer(t *testing.T) {
	st := newBetaStand(t, "1.2.0", "stable")
	f := newRollbackFixture(t)
	started := captureWorkerStarts(t)
	s := betaRollbackService(t, st, f)

	if _, err := s.CheckForUpdate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.InstallStable(context.Background(), "en"); err != nil {
		t.Fatalf("InstallStable: %v", err)
	}
	if len(started.specs) != 1 {
		t.Fatalf("workers started = %d, want 1", len(started.specs))
	}
	spec, err := readUpdateSpec(started.specs[0])
	if err != nil {
		t.Fatal(err)
	}
	if spec.Kind != specKindRollback || spec.Version != "1.2.0" || spec.FromVersion != "1.3.0-beta.1" {
		t.Fatalf("spec = %+v, want a rollback to 1.2.0", spec)
	}
	st.mu.Lock()
	hits := st.hits
	st.mu.Unlock()
	if hits != 0 {
		t.Fatalf("%d artifact downloads, the snapshot needs none", hits)
	}
	if !s.rollbackPending {
		t.Fatal("rollback not pending")
	}
	if err := s.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
}

func TestInstallStableKeepsAnOlderSnapshotAndDoesNotSaveTheBeta(t *testing.T) {
	st := newBetaStand(t, "1.2.0", "stable")
	f := newRollbackFixture(t)
	if err := dropPrevious(f.configDir); err != nil {
		t.Fatal(err)
	}
	if err := snapshotPrevious(context.Background(), f.configDir, f.target, "1.1.0"); err != nil {
		t.Fatal(err)
	}
	if err := commitSnapshot(f.configDir); err != nil {
		t.Fatal(err)
	}
	started := captureWorkerStarts(t)
	s := betaRollbackService(t, st, f)

	if _, err := s.CheckForUpdate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.InstallStable(context.Background(), "en"); err != nil {
		t.Fatalf("InstallStable: %v", err)
	}
	spec, err := readUpdateSpec(started.specs[0])
	if err != nil {
		t.Fatal(err)
	}
	if spec.Kind == specKindRollback || spec.Version != "1.2.0" || !spec.Downgrade {
		t.Fatalf("spec = %+v, want an ordinary install of 1.2.0 flagged as a downgrade", spec)
	}
	if rec, err := loadPrevious(f.configDir); err != nil || rec.Version != "1.1.0" {
		t.Fatalf("snapshot = %+v, %v, want the untouched 1.1.0", rec, err)
	}
	if err := s.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
}

func TestFailedInstallStableLeavesTheOfferAndNothingPending(t *testing.T) {
	st := newBetaStand(t, "1.2.0", "stable")
	captureWorkerStarts(t)
	s := st.service(t, t.TempDir(), "1.3.0-beta.1", nil)
	if _, err := s.CheckForUpdate(context.Background()); err != nil {
		t.Fatal(err)
	}
	st.mu.Lock()
	st.content[0] ^= 0xff
	st.mu.Unlock()

	status, err := s.InstallStable(context.Background(), "en")
	if err == nil {
		t.Fatal("want an error for a tampered artifact")
	}
	if status.State != StateIdle || status.AvailableVersion != "" || status.StableOffer != "1.2.0" || status.Error == "" {
		t.Fatalf("status = %+v, want idle with the offer kept and the error shown", status)
	}
	if s.pendingArtifact != nil || s.busy {
		t.Fatalf("pending = %v, busy = %v after a failed install", s.pendingArtifact, s.busy)
	}
	if _, err := s.DownloadUpdate(context.Background()); err == nil {
		t.Fatal("the ordinary download took the downgrade")
	}

	st.mu.Lock()
	st.content[0] ^= 0xff
	st.mu.Unlock()
	if _, err := s.InstallStable(context.Background(), "en"); err != nil {
		t.Fatalf("a retry after the failure: %v", err)
	}
}

func TestWorkerDowngradeDoesNotSnapshotTheBeta(t *testing.T) {
	cases := []struct {
		name         string
		downgrade    bool
		previous     string
		wantSaves    int
		wantPrevious string
	}{
		{"ordinary update saves the running launcher", false, "", 1, ""},
		{"downgrade saves nothing", true, "", 0, ""},
		{"downgrade drops a beta snapshot", true, "1.3.0-beta.1", 0, ""},
		{"downgrade keeps a stable snapshot", true, "1.1.0", 0, "1.1.0"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			shortWorkerTimeouts(t)
			stubRollbackSupported(t, nil)
			configDir := testConfigDir(t)
			target := filepath.Join(t.TempDir(), "typhon.exe")
			writeTestFile(t, target, []byte("beta launcher"))
			fakes := &fakePrimitives{applyWrites: []byte("stable launcher")}
			installFakePrimitives(t, fakes)
			saves := 0
			saveCurrent = func(context.Context, string, string, string) error { saves++; return nil }
			if c.previous != "" {
				if err := snapshotPrevious(context.Background(), configDir, target, c.previous); err != nil {
					t.Fatal(err)
				}
				if err := commitSnapshot(configDir); err != nil {
					t.Fatal(err)
				}
			}

			spec := updateSpec{
				InstallerPath: filepath.Join(t.TempDir(), "setup.exe"),
				InstallDir:    filepath.Dir(target),
				ParentPID:     1,
				RelaunchPath:  target,
				Version:       "1.2.0",
				FromVersion:   "1.3.0-beta.1",
				Downgrade:     c.downgrade,
			}
			snapshotErr, err := runUpdate(context.Background(), context.Background(), configDir, spec, silentReporter{})
			if err != nil || snapshotErr != nil {
				t.Fatalf("runUpdate: snapshot error %v, error %v", snapshotErr, err)
			}
			if saves != c.wantSaves {
				t.Fatalf("snapshots taken = %d, want %d", saves, c.wantSaves)
			}
			rec, err := loadPrevious(configDir)
			switch {
			case c.wantPrevious == "" && c.previous != "" && err == nil:
				t.Fatalf("snapshot %q survived the downgrade", rec.Version)
			case c.wantPrevious != "" && (err != nil || rec.Version != c.wantPrevious):
				t.Fatalf("snapshot = %+v, %v, want %q kept", rec, err, c.wantPrevious)
			}
		})
	}
}
