package savebackup

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const legacyReplacedSuffix = ".replaced"

// migrateLegacy moves the single snapshot the update flow used to keep in
// <ConfigDir>/saves/<gameID> into the snapshot layout. It copies instead of
// renaming, so at every instant either the old directory or a finished
// snapshot holds the data; a crash after the copy but before the old directory
// is removed is recognized by the digest and only finishes the removal.
func (s *Service) migrateLegacy(ctx context.Context, gameID string) error {
	legacy := filepath.Join(s.legacyRoot, gameID)
	stale := legacy + partialSuffix
	replaced := legacy + legacyReplacedSuffix

	source := ""
	for _, candidate := range []string{legacy, replaced} {
		info, err := os.Stat(candidate)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return errors.Join(errRecovery, err)
		}
		if info.IsDir() {
			source = candidate
			break
		}
	}

	if source != "" {
		info, err := os.Stat(source)
		if err != nil {
			return errors.Join(errRecovery, err)
		}
		if _, _, err := s.capture(ctx, gameID, source, KindUpdate, captureOpts{dedup: dedupAny, at: info.ModTime()}); err != nil {
			return fmt.Errorf("перенос снимка сохранений из %s: %w", source, err)
		}
	}
	var errs []error
	for _, path := range []string{legacy, replaced, stale} {
		if err := os.RemoveAll(path); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(append([]error{errRecovery}, errs...)...)
	}
	return nil
}
