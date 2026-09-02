package download

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

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
	dbPath := filepath.Join(metaDir, ".torrent.bolt.db")
	if err := os.WriteFile(dbPath, []byte("this is not a bolt database"), 0o644); err != nil {
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
		case strings.HasPrefix(e.Name(), ".torrent.bolt.db.broken-"):
			brokenFound = true
		case e.Name() == ".torrent.bolt.db":
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
