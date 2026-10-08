package download

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"time"

	"typhon/internal/uierr"
)

var (
	errPartPromotionFailed = uierr.New("download.part_promotion_failed", "не удалось завершить файл: временный файл (.part) недоступен или занят другой программой")
	errPartAndFinalExist   = errors.New("рядом лежат и готовый файл, и его .part")
)

const promotedFileMode os.FileMode = 0o444

var promoteDelays = []time.Duration{
	50 * time.Millisecond,
	100 * time.Millisecond,
	200 * time.Millisecond,
	400 * time.Millisecond,
	800 * time.Millisecond,
	1000 * time.Millisecond,
	1000 * time.Millisecond,
	1000 * time.Millisecond,
}

// partPromoter finishes what the torrent engine's own promotion of a finished
// file to its final name failed to do. The engine logs the failure, still marks
// the piece complete and never retries, so without this the download stays
// Failed on a file whose data is entirely on disk.
type partPromoter struct {
	rename    func(from, to string) error
	transient func(error) bool
	wait      func(ctx context.Context, d time.Duration) error
	delays    []time.Duration
}

func newPartPromoter() *partPromoter {
	return &partPromoter{
		rename:    renameExclusive,
		transient: isSharingViolation,
		wait:      waitFor,
		delays:    promoteDelays,
	}
}

func waitFor(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// promoteComplete renames the .part of every selected file that the engine
// reports fully hashed and that exists under no other name. A file the engine
// does not call complete is left to verifyFilesOnDisk, which reports it as
// unfinished.
func (p *partPromoter) promoteComplete(ctx context.Context, files []FileState, paths []string, complete []bool) error {
	for i, f := range files {
		if !f.Selected || i >= len(paths) || i >= len(complete) || !complete[i] {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !onlyPartExists(paths[i]) {
			continue
		}
		if err := p.promote(ctx, paths[i]); err != nil {
			return fmt.Errorf("%s: %w", f.Path, err)
		}
	}
	return nil
}

// onlyPartExists is false for any stat failure other than a missing final
// file: verifyFilesOnDisk stats the same paths right after and reports those
// failures with their own codes.
func onlyPartExists(final string) bool {
	if _, err := os.Stat(final); !errors.Is(err, fs.ErrNotExist) {
		return false
	}
	_, err := os.Stat(final + PartFileSuffix)
	return err == nil
}

func (p *partPromoter) promote(ctx context.Context, final string) error {
	part := final + PartFileSuffix
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := p.rename(part, final)
		switch {
		case err == nil:
			sealPromoted(final)
			return nil
		case errors.Is(err, fs.ErrExist):
			return fmt.Errorf("%w: %w: %w", errPartPromotionFailed, errPartAndFinalExist, err)
		case errors.Is(err, fs.ErrNotExist):
			// The engine finished its own promotion between our check and the
			// rename; the data is under the final name, which is all we need.
			if _, statErr := os.Stat(final); statErr == nil {
				return nil
			}
			return fmt.Errorf("%w: %w", errPartPromotionFailed, err)
		case !p.transient(err) || attempt >= len(p.delays):
			return fmt.Errorf("%w: %w", errPartPromotionFailed, err)
		}
		if err := p.wait(ctx, p.delays[attempt]); err != nil {
			return err
		}
	}
}

// sealPromoted gives the file the mode the engine gives a file it promotes
// itself. Failing to do so leaves a complete, usable file, so it is only logged.
func sealPromoted(final string) {
	if err := os.Chmod(final, promotedFileMode); err != nil {
		slog.Info("set promoted file to read-only", "file", final, "error", err)
	}
}
