package install

import (
	"archive/zip"
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// Пропущенная при распаковке запись — это игра, в которой не хватает файла:
// она ставится «успешно» и потом не запускается. Молча такое пропускать
// нельзя, иначе разбирать присланный лог не по чему.
func TestIrregularArchiveEntryIsLogged(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "game.zip")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	w := zip.NewWriter(f)
	header := &zip.FileHeader{Name: "link.txt"}
	header.SetMode(os.ModeSymlink | 0o777)
	entry, err := w.CreateHeader(header)
	if err != nil {
		t.Fatalf("create symlink entry: %v", err)
	}
	if _, err := entry.Write([]byte("elsewhere")); err != nil {
		t.Fatalf("write symlink entry: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}

	logs := captureArchiveLogs(t)
	dest := filepath.Join(dir, "out")
	if err := extractZip(context.Background(), archive, dest, nil); err != nil {
		t.Fatalf("extractZip: %v", err)
	}
	if !strings.Contains(logs.String(), "skip non-regular archive entry") {
		t.Fatalf("пропуск записи не попал в журнал:\n%s", logs.String())
	}
	if _, err := os.Lstat(filepath.Join(dest, "link.txt")); !os.IsNotExist(err) {
		t.Fatalf("нерегулярная запись всё-таки распакована: %v", err)
	}
}

type archiveLogSink struct {
	mu   sync.Mutex
	text bytes.Buffer
}

func (s *archiveLogSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.text.Write(p)
}

func (s *archiveLogSink) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.text.String()
}

func captureArchiveLogs(t *testing.T) *archiveLogSink {
	t.Helper()
	sink := &archiveLogSink{}
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(sink, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return sink
}
