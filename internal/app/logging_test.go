package app

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestNewRotatingWriter_RejectsBadInput(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		backups int
	}{
		{"empty path", "", 3},
		{"zero backups", filepath.Join(t.TempDir(), "typhon.log"), 0},
		{"negative backups", filepath.Join(t.TempDir(), "typhon.log"), -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := newRotatingWriter(tc.path, 1024, tc.backups); err == nil {
				t.Fatal("expected an error, got nil")
			}
		})
	}
}

func TestRotatingWriter_RotatesAtThreshold(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "typhon.log")
	w, err := newRotatingWriter(path, 20, 3)
	if err != nil {
		t.Fatalf("newRotatingWriter: %v", err)
	}
	defer func() {
		if err := w.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
	}()

	chunk := []byte("0123456789")
	for i := 0; i < 2; i++ {
		if _, err := w.Write(chunk); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}
	if _, err := os.Stat(path + ".1"); !os.IsNotExist(err) {
		t.Fatalf("expected no backup yet, stat err: %v", err)
	}

	// written == 20 now; one more chunk pushes past maxSize and must rotate
	// before this write lands.
	if _, err := w.Write(chunk); err != nil {
		t.Fatalf("write 3: %v", err)
	}

	backup := path + ".1"
	backupInfo, err := os.Stat(backup)
	if err != nil {
		t.Fatalf("expected backup %s to exist: %v", backup, err)
	}
	if backupInfo.Size() != 20 {
		t.Fatalf("expected backup to hold the pre-rotation 20 bytes, got %d", backupInfo.Size())
	}

	current, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat current log: %v", err)
	}
	if current.Size() != int64(len(chunk)) {
		t.Fatalf("expected current log to hold only the latest chunk, got %d bytes", current.Size())
	}
}

func TestRotatingWriter_RetainsLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "typhon.log")
	const backups = 2
	w, err := newRotatingWriter(path, 10, backups)
	if err != nil {
		t.Fatalf("newRotatingWriter: %v", err)
	}
	defer func() {
		if err := w.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
	}()

	chunk := []byte("AAAAA")
	for i := 0; i < 12; i++ {
		if _, err := w.Write(chunk); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}

	for n := 1; n <= backups; n++ {
		p := fmt.Sprintf("%s.%d", path, n)
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected retained backup %s: %v", p, err)
		}
	}
	beyond := fmt.Sprintf("%s.%d", path, backups+1)
	if _, err := os.Stat(beyond); !os.IsNotExist(err) {
		t.Fatalf("expected %s to not exist (limit is %d backups), stat err: %v", beyond, backups, err)
	}
}

func TestRotatingWriter_FailingRemoveReported(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "typhon.log")
	oldest := path + ".1"
	if err := os.Mkdir(oldest, 0o755); err != nil {
		t.Fatalf("seed oldest dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldest, "stray.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("seed stray file: %v", err)
	}

	w, err := newRotatingWriter(path, 5, 1)
	if err != nil {
		t.Fatalf("newRotatingWriter: %v", err)
	}
	defer func() {
		if err := w.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
	}()

	if _, err := w.Write([]byte("12")); err != nil {
		t.Fatalf("write 1: %v", err)
	}
	_, err = w.Write([]byte("123456"))
	if err == nil {
		t.Fatal("expected rotation to fail because the oldest backup directory is not empty")
	}
	if !strings.Contains(err.Error(), oldest) {
		t.Fatalf("expected error to mention %s, got: %v", oldest, err)
	}

	// The failed rotation must not lose data: the write itself still landed.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read current log: %v", err)
	}
	if !strings.Contains(string(data), "12") || !strings.Contains(string(data), "123456") {
		t.Fatalf("expected current log to retain the write made during the failed rotation, got: %q", data)
	}

	// Once the obstruction is gone, the writer self-heals on the next write.
	if err := os.RemoveAll(oldest); err != nil {
		t.Fatalf("clear obstruction: %v", err)
	}
	if _, err := w.Write([]byte("more")); err != nil {
		t.Fatalf("write after clearing obstruction: %v", err)
	}
	if _, err := os.Stat(oldest); err != nil {
		t.Fatalf("expected rotation to succeed and recreate the backup: %v", err)
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read current log after recovery: %v", err)
	}
	if string(current) != "more" {
		t.Fatalf("expected current log to hold only the post-rotation write, got: %q", current)
	}
}

func TestRotatingWriter_FailingRenameReported(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "typhon.log")
	backup1 := path + ".1"
	if err := os.WriteFile(backup1, []byte("previous backup"), 0o644); err != nil {
		t.Fatalf("seed backup: %v", err)
	}
	// Go opens files without FILE_SHARE_DELETE on Windows, so a held-open
	// handle blocks any rename/remove targeting this exact path from
	// elsewhere in the process, standing in for a locked file (AV scan,
	// another process) without needing OS-specific syscalls.
	locked, err := os.Open(backup1)
	if err != nil {
		t.Fatalf("lock backup: %v", err)
	}
	closeLock := func() {
		if locked == nil {
			return
		}
		if err := locked.Close(); err != nil {
			t.Fatalf("close lock: %v", err)
		}
		locked = nil
	}
	defer closeLock()

	w, err := newRotatingWriter(path, 5, 2)
	if err != nil {
		t.Fatalf("newRotatingWriter: %v", err)
	}
	defer func() {
		if err := w.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
	}()

	if _, err := w.Write([]byte("12")); err != nil {
		t.Fatalf("write 1: %v", err)
	}
	_, err = w.Write([]byte("123456"))
	if err == nil {
		t.Fatal("expected rotation to fail while typhon.log.1 is locked")
	}
	if !strings.Contains(err.Error(), backup1) {
		t.Fatalf("expected error to mention %s, got: %v", backup1, err)
	}

	// Once the lock is released, the writer self-heals on the next write.
	closeLock()
	if _, err := w.Write([]byte("more")); err != nil {
		t.Fatalf("write after releasing the lock: %v", err)
	}
	if _, err := os.Stat(path + ".2"); err != nil {
		t.Fatalf("expected the previously locked backup to shift to .2: %v", err)
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read current log after recovery: %v", err)
	}
	if string(current) != "more" {
		t.Fatalf("expected current log to hold only the post-rotation write, got: %q", current)
	}
}

func TestRotatingWriter_ConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "typhon.log")
	w, err := newRotatingWriter(path, 256, 4)
	if err != nil {
		t.Fatalf("newRotatingWriter: %v", err)
	}
	defer func() {
		if err := w.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
	}()

	const goroutines = 20
	const perGoroutine = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			line := []byte(fmt.Sprintf("goroutine-%02d-line\n", id))
			for j := 0; j < perGoroutine; j++ {
				if _, err := w.Write(line); err != nil {
					t.Errorf("write from goroutine %d: %v", id, err)
				}
			}
		}(i)
	}
	wg.Wait()
}

func TestInitLogging_OpenErrorSurfaced(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocked-appdata")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("seed blocker file: %v", err)
	}
	t.Setenv("AppData", blocker)

	origHandler := slog.Default()
	defer slog.SetDefault(origHandler)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	origStderr := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()

	initErr := InitLogging()
	if initErr == nil {
		t.Fatal("expected InitLogging to surface the log file open error")
	}

	slog.Info("still alive")

	if err := w.Close(); err != nil {
		t.Fatalf("close pipe writer: %v", err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("close pipe reader: %v", err)
	}

	if !strings.Contains(string(data), "still alive") {
		t.Fatalf("expected stderr sink to still receive log lines despite the open error, got: %q", data)
	}
}

func TestLogger_AttachesComponent(t *testing.T) {
	var buf bytes.Buffer
	origHandler := slog.Default()
	defer slog.SetDefault(origHandler)
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))

	Logger("download").Info("started")

	out := buf.String()
	if !strings.Contains(out, "component=download") {
		t.Fatalf("expected component=download field in log line, got: %q", out)
	}
}
