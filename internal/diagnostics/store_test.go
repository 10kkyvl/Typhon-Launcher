package diagnostics

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSavePendingCapsAtTwentyFilesEvictingOldest(t *testing.T) {
	dir := t.TempDir()
	base := time.Now()
	var firstPath string

	for i := 0; i < 21; i++ {
		now := base.Add(time.Duration(i) * time.Second)
		if err := savePending(dir, now, []reportPayload{{ErrorID: "e"}}); err != nil {
			t.Fatalf("savePending #%d: %v", i, err)
		}
		if i == 0 {
			names, err := listPendingFiles(dir)
			if err != nil {
				t.Fatalf("listPendingFiles: %v", err)
			}
			if len(names) != 1 {
				t.Fatalf("pending files after first save = %d, want 1", len(names))
			}
			firstPath = filepath.Join(dir, names[0])
		}
	}

	names, err := listPendingFiles(dir)
	if err != nil {
		t.Fatalf("listPendingFiles: %v", err)
	}
	if len(names) != 20 {
		t.Fatalf("pending files = %d, want capped at 20", len(names))
	}
	if _, err := os.Stat(firstPath); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("oldest pending file should have been evicted")
	}
}

func TestListPendingFilesOrdersOldestFirst(t *testing.T) {
	dir := t.TempDir()
	base := time.Now()
	for i := 0; i < 5; i++ {
		now := base.Add(time.Duration(i) * time.Second)
		if err := savePending(dir, now, []reportPayload{{ErrorID: "e"}}); err != nil {
			t.Fatalf("savePending #%d: %v", i, err)
		}
	}

	names, err := listPendingFiles(dir)
	if err != nil {
		t.Fatalf("listPendingFiles: %v", err)
	}
	if len(names) != 5 {
		t.Fatalf("pending files = %d, want 5", len(names))
	}
	for i := 1; i < len(names); i++ {
		if pendingTimestamp(names[i-1]) >= pendingTimestamp(names[i]) {
			t.Fatalf("pending files not ordered oldest first: %v", names)
		}
	}
}

func TestListPendingFilesMissingDirIsEmpty(t *testing.T) {
	names, err := listPendingFiles(filepath.Join(t.TempDir(), "absent"))
	if err != nil {
		t.Fatalf("listPendingFiles() error = %v, want nil", err)
	}
	if len(names) != 0 {
		t.Fatalf("names = %v, want none", names)
	}
}

func TestSaveAndLoadPendingRoundTrips(t *testing.T) {
	dir := t.TempDir()
	batch := []reportPayload{{ErrorID: "e1", Component: "download"}, {ErrorID: "e2", Component: "install"}}
	if err := savePending(dir, time.Now(), batch); err != nil {
		t.Fatalf("savePending: %v", err)
	}

	names, err := listPendingFiles(dir)
	if err != nil || len(names) != 1 {
		t.Fatalf("listPendingFiles: names=%v err=%v", names, err)
	}

	loaded, err := loadPending(filepath.Join(dir, names[0]))
	if err != nil {
		t.Fatalf("loadPending: %v", err)
	}
	if len(loaded) != 2 || loaded[0].ErrorID != "e1" || loaded[1].ErrorID != "e2" {
		t.Fatalf("loaded = %+v, want the saved batch", loaded)
	}
}

func TestLoadPendingRejectsCorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "1.json")
	if err := os.WriteFile(path, []byte("{ not json"), 0o600); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}
	if _, err := loadPending(path); err == nil {
		t.Fatal("loadPending() error = nil, want a parse error")
	}
}

func TestRemovePendingDirRemovesEverything(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pending")
	if err := savePending(dir, time.Now(), []reportPayload{{ErrorID: "e"}}); err != nil {
		t.Fatalf("savePending: %v", err)
	}
	if err := removePendingDir(dir); err != nil {
		t.Fatalf("removePendingDir: %v", err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("pending dir should be gone")
	}
	if err := removePendingDir(dir); err != nil {
		t.Fatalf("removePendingDir on an already-missing dir: %v", err)
	}
	if err := removePendingDir(""); err != nil {
		t.Fatalf("removePendingDir(\"\") error = %v, want nil", err)
	}
}
