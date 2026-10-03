package install

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestExtractArchiveRarCancelledDuringDecode(t *testing.T) {
	neverConsultTools(t)
	chunk := bytes.Repeat([]byte{'z'}, 64<<10)
	archive := storedRar(t,
		rarEntry{name: "Game/1.bin", data: chunk},
		rarEntry{name: "Game/2.bin", data: chunk},
		rarEntry{name: "Game/3.bin", data: chunk},
	)
	dest := filepath.Join(t.TempDir(), "out")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := ExtractArchive(ctx, archive, dest, func(Progress) { cancel() })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	var decodeErr *rarDecodeError
	if errors.As(err, &decodeErr) || errors.Is(err, errArchiveToolMissing) || errors.Is(err, errArchiveToolFailed) {
		t.Fatalf("err = %v: a cancelled decode must not look like a decoder failure", err)
	}
	if _, statErr := os.Stat(filepath.Join(dest, "Game", "3.bin")); statErr == nil {
		t.Fatal("extraction went on after the cancel")
	}
}

func TestDecodeRarCancelledBeforeStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dest := t.TempDir()
	err := decodeRar(ctx, storedRar(t, rarEntry{name: "Game/1.bin", data: []byte("data")}), dest, newReporter(nil, 0))
	var decodeErr *rarDecodeError
	if !errors.Is(err, context.Canceled) || errors.As(err, &decodeErr) {
		t.Fatalf("err = %v, want a bare context.Canceled that never starts the external tool search", err)
	}
	if _, statErr := os.Stat(filepath.Join(dest, "Game")); statErr == nil {
		t.Fatal("an entry was extracted after the cancel")
	}
}

func TestCancelDuringRarExtractionCleansPartial(t *testing.T) {
	neverConsultTools(t)
	s, downloads, _ := newTestService(t)
	root := t.TempDir()
	dir := filepath.Join(root, "Game")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	chunk := bytes.Repeat([]byte{'z'}, 512<<10)
	entries := make([]rarEntry, 0, 120)
	for i := 0; i < 120; i++ {
		entries = append(entries, rarEntry{name: "Game/part" + strconv.Itoa(i) + ".bin", data: chunk})
	}
	writeStoredRar(t, filepath.Join(dir, "game.rar"), entries)
	downloads.add("d1", "Game", root)

	dest := filepath.Join(t.TempDir(), "Game")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	item, err := s.Start("d1", StartOptions{Destination: dest})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitFor(t, "extraction to start", func() bool {
		got, ok := s.snapshot(item.ID)
		return ok && got.Status == StatusExtracting && exists(dest+partialSuffix)
	})
	if err := s.Cancel(item.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	s.waitStatus(t, item.ID, StatusCancelled)
	waitFor(t, "partial dir removal", func() bool { return !exists(dest + partialSuffix) })
	if left, err := os.ReadDir(dest); err != nil || len(left) != 0 {
		t.Fatalf("destination contents = %v (%v)", left, err)
	}
}
