package relocate

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"

	"typhon/internal/hashdir"
	"typhon/internal/library"
	"typhon/internal/platform"
	"typhon/internal/settings"
)

// recoverAll resumes or undoes every job left in moves.json from a previous
// run. It runs once from ServiceStartup's goroutine, which sets the service's
// context first: MoveGame and MoveLibrary are open while it works, and a new
// move of a game ends that game's resumable job (supersedeLocked). recoverAll
// works from a copy of the journal taken at its start, so every job is read
// again under its reservation, and every change it makes to a job holds only
// while the job is still where that copy had it (reserveForRecovery).
func (s *Service) recoverAll(ctx context.Context) {
	s.mu.Lock()
	jobs := make([]Job, len(s.jobs))
	copy(jobs, s.jobs)
	s.mu.Unlock()

	for _, job := range jobs {
		if err := ctx.Err(); err != nil {
			return
		}
		s.recoverJob(ctx, job)
		s.logRecovery(job)
	}
}

// logRecovery writes one line per journal entry: the stage recovery found it
// in and what became of it. A job that ended failed is a warning with the
// reason; a job left as it was (game busy, shutdown) says so.
func (s *Service) logRecovery(found Job) {
	now, err := s.jobSnapshot(found.ID)
	switch {
	case err != nil && (found.Stage == StageDone || found.Stage == StageCancelled || found.Stage == StageFailed):
		slog.Info("move recovery finished", "component", "relocate", "operation", "recover",
			"job", found.ID, "game", found.GameID, "found_stage", found.Stage, "outcome", "surfaced")
	case err != nil:
		slog.Info("move recovery finished", "component", "relocate", "operation", "recover",
			"job", found.ID, "game", found.GameID, "found_stage", found.Stage, "outcome", "completed")
	case now.Stage == StageFailed:
		slog.Warn("move recovery finished", "component", "relocate", "operation", "recover",
			"job", found.ID, "game", found.GameID, "found_stage", found.Stage, "outcome", "failed", "reason", now.Error)
	case now.Stage == found.Stage:
		slog.Info("move recovery finished", "component", "relocate", "operation", "recover",
			"job", found.ID, "game", found.GameID, "found_stage", found.Stage, "outcome", "left_for_next_start")
	default:
		slog.Info("move recovery finished", "component", "relocate", "operation", "recover",
			"job", found.ID, "game", found.GameID, "found_stage", found.Stage, "outcome", "advanced", "stage", now.Stage)
	}
}

func (s *Service) recoverJob(ctx context.Context, job Job) {
	if job.Orphaned {
		s.recoverOrphan(job)
		return
	}
	release, ok := s.reserveForRecovery(job)
	if !ok {
		return
	}
	defer release()
	switch job.Stage {
	case StageDone, StageCancelled:
		s.surfaceStale(job)
	case StageFailed:
		s.recoverFailed(ctx, job)
	case StagePrepare:
		s.recoverPrepare(ctx, job)
	case StageCopy, StageVerify:
		s.recoverCopyOrVerify(job)
	case StageCommit:
		s.recoverCommit(ctx, job)
	case StageRepoint:
		s.runAttempt(ctx, job, StageRepoint, s.recoverRepoint)
	case StageCleanup:
		s.runAttempt(ctx, job, StageCleanup, s.recoverCleanup)
	default:
		slog.Error("unknown move stage on recovery", "job", job.ID, "stage", job.Stage)
		s.markRecoveryFailed(job.ID, fmt.Errorf("неизвестная стадия переноса: %s", job.Stage))
	}
}

// surfaceStale handles a job that was already terminal when the process
// crashed, before the UI could see the event: emit it once more and drop
// the leftover journal entry. A job that still holds a folder (holdsCopies)
// stays, without the event: the entry is what lists that folder for Disk
// usage and what lets a retry of the same move take its staging folder over,
// and it goes at the first start after the last such folder does.
func (s *Service) surfaceStale(job Job) {
	if s.holdsCopies(job) {
		return
	}
	emit(eventForStage(job.Stage), job.clone())
	if err := s.removeJob(job.ID); err != nil {
		// Logged and surfaced via move:degraded inside removeJob already;
		// recoverAll's goroutine has no synchronous caller to report to.
		return
	}
}

// reserveForRecovery takes the move reservation of the game a resumable job
// moves, with the job's folders held: the old folder as a live move holds the
// game's, the copy's folders against a card inside them. The game cannot
// start and no card can be added in them while recovery touches them. A card
// that came to lie in one of them since the crash fails the job at its stage
// (see failShared). A busy game or a card still being added leaves the job as
// it is for a later start: the journal has no stage that says "waiting", so
// until then it reads as the stage it was killed in. A game that is gone from
// the library or uninstalled, or whose card moved on from the job, ends the
// job at once without touching any data (endedForCard).
func (s *Service) reserveForRecovery(job Job) (func(), bool) {
	noop := func() {}
	if s.lib == nil || !recoveryTouchesGame(job) {
		return noop, true
	}
	release, err := s.lib.BeginRecovery(job.GameID, job.Source, job.Target, job.Staging)
	if err != nil {
		// A game that left the library cannot be reserved, and a card that
		// moved on may be what is in the way: the job ends without touching
		// any data, so no reservation is needed for that.
		if s.endedForCard(job) {
			return nil, false
		}
		if errors.Is(err, library.ErrFolderShared) {
			s.failShared(job, err)
			return nil, false
		}
		slog.Warn("recovery skipped: game is busy", "job", job.ID, "game", job.GameID, "error", err)
		return nil, false
	}
	// recoverAll works from a copy of the journal: a new move may have ended
	// the job, or moved the game, since. Under the reservation no move or
	// removal changes either, so both are read again here.
	current, err := s.jobSnapshot(job.ID)
	if err != nil || !sameStage(current, job) {
		release()
		return nil, false
	}
	if s.endedForCard(current) {
		release()
		return nil, false
	}
	return release, true
}

// sameStage says whether a job is still where recovery found it: not over for
// its card, at the same stage, failed at the same stage.
func sameStage(current, found Job) bool {
	return !current.Orphaned && current.Stage == found.Stage && current.FailedAt == found.FailedAt
}

// endedForCard says whether a job is not to be resumed for its game's card,
// and ends it when it is still where recovery found it: the game left the
// library or was uninstalled, or its card names a folder the job no longer
// leads to (cardMovedOn). Nothing is repointed or deleted; the copies stay
// listed through OrphanedCopies. A job that changed since recovery read it (a
// new move ended it) is left exactly as it is.
func (s *Service) endedForCard(job Job) bool {
	game, err := s.lib.Find(job.GameID)
	switch {
	case err != nil || game.Uninstalled:
		s.endUnchanged(job, err == nil && platform.SamePath(game.InstallDir, job.Target), ErrGameRemoved)
	case cardMovedOn(job, game.InstallDir):
		s.endUnchanged(job, stageOf(job) == StageCleanup, ErrMovedOn)
	default:
		return false
	}
	return true
}

// errJobChanged refuses a recovery's change to a job that is no longer where
// recovery found it.
var errJobChanged = errors.New("the move changed since recovery read it")

// unchangedGuard is a guardedTransition guard that holds while the job is
// where recovery found it.
func (s *Service) unchangedGuard(found Job) func() error {
	return func() error {
		current := s.findJobLocked(found.ID)
		if current == nil || !sameStage(*current, found) {
			return errJobChanged
		}
		return nil
	}
}

// endUnchanged ends a job for its card (endForCard) only while it is where
// recovery found it, in one hold of s.mu: a job that is over is never ended
// again.
func (s *Service) endUnchanged(found Job, repointed bool, cause error) {
	_, err := s.guardedTransition(found.ID, StageFailed, s.unchangedGuard(found), func(j *Job) { endForCard(j, repointed, cause) })
	if err != nil && !errors.Is(err, errJobChanged) {
		slog.Error("end a move its game's card is done with", "job", found.ID, "error", err)
	}
}

// failShared fails a job another card's files are in the way of, at the stage
// it was found in, so the player sees a failed move with its reason instead
// of a move that never ends. Being blocked is not the move's failure: it
// spends no recovery attempt, and every start resumes the job from that stage
// for as long as the card stays in the way. A job that changed since recovery
// read it is left as it is.
func (s *Service) failShared(job Job, cause error) {
	err := fmt.Errorf("%w: %w", ErrFolderShared, cause)
	stage := stageOf(job)
	if _, tErr := s.guardedTransition(job.ID, StageFailed, s.unchangedGuard(job), func(j *Job) {
		j.Error = err.Error()
		j.FailedAt = stage
	}); tErr != nil && !errors.Is(tErr, errJobChanged) {
		slog.Error("mark move failed on recovery", "job", job.ID, "error", tErr)
	}
}

func recoveryTouchesGame(job Job) bool {
	switch job.GameID {
	case "", itemDownloads, itemScreenshots, itemSettings:
		return false
	}
	return resumable(job.Stage) || job.Stage == StageFailed && resumable(job.FailedAt)
}

// resumable names the stages a move resumes from at the next start. A job
// fails at the first two only when another card was in the way of recovery.
func resumable(stage Stage) bool {
	switch stage {
	case StagePrepare, StageCommit, StageRepoint, StageCleanup:
		return true
	}
	return false
}

// recoverFailed resumes a job that failed after its bytes reached Target: a
// failed repoint is retried, and a failed cleanup retries the removal of the
// old copy, but only while Target is the copy the library points at. A job
// failed at its prepare or commit because another card was in the way is
// resumed from there. Every other failed job is just surfaced and dropped.
func (s *Service) recoverFailed(ctx context.Context, job Job) {
	switch job.FailedAt {
	case StagePrepare:
		s.recoverPrepare(ctx, job)
	case StageCommit:
		s.recoverCommit(ctx, job)
	case StageRepoint:
		s.runAttempt(ctx, job, StageRepoint, s.recoverRepoint)
	case StageCleanup:
		if !s.targetIsRegistered(job) {
			s.surfaceStale(job)
			return
		}
		s.runAttempt(ctx, job, StageCleanup, s.recoverCleanup)
	default:
		s.surfaceStale(job)
	}
}

// runAttempt runs one recovery attempt of a repoint or cleanup. The attempt
// is counted and persisted before the action, whatever stage the job starts
// from, so a process that dies mid-action still spends it. The third failed
// attempt ends the job for good. A shutdown that interrupts the action is not
// a failed attempt: the counter is given back.
func (s *Service) runAttempt(ctx context.Context, job Job, stage Stage, resume func(context.Context, Job)) {
	if job.RecoveryAttempts >= maxRecoveryAttempts {
		s.markRecoveryFailed(job.ID, exhausted(job.Error))
		return
	}
	previous := job.Stage
	if _, err := s.transition(job.ID, stage, func(j *Job) { j.RecoveryAttempts++ }); err != nil {
		slog.Error("resume move", "job", job.ID, "error", err)
		return
	}
	resume(ctx, s.mustSnapshot(job.ID))
	current, err := s.jobSnapshot(job.ID)
	if err != nil || current.Stage != stage {
		return
	}
	if ctx.Err() != nil {
		if _, tErr := s.transition(job.ID, previous, func(j *Job) { j.RecoveryAttempts-- }); tErr != nil {
			slog.Error("give back interrupted recovery attempt", "job", job.ID, "error", tErr)
		}
		return
	}
	if current.RecoveryAttempts >= maxRecoveryAttempts {
		s.markRecoveryFailed(job.ID, exhausted(job.Error))
		return
	}
	if fErr := s.failOrCancel(job.ID, fmt.Errorf("повторная попытка не удалась: %s", job.Error)); fErr != nil {
		slog.Warn("recovery attempt did not finish", "job", job.ID, "stage", stage, "error", fErr)
	}
}

func exhausted(last string) error {
	if last == "" {
		return ErrRetriesExhausted
	}
	return fmt.Errorf("%w: %s", ErrRetriesExhausted, last)
}

func (s *Service) targetIsRegistered(job Job) bool {
	switch job.GameID {
	case "", itemDownloads, itemScreenshots, itemSettings:
		return true
	}
	return s.registeredAtTarget(job)
}

func (s *Service) registeredAtTarget(job Job) bool {
	switch job.GameID {
	case "", itemDownloads, itemScreenshots, itemSettings:
		return false
	}
	game, err := s.lib.Find(job.GameID)
	return err == nil && platform.SamePath(game.InstallDir, job.Target)
}

// keepReappearedSource settles a renamed move whose old folder is on disk
// again at the repoint. The rename took that folder away, so what is there
// now is not the move's (the game was installed there again, or the player or
// another program made it; the launcher's own screenshot capture refuses a
// missing folder): it is never deleted. A card that already names the
// move's copy finished the move; one that names the old folder is done with
// the move, which ends with its copy listed; an item with no card fails for
// good, both folders kept.
func (s *Service) keepReappearedSource(job Job) {
	switch {
	case !gameItem(job.GameID):
		s.markRecoveryFailed(job.ID, fmt.Errorf("%w: %s", ErrSourceChanged, job.Source))
	case s.registeredAtTarget(job):
		if err := s.st.removeManifest(job.ID); err != nil {
			slog.Warn("remove move manifest on recovery", "job", job.ID, "error", err)
		}
		s.continueAfterItem(job)
	default:
		s.endUnchanged(job, false, ErrMovedOn)
	}
}

// present says whether something is at path: only a path proven missing is
// not, since the answer decides whether a folder may be deleted.
func present(path string) bool {
	_, err := os.Lstat(path)
	return !errors.Is(err, fs.ErrNotExist)
}

// repointSafe proves, before a repoint is retried, that retrying cannot lose
// data: Source must still match the manifest taken before the move, since the
// library may have kept running from it after the failure. When Source has
// changed it is never touched, and a Target that is still a pure copy of the
// old Source is discarded.
func (s *Service) repointSafe(ctx context.Context, job Job) bool {
	if job.GameID == itemSettings || job.Renamed || !exists(job.Source) {
		return true
	}
	manifest, err := s.st.loadManifest(job.ID)
	if err != nil {
		if job.GameID == itemDownloads && !exists(job.Target) {
			return true
		}
		s.markRecoveryFailed(job.ID, fmt.Errorf("%w: %w", ErrSourceUnverifiable, err))
		return false
	}
	result, err := checkManifest(ctx, job.Source, manifest)
	if err != nil {
		s.verifyInterrupted(ctx, job, err)
		return false
	}
	if hasIssues(result) {
		if sourceDamaged(result) {
			s.markRecoveryFailed(job.ID, fmt.Errorf("%w: %s", ErrSourceDamaged, describeVerify(result)))
			return false
		}
		if !s.discardStaleTarget(ctx, job, manifest) {
			return false
		}
		s.markRecoveryFailed(job.ID, fmt.Errorf("%w: %s", ErrSourceChanged, describeVerify(result)))
		return false
	}
	if job.GameID != itemDownloads {
		result, err := checkManifest(ctx, job.Target, manifest)
		if err != nil {
			s.verifyInterrupted(ctx, job, err)
			return false
		}
		if hasIssues(result) {
			s.markRecoveryFailed(job.ID, fmt.Errorf("%w: %s", ErrVerifyFailed, describeVerify(result)))
			return false
		}
	}
	return true
}

// discardStaleTarget removes a Target that is provably a pure copy of the old
// Source. It returns false only when a shutdown interrupted the check.
func (s *Service) discardStaleTarget(ctx context.Context, job Job, manifest hashdir.Manifest) bool {
	if !exists(job.Target) || s.registeredAtTarget(job) {
		return true
	}
	result, err := checkManifest(ctx, job.Target, manifest)
	if err != nil {
		return ctx.Err() == nil
	}
	if hasIssues(result) {
		return true
	}
	if err := removeTree(job.Target); err != nil {
		slog.Warn("remove stale move target", "job", job.ID, "path", job.Target, "error", err)
	}
	return true
}

// verifyInterrupted settles a verification that could not run to the end. A
// shutdown leaves the job exactly as it was; a failed walk keeps it retryable.
func (s *Service) verifyInterrupted(ctx context.Context, job Job, err error) {
	if ctx.Err() != nil {
		return
	}
	s.markRecoveryRetryable(job.ID, err)
}

func (s *Service) markRecoveryFailed(jobID string, err error) {
	s.failRecovery(jobID, err, false)
}

func (s *Service) markRecoveryRetryable(jobID string, err error) {
	s.failRecovery(jobID, err, true)
}

func (s *Service) failRecovery(jobID string, err error, retry bool) {
	if _, tErr := s.transition(jobID, StageFailed, func(j *Job) {
		j.Error = err.Error()
		j.FailedAt = ""
		if retry {
			j.FailedAt = retryableStage(j.Stage)
		}
	}); tErr != nil {
		slog.Error("mark move failed on recovery", "job", jobID, "error", tErr)
	}
}

func exists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func (s *Service) mustSnapshot(id string) Job {
	job, err := s.jobSnapshot(id)
	if err != nil {
		return Job{}
	}
	return job
}

// recoverPrepare handles a crash before the copy/rename for the current
// item started — with one exception: os.Rename is not itself checkpointed,
// so a crash between a successful rename and the Stage=Repoint persist
// would otherwise look identical to "nothing happened". Source missing
// while Target exists is the tell that the rename actually landed.
func (s *Service) recoverPrepare(ctx context.Context, job Job) {
	if job.GameID == "" {
		s.markRecoveryFailed(job.ID, errors.New("перенос прерван до начала копирования"))
		return
	}
	if !exists(job.Source) && exists(job.Target) {
		if _, err := s.transition(job.ID, StageRepoint, func(j *Job) {
			j.Renamed = true
			j.CopiedBytes = j.TotalBytes
		}); err != nil {
			return
		}
		s.runAttempt(ctx, s.mustSnapshot(job.ID), StageRepoint, s.recoverRepoint)
		return
	}
	s.markRecoveryFailed(job.ID, errors.New("перенос прерван на этапе подготовки"))
}

// recoverCopyOrVerify undoes an interrupted copy: Source was never touched
// by CopyDir/hashdir.Build, so only Staging needs cleaning up. That holds only
// while Source is there: with it gone the staging folder may hold the only
// bytes of the game, so nothing is deleted, the job ends for good and the
// folder stays listed (stagingVerdict: possibly the only copy).
func (s *Service) recoverCopyOrVerify(job Job) {
	if job.Staging != "" && !exists(job.Source) && present(job.Staging) {
		s.markRecoveryFailed(job.ID, fmt.Errorf("%w: %s is gone, %s kept", ErrAmbiguousRecovery, job.Source, job.Staging))
		return
	}
	if job.Staging != "" {
		release, err := s.holdStaging(job)
		switch {
		case errors.Is(err, library.ErrFolderBusy):
			slog.Warn("recovery skipped: staging folder is busy", "job", job.ID, "path", job.Staging, "error", err)
			return
		case err != nil:
			s.markRecoveryFailed(job.ID, fmt.Errorf("%w: %s: %w", ErrFolderShared, job.Staging, err))
			return
		}
		removeStaging(job.Staging)
		release()
	}
	if err := s.st.removeManifest(job.ID); err != nil {
		slog.Warn("remove move manifest on recovery", "job", job.ID, "error", err)
	}
	s.markRecoveryFailed(job.ID, fmt.Errorf("перенос прерван на этапе %s", job.Stage))
}

// holdStaging keeps new cards out of a killed copy's staging folder while it
// is deleted, and refuses one a card lies at or inside: such a card came
// after the crash, and its files are not the move's. Without the game's
// reservation (the game left the library, or runs) the cards are looked at
// instead; another game's hold on the folder is ErrFolderBusy.
func (s *Service) holdStaging(job Job) (func(), error) {
	noop := func() {}
	if s.lib == nil || !gameItem(job.GameID) {
		return noop, nil
	}
	release, err := s.lib.BeginRecovery(job.GameID, "", job.Staging)
	switch {
	case err == nil:
		return release, nil
	case errors.Is(err, library.ErrFolderShared), errors.Is(err, library.ErrFolderBusy):
		return nil, err
	}
	for _, g := range s.lib.GetGames() {
		if g.Uninstalled {
			continue
		}
		for _, root := range g.Footprint() {
			if platform.SamePath(root, job.Staging) || platform.Inside(job.Staging, root) {
				return nil, fmt.Errorf("%w: %s", library.ErrFolderShared, g.Title)
			}
		}
	}
	return noop, nil
}

func (s *Service) recoverCommit(ctx context.Context, job Job) {
	targetExists := exists(job.Target)
	stagingExists := job.Staging != "" && exists(job.Staging)
	switch {
	case targetExists && !stagingExists:
		if _, err := s.transition(job.ID, StageRepoint, func(j *Job) {
			j.Staging = ""
			j.CopiedBytes = j.TotalBytes
		}); err != nil {
			return
		}
		s.runAttempt(ctx, s.mustSnapshot(job.ID), StageRepoint, s.recoverRepoint)
	case stagingExists && !targetExists:
		if err := os.Rename(job.Staging, job.Target); err != nil {
			s.markRecoveryFailed(job.ID, fmt.Errorf("повторное перемещение %s -> %s: %w", job.Staging, job.Target, err))
			return
		}
		if _, err := s.transition(job.ID, StageRepoint, func(j *Job) {
			j.Staging = ""
			j.CopiedBytes = j.TotalBytes
		}); err != nil {
			return
		}
		s.runAttempt(ctx, s.mustSnapshot(job.ID), StageRepoint, s.recoverRepoint)
	default:
		// Both exist, or neither does: which one holds the good copy cannot
		// be decided automatically. Nothing is deleted; the user has to look.
		s.markRecoveryFailed(job.ID, fmt.Errorf("%w: %s и %s", ErrAmbiguousRecovery, job.Staging, job.Target))
	}
}

func (s *Service) recoverRepoint(ctx context.Context, job Job) {
	if job.Renamed && job.GameID != itemSettings && present(job.Source) {
		s.keepReappearedSource(job)
		return
	}
	if !s.repointSafe(ctx, job) {
		return
	}
	switch job.GameID {
	case itemSettings:
		if _, err := s.settings.Update(func(next *settings.Settings) error {
			next.LibraryPath = job.Target
			return nil
		}); err != nil {
			slog.Warn("retry library settings save on recovery", "job", job.ID, "error", err)
			return
		}
		s.continueAfterItem(job)
		return
	case itemDownloads:
		if s.dl != nil {
			if err := s.dl.Repoint(ctx, job.Source, job.Target); err != nil {
				slog.Warn("retry downloads repoint on recovery", "job", job.ID, "error", err)
				return
			}
		}
	case itemScreenshots:
		// nothing external to repoint
	default:
		if job.GameID == "" {
			slog.Error("move job at repoint stage without an item", "job", job.ID)
			s.markRecoveryFailed(job.ID, errors.New("восстановление переноса: неизвестный элемент"))
			return
		}
		if _, err := s.lib.Relocate(job.GameID, job.Target); err != nil {
			slog.Warn("retry game relocate on recovery", "job", job.ID, "game", job.GameID, "error", err)
			return
		}
	}
	if err := s.cleanupItem(job.ID, job.Source); err != nil {
		return
	}
	s.continueAfterItem(job)
}

// recoverCleanup redoes the one check the crash left unproven: that Target
// still matches the manifest built before the commit rename. Only a clean
// result allows Source to be deleted (invariant 10).
// recoverCleanup resumes StageCleanup. It first rules out the case where
// there is nothing left to do at all: a prior run already finished
// cleanupItem's RemoveAll(Source) — which only ever runs after a clean
// verify (or after a rename, which needs no verify) — and crashed before
// clearing the manifest/journal entry behind it. Source already being gone
// is proof of that: reading the (possibly already-deleted) manifest and
// re-verifying at that point would either fail to open a manifest that was
// rightfully removed, or hash a Target that was never in doubt, and either
// way would wrongly report an already-successful move as failed (KRIT-2).
// Only a Source that still exists means the crash landed before RemoveAll,
// and a full re-verify against Target is actually required before it runs.
// Source is checked against the manifest too: one that holds a file the copy
// lacks (added or changed since) is kept and the job fails for good by
// ErrSourceChanged, the folder listed through OrphanedCopies; files it lost
// are the part a failed delete already took. A file it cannot read (held
// locked) deletes nothing and fails the attempt as a failed delete does, for
// the next start to try again.
func (s *Service) recoverCleanup(ctx context.Context, job Job) {
	if job.Renamed || !exists(job.Source) {
		if err := s.st.removeManifest(job.ID); err != nil {
			slog.Warn("remove move manifest on recovery", "job", job.ID, "error", err)
		}
		s.continueAfterItem(job)
		return
	}
	manifest, err := s.st.loadManifest(job.ID)
	if err != nil {
		s.markRecoveryFailed(job.ID, fmt.Errorf("не удалось прочитать манифест для проверки: %w", err))
		return
	}
	result, err := checkManifest(ctx, job.Target, manifest)
	if err != nil {
		s.verifyInterrupted(ctx, job, err)
		return
	}
	if hasIssues(result) {
		s.markRecoveryFailed(job.ID, fmt.Errorf("%w: %s", ErrVerifyFailed, describeVerify(result)))
		return
	}
	source, err := checkManifest(ctx, job.Source, manifest)
	if err != nil {
		s.verifyInterrupted(ctx, job, err)
		return
	}
	if gained := withoutMissing(source); hasIssues(gained) {
		s.markRecoveryFailed(job.ID, fmt.Errorf("%w: %s", ErrSourceChanged, describeVerify(gained)))
		return
	}
	if locked := unreadable(source); hasIssues(locked) {
		s.markRecoveryRetryable(job.ID, fmt.Errorf("remove old %s: cannot read %s", job.Source, describeVerify(locked)))
		return
	}
	if err := removeTree(job.Source); err != nil {
		s.markRecoveryRetryable(job.ID, fmt.Errorf("remove old %s: %w", job.Source, err))
		return
	}
	if err := s.st.removeManifest(job.ID); err != nil {
		slog.Warn("remove move manifest on recovery", "job", job.ID, "error", err)
	}
	s.continueAfterItem(job)
}

var verifyTree = hashdir.Verify

func checkManifest(ctx context.Context, root string, manifest hashdir.Manifest) (hashdir.Result, error) {
	result, err := verifyTree(ctx, root, manifest, nil)
	if err != nil {
		return hashdir.Result{}, fmt.Errorf("повторная проверка не удалась: %w", err)
	}
	return withoutMarker(result), nil
}

func hasIssues(r hashdir.Result) bool { return len(r.Issues) > 0 || len(r.Extra) > 0 }

// sourceDamaged tells a folder the user kept playing from (changed contents,
// extra files) from one that lost or cannot read files it had: only the first
// kind proves the copy at Target is the stale one.
func sourceDamaged(r hashdir.Result) bool {
	for _, issue := range r.Issues {
		if issue.Kind == hashdir.IssueMissing || issue.Kind == hashdir.IssueUnreadable {
			return true
		}
	}
	return false
}

// withoutMissing keeps what an old folder holds that the verified copy does
// not: files added or changed since the manifest. A file it lost is what a
// cleanup that failed half way leaves; a file it cannot read (held locked) is
// not known to differ, and unreadable answers for it.
func withoutMissing(r hashdir.Result) hashdir.Result {
	issues := r.Issues[:0:0]
	for _, issue := range r.Issues {
		if issue.Kind != hashdir.IssueMissing && issue.Kind != hashdir.IssueUnreadable {
			issues = append(issues, issue)
		}
	}
	r.Issues = issues
	return r
}

// unreadable keeps the files of a verification that could not be read.
func unreadable(r hashdir.Result) hashdir.Result {
	var issues []hashdir.Issue
	for _, issue := range r.Issues {
		if issue.Kind == hashdir.IssueUnreadable {
			issues = append(issues, issue)
		}
	}
	return hashdir.Result{Issues: issues}
}

// withoutMarker drops the install marker from a re-verification: the manifest
// is built before the repoint, and library.Relocate rewrites the marker in
// Target, so its presence or content says nothing about the moved files.
func withoutMarker(r hashdir.Result) hashdir.Result {
	issues := r.Issues[:0:0]
	for _, issue := range r.Issues {
		if issue.Path != library.MarkerName {
			issues = append(issues, issue)
		}
	}
	extra := r.Extra[:0:0]
	for _, path := range r.Extra {
		if path != library.MarkerName {
			extra = append(extra, path)
		}
	}
	r.Issues, r.Extra = issues, extra
	return r
}

// continueAfterItem runs after an item's repoint+cleanup finishes during
// recovery. A single-game job is done outright. A library job that still
// has queue items left cannot safely continue: only the item that just
// finished had its own Source/Target in the journal — the operation's
// original old/new library roots were never journaled on their own, so the
// next item's destination cannot be recomputed after a restart. The already
// -moved items stay moved; the user re-runs MoveLibrary for the rest.
func (s *Service) continueAfterItem(job Job) {
	current, err := s.jobSnapshot(job.ID)
	if err != nil {
		return
	}
	if job.GameID != itemSettings && job.GameID != itemDownloads && job.GameID != itemScreenshots && job.GameID != "" {
		s.recordHistory(current)
	}
	if job.Scope == ScopeGame || len(current.Queue) == 0 {
		s.completeJob(job.ID)
		return
	}
	s.markRecoveryFailed(job.ID, errors.New("перенос библиотеки прерван, повторите операцию для оставшихся элементов"))
}
