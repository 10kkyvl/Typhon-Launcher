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
	if s.holdsCopies(job) {
		slog.Info("copy of a move its game's card is done with kept", "job", job.ID, "game", job.GameID)
		return
	}
	if err := s.removeJob(job.ID); err != nil {
		// removeJob logged it and raised move:degraded; the entry is checked
		// again at the next start.
		return
	}
}

// holdsCopies says whether a finished job must stay in the journal: it still
// holds a folder (heldCopies), or the disk could not be looked at.
func (s *Service) holdsCopies(job Job) bool {
	var games []library.Game
	if s.lib != nil {
		games = s.lib.GetGames()
	}
	held, err := heldCopies(job, games)
	if err != nil {
		slog.Warn("check the folders a finished move wrote", "job", job.ID, "error", err)
		return true
	}
	return len(held) > 0
}

// heldCopies answers, for a finished job, which folders it may have written
// are still on disk with no card naming them: the staging folder, and for a
// game the target, and the old folder once the repoint had landed (OrphanedAt
// is the cleanup, or the card names the target), which is then the leftover
// copy the cleanup was to delete. Before that the card may still name the old
// folder, so it is the player's own. A move that renamed its folder has no old
// folder: whatever is at that place now came later. A folder counts as named only by a card
// whose own folder it is: a card around the copy or inside it does not
// account for it. The journal keeper, OrphanedCopies and Stagings all ask
// this one question.
func heldCopies(job Job, games []library.Game) ([]OrphanedCopy, error) {
	candidates := []OrphanedCopy{{Kind: CopyStaging, Path: job.Staging}}
	if gameItem(job.GameID) {
		candidates = append(candidates, OrphanedCopy{Kind: CopyTarget, Path: job.Target})
		if !job.Renamed && (job.OrphanedAt == StageCleanup || !job.Orphaned && cardNames(job.GameID, job.Target, games)) {
			candidates = append(candidates, OrphanedCopy{Kind: CopySource, Path: job.Source})
		}
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
		if named(c.Path, games) {
			continue
		}
		c.JobID, c.GameID, c.Title = job.ID, job.GameID, job.Title
		c.MovedOn = cardAlive(job.GameID, games)
		out = append(out, c)
	}
	return out, nil
}

func gameItem(id string) bool {
	switch id {
	case "", itemDownloads, itemScreenshots, itemSettings:
		return false
	}
	return true
}

func cardAlive(id string, games []library.Game) bool {
	for _, g := range games {
		if g.ID == id {
			return !g.Uninstalled
		}
	}
	return false
}

func cardNames(id, path string, games []library.Game) bool {
	for _, g := range games {
		if g.ID == id {
			return !g.Uninstalled && platform.SamePath(g.InstallDir, path)
		}
	}
	return false
}

// named says whether path is the folder of a card that owns files on disk.
func named(path string, games []library.Game) bool {
	for _, g := range games {
		if g.Uninstalled {
			continue
		}
		for _, root := range g.Footprint() {
			if platform.SamePath(root, path) {
				return true
			}
		}
	}
	return false
}

// OrphanedCopies lists, for the disk usage screen, the copies of moves that
// are over: every folder a move its game's card is done with still holds, and
// the target and old folder a move that ended for good holds (its staging
// folder is a leftover Stagings lists). A move still to be resumed is not
// over, and its folders are not listed here.
//
//wails:ignore
func (s *Service) OrphanedCopies() ([]OrphanedCopy, error) {
	if s.lib == nil {
		return nil, errors.New("библиотека недоступна, копии переноса не сверить с играми")
	}
	games := s.lib.GetGames()
	var out []OrphanedCopy
	for _, job := range s.List() {
		if !terminal(job.Stage) || job.FailedAt != "" {
			continue
		}
		copies, err := heldCopies(job, games)
		if err != nil {
			return nil, err
		}
		for _, c := range copies {
			if job.Orphaned || c.Kind != CopyStaging {
				out = append(out, c)
			}
		}
	}
	return out, nil
}
