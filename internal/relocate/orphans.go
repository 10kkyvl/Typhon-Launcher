package relocate

import (
	"errors"
	"io/fs"
	"log/slog"
	"os"

	"typhon/internal/library"
)

// OrphanedCopy is a copy a move made of a game that left the library before
// the move was repointed. Nothing deletes it on its own: the player did not
// ask for that, and with the source gone it is the only copy of the game.
type OrphanedCopy struct {
	JobID  string `json:"jobId"`
	GameID string `json:"gameId"`
	Title  string `json:"title"`
	Path   string `json:"path"`
}

// markOrphaned ends a move whose game was removed or uninstalled before the
// repoint. Neither copy is touched and the card is not brought back; the
// entry stays in the journal so the copy stays listed.
func (s *Service) markOrphaned(jobID string) {
	if _, err := s.transition(jobID, StageFailed, func(j *Job) {
		j.Error = ErrGameRemoved.Error()
		j.OrphanedAt = j.Stage
		if j.Stage == StageFailed {
			j.OrphanedAt = j.FailedAt
		}
		j.FailedAt = ""
		j.Orphaned = true
	}); err != nil {
		slog.Error("mark move of a removed game", "job", jobID, "error", err)
	}
}

// recoverOrphan drops the entry of an orphaned move once nothing of it is left
// to list: its copy was deleted, or a library card claims it again.
func (s *Service) recoverOrphan(job Job) {
	if s.lib == nil {
		return
	}
	path, err := orphanCopy(job, s.lib.GetGames())
	if err != nil {
		slog.Warn("check the copy of a move whose game left the library", "job", job.ID, "error", err)
		return
	}
	if path != "" {
		slog.Info("copy of a move whose game left the library kept", "job", job.ID, "game", job.GameID, "path", path)
		return
	}
	if err := s.removeJob(job.ID); err != nil {
		// removeJob logged it and raised move:degraded; the entry is checked
		// again at the next start.
		return
	}
}

// orphanCopy is the folder of an orphaned move that is on disk and claimed by
// no card: the staging folder before the commit, the target after it, and at
// the cleanup also the old folder. Only the cleanup proves the repoint landed:
// the old folder is then the launcher's leftover copy the cleanup was to
// delete. Up to the repoint stage the card may still have named the old
// folder, so it is the player's own, kept or deleted by the player's removal.
func orphanCopy(job Job, games []library.Game) (string, error) {
	paths := []string{job.Staging, job.Target}
	if job.OrphanedAt == StageCleanup {
		paths = append(paths, job.Source)
	}
	for _, path := range paths {
		if path == "" {
			continue
		}
		if _, err := os.Lstat(path); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return "", err
		}
		if !claimed(path, games) {
			return path, nil
		}
	}
	return "", nil
}

func claimed(path string, games []library.Game) bool {
	for _, g := range games {
		if g.Uninstalled {
			continue
		}
		for _, root := range g.Footprint() {
			if pathsConflict(path, "", root, "") {
				return true
			}
		}
	}
	return false
}

// OrphanedCopies lists the copies of moves whose game left the library, for
// the disk usage screen.
//
//wails:ignore
func (s *Service) OrphanedCopies() ([]OrphanedCopy, error) {
	if s.lib == nil {
		return nil, errors.New("библиотека недоступна, копии переноса не сверить с играми")
	}
	games := s.lib.GetGames()
	var out []OrphanedCopy
	for _, job := range s.List() {
		if !job.Orphaned {
			continue
		}
		path, err := orphanCopy(job, games)
		if err != nil {
			return nil, err
		}
		if path != "" {
			out = append(out, OrphanedCopy{JobID: job.ID, GameID: job.GameID, Title: job.Title, Path: path})
		}
	}
	return out, nil
}
