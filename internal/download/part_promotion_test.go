package download

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"typhon/internal/uierr"
)

var errStubSharing = errors.New("stub sharing violation")

type renameCall struct{ from, to string }

type promoterStub struct {
	calls []renameCall
	waits []time.Duration
}

func (s *promoterStub) promoter(rename func(from, to string) error) *partPromoter {
	return &partPromoter{
		rename: func(from, to string) error {
			s.calls = append(s.calls, renameCall{from, to})
			return rename(from, to)
		},
		transient: func(err error) bool { return errors.Is(err, errStubSharing) },
		wait: func(ctx context.Context, d time.Duration) error {
			s.waits = append(s.waits, d)
			return ctx.Err()
		},
		delays: []time.Duration{time.Millisecond, 2 * time.Millisecond, 4 * time.Millisecond},
	}
}

func writePart(t *testing.T, final string, size int) {
	t.Helper()
	if err := os.WriteFile(final+PartFileSuffix, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

func sharingError(from, to string) error {
	return &os.LinkError{Op: "rename", Old: from, New: to, Err: errStubSharing}
}

func TestCompletionPromotesPartFileTheEngineLeftBehind(t *testing.T) {
	m := newTestManager(t, 2)
	eng := m.addTestDownload("a")
	dir := t.TempDir()
	final := filepath.Join(dir, "game.bin")
	writePart(t, final, 100)
	eng.finish()
	eng.mu.Lock()
	eng.paths = []string{final}
	eng.mu.Unlock()

	m.sample(context.Background(), time.Now())

	waitUntil(t, "download to settle", func() bool {
		st := m.statusOf(t, "a")
		return st == StatusCompleted || st == StatusFailed
	})
	if got := m.statusOf(t, "a"); got != StatusCompleted {
		d, err := m.Get("a")
		if err != nil {
			t.Fatal(err)
		}
		t.Fatalf("status = %s (%q), want %s", got, d.Error, StatusCompleted)
	}
	if _, err := os.Stat(final); err != nil {
		t.Fatalf("final file after promotion: %v", err)
	}
	if _, err := os.Stat(final + PartFileSuffix); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("part file after promotion: %v, want not exist", err)
	}
}

func TestCompletionFailsWithPromotionErrorNotIncomplete(t *testing.T) {
	m := newTestManager(t, 2)
	stub := &promoterStub{}
	m.promoter = stub.promoter(func(from, to string) error { return sharingError(from, to) })
	eng := m.addTestDownload("a")
	final := filepath.Join(t.TempDir(), "game.bin")
	writePart(t, final, 100)
	eng.finish()
	eng.mu.Lock()
	eng.paths = []string{final}
	eng.mu.Unlock()

	m.sample(context.Background(), time.Now())

	waitUntil(t, "download to fail", func() bool { return m.statusOf(t, "a") == StatusFailed })
	d, err := m.Get("a")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(d.Error, "download.part_promotion_failed") || strings.Contains(d.Error, "download.file_incomplete") {
		t.Fatalf("error = %q, want the promotion code and not the incomplete one", d.Error)
	}
}

func TestPromoteCompleteFiles(t *testing.T) {
	tests := []struct {
		name     string
		complete bool
		wantDone bool
	}{
		{name: "engine reports the file complete", complete: true, wantDone: true},
		{name: "engine reports the file incomplete", complete: false, wantDone: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			final := filepath.Join(dir, "game.bin")
			writePart(t, final, 10)
			files := []FileState{{Path: "game.bin", Size: 10, Selected: true}}
			p := newPartPromoter()

			if err := p.promoteComplete(context.Background(), files, []string{final}, []bool{tc.complete}); err != nil {
				t.Fatalf("promoteComplete: %v", err)
			}
			verr := verifyFilesOnDisk(context.Background(), files, []string{final})
			_, partErr := os.Stat(final + PartFileSuffix)
			if tc.wantDone {
				if verr != nil {
					t.Fatalf("verify after promotion: %v", verr)
				}
				if !errors.Is(partErr, os.ErrNotExist) {
					t.Fatalf("part file after promotion: %v, want not exist", partErr)
				}
				return
			}
			if !errors.Is(verr, errFileIncomplete) {
				t.Fatalf("verify = %v, want errFileIncomplete", verr)
			}
			if partErr != nil {
				t.Fatalf("incomplete part file must stay untouched: %v", partErr)
			}
		})
	}
}

func TestPromoteCompleteSkips(t *testing.T) {
	tests := []struct {
		name     string
		files    []FileState
		complete []bool
		final    bool
	}{
		{
			name:     "unselected file",
			files:    []FileState{{Path: "game.bin", Size: 10, Selected: false}},
			complete: []bool{true},
		},
		{
			name:     "no completion flag for the file",
			files:    []FileState{{Path: "game.bin", Size: 10, Selected: true}},
			complete: nil,
		},
		{
			name:     "final file already present",
			files:    []FileState{{Path: "game.bin", Size: 10, Selected: true}},
			complete: []bool{true},
			final:    true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			final := filepath.Join(t.TempDir(), "game.bin")
			writePart(t, final, 10)
			if tc.final {
				if err := os.WriteFile(final, make([]byte, 10), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			stub := &promoterStub{}

			err := stub.promoter(os.Rename).promoteComplete(context.Background(), tc.files, []string{final}, tc.complete)

			if err != nil {
				t.Fatalf("promoteComplete: %v", err)
			}
			if len(stub.calls) != 0 {
				t.Fatalf("renames = %v, want none", stub.calls)
			}
		})
	}
}

func TestPromoteCompleteStopsOnCancelledContext(t *testing.T) {
	final := filepath.Join(t.TempDir(), "game.bin")
	writePart(t, final, 10)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	files := []FileState{{Path: "game.bin", Size: 10, Selected: true}}

	err := newPartPromoter().promoteComplete(ctx, files, []string{final}, []bool{true})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if _, statErr := os.Stat(final + PartFileSuffix); statErr != nil {
		t.Fatalf("part file: %v", statErr)
	}
}

func TestPromoteNeverOverwritesFinalFile(t *testing.T) {
	dir := t.TempDir()
	final := filepath.Join(dir, "game.bin")
	if err := os.WriteFile(final, []byte("final"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(final+PartFileSuffix, []byte("part"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := newPartPromoter().promote(context.Background(), final)

	if !errors.Is(err, errPartPromotionFailed) || !errors.Is(err, errPartAndFinalExist) {
		t.Fatalf("err = %v, want errPartPromotionFailed wrapping errPartAndFinalExist", err)
	}
	if got, readErr := os.ReadFile(final); readErr != nil || string(got) != "final" {
		t.Fatalf("final file = %q, %v, want it untouched", got, readErr)
	}
	if got, readErr := os.ReadFile(final + PartFileSuffix); readErr != nil || string(got) != "part" {
		t.Fatalf("part file = %q, %v, want it untouched", got, readErr)
	}
}

func TestPromoteRetriesSharingViolation(t *testing.T) {
	t.Run("recovers once the handle is gone", func(t *testing.T) {
		final := filepath.Join(t.TempDir(), "game.bin")
		writePart(t, final, 10)
		stub := &promoterStub{}
		failures := 2
		p := stub.promoter(func(from, to string) error {
			if failures > 0 {
				failures--
				return sharingError(from, to)
			}
			return renameExclusive(from, to)
		})

		if err := p.promote(context.Background(), final); err != nil {
			t.Fatalf("promote: %v", err)
		}
		if len(stub.calls) != 3 {
			t.Fatalf("rename attempts = %d, want 3", len(stub.calls))
		}
		if _, err := os.Stat(final); err != nil {
			t.Fatalf("final file: %v", err)
		}
	})

	t.Run("gives up after the budget with the real cause", func(t *testing.T) {
		final := filepath.Join(t.TempDir(), "game.bin")
		writePart(t, final, 10)
		stub := &promoterStub{}
		p := stub.promoter(func(from, to string) error { return sharingError(from, to) })

		err := p.promote(context.Background(), final)

		if !errors.Is(err, errPartPromotionFailed) || !errors.Is(err, errStubSharing) {
			t.Fatalf("err = %v, want errPartPromotionFailed wrapping the sharing violation", err)
		}
		if errors.Is(err, errFileIncomplete) {
			t.Fatalf("err = %v must not be reported as an incomplete download", err)
		}
		if code := uierr.Code(err); code != "download.part_promotion_failed" {
			t.Fatalf("code = %q, want download.part_promotion_failed", code)
		}
		if len(stub.calls) != len(p.delays)+1 {
			t.Fatalf("rename attempts = %d, want %d", len(stub.calls), len(p.delays)+1)
		}
		if len(stub.waits) != len(p.delays) {
			t.Fatalf("waits = %v, want one per delay %v", stub.waits, p.delays)
		}
		if _, statErr := os.Stat(final + PartFileSuffix); statErr != nil {
			t.Fatalf("part file must stay after a failed promotion: %v", statErr)
		}
	})

	t.Run("other errors are not retried", func(t *testing.T) {
		final := filepath.Join(t.TempDir(), "game.bin")
		writePart(t, final, 10)
		stub := &promoterStub{}
		p := stub.promoter(func(from, to string) error {
			return &os.LinkError{Op: "rename", Old: from, New: to, Err: os.ErrPermission}
		})

		err := p.promote(context.Background(), final)

		if !errors.Is(err, errPartPromotionFailed) || !errors.Is(err, os.ErrPermission) {
			t.Fatalf("err = %v, want errPartPromotionFailed wrapping ErrPermission", err)
		}
		if len(stub.calls) != 1 || len(stub.waits) != 0 {
			t.Fatalf("attempts = %d, waits = %d, want 1 and 0", len(stub.calls), len(stub.waits))
		}
	})

	t.Run("cancelled context stops the retries", func(t *testing.T) {
		final := filepath.Join(t.TempDir(), "game.bin")
		writePart(t, final, 10)
		ctx, cancel := context.WithCancel(context.Background())
		stub := &promoterStub{}
		p := stub.promoter(func(from, to string) error {
			cancel()
			return sharingError(from, to)
		})

		err := p.promote(ctx, final)

		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
		if len(stub.calls) != 1 {
			t.Fatalf("rename attempts = %d, want 1", len(stub.calls))
		}
	})

	t.Run("already cancelled context renames nothing", func(t *testing.T) {
		final := filepath.Join(t.TempDir(), "game.bin")
		writePart(t, final, 10)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		stub := &promoterStub{}

		err := stub.promoter(os.Rename).promote(ctx, final)

		if !errors.Is(err, context.Canceled) || len(stub.calls) != 0 {
			t.Fatalf("err = %v, attempts = %d, want context.Canceled and 0", err, len(stub.calls))
		}
	})
}

func TestPromoteWhenThePartVanishes(t *testing.T) {
	t.Run("engine promoted it first", func(t *testing.T) {
		final := filepath.Join(t.TempDir(), "game.bin")
		writePart(t, final, 10)
		stub := &promoterStub{}
		p := stub.promoter(func(from, to string) error {
			if err := os.Rename(from, to); err != nil {
				return err
			}
			return &os.LinkError{Op: "rename", Old: from, New: to, Err: os.ErrNotExist}
		})

		if err := p.promote(context.Background(), final); err != nil {
			t.Fatalf("promote: %v", err)
		}
	})

	t.Run("neither name exists", func(t *testing.T) {
		final := filepath.Join(t.TempDir(), "game.bin")
		stub := &promoterStub{}

		err := stub.promoter(renameExclusive).promote(context.Background(), final)

		if !errors.Is(err, errPartPromotionFailed) || !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("err = %v, want errPartPromotionFailed wrapping ErrNotExist", err)
		}
	})
}
