package selfupdate

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	oldExe = "old launcher"
	newExe = "new launcher"
)

type portableZipEntry struct {
	name string
	body string
}

func buildZip(t *testing.T, entries ...portableZipEntry) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		w, err := zw.Create(e.name)
		if err != nil {
			t.Fatalf("create zip entry %q: %v", e.name, err)
		}
		if _, err := w.Write([]byte(e.body)); err != nil {
			t.Fatalf("write zip entry %q: %v", e.name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	path := filepath.Join(t.TempDir(), "typhon-windows-portable.zip")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func portableTarget(t *testing.T) string {
	t.Helper()
	target := filepath.Join(t.TempDir(), "typhon.exe")
	if err := os.WriteFile(target, []byte(oldExe), 0o755); err != nil {
		t.Fatal(err)
	}
	return target
}

func readText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func missing(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	if err == nil {
		return false
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stat %s: %v", path, err)
	}
	return true
}

func TestApplyPortableZipReplacesTheExecutableAndKeepsTheOldOne(t *testing.T) {
	target := portableTarget(t)
	zipPath := buildZip(t,
		portableZipEntry{"portable.txt", ""},
		portableZipEntry{"typhon.exe", newExe},
	)

	if err := applyPortableZip(context.Background(), zipPath, target); err != nil {
		t.Fatalf("applyPortableZip: %v", err)
	}
	if got := readText(t, target); got != newExe {
		t.Fatalf("target = %q, want the new launcher", got)
	}
	if got := readText(t, target+previousSuffix); got != oldExe {
		t.Fatalf("%s = %q, want the old launcher", previousSuffix, got)
	}
	if !missing(t, target+newSuffix) {
		t.Fatalf("staged %s was left behind", newSuffix)
	}
}

func TestApplyPortableZipRefusesUnsafeEntryNames(t *testing.T) {
	cases := map[string]string{
		"parent":         "../typhon.exe",
		"nested parent":  "data/../../x",
		"backslash":      `..\typhon.exe`,
		"absolute":       "/typhon.exe",
		"windows root":   `\typhon.exe`,
		"drive":          `C:/typhon.exe`,
		"alternate data": "typhon.exe:evil",
	}
	for name, entry := range cases {
		t.Run(name, func(t *testing.T) {
			target := portableTarget(t)
			zipPath := buildZip(t, portableZipEntry{"typhon.exe", newExe}, portableZipEntry{entry, "x"})

			err := applyPortableZip(context.Background(), zipPath, target)
			if !errors.Is(err, ErrPortableZipUnsafeEntry) {
				t.Fatalf("applyPortableZip error = %v, want ErrPortableZipUnsafeEntry", err)
			}
			if got := readText(t, target); got != oldExe {
				t.Fatalf("target = %q, a rejected archive must not touch the launcher", got)
			}
			if !missing(t, target+newSuffix) || !missing(t, target+previousSuffix) {
				t.Fatal("a rejected archive left staging files behind")
			}
		})
	}
}

func TestApplyPortableZipNeedsExactlyOneExecutable(t *testing.T) {
	cases := map[string]struct {
		entries []portableZipEntry
		want    error
	}{
		"empty archive": {nil, ErrPortableZipNoExecutable},
		"only marker":   {[]portableZipEntry{{"portable.txt", ""}}, ErrPortableZipNoExecutable},
		"nested only":   {[]portableZipEntry{{"app/typhon.exe", newExe}}, ErrPortableZipNoExecutable},
		"another case":  {[]portableZipEntry{{"Typhon.exe", newExe}}, ErrPortableZipNoExecutable},
		"duplicate":     {[]portableZipEntry{{"typhon.exe", newExe}, {"typhon.exe", "other"}}, ErrPortableZipExecutable},
		"directory":     {[]portableZipEntry{{"typhon.exe/", ""}}, ErrPortableZipExecutable},
		"empty binary":  {[]portableZipEntry{{"typhon.exe", ""}}, ErrPortableZipExecutable},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			target := portableTarget(t)
			err := applyPortableZip(context.Background(), buildZip(t, tc.entries...), target)
			if !errors.Is(err, tc.want) {
				t.Fatalf("applyPortableZip error = %v, want %v", err, tc.want)
			}
			if got := readText(t, target); got != oldExe {
				t.Fatalf("target = %q, want it untouched", got)
			}
		})
	}
}

func TestApplyPortableZipBrokenInput(t *testing.T) {
	target := portableTarget(t)
	notZip := filepath.Join(t.TempDir(), "x.zip")
	if err := os.WriteFile(notZip, []byte("not a zip"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := applyPortableZip(context.Background(), notZip, target); err == nil {
		t.Fatal("a file that is not a zip was applied")
	}
	if err := applyPortableZip(context.Background(), filepath.Join(t.TempDir(), "gone.zip"), target); err == nil {
		t.Fatal("a missing archive was applied")
	}
	if err := applyPortableZip(context.Background(), buildZip(t, portableZipEntry{"typhon.exe", newExe}), filepath.Join(t.TempDir(), "typhon.exe")); err == nil {
		t.Fatal("applied over a launcher that does not exist")
	}
	if err := applyPortableZip(context.Background(), buildZip(t, portableZipEntry{"typhon.exe", newExe}), ""); err == nil {
		t.Fatal("applied to an empty target")
	}
	if got := readText(t, target); got != oldExe {
		t.Fatalf("target = %q, want it untouched", got)
	}
}

func TestApplyPortableZipCancelledContextLeavesTheLauncher(t *testing.T) {
	target := portableTarget(t)
	zipPath := buildZip(t, portableZipEntry{"typhon.exe", strings.Repeat("x", 1<<16)})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := applyPortableZip(ctx, zipPath, target); !errors.Is(err, context.Canceled) {
		t.Fatalf("applyPortableZip error = %v, want context.Canceled", err)
	}
	if got := readText(t, target); got != oldExe {
		t.Fatalf("target = %q, want it untouched", got)
	}
	if !missing(t, target+newSuffix) {
		t.Fatalf("a cancelled apply left %s behind", newSuffix)
	}
}

func TestApplyPortableZipRollsBackWhenTheNewExecutableCannotTakeItsPlace(t *testing.T) {
	target := portableTarget(t)
	zipPath := buildZip(t, portableZipEntry{"typhon.exe", newExe})

	boom := errors.New("antivirus holds the file")
	orig := portableRename
	calls := 0
	portableRename = func(from, to string) error {
		calls++
		if calls == 2 {
			return boom
		}
		return orig(from, to)
	}
	t.Cleanup(func() { portableRename = orig })

	if err := applyPortableZip(context.Background(), zipPath, target); !errors.Is(err, boom) {
		t.Fatalf("applyPortableZip error = %v, want %v", err, boom)
	}
	if got := readText(t, target); got != oldExe {
		t.Fatalf("target = %q, want the old launcher back", got)
	}
	if !missing(t, target+newSuffix) || !missing(t, target+previousSuffix) {
		t.Fatal("a rolled back swap left staging files behind")
	}
}

func TestApplyPortableZipReportsAFailedRollbackToo(t *testing.T) {
	target := portableTarget(t)
	zipPath := buildZip(t, portableZipEntry{"typhon.exe", newExe})

	first := errors.New("second rename failed")
	second := errors.New("restore failed")
	orig := portableRename
	calls := 0
	portableRename = func(from, to string) error {
		calls++
		switch calls {
		case 2:
			return first
		case 3:
			return second
		}
		return orig(from, to)
	}
	t.Cleanup(func() { portableRename = orig })

	err := applyPortableZip(context.Background(), zipPath, target)
	if !errors.Is(err, first) || !errors.Is(err, second) {
		t.Fatalf("applyPortableZip error = %v, want both failures", err)
	}
}

func TestRecoverPortableSwap(t *testing.T) {
	cases := []struct {
		name     string
		exe      *string
		previous *string
		staged   *string
		wantExe  string
	}{
		{name: "nothing to do", exe: str(newExe), wantExe: newExe},
		{name: "died after staging", exe: str(oldExe), staged: str("half"), wantExe: oldExe},
		{name: "died after parking the old launcher", previous: str(oldExe), staged: str(newExe), wantExe: oldExe},
		{name: "died after parking, nothing staged", previous: str(oldExe), wantExe: oldExe},
		{name: "swap finished", exe: str(newExe), previous: str(oldExe), wantExe: newExe},
		{name: "swap finished with a stale stage", exe: str(newExe), previous: str(oldExe), staged: str("x"), wantExe: newExe},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			exe := filepath.Join(t.TempDir(), "typhon.exe")
			for path, body := range map[string]*string{exe: tc.exe, exe + previousSuffix: tc.previous, exe + newSuffix: tc.staged} {
				if body != nil {
					if err := os.WriteFile(path, []byte(*body), 0o755); err != nil {
						t.Fatal(err)
					}
				}
			}

			if err := recoverPortableSwap(exe); err != nil {
				t.Fatalf("recoverPortableSwap: %v", err)
			}
			if got := readText(t, exe); got != tc.wantExe {
				t.Fatalf("exe = %q, want %q", got, tc.wantExe)
			}
			if !missing(t, exe+previousSuffix) || !missing(t, exe+newSuffix) {
				t.Fatal("recovery left a leftover file behind")
			}
		})
	}
}

func TestRecoverPortableSwapRestoreFailureIsReported(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "typhon.exe")
	if err := os.WriteFile(exe+previousSuffix, []byte(oldExe), 0o755); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("cannot rename")
	orig := portableRename
	portableRename = func(string, string) error { return boom }
	t.Cleanup(func() { portableRename = orig })

	if err := recoverPortableSwap(exe); !errors.Is(err, boom) {
		t.Fatalf("recoverPortableSwap error = %v, want %v", err, boom)
	}
	if got := readText(t, exe+previousSuffix); got != oldExe {
		t.Fatalf("the only copy of the launcher was lost: %q", got)
	}
}

func str(s string) *string { return &s }

func portableArtifact(kind Kind) Artifact {
	a := kindArtifact("windows", "amd64", kind, "typhon-setup.exe")
	a.PortableName = "typhon-windows-portable.zip"
	a.PortableURL = "https://cdn.example.com/typhon-windows-portable.zip"
	a.PortableSize = 2048
	a.PortableSHA256 = strings.Repeat("fedcba9876543210", 4)
	return a
}

func TestPortableArtifactForBuildsTheZipArtifact(t *testing.T) {
	m := Manifest{Version: "1.2.3", Artifacts: []Artifact{portableArtifact(KindInstaller)}}
	got, err := m.PortableArtifactFor("windows", "amd64")
	if err != nil {
		t.Fatalf("PortableArtifactFor: %v", err)
	}
	want := Artifact{
		OS: "windows", Arch: "amd64", Kind: KindPortable, Name: "typhon-windows-portable.zip",
		URL: "https://cdn.example.com/typhon-windows-portable.zip", Size: 2048, SHA256: strings.Repeat("fedcba9876543210", 4),
	}
	if got != want {
		t.Fatalf("PortableArtifactFor() = %+v, want %+v", got, want)
	}
}

func TestPortableArtifactForWithoutAPortableBuild(t *testing.T) {
	plain := kindArtifact("windows", "amd64", KindInstaller, "typhon-setup.exe")
	m := Manifest{Version: "1.2.3", Artifacts: []Artifact{plain}}
	if _, err := m.PortableArtifactFor("windows", "amd64"); !errors.Is(err, ErrPortableUpdateUnavailable) {
		t.Fatalf("error = %v, want ErrPortableUpdateUnavailable", err)
	}
	if _, err := m.PortableArtifactFor("darwin", "arm64"); !errors.Is(err, ErrNoArtifact) {
		t.Fatalf("error = %v, want ErrNoArtifact", err)
	}
}

func TestManifestValidateHoldsThePortableFieldsToTheInstallerRules(t *testing.T) {
	mutate := func(f func(*Artifact)) error {
		a := portableArtifact(KindInstaller)
		f(&a)
		return Manifest{Version: "1.2.3", Artifacts: []Artifact{a}}.Validate()
	}
	if err := mutate(func(*Artifact) {}); err != nil {
		t.Fatalf("a complete portable asset was rejected: %v", err)
	}
	cases := map[string]func(*Artifact){
		"name only":      func(a *Artifact) { a.PortableURL, a.PortableSize, a.PortableSHA256 = "", 0, "" },
		"no hash":        func(a *Artifact) { a.PortableSHA256 = "" },
		"short hash":     func(a *Artifact) { a.PortableSHA256 = "abc" },
		"no size":        func(a *Artifact) { a.PortableSize = 0 },
		"huge size":      func(a *Artifact) { a.PortableSize = MaxArtifactSize + 1 },
		"plain http":     func(a *Artifact) { a.PortableURL = "http://cdn.example.com/p.zip" },
		"not a zip":      func(a *Artifact) { a.PortableName = "typhon.exe" },
		"path in name":   func(a *Artifact) { a.PortableName = "../p.zip" },
		"bundle carries": func(a *Artifact) { a.Kind = KindBundle },
	}
	for name, f := range cases {
		t.Run(name, func(t *testing.T) {
			if err := mutate(f); err == nil {
				t.Fatal("a malformed portable asset was accepted")
			}
		})
	}
}

func TestManifestRejectsAPortableKindEntry(t *testing.T) {
	a := kindArtifact("windows", "amd64", KindPortable, "typhon-windows-portable.zip")
	if err := (Manifest{Version: "1.2.3", Artifacts: []Artifact{a}}).Validate(); !errors.Is(err, ErrUnsupportedKind) {
		t.Fatalf("Validate() = %v, want ErrUnsupportedKind: the kind is derived by the client, never published", err)
	}
}

func TestPlainInstallerArtifactKeepsItsWireFormat(t *testing.T) {
	raw, err := json.Marshal(kindArtifact("windows", "amd64", KindInstaller, "typhon-setup.exe"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "portable") {
		t.Fatalf("artifact JSON = %s: launchers that predate the portable build must see the format they know", raw)
	}
}

func portableService(t *testing.T) (*Service, string) {
	t.Helper()
	dir := t.TempDir()
	s := &Service{dir: dir, notes: mustNotesStore(t, dir), store: mustStore(t, dir), currentVersion: "1.0.0", portable: true}
	return s, dir
}

func TestApplyUpdateInPortableModeHandsTheWorkerAZip(t *testing.T) {
	s, dir := portableService(t)
	readyPath := filepath.Join(dir, "selfupdate", "1.2.3", "typhon-windows-portable.zip")
	writeTestFile(t, readyPath, []byte("zip"))
	s.readyPath = readyPath
	s.readyArtifact = &Artifact{OS: runtime.GOOS, Arch: runtime.GOARCH, Kind: KindPortable, Name: "typhon-windows-portable.zip"}
	s.status = Status{State: StateReady, CurrentVersion: "1.0.0", AvailableVersion: "1.2.3"}

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
	if !spec.Portable || spec.InstallerPath != readyPath {
		t.Fatalf("spec = %+v, want a portable spec for %s", spec, readyPath)
	}
}

func TestApplyUpdateRefusesAnArtifactOfTheOtherMode(t *testing.T) {
	for name, tc := range map[string]struct {
		portable bool
		artifact *Artifact
	}{
		"portable launcher, installer ready":  {true, &Artifact{Kind: KindInstaller}},
		"portable launcher, nothing recorded": {true, nil},
		"installed launcher, zip ready":       {false, &Artifact{Kind: KindPortable}},
	} {
		t.Run(name, func(t *testing.T) {
			s, dir := portableService(t)
			s.portable = tc.portable
			s.readyPath = filepath.Join(dir, "ready")
			s.readyArtifact = tc.artifact
			s.status = Status{State: StateReady, CurrentVersion: "1.0.0", AvailableVersion: "1.2.3"}

			started := false
			restore := startWorker
			startWorker = func(string, string) error { started = true; return nil }
			t.Cleanup(func() { startWorker = restore })

			if err := s.ApplyUpdate("en"); !errors.Is(err, ErrNotReady) {
				t.Fatalf("ApplyUpdate error = %v, want ErrNotReady", err)
			}
			if started {
				t.Fatal("a worker was started for an artifact of the wrong mode")
			}
			if s.busy || s.GetStatus().State != StateReady {
				t.Fatalf("the service did not return to ready: busy=%v state=%v", s.busy, s.GetStatus().State)
			}
		})
	}
}

func TestPortableCheckWithoutAPortableBuildStopsAtTheCheck(t *testing.T) {
	priv, pub := testKeyPair(t)
	m := releaseManifest("1.2.3")
	if runtime.GOOS == "darwin" {
		m.Artifacts[0].Kind = KindInstaller
	}
	ps := newPollServer(t)
	ps.serve(signManifest(t, priv, m))
	client, err := newClientWithKey(ps.srv.URL, pub)
	if err != nil {
		t.Fatal(err)
	}
	s, dir := portableService(t)
	s.client = client

	status, err := s.CheckForUpdate(context.Background())
	if !errors.Is(err, ErrPortableUpdateUnavailable) {
		t.Fatalf("CheckForUpdate error = %v, want ErrPortableUpdateUnavailable", err)
	}
	if status.State == StateAvailable || status.ErrorCode != "artifact" {
		t.Fatalf("status = %+v, want no update offered and the artifact error reported", status)
	}
	if s.pendingArtifact != nil {
		t.Fatal("an installer was queued for a portable launcher")
	}
	if v, err := mustStore(t, dir).Load(); err != nil || v.AvailableVersion != "" {
		t.Fatalf("saved state = %+v, %v; nothing may be recorded", v, err)
	}
}

func TestPortableCheckQueuesTheZip(t *testing.T) {
	priv, pub := testKeyPair(t)
	m := releaseManifest("1.2.3")
	m.Artifacts[0].Kind = KindInstaller
	m.Artifacts[0].PortableName = "typhon-windows-portable.zip"
	m.Artifacts[0].PortableURL = "https://cdn.example.com/typhon-windows-portable.zip"
	m.Artifacts[0].PortableSize = 2048
	m.Artifacts[0].PortableSHA256 = strings.Repeat("fedcba9876543210", 4)
	ps := newPollServer(t)
	ps.serve(signManifest(t, priv, m))
	client, err := newClientWithKey(ps.srv.URL, pub)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := portableService(t)
	s.client = client

	status, err := s.CheckForUpdate(context.Background())
	if err != nil {
		t.Fatalf("CheckForUpdate: %v", err)
	}
	if status.State != StateAvailable {
		t.Fatalf("status = %+v, want an available update", status)
	}
	if s.pendingArtifact == nil || s.pendingArtifact.Kind != KindPortable || s.pendingArtifact.Name != "typhon-windows-portable.zip" {
		t.Fatalf("pendingArtifact = %+v, want the portable zip", s.pendingArtifact)
	}
}

func TestServiceStartupFinishesAnInterruptedPortableSwap(t *testing.T) {
	s, _ := portableService(t)
	s.client = mustQuietClient(t)

	exe := filepath.Join(t.TempDir(), "typhon.exe")
	if err := os.WriteFile(exe+previousSuffix, []byte(oldExe), 0o755); err != nil {
		t.Fatal(err)
	}
	orig := portableExecutable
	portableExecutable = func() (string, error) { return exe, nil }
	t.Cleanup(func() { portableExecutable = orig })

	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	if err := s.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
	if got := readText(t, exe); got != oldExe {
		t.Fatalf("exe = %q, want the parked launcher restored", got)
	}
}

func TestServiceStartupFailsWhenAPortableSwapCannotBeRecovered(t *testing.T) {
	s, _ := portableService(t)
	s.client = mustQuietClient(t)

	exe := filepath.Join(t.TempDir(), "typhon.exe")
	if err := os.WriteFile(exe+previousSuffix, []byte(oldExe), 0o755); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("cannot rename")
	origExe, origRename := portableExecutable, portableRename
	portableExecutable = func() (string, error) { return exe, nil }
	portableRename = func(string, string) error { return boom }
	t.Cleanup(func() { portableExecutable, portableRename = origExe, origRename })

	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); !errors.Is(err, boom) {
		t.Fatalf("ServiceStartup error = %v, want %v", err, boom)
	}
}

func TestWorkerEnvironmentOutsidePortableModeInheritsTheLauncher(t *testing.T) {
	env, err := workerEnvironment()
	if err != nil || env != nil {
		t.Fatalf("workerEnvironment() = %v, %v; want nil, nil outside portable mode", env, err)
	}
}
