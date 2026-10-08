package savebackup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"typhon/internal/hashdir"
	"typhon/internal/install"
	"typhon/internal/storage"
	"typhon/internal/uierr"
)

const (
	journalFile    = "restore.json"
	stagingSuffix  = ".typhon-restore"
	previousSuffix = ".typhon-previous"
)

// journal records a restore between the moment the verified copy sits next to
// the saves folder and the moment the swap is finished. Only Dest is stored:
// the staging and previous paths are derived from it, so a damaged journal
// cannot point the recovery at some other directory.
type journal struct {
	Dest        string    `json:"dest"`
	SnapshotID  string    `json:"snapshotId"`
	HadPrevious bool      `json:"hadPrevious"`
	Copying     bool      `json:"copying,omitempty"`
	StartedAt   time.Time `json:"startedAt"`
}

func (s *Service) writeJournal(gameID string, j journal) error {
	data, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	return storage.WriteAtomic(s.journalPath(gameID), data)
}

// checkIntegrity re-hashes the snapshot files against what snapshot.json
// recorded. The snapshot is about to replace live saves, so a copy that rotted
// on disk must be refused before anything is touched, not restored as if it
// were good. The manifest is reused to verify the staging copy, so the
// snapshot is read once.
func (s *Service) checkIntegrity(ctx context.Context, snap Snapshot) (hashdir.Manifest, error) {
	manifest, err := hashdir.Build(ctx, snap.Path, nil)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return hashdir.Manifest{}, ctxErr
		}
		return hashdir.Manifest{}, fmt.Errorf("%w: %w", errSnapshotBroken, err)
	}
	switch {
	case len(manifest.Entries) != snap.Files:
		return hashdir.Manifest{}, fmt.Errorf("%w: файлов %d, записано %d", errSnapshotBroken, len(manifest.Entries), snap.Files)
	case manifest.TotalSize != snap.SizeBytes:
		return hashdir.Manifest{}, fmt.Errorf("%w: размер %d, записано %d", errSnapshotBroken, manifest.TotalSize, snap.SizeBytes)
	case digestOf(manifest.Entries) != snap.Digest:
		return hashdir.Manifest{}, fmt.Errorf("%w: содержимое не совпало с записанным", errSnapshotBroken)
	}
	return manifest, nil
}

func (s *Service) journalPath(gameID string) string {
	return filepath.Join(s.gameDir(gameID), journalFile)
}

func (s *Service) Restore(ctx context.Context, gameID, snapshotID string) error {
	if !s.enabled {
		return errDisabled
	}
	ctx, end, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer end()
	if !validName(gameID) || !validSnapshotID(snapshotID) {
		return errInvalidID
	}
	unlock := s.lockGame(gameID)
	defer unlock()
	if err := s.recoverGame(ctx, gameID); err != nil {
		return err
	}
	if s.isRunning(gameID) {
		return errGameRunning
	}

	snap, ok := s.readSnapshot(gameID, snapshotID)
	if !ok {
		return errSnapshotNotFound
	}
	if snap.Broken {
		return fmt.Errorf("%w: %s", errSnapshotBroken, snap.Problem)
	}
	manifest, err := s.checkIntegrity(ctx, snap)
	if err != nil {
		return err
	}
	dest, exists, err := s.restoreTarget(ctx, gameID)
	if err != nil {
		return err
	}
	dest = filepath.Clean(dest)
	if !filepath.IsAbs(dest) {
		return uierr.Wrap(codeSavesUnavailable, fmt.Errorf("путь сохранений %q не абсолютный", dest))
	}
	staging, previous := dest+stagingSuffix, dest+previousSuffix
	for _, leftover := range []string{staging, previous} {
		if _, err := os.Lstat(leftover); err == nil {
			return fmt.Errorf("%w: %s", errLeftovers, leftover)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}

	// The safety copy comes first: if anything after it fails, the player still
	// has the saves as they were. It is protected from rotation because the
	// snapshot being restored may be the oldest one.
	var rotationErr error
	if exists {
		pre, created, err := s.capture(ctx, gameID, dest, KindPreRestore, captureOpts{dedup: dedupLatest, protect: snapshotID, rotate: true})
		if created {
			s.emit(createdEvent(pre, err))
			rotationErr = err
		} else if err != nil {
			return fmt.Errorf("страховочная копия перед восстановлением: %w", err)
		}
	} else if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}

	if err := install.CheckFreeSpace(filepath.Dir(dest), snap.SizeBytes); err != nil {
		return uierr.Wrap(codeNoFreeSpace, err)
	}

	// The journal exists before the first byte reaches staging: a process that
	// dies mid-copy leaves a staging folder that recovery knows to discard,
	// instead of one that blocks every later restore as an unexplained leftover.
	j := journal{Dest: dest, SnapshotID: snapshotID, HadPrevious: exists, Copying: true, StartedAt: s.now().UTC()}
	if err := s.writeJournal(gameID, j); err != nil {
		return err
	}
	if err := install.CopyDir(ctx, snap.Path, staging, nil); err != nil {
		return errors.Join(fmt.Errorf("копирование сохранений из копии %s: %w", snapshotID, err), s.resolve(gameID, j))
	}
	if err := verifyFiles(ctx, staging, manifest); err != nil {
		return errors.Join(err, s.resolve(gameID, j))
	}
	j.Copying = false
	if err := s.writeJournal(gameID, j); err != nil {
		return errors.Join(err, s.resolve(gameID, j))
	}

	if err := ctx.Err(); err != nil {
		return errors.Join(err, s.resolve(gameID, j))
	}
	if s.isRunning(gameID) {
		return errors.Join(errGameRunning, s.resolve(gameID, j))
	}
	if exists {
		if err := os.Rename(dest, previous); err != nil {
			return errors.Join(fmt.Errorf("перенос текущих сохранений: %w", err), s.resolve(gameID, j))
		}
	}
	if err := os.Rename(staging, dest); err != nil {
		return errors.Join(fmt.Errorf("установка восстановленных сохранений: %w", err), s.resolve(gameID, j))
	}
	cleanupErr := s.resolve(gameID, j)
	s.emit(Event{GameID: gameID, Kind: snap.Kind, Snapshot: &snap, Status: StatusRestored})
	if cleanupErr != nil {
		return uierr.Wrap(codeCleanupFailed, fmt.Errorf("сохранения восстановлены, но очистка не завершена: %w", cleanupErr))
	}
	return rotationErr
}

// restoreTarget prefers the folder the player chose: LocateSaves would fall
// back to detection when that folder is gone and could point at a different
// one, while a restore into a deleted folder is a legitimate way to get saves
// back.
func (s *Service) restoreTarget(ctx context.Context, gameID string) (string, bool, error) {
	game, err := s.games.Find(gameID)
	if err != nil {
		return "", false, err
	}
	if game.SavesDir != "" {
		info, err := os.Stat(game.SavesDir)
		switch {
		case err == nil && info.IsDir():
			return game.SavesDir, true, nil
		case err == nil:
			return "", false, fmt.Errorf("%w: %s", errSavesNotDir, game.SavesDir)
		case errors.Is(err, fs.ErrNotExist):
			return game.SavesDir, false, nil
		}
		return "", false, uierr.Wrap(codeSavesUnavailable, fmt.Errorf("папка сохранений %s: %w", game.SavesDir, err))
	}
	dest, err := s.locate(ctx, gameID)
	if err != nil {
		return "", false, err
	}
	return dest, true, nil
}

// recoverGame finishes or undoes what an interrupted process left behind for
// one game. It runs at startup and before every operation on the game, under
// the game lock, so a failure to recover is reported by the operation that
// would otherwise build on an unknown state.
func (s *Service) recoverGame(ctx context.Context, gameID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.recoverJournal(gameID); err != nil {
		return err
	}
	if err := s.sweepPartials(gameID); err != nil {
		return uierr.Wrap(codeRecoveryFailed, err)
	}
	return s.migrateLegacy(ctx, gameID)
}

func (s *Service) recoverJournal(gameID string) error {
	data, err := readLimited(s.journalPath(gameID), maxMetadataSize)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return uierr.Wrap(codeRecoveryFailed, fmt.Errorf("журнал восстановления: %w", err))
	}
	var j journal
	if err := json.Unmarshal(data, &j); err != nil {
		return uierr.Wrap(codeRecoveryFailed, fmt.Errorf("журнал восстановления: %w", err))
	}
	if err := s.resolve(gameID, j); err != nil {
		return uierr.Wrap(codeRecoveryFailed, err)
	}
	return nil
}

func present(path string) (bool, error) {
	_, err := os.Lstat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// resolve brings the disk to a consistent state from whatever the swap left
// and then removes the journal. The three states a crash can leave between the
// steps are: current moved aside but the new copy not yet in place (undo),
// swap done but the old copy not yet deleted (finish), and nothing moved yet
// (drop the staging copy). Any other combination is refused: guessing there
// could delete the only copy of the saves.
func (s *Service) resolve(gameID string, j journal) error {
	if !filepath.IsAbs(j.Dest) || filepath.Clean(j.Dest) != j.Dest {
		return fmt.Errorf("путь %q в журнале восстановления не является нормализованным абсолютным", j.Dest)
	}
	staging, previous := j.Dest+stagingSuffix, j.Dest+previousSuffix
	dest, err := present(j.Dest)
	if err != nil {
		return err
	}
	stage, err := present(staging)
	if err != nil {
		return err
	}
	prev, err := present(previous)
	if err != nil {
		return err
	}

	var step error
	switch {
	case j.Copying && prev:
		return fmt.Errorf("копирование в %s не завершено, но каталог предыдущих сохранений уже есть", staging)
	case j.Copying:
		step = os.RemoveAll(staging)
	case prev && !dest:
		step = os.Rename(previous, j.Dest)
		if step == nil && stage {
			step = os.RemoveAll(staging)
		}
	case prev && dest && !stage:
		step = os.RemoveAll(previous)
	case !prev && dest && stage:
		step = os.RemoveAll(staging)
	case !prev && dest && !stage:
	case !prev && !dest && stage && !j.HadPrevious:
		step = os.Rename(staging, j.Dest)
	case !prev && !dest && !stage && !j.HadPrevious:
	default:
		return fmt.Errorf("состояние папок %s не позволяет безопасно продолжить (сохранения: %t, копия: %t, предыдущие: %t)", j.Dest, dest, stage, prev)
	}
	if step != nil {
		return step
	}
	if err := os.Remove(s.journalPath(gameID)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func (s *Service) sweepPartials(gameID string) error {
	entries, err := os.ReadDir(s.gameDir(gameID))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var errs []error
	for _, entry := range entries {
		if entry.IsDir() && strings.HasSuffix(entry.Name(), partialSuffix) {
			if err := os.RemoveAll(filepath.Join(s.gameDir(gameID), entry.Name())); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// recoveryTargets lists every game that has state on disk. Only an unreadable
// root is fatal, because then nothing can be said about any game; the listing
// is cheap and stays synchronous so that failure still stops the startup.
func (s *Service) recoveryTargets() ([]string, error) {
	ids := map[string]bool{}
	for _, dir := range []string{s.root, s.legacyRoot} {
		info, err := os.Stat(dir)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("чтение %s: %w", dir, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("%s не является папкой", dir)
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, fmt.Errorf("чтение %s: %w", dir, err)
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := strings.TrimSuffix(strings.TrimSuffix(entry.Name(), partialSuffix), legacyReplacedSuffix)
			if validName(name) {
				ids[name] = true
			}
		}
	}
	out := make([]string, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	return out, nil
}

// recoverGames runs in the background after startup: hashing a legacy
// snapshot must not delay the launcher. Nothing depends on it having finished,
// because every operation on a game runs recoverGame itself under the same
// game lock. A game that cannot be recovered is logged here and reports the
// same error from each later operation on it.
func (s *Service) recoverGames(ctx context.Context, ids []string) {
	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		unlock := s.lockGame(id)
		err := s.recoverGame(ctx, id)
		unlock()
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			slog.Error("save backups recovery failed", "game", id, "error", err)
		}
	}
}
