package updates

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"typhon/internal/savebackup"
)

var errNoSaveBackups = errors.New("сервис резервных копий сохранений недоступен")

// saveBackups is the only thing that copies saves: the update asks for a
// snapshot and records where it went, it never copies the folder itself.
type saveBackups interface {
	SnapshotPath(ctx context.Context, gameID, sourcePath string, kind savebackup.Kind) (savebackup.Snapshot, error)
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
// does nothing. The snapshot lives outside the installation because every
// update strategy either replaces that directory or writes over its files.
func (s *Service) backupSaves(ctx context.Context, plan UpdatePlan) (string, error) {
	if plan.SavesPath == "" || !s.config().UpdateSaveBackup {
		return "", nil
	}
	if s.saves == nil {
		return "", errNoSaveBackups
	}
	snap, err := s.saves.SnapshotPath(ctx, plan.GameID, plan.SavesPath, savebackup.KindUpdate)
	if err != nil && snap.ID == "" {
		return "", fmt.Errorf("снимок сохранений %s: %w", plan.SavesPath, err)
	}
	if err != nil {
		// The snapshot exists and is verified; what failed is trimming older
		// ones, which the saves:backups event already reports to the player.
		slog.Warn("saves snapshot rotation failed", "game", plan.GameID, "error", err)
	}
	if snap.Path == "" {
		return "", fmt.Errorf("снимок сохранений %s: сервис не вернул путь копии", plan.SavesPath)
	}
	return snap.Path, nil
}
