package updates

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"typhon/internal/install"
)

const (
	savesBackupDirName = "saves"
	savesStagingSuffix = ".partial"
)

var errNoSavesDir = errors.New("каталог снимков сохранений недоступен")

// savesBackupDir keeps the snapshot outside the installation: every update
// strategy either replaces that directory or writes over its files, so a copy
// kept inside it is gone exactly when it is needed.
func (s *Service) savesBackupDir(gameID string) (string, error) {
	if s.store == nil || s.store.dir == "" || gameID == "" {
		return "", errNoSavesDir
	}
	return filepath.Join(s.store.dir, savesBackupDirName, gameID), nil
}

// locateSaves resolves the folder a snapshot would be taken from. It runs once
// at plan time, so the plan can promise a backup only when there is a single
// known path to copy and the update itself does not repeat the scan
// (invariant 35). A detection failure is returned rather than reported as
// "no saves": the switch asking for the snapshot is worth nothing if a broken
// saves path silently turns into a skipped backup.
func (s *Service) locateSaves(ctx context.Context, gameID string) (string, error) {
	if !s.config().UpdateSaveBackup {
		return "", nil
	}
	if s.library == nil {
		return "", errNoLibrary
	}
	result, err := s.library.LocateSaves(ctx, gameID)
	if err != nil {
		return "", err
	}
	return result.Path, nil
}

// backupSaves copies the saves resolved at plan time before the update makes
// its first write, and fails the update when it cannot: an update that
// proceeds after a failed backup is the same broken promise as a switch that
// does nothing.
func (s *Service) backupSaves(ctx context.Context, plan UpdatePlan) (string, error) {
	if plan.SavesPath == "" || !s.config().UpdateSaveBackup {
		return "", nil
	}
	target, err := s.savesBackupDir(plan.GameID)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	total, err := install.DirSize(ctx, plan.SavesPath)
	if err != nil {
		return "", err
	}
	if err := checkBackupFreeSpace(filepath.Dir(target), total); err != nil {
		return "", err
	}

	staging := target + savesStagingSuffix
	removeTree(staging)
	if err := install.CopyDirVerified(ctx, plan.SavesPath, staging, nil); err != nil {
		removeTree(staging)
		return "", fmt.Errorf("снимок сохранений %s: %w", plan.SavesPath, err)
	}

	// The previous snapshot is moved aside rather than deleted, so a crash
	// between the two renames leaves the older copy on disk instead of no
	// copy at all. Nothing here touches the installation, so this needs no
	// journal: the next update overwrites both paths from scratch.
	replaced := target + replacedSuffix
	removeTree(replaced)
	if _, err := os.Stat(target); err == nil {
		if err := os.Rename(target, replaced); err != nil {
			removeTree(staging)
			return "", err
		}
	}
	if err := os.Rename(staging, target); err != nil {
		removeTree(staging)
		return "", err
	}
	removeTree(replaced)
	return target, nil
}
