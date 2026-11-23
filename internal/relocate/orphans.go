package relocate

import (
	"errors"
	"io/fs"
	"log/slog"
	"os"

	"typhon/internal/library"
	"typhon/internal/platform"
)

// OrphanedCopy is a copy a move made of a game that left the library before
// the move was finished. Nothing deletes it on its own: the player did not
// ask for that, and with the source gone it is the only copy of the game.
// Kind says which folder of the move it is: CopyStaging (the copy before the
// commit), CopyTarget (the copy after it) or CopySource (the old folder, once
// the card had been repointed to the target).
type OrphanedCopy struct {
	JobID  string `json:"jobId"`
	GameID string `json:"gameId"`
	Title  string `json:"title"`
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	// MovedOn says the game's card is still in the library and names another
	// folder: the copy is one the card moved on from, not a copy of a game
	// that left the library.
	MovedOn bool `json:"movedOn"`
}

const (
	CopyStaging = "staging"
	CopyTarget  = "target"
	CopySource  = "source"
)

// markOrphaned ends a move that is over for its game's card, see endForCard.
func (s *Service) markOrphaned(jobID string, repointed bool, cause error) {
	if _, err := s.transition(jobID, StageFailed, func(j *Job) { endForCard(j, repointed, cause) }); err != nil {
		slog.Error("end a move its game's card is done with", "job", jobID, "error", err)
	}
}

// endForCard ends a move before it was finished because its game's card is
// done with it: the game was removed or uninstalled (ErrGameRemoved), or the
// card names a folder the move no longer leads to (ErrMovedOn). No copy is
// touched and the card is not brought back; the entry stays in the journal so
// the copies stay listed. repointed says the repoint had landed and the
// cleanup was all that was left: the old folder is then one of the copies.
func endForCard(j *Job, repointed bool, cause error) {
	j.Error = cause.Error()
	j.OrphanedAt = j.Stage
	if j.Stage == StageFailed {
		j.OrphanedAt = j.FailedAt
	}
	if repointed {
		j.OrphanedAt = StageCleanup
	}
	j.FailedAt = ""
	j.Orphaned = true
	j.Stage = StageFailed
}

// cardMovedOn says whether a card at installDir has moved on from a job: a
// recovery repoints it only while it names the job's source (the repoint is
// owed) or its target (done), and past the repoint, where the old folder is
// deleted, only while it names the target.
func cardMovedOn(job Job, installDir string) bool {
	if platform.SamePath(installDir, job.Target) {
		return false
	}
	return stageOf(job) == StageCleanup || !platform.SamePath(installDir, job.Source)
}

// stageOf is the stage a job is at or failed at.
func stageOf(job Job) Stage {
	if job.Stage == StageFailed {
		return job.FailedAt
	}
	return job.Stage
}

// recoverOrphan drops the entry of an orphaned move once nothing of it is left
// to list: its copy was deleted, or a library card claims it again.
func (s *Service) recoverOrphan(job Job) {
	if s.lib == nil {
		return
	}
	copies, err := orphanCopies(job, s.lib.GetGames())
	if err != nil {
		slog.Warn("check the copy of a move whose game left the library", "job", job.ID, "error", err)
		return
	}
	if len(copies) > 0 {
		slog.Info("copy of a move whose game left the library kept", "job", job.ID, "game", job.GameID, "copies", len(copies))
		return
	}
	if err := s.removeJob(job.ID); err != nil {
		// removeJob logged it and raised move:degraded; the entry is checked
		// again at the next start.
		return
	}
}

// orphanCopies lists the folders of an orphaned move that are on disk and
// claimed by no card: the staging folder before the commit, the target after
// it, and once the repoint had landed (OrphanedAt is the cleanup) also the old
// folder, which is then the launcher's leftover copy the cleanup was to
// delete. Before that the card may still have named the old folder, so it is
// the player's own, kept or deleted by the player's removal.
func orphanCopies(job Job, games []library.Game) ([]OrphanedCopy, error) {
	candidates := []OrphanedCopy{{Kind: CopyStaging, Path: job.Staging}, {Kind: CopyTarget, Path: job.Target}}
	if job.OrphanedAt == StageCleanup {
		candidates = append(candidates, OrphanedCopy{Kind: CopySource, Path: job.Source})
	}
	var out []OrphanedCopy
	for _, c := range candidates {
		if c.Path == "" {
			continue
		}
		if _, err := os.Lstat(c.Path); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, err
		}
		if claimed(c.Path, games) {
			continue
		}
		c.JobID, c.GameID, c.Title = job.ID, job.GameID, job.Title
		c.MovedOn = cardAlive(job.GameID, games)
		out = append(out, c)
	}
	return out, nil
}

func cardAlive(id string, games []library.Game) bool {
	for _, g := range games {
		if g.ID == id {
			return !g.Uninstalled
		}
	}
	return false
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

// OrphanedCopies lists the copies of moves that are over for their game's
// card, for the disk usage screen.
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
		copies, err := orphanCopies(job, games)
		if err != nil {
			return nil, err
		}
		out = append(out, copies...)
	}
	return out, nil
}
