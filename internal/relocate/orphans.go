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
	// folder that is on disk: the copy is one the card moved on from, not a
	// copy of a game that left the library.
	MovedOn bool `json:"movedOn"`
	// OnlyCopy says the game's card names a folder that is gone, or the game
	// has no card with files, and this copy is the newest complete one the
	// move knows: deleting it may delete the game. False also where that
	// cannot be known from the journal (see onlyCopy).
	OnlyCopy bool `json:"onlyCopy"`
	// HoldsGame says a library card's files lie at, in or around this copy:
	// it must never be offered for deletion.
	HoldsGame bool `json:"holdsGame"`
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
	onDisk := map[string]bool{}
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
		onDisk[c.Kind] = true
		if named(c.Path, games) {
			continue
		}
		c.JobID, c.GameID, c.Title = job.ID, job.GameID, job.Title
		out = append(out, c)
	}
	if len(out) == 0 {
		return nil, nil
	}
	card, err := cardFolder(job.GameID, games)
	if err != nil {
		return nil, err
	}
	var withFiles []library.Game
	for _, g := range games {
		if !g.Uninstalled {
			withFiles = append(withFiles, g)
		}
	}
	for i := range out {
		c := &out[i]
		c.MovedOn = card.onDisk && !platform.SamePath(card.dir, c.Path)
		c.OnlyCopy = !card.onDisk && newestComplete(job, c.Kind, onDisk)
		c.HoldsGame = holdsGame(c.Path, withFiles)
	}
	return out, nil
}

// gameFolder is what a job's game's card names: dir, and whether it is a
// card with files whose folder is on disk.
type gameFolder struct {
	dir    string
	onDisk bool
}

func cardFolder(id string, games []library.Game) (gameFolder, error) {
	for _, g := range games {
		if g.ID != id || g.Uninstalled || g.InstallDir == "" {
			continue
		}
		_, err := os.Lstat(g.InstallDir)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return gameFolder{}, err
		}
		return gameFolder{dir: g.InstallDir, onDisk: err == nil}, nil
	}
	return gameFolder{}, nil
}

// newestComplete says whether a copy of kind is, from the journal alone, the
// newest complete copy of the game the job knows; onDisk says which kinds are
// on disk. The target is complete once the rename that made it landed: a
// renamed move's always, a copied move's from the repoint on, or at the commit
// once the staging folder is gone, which only an ended move's OrphanedAt still
// says (a move that ended for good keeps no stage, so its target is never
// called the only copy). The staging folder is complete and newest at the
// commit while no target is there to compete with it. The old folder may be a
// cleanup's half deleted remains: never.
func newestComplete(job Job, kind string, onDisk map[string]bool) bool {
	switch kind {
	case CopyTarget:
		if job.Renamed {
			return true
		}
		if !job.Orphaned {
			return false
		}
		switch job.OrphanedAt {
		case StageRepoint, StageCleanup:
			return true
		case StageCommit:
			// The commit renames the staging folder into the target: a target
			// with no staging folder left is that rename landed.
			return !onDisk[CopyStaging]
		}
		return false
	case CopyStaging:
		return job.Orphaned && job.OrphanedAt == StageCommit && !onDisk[CopyTarget]
	}
	return false
}

func gameItem(id string) bool {
	switch id {
	case "", itemDownloads, itemScreenshots, itemSettings:
		return false
	}
	return true
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
