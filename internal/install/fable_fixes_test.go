package install

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"typhon/internal/library"
)

func TestAuditFix001(t *testing.T) {
	lib, err := library.NewServiceAt(filepath.Join(t.TempDir(), "library.json"))
	if err != nil {
		t.Fatal(err)
	}
	dir := gameDir(t, "Foreign")
	save := filepath.Join(dir, "saves", "slot.sav")
	mkFile(t, save, 10)
	g, _, err := lib.ApplyDiscovered(library.Discovered{Title: "Foreign", Executable: filepath.Join(dir, "Foreign.exe"), InstallDir: dir, CanonicalGameID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	s, err := newServiceAt(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	s.library = lib
	s.releaseRuntime = func(string) error { return nil }
	if err := lib.ApplySync([]library.SyncGame{{CanonicalGameID: "c1", Owned: true}}); err != nil {
		t.Fatal(err)
	}
	err = s.RemoveGame(g.ID, RemoveOptions{DeleteFiles: true, KeepInLibrary: true})
	_, statErr := os.Stat(save)
	if err == nil || statErr != nil {
		t.Fatalf("unsafe removal: err=%v save=%v", err, statErr)
	}
}

func TestAuditFix009(t *testing.T) {
	s, err := newServiceAt(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := gameDir(t, "Incomplete")
	_, cause := readFinalWorkerState(filepath.Join(t.TempDir(), "missing.json"), "run")
	if !errors.Is(cause, errInstallerNotConfirmedStopped) || !errors.Is(cause, errWorkerNotFinished) {
		t.Fatal(cause)
	}
	s.discardSilent(Installation{Destination: dir}, fsSnapshot{}, cause)
	if missing(t, dir) {
		t.Fatal("unconfirmed installer destination deleted")
	}
}

func TestAuditFix010(t *testing.T) {
	dir, pin := brokerDirs(t)
	spec := goodSpec(pin)
	spec.InstallerPath = filepath.Join(pin.ContentRoot, "unrelated.exe")
	if err := writeWorkerSpec(brokerSpecPath(dir), spec); err != nil {
		t.Fatal(err)
	}
	_, found, err := readBrokerSpec(dir, pin)
	if err == nil || found {
		t.Fatal("010 validation reproduced: arbitrary spec within writable roots accepted without parent authentication; no executable launched")
	}
}

func TestAuditFix013(t *testing.T) {
	s, downloads, _ := newTestService(t)
	root := t.TempDir()
	portableSource(t, root, "Game")
	downloads.add("d1", "Game", root)
	dest := filepath.Join(t.TempDir(), "Destination")
	s.mu.Lock()
	s.items = append(s.items, &Installation{ID: "existing", DownloadID: "d2", Destination: dest, Status: StatusInstalling})
	s.mu.Unlock()
	item, err := s.Start("d1", StartOptions{Destination: dest, Mode: ModeCopy})
	if err == nil {
		s.waitJobDone(t, item.ID)
		t.Fatal("accepted an occupied destination")
	}
}

func TestAuditFix012CleanupFailureDoesNotFailInstallation(t *testing.T) {
	s, downloads, registrar := newTestService(t)
	root := t.TempDir()
	portableSource(t, root, "Game")
	downloads.add("d1", "Game", root)
	old := removeInstalledSource
	removeInstalledSource = func(string) error { return os.ErrPermission }
	defer func() { removeInstalledSource = old }()
	item, err := s.Start("d1", StartOptions{Destination: filepath.Join(t.TempDir(), "Game"), Mode: ModeMove})
	if err != nil {
		t.Fatal(err)
	}
	done := s.waitStatus(t, item.ID, StatusCompleted)
	s.waitJobDone(t, item.ID)
	if _, err := registrar.Find(done.GameID); err != nil {
		t.Fatal(err)
	}
	if missing(t, root) {
		t.Fatal("failed cleanup discarded source")
	}
}

func TestAuditFix010RejectsTamperedSignedSpec(t *testing.T) {
	dir, pin := brokerDirs(t)
	spec := goodSpec(pin)
	spec.Run = "fresh"
	if err := os.WriteFile(spec.InstallerPath, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeSignedBrokerSpec(dir, spec, brokerTestPrivate); err != nil {
		t.Fatal(err)
	}
	signed, found, err := readBrokerSpec(dir, pin, brokerTestPublic)
	if err != nil || !found {
		t.Fatalf("valid signature: %v", err)
	}
	signed.Destination = filepath.Join(pin.LibraryRoot, "Different")
	if err := writeWorkerSpec(brokerSpecPath(dir), signed); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readBrokerSpec(dir, pin, brokerTestPublic); err == nil {
		t.Fatal("tampered destination accepted")
	}
	if err := writeSignedBrokerSpec(dir, spec, brokerTestPrivate); err != nil {
		t.Fatal(err)
	}
	signed, _, err = readBrokerSpec(dir, pin, brokerTestPublic)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(spec.InstallerPath, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runSignedBrokerSpec(signed); err == nil {
		t.Fatal("changed installer accepted")
	}
}

func TestAuditFix001UntypedMarkerCannotAuthorizeDeletion(t *testing.T) {
	s, _, lib := newTestService(t)
	dir := gameDir(t, "Foreign")
	if err := library.WriteMarker(dir, library.Marker{GameID: "g1", Title: "Foreign", Owned: true}); err != nil {
		t.Fatal(err)
	}
	lib.put(library.Game{ID: "g1", InstallDir: dir})
	if err := s.RemoveGame("g1", RemoveOptions{DeleteFiles: true}); err == nil {
		t.Fatal("untyped marker authorized deletion")
	}
	if missing(t, dir) {
		t.Fatal("foreign folder deleted")
	}
}
