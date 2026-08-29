package lan

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/anacrolix/torrent/metainfo"
)

func writeTree(t *testing.T, root string) {
	t.Helper()
	files := map[string]string{
		"game.exe":          "stub executable",
		"data/assets.bin":   "some binary payload, repeated a bit to cross a byte or two",
		"data/nested/x.txt": "nested file",
	}
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBuildInfoMatchesReference(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root)

	var reference metainfo.Info
	if err := reference.BuildFromFilePath(root); err != nil {
		t.Fatalf("reference build: %v", err)
	}

	mine, err := BuildInfo(context.Background(), root, nil)
	if err != nil {
		t.Fatalf("BuildInfo: %v", err)
	}

	if mine.Name != reference.Name {
		t.Errorf("Name = %q, want %q", mine.Name, reference.Name)
	}
	if mine.PieceLength != reference.PieceLength {
		t.Errorf("PieceLength = %d, want %d", mine.PieceLength, reference.PieceLength)
	}
	if !reflect.DeepEqual(mine.Files, reference.Files) {
		t.Errorf("Files = %+v, want %+v", mine.Files, reference.Files)
	}
	if !reflect.DeepEqual(mine.Pieces, reference.Pieces) {
		t.Errorf("Pieces mismatch: got %x, want %x", mine.Pieces, reference.Pieces)
	}
}

func TestBuildInfoCancelled(t *testing.T) {
	root := t.TempDir()
	// A payload large enough that GeneratePieces needs more than one Read
	// call, so cancellation lands mid-hash rather than before the first byte.
	big := make([]byte, 4*1024*1024)
	if err := os.WriteFile(filepath.Join(root, "big.bin"), big, 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := BuildInfo(ctx, root, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("BuildInfo error = %v, want context.Canceled", err)
	}
}

func TestBuildInfoRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root)
	target := filepath.Join(root, "game.exe")
	link := filepath.Join(root, "link.exe")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink not supported on this system: %v", err)
	}

	_, err := BuildInfo(context.Background(), root, nil)
	if !errors.Is(err, errSymlinkInShare) {
		t.Fatalf("BuildInfo error = %v, want errSymlinkInShare", err)
	}
}

func TestBuildInfoRejectsNonRegular(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root)
	if err := os.Mkdir(filepath.Join(root, "empty-but-fine"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A named pipe is the simplest non-regular, non-symlink, non-directory
	// entry available without platform-specific device creation; skip where
	// FIFOs are not supported (e.g. plain Windows filesystems).
	fifoErr := makeFifo(t, filepath.Join(root, "pipe"))
	if fifoErr != nil {
		t.Skipf("cannot create a non-regular file on this system: %v", fifoErr)
	}

	_, err := BuildInfo(context.Background(), root, nil)
	if !errors.Is(err, errIrregularInShare) {
		t.Fatalf("BuildInfo error = %v, want errIrregularInShare", err)
	}
}

func TestBuildInfoProgressMonotonic(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root)

	var last int64 = -1
	var sawFull bool
	err := func() error {
		_, err := BuildInfo(context.Background(), root, func(p Progress) {
			if p.ProcessedBytes < last {
				t.Fatalf("progress went backwards: %d after %d", p.ProcessedBytes, last)
			}
			last = p.ProcessedBytes
			if p.ProcessedBytes == p.TotalBytes && p.TotalBytes > 0 {
				sawFull = true
			}
		})
		return err
	}()
	if err != nil {
		t.Fatalf("BuildInfo: %v", err)
	}
	if !sawFull {
		t.Fatal("progress never reached total")
	}
}

func TestFingerprintChangesWithContent(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root)

	fp1, err := fingerprint(context.Background(), root)
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	fp2, err := fingerprint(context.Background(), root)
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	if fp1 != fp2 {
		t.Fatalf("fingerprint not stable across calls: %s != %s", fp1, fp2)
	}

	exe := filepath.Join(root, "game.exe")
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(exe, future, future); err != nil {
		t.Fatal(err)
	}
	fp3, err := fingerprint(context.Background(), root)
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	if fp3 == fp1 {
		t.Fatal("fingerprint did not change after mtime changed")
	}
}
