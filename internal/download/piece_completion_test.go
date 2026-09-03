package download

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/anacrolix/torrent/metainfo"
)

func TestNewManagerPersistsPieceCompletionAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	pk := metainfo.PieceKey{InfoHash: metainfo.Hash{1, 2, 3}, Index: 5}

	m1 := mustManagerAt(t, dir)
	if m1.pieceCompletion == nil {
		t.Fatal("manager has no piece completion db")
	}
	if err := m1.pieceCompletion.Set(pk, true); err != nil {
		t.Fatalf("set piece complete: %v", err)
	}
	if err := m1.pieceCompletion.Close(); err != nil {
		t.Fatalf("close piece completion: %v", err)
	}
	m1.pieceCompletion = nil

	m2 := mustManagerAt(t, dir)
	got, err := m2.pieceCompletion.Get(pk)
	if err != nil {
		t.Fatalf("get piece complete: %v", err)
	}
	if !got.Ok || !got.Complete {
		t.Fatalf("piece completion lost across restart: %+v", got)
	}
}

func TestNewManagerRecoversFromCorruptPieceCompletionDB(t *testing.T) {
	dir := t.TempDir()
	metaDir := filepath.Join(dir, "meta")
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(metaDir, completionFileName)
	if err := os.WriteFile(dbPath, []byte("this is not json at all"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := mustManagerAt(t, dir)
	if m.pieceCompletion == nil {
		t.Fatal("manager has no piece completion after recovering from a corrupt db")
	}

	entries, err := os.ReadDir(metaDir)
	if err != nil {
		t.Fatal(err)
	}
	var brokenFound, freshFound bool
	for _, e := range entries {
		switch {
		case strings.HasPrefix(e.Name(), completionFileName+".broken-"):
			brokenFound = true
		case e.Name() == completionFileName:
			freshFound = true
		}
	}
	if !brokenFound {
		t.Fatal("corrupt piece completion db was not renamed aside")
	}
	if !freshFound {
		t.Fatal("no fresh piece completion db was created")
	}
}

func TestNewManagerFailsWhenPieceCompletionDirUnwritable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based permission denial is not meaningful on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores the read-only permission this test relies on")
	}
	dir := t.TempDir()
	//nolint:gosec // G302: каталог, а не файл: без бита x он не читается; тесту нужен read-only каталог, а не 0600
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		//nolint:gosec // G302: возвращаем права, снятые выше в этом же тесте
		if err := os.Chmod(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	})

	if _, err := newManagerAt(dir, nil); err == nil {
		t.Fatal("expected an error when the piece completion db cannot be created")
	}
}

func TestPieceCompletionCloseFlushesRecentSet(t *testing.T) {
	dir := t.TempDir()
	pk := metainfo.PieceKey{InfoHash: metainfo.Hash{9}, Index: 2}

	fc, err := openPieceCompletion(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := fc.Set(pk, true); err != nil {
		t.Fatalf("set: %v", err)
	}
	// Close must flush synchronously, well inside completionFlushInterval,
	// instead of waiting for the periodic tick.
	if err := fc.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	reopened, err := openPieceCompletion(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() {
		if err := reopened.Close(); err != nil {
			t.Fatalf("close reopened: %v", err)
		}
	})
	got, err := reopened.Get(pk)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.Ok || !got.Complete {
		t.Fatalf("Set made less than a flush interval before Close was lost: %+v", got)
	}
}

func TestPieceCompletionSurfacesFlushErrorOnClose(t *testing.T) {
	dir := t.TempDir()
	fc, err := openPieceCompletion(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	real, ok := fc.(*fileCompletion)
	if !ok {
		t.Fatalf("openPieceCompletion returned %T, want *fileCompletion", fc)
	}

	pk := metainfo.PieceKey{InfoHash: metainfo.Hash{7}, Index: 0}
	if err := fc.Set(pk, true); err != nil {
		t.Fatalf("set: %v", err)
	}

	// Simulate the periodic flush loop hitting a persist error in the
	// background: it must be remembered, not logged and forgotten, and
	// surfaced by the very next Set or Close.
	injected := errors.New("simulated flush failure")
	real.mu.Lock()
	real.lastErr = injected
	real.mu.Unlock()

	if err := fc.Close(); !errors.Is(err, injected) {
		t.Fatalf("close error = %v, want %v", err, injected)
	}

	// The remembered error is reported once: a second Close must not repeat it.
	if err := fc.Close(); err != nil {
		t.Fatalf("second close = %v, want nil", err)
	}
}

func TestPieceCompletionFlushLoopPersistsWithoutClose(t *testing.T) {
	dir := t.TempDir()
	previous := completionFlushInterval
	completionFlushInterval = 10 * time.Millisecond
	t.Cleanup(func() { completionFlushInterval = previous })

	fc, err := openPieceCompletion(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() {
		if err := fc.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
	})

	pk := metainfo.PieceKey{InfoHash: metainfo.Hash{3}, Index: 1}
	if err := fc.Set(pk, true); err != nil {
		t.Fatalf("set: %v", err)
	}

	waitUntil(t, "background flush to run", func() bool {
		data, err := os.ReadFile(filepath.Join(dir, completionFileName))
		return err == nil && strings.Contains(string(data), pk.InfoHash.HexString())
	})
}
