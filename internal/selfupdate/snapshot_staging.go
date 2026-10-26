package selfupdate

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"typhon/internal/storage"
	"typhon/internal/uierr"
)

const (
	stagingDirName     = "previous.staging"
	stagingRecordName  = "previous.staging.json"
	snapshotFailedName = "snapshot-failed.json"
	snapshotFailedVer  = 1
	previousTempAge    = time.Hour
	previousTempPrefix = ".tmp-"
)

// ErrSnapshotFailed marks an update that went ahead without saving the
// launcher it replaced: the rollback to that version is not on offer.
var ErrSnapshotFailed = uierr.New("selfupdate.snapshot_failed", "selfupdate: the previous version could not be saved, there is nothing to roll back to")

func stagingDir(configDir string) (string, error) {
	dir, err := CacheDir(configDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, stagingDirName), nil
}

func stagingRecordPath(configDir string) (string, error) {
	dir, err := CacheDir(configDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, stagingRecordName), nil
}

func snapshotFailedPath(configDir string) (string, error) {
	dir, err := CacheDir(configDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, snapshotFailedName), nil
}

// discardStaging removes a snapshot that was never promoted. The record goes
// first: a directory without a record is never promoted, a record without its
// directory is the sign of a promotion that stopped half-way.
func discardStaging(configDir string) error {
	dir, err := stagingDir(configDir)
	if err != nil {
		return err
	}
	recordPath, err := stagingRecordPath(configDir)
	if err != nil {
		return err
	}
	if err := removeIfExists(recordPath); err != nil {
		return err
	}
	return os.RemoveAll(dir)
}

// commitSnapshot makes the staged snapshot the one rollbacks use, once the
// update it was taken for has gone through. The old snapshot is dropped first
// and the staged one moved in as a directory and then a record, in that
// order; recoverSnapshot finishes the move if the process dies in the middle.
func commitSnapshot(configDir string) error {
	recordPath, err := stagingRecordPath(configDir)
	if err != nil {
		return err
	}
	if _, err := loadRecord(recordPath); err != nil {
		return fmt.Errorf("selfupdate: the staged snapshot cannot be promoted: %w", err)
	}
	return promoteStaging(configDir)
}

func promoteStaging(configDir string) error {
	staged, err := stagingDir(configDir)
	if err != nil {
		return err
	}
	stagedRecord, err := stagingRecordPath(configDir)
	if err != nil {
		return err
	}
	if err := dropPrevious(configDir); err != nil {
		return fmt.Errorf("selfupdate: drop the replaced snapshot: %w", err)
	}
	live, err := PreviousDir(configDir)
	if err != nil {
		return err
	}
	liveRecord, err := previousRecordPath(configDir)
	if err != nil {
		return err
	}
	if err := storage.Rename(staged, live); err != nil {
		return fmt.Errorf("selfupdate: move the new snapshot in: %w", err)
	}
	return finishPromotion(configDir, stagedRecord, liveRecord)
}

func finishPromotion(configDir, stagedRecord, liveRecord string) error {
	if err := storage.Rename(stagedRecord, liveRecord); err != nil {
		return fmt.Errorf("selfupdate: move the new snapshot record in: %w", err)
	}
	failedPath, err := snapshotFailedPath(configDir)
	if err != nil {
		return err
	}
	if err := removeIfExists(failedPath); err != nil {
		return fmt.Errorf("selfupdate: clear the snapshot failure note: %w", err)
	}
	return nil
}

// recoverSnapshot settles a staged snapshot found at start. current is the
// running version: if it is newer than the version the staged copy holds, the
// update it was taken for went through and the promotion is finished;
// otherwise the update did not happen and the copy is dropped.
func recoverSnapshot(configDir, current string) error {
	staged, err := stagingDir(configDir)
	if err != nil {
		return err
	}
	stagedRecord, err := stagingRecordPath(configDir)
	if err != nil {
		return err
	}
	liveRecord, err := previousRecordPath(configDir)
	if err != nil {
		return err
	}
	_, dirErr := os.Stat(staged)
	_, recErr := os.Stat(stagedRecord)
	hasDir, hasRecord := dirErr == nil, recErr == nil
	for _, e := range []error{dirErr, recErr} {
		if e != nil && !errors.Is(e, fs.ErrNotExist) {
			return fmt.Errorf("selfupdate: look for a staged snapshot: %w", e)
		}
	}
	switch {
	case !hasDir && !hasRecord:
		return nil
	case !hasRecord:
		return discardStaging(configDir)
	}

	rec, err := loadRecord(stagedRecord)
	if err != nil {
		slog.Warn("selfupdate: the staged snapshot is unusable, dropping it", "error", err)
		return discardStaging(configDir)
	}
	if !hasDir {
		live, err := PreviousDir(configDir)
		if err != nil {
			return err
		}
		if _, err := os.Stat(live); err != nil {
			slog.Warn("selfupdate: a staged record without any snapshot, dropping it", "error", err)
			return discardStaging(configDir)
		}
		return finishPromotion(configDir, stagedRecord, liveRecord)
	}
	newer, err := IsNewer(current, rec.Version)
	if err != nil || !newer {
		return discardStaging(configDir)
	}
	return promoteStaging(configDir)
}

// snapshotFailure is the note an update leaves when it went ahead without
// saving the launcher it replaced.
type snapshotFailure struct {
	Version string    `json:"version"`
	Reason  string    `json:"reason"`
	At      time.Time `json:"at"`
}

func recordSnapshotFailure(configDir, ver string, cause error) error {
	path, err := snapshotFailedPath(configDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), cacheDirMode); err != nil {
		return err
	}
	return storage.Save(path, snapshotFailedVer, snapshotFailure{Version: ver, Reason: cause.Error(), At: time.Now().UTC()})
}

// loadSnapshotFailure reports the note, if there is one. A note that cannot be
// read is an error, not an absent note: the snapshot it speaks of is in doubt.
func loadSnapshotFailure(configDir string) (snapshotFailure, bool, error) {
	path, err := snapshotFailedPath(configDir)
	if err != nil {
		return snapshotFailure{}, false, err
	}
	var f snapshotFailure
	if err := storage.Load(path, snapshotFailedVer, nil, &f); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return snapshotFailure{}, false, nil
		}
		return snapshotFailure{}, false, err
	}
	return f, true, nil
}

// sweepPreviousTemps removes the temp files a worker that died left inside the
// snapshot directories. Only the updater's own directories are searched.
func sweepPreviousTemps(configDir string) {
	for _, name := range []func(string) (string, error){PreviousDir, stagingDir} {
		root, err := name(configDir)
		if err != nil {
			slog.Warn("selfupdate: locate snapshot directory", "error", err)
			continue
		}
		err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				if errors.Is(walkErr, fs.ErrNotExist) {
					return nil
				}
				return walkErr
			}
			if d.IsDir() || !strings.HasPrefix(d.Name(), previousTempPrefix) || !d.Type().IsRegular() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if time.Since(info.ModTime()) < previousTempAge {
				return nil
			}
			if err := removeIfExists(path); err != nil {
				slog.Warn("selfupdate: remove a snapshot temp file, will retry on the next start", "path", path, "error", err)
			}
			return nil
		})
		if err != nil {
			slog.Warn("selfupdate: sweep snapshot temp files", "dir", root, "error", err)
		}
	}
}

// RecoverInterrupted settles what an update or a rollback that died left
// behind. It has to run before any service reads its state: restoring a file
// after a service has loaded it would be undone by that service's next save.
// current is the version of the running launcher.
func RecoverInterrupted(configDir, current string) error {
	var errs []error
	if err := recoverReplaced(configDir, current); err != nil {
		errs = append(errs, fmt.Errorf("selfupdate: undo an interrupted rollback: %w", err))
	}
	if err := recoverSnapshot(configDir, current); err != nil {
		errs = append(errs, fmt.Errorf("selfupdate: settle a staged snapshot: %w", err))
	}
	sweepPreviousTemps(configDir)
	return errors.Join(errs...)
}
