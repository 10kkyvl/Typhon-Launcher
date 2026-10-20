package download

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func addPending(t *testing.T, m *Manager, id string, paths ...string) {
	t.Helper()
	m.mu.Lock()
	err := m.addCleanupLocked(id, paths)
	m.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
}

func TestCleanupVerdictRefusesWhatARecordCouldBeUsing(t *testing.T) {
	dest := t.TempDir()
	cases := []struct {
		name  string
		setup func(m *Manager)
		path  string
		want  bool
	}{
		{"nothing refers to it", func(*Manager) {}, filepath.Join(dest, "left"), true},
		{"a record of the same id still exists", func(m *Manager) {
			m.addTestItem("gone", StatusCompleted)
		}, filepath.Join(dest, "left"), false},
		{"a current download owns the folder", func(m *Manager) {
			d := m.addTestItem("new", StatusDownloading)
			m.mu.Lock()
			d.Destination, d.root = dest, "left"
			m.mu.Unlock()
		}, filepath.Join(dest, "left"), false},
		{"a record whose folder is not known yet covers its destination", func(m *Manager) {
			d := m.addTestItem("meta", StatusMetadata)
			m.mu.Lock()
			d.Destination, d.root = dest, ""
			m.mu.Unlock()
		}, filepath.Join(dest, "left"), false},
		{"a flat record without a file list covers its destination", func(m *Manager) {
			d := m.addTestItem("flat", StatusDownloading)
			m.mu.Lock()
			d.Destination, d.Flat, d.Files = dest, true, nil
			m.mu.Unlock()
		}, filepath.Join(dest, "left"), false},
		{"an in-place record covers its destination", func(m *Manager) {
			d := m.addTestItem("inplace", StatusCompleted)
			m.mu.Lock()
			d.Destination, d.InPlace = dest, true
			m.mu.Unlock()
		}, filepath.Join(dest, "left"), false},
		{"a record elsewhere does not matter", func(m *Manager) {
			d := m.addTestItem("far", StatusDownloading)
			m.mu.Lock()
			d.Destination = t.TempDir()
			m.mu.Unlock()
		}, filepath.Join(dest, "left"), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := newTestManager(t, 2)
			addPending(t, m, "gone", c.path)
			c.setup(m)
			ok, code, reason := m.CleanupVerdict(c.path)
			if ok != c.want {
				t.Fatalf("CleanupVerdict = %v %q (%s), want %v", ok, code, reason, c.want)
			}
			if !ok && (code == "" || reason == "") {
				t.Fatal("a refusal needs a code and a reason")
			}
		})
	}
}

func TestPendingEntryOfALiveDownloadIsPrunedByRemove(t *testing.T) {
	m, root := completedWithFolder(t)
	// What a failed record drop followed by a failed rollback leaves behind:
	// the list names the folder of a download that still exists.
	addPending(t, m, "a", root, root+PartFileSuffix)

	if ok, code, _ := m.CleanupVerdict(root); ok || code == "" {
		t.Fatalf("CleanupVerdict = %v %q, want a refusal while the record exists", ok, code)
	}
	if _, err := m.ReserveCleanup(root); !errors.Is(err, errCleanupInUse) {
		t.Fatalf("ReserveCleanup = %v, want errCleanupInUse", err)
	}
	if err := m.Remove("a"); err != nil {
		t.Fatal(err)
	}
	if got := m.PendingCleanups(); len(got) != 0 {
		t.Fatalf("pending after Remove = %+v, want the keep-files removal to prune it", got)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("Remove must keep the files: %v", err)
	}
}

func TestReserveCleanupBarsNewDownloadsUntilReleased(t *testing.T) {
	mi, _ := makeSeedData(t, 64<<10)
	hash := mi.HashInfoBytes().HexString()
	m := newTestManager(t, 5)
	m.client = offlineClient(t)
	if err := m.store.saveMetainfo(hash, mi); err != nil {
		t.Fatalf("save metainfo: %v", err)
	}
	dest := t.TempDir()
	info, err := mi.UnmarshalInfo()
	if err != nil {
		t.Fatal(err)
	}
	left := filepath.Join(dest, info.BestName())
	if err := os.MkdirAll(left, 0o755); err != nil {
		t.Fatal(err)
	}
	addPending(t, m, "gone", left)

	release, err := m.ReserveCleanup(left)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.AddTask(context.Background(), AddRequest{InfoHash: hash, Destination: dest}); !errors.Is(err, errCleanupClaimed) {
		t.Fatalf("AddTask under a claim = %v, want errCleanupClaimed", err)
	}
	if len(m.List()) != 0 {
		t.Fatal("a refused task must not be tracked")
	}
	release()
	if _, err := m.AddTask(context.Background(), AddRequest{InfoHash: hash, Destination: dest}); err != nil {
		t.Fatalf("AddTask after the release = %v", err)
	}
}

func TestReserveCleanupRefusesUnknownAndClaimedPaths(t *testing.T) {
	m := newTestManager(t, 2)
	if _, err := m.ReserveCleanup(t.TempDir()); !errors.Is(err, errCleanupUnknown) {
		t.Fatalf("ReserveCleanup(unknown) = %v, want errCleanupUnknown", err)
	}
	dest := t.TempDir()
	left := filepath.Join(dest, "left")
	addPending(t, m, "gone", left)
	d := m.addTestItem("new", StatusDownloading)
	m.mu.Lock()
	d.Destination, d.root = dest, "left"
	m.mu.Unlock()
	if _, err := m.ReserveCleanup(left); !errors.Is(err, errCleanupInUse) {
		t.Fatalf("ReserveCleanup(claimed by a record) = %v, want errCleanupInUse", err)
	}
}

func TestDeleteDataClearsItsClaimWhenTheRecheckRefuses(t *testing.T) {
	m, root := completedWithFolder(t)
	m.SetDeleteGuard(func(Download) error {
		m.mu.Lock()
		m.findLocked("a").Status = StatusDownloading
		m.mu.Unlock()
		return nil
	})
	if err := m.DeleteData("a"); !errors.Is(err, errUnavailable) {
		t.Fatalf("DeleteData = %v, want errUnavailable", err)
	}
	got, err := m.Get("a")
	if err != nil || got.Deleting() {
		t.Fatalf("download after the recheck = %+v, %v; want the claim cleared", got, err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("files removed: %v", err)
	}
}
