package download

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	"typhon/internal/uierr"
)

func TestDeleteDataClaimsTheDownloadWhileTheGuardRuns(t *testing.T) {
	m, root := completedWithFolder(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	refused := uierr.New("test.refused", "in use")
	m.SetDeleteGuard(func(Download) error {
		close(entered)
		<-release
		return refused
	})
	done := make(chan error, 1)
	go func() { done <- m.DeleteData("a") }()
	<-entered

	got, err := m.Get("a")
	if err != nil || !got.Deleting() {
		t.Fatalf("download during the guard = %+v, %v; want it marked deleting", got, err)
	}
	if err := m.DeleteData("a"); !errors.Is(err, errDeleting) {
		t.Fatalf("second DeleteData = %v, want errDeleting", err)
	}
	close(release)
	if err := <-done; !errors.Is(err, refused) {
		t.Fatalf("DeleteData = %v, want the guard error", err)
	}
	got, err = m.Get("a")
	if err != nil || got.Deleting() {
		t.Fatalf("download after a refusal = %+v, %v; want the claim cleared", got, err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("files removed despite the refusal: %v", err)
	}
}

func TestDeleteDataKeepsTheClaimUntilTheRecordIsGone(t *testing.T) {
	m, root := completedWithFolder(t)
	var seen Download
	m.SetDeleteGuard(func(d Download) error {
		seen = d
		return nil
	})
	if err := m.DeleteData("a"); err != nil {
		t.Fatal(err)
	}
	if !seen.Deleting() {
		t.Fatal("the guard must already see the claim")
	}
	if _, err := m.Get("a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after delete = %v, want ErrNotFound", err)
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("folder survived: %v", err)
	}
}

func TestDeleteDataWritesDownTheFilesBeforeTheRecordGoes(t *testing.T) {
	failure := errors.New("access denied")
	m, root := completedWithFolder(t)
	old := removeAll
	removeAll = func(string) error { return failure }
	t.Cleanup(func() { removeAll = old })

	if err := m.DeleteData("a"); !errors.Is(err, errDataLeft) {
		t.Fatalf("DeleteData = %v, want errDataLeft", err)
	}
	pending := m.PendingCleanups()
	if len(pending) != 2 || pending[0].Path != root || pending[1].Path != root+PartFileSuffix {
		t.Fatalf("pending = %+v, want the folder and its part file", pending)
	}

	restarted := mustManagerAt(t, m.store.dir)
	restarted.mu.Lock()
	err := restarted.loadLocked()
	restarted.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if got := restarted.PendingCleanups(); len(got) != 2 {
		t.Fatalf("pending after a restart = %+v, want the two paths back", got)
	}
	removeAll = old
	if err := restarted.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := restarted.ServiceShutdown(); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("starting the manager deleted a leftover by itself: %v", err)
	}
	if got := restarted.PendingCleanups(); len(got) != 2 {
		t.Fatalf("pending after startup = %+v, want it untouched", got)
	}
	if err := restarted.RemoveCleanup(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("explicit removal left the folder: %v", err)
	}
}

func TestDeleteDataRefusesWhenTheCleanupListCannotBeSaved(t *testing.T) {
	m, root := completedWithFolder(t)
	if err := os.Mkdir(filepath.Join(m.store.dir, "cleanup.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := m.DeleteData("a"); err == nil {
		t.Fatal("DeleteData must fail when it cannot write the files down")
	}
	if got, err := m.Get("a"); err != nil || got.Deleting() {
		t.Fatalf("record = %+v, %v; want it kept and unclaimed", got, err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("files removed: %v", err)
	}
	if len(m.PendingCleanups()) != 0 {
		t.Fatal("a list that was not saved must not stay in memory")
	}
}

func TestDeleteDataRefusesWhenTheCleanupListIsUnreadable(t *testing.T) {
	m, root := completedWithFolder(t)
	m.mu.Lock()
	m.cleanupErr = errors.New("corrupt")
	m.mu.Unlock()
	if err := m.DeleteData("a"); err == nil {
		t.Fatal("DeleteData must refuse while the list is unreadable")
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("files removed: %v", err)
	}
}

func TestRemoveCleanup(t *testing.T) {
	setup := func(t *testing.T) (*Manager, string) {
		m := newTestManager(t, 2)
		dir := filepath.Join(t.TempDir(), "left")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		m.mu.Lock()
		err := m.addCleanupLocked("", []string{dir})
		m.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
		return m, dir
	}
	t.Run("removes the files and the entry", func(t *testing.T) {
		m, dir := setup(t)
		if err := m.RemoveCleanup(dir); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) || len(m.PendingCleanups()) != 0 {
			t.Fatalf("left: %v / %v", err, m.PendingCleanups())
		}
	})
	t.Run("unknown path", func(t *testing.T) {
		m, _ := setup(t)
		if err := m.RemoveCleanup(t.TempDir()); !errors.Is(err, errCleanupUnknown) {
			t.Fatalf("RemoveCleanup = %v, want errCleanupUnknown", err)
		}
	})
	t.Run("a new download claimed the folder", func(t *testing.T) {
		m, dir := setup(t)
		m.addTestItem("fresh", StatusDownloading)
		m.mu.Lock()
		d := m.findLocked("fresh")
		d.Destination, d.root = filepath.Dir(dir), filepath.Base(dir)
		m.mu.Unlock()
		if err := m.RemoveCleanup(dir); !errors.Is(err, errCleanupInUse) {
			t.Fatalf("RemoveCleanup = %v, want errCleanupInUse", err)
		}
		if _, err := os.Stat(dir); err != nil {
			t.Fatalf("folder removed: %v", err)
		}
	})
	t.Run("removal failure keeps the entry", func(t *testing.T) {
		m, dir := setup(t)
		old := removeAll
		removeAll = func(string) error { return errors.New("locked") }
		t.Cleanup(func() { removeAll = old })
		if err := m.RemoveCleanup(dir); err == nil {
			t.Fatal("want the removal error")
		}
		if len(m.PendingCleanups()) != 1 {
			t.Fatal("entry lost although the files are still there")
		}
	})
	t.Run("a link that replaced the folder is refused", func(t *testing.T) {
		m, dir := setup(t)
		target := t.TempDir()
		if err := os.WriteFile(filepath.Join(target, "precious.bin"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.RemoveAll(dir); err != nil {
			t.Fatal(err)
		}
		linkDir(t, dir, target)
		if err := m.RemoveCleanup(dir); !errors.Is(err, errCleanupUnsafe) {
			t.Fatalf("RemoveCleanup = %v, want errCleanupUnsafe", err)
		}
		if _, err := os.Stat(filepath.Join(target, "precious.bin")); err != nil {
			t.Fatalf("link target touched: %v", err)
		}
	})
}

// linkDir links a folder the way the platform really does it: Windows folders
// by junction, which needs no privilege.
func linkDir(t *testing.T, link, target string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		//nolint:gosec // G204: arguments are t.TempDir() paths, no external input (invariant 32 does not apply)
		if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
			t.Fatalf("junction: %v (%s)", err, out)
		}
		return
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}
