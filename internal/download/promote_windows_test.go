package download

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func holdOpen(t *testing.T, path string) *os.File {
	t.Helper()
	held, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := held.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			t.Errorf("close held handle: %v", err)
		}
	})
	return held
}

func TestRenameExclusiveSeesSharingViolationOnOpenHandle(t *testing.T) {
	final := filepath.Join(t.TempDir(), "game.bin")
	writePart(t, final, 10)
	holdOpen(t, final+PartFileSuffix)

	err := renameExclusive(final+PartFileSuffix, final)

	if err == nil {
		t.Fatal("rename of a file with an open handle succeeded")
	}
	if !isSharingViolation(err) {
		t.Fatalf("err = %v, want a sharing violation", err)
	}
	if _, statErr := os.Stat(final); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("final file after the failed rename: %v, want not exist", statErr)
	}
}

func TestPromoteSucceedsOnceTheHandleIsReleased(t *testing.T) {
	final := filepath.Join(t.TempDir(), "game.bin")
	writePart(t, final, 10)
	held := holdOpen(t, final+PartFileSuffix)
	waits := 0
	p := newPartPromoter()
	p.delays = []time.Duration{0, 0, 0}
	p.wait = func(ctx context.Context, _ time.Duration) error {
		waits++
		if waits == 2 {
			return held.Close()
		}
		return ctx.Err()
	}

	if err := p.promote(context.Background(), final); err != nil {
		t.Fatalf("promote: %v", err)
	}
	if waits != 2 {
		t.Fatalf("waits = %d, want 2", waits)
	}
	if _, err := os.Stat(final); err != nil {
		t.Fatalf("final file: %v", err)
	}
	if _, err := os.Stat(final + PartFileSuffix); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("part file: %v, want not exist", err)
	}
}

func TestPromoteFailsWithSharingViolationWhileHandleStaysOpen(t *testing.T) {
	final := filepath.Join(t.TempDir(), "game.bin")
	writePart(t, final, 10)
	holdOpen(t, final+PartFileSuffix)
	p := newPartPromoter()
	p.delays = []time.Duration{0, 0}
	p.wait = func(ctx context.Context, _ time.Duration) error { return ctx.Err() }

	err := p.promote(context.Background(), final)

	if !errors.Is(err, errPartPromotionFailed) {
		t.Fatalf("err = %v, want errPartPromotionFailed", err)
	}
	if !isSharingViolation(err) {
		t.Fatalf("err = %v, want the sharing violation preserved as the cause", err)
	}
}
