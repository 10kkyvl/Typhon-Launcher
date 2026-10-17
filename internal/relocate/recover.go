package relocate

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"typhon/internal/hashdir"
	"typhon/internal/library"
	"typhon/internal/platform"
	"typhon/internal/settings"
)

// recoverAll resumes or undoes every job left in moves.json from a previous
// run. It runs once from ServiceStartup's goroutine, before any new
// MoveGame/MoveLibrary call can register another job, so it does not need
// to coordinate with the live queue processing.
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
	}
}

func (s *Service) recoverJob(ctx context.Context, job Job) {
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
// the leftover journal entry.
func (s *Service) surfaceStale(job Job) {
	emit(eventForStage(job.Stage), job.clone())
	if err := s.removeJob(job.ID); err != nil {
		// Logged and surfaced via move:degraded inside removeJob already;
		// recoverAll's goroutine has no synchronous caller to report to.
		return
	}
}

// reserveForRecovery takes the launch lock of the game a resumable job moves,
// so the game cannot start while recovery touches its folders. A game that is
// gone from the library ends the job at once without touching any data.
func (s *Service) reserveForRecovery(job Job) (func(), bool) {
	noop := func() {}
	if s.lib == nil || !recoveryTouchesGame(job) {
		return noop, true
	}
	if _, err := s.lib.Find(job.GameID); err != nil {
		s.markRecoveryFailed(job.ID, ErrGameRemoved)
		return nil, false
	}
	release, err := s.lib.BeginRelocate(job.GameID)
	if err != nil {
		slog.Warn("recovery skipped: game is busy", "job", job.ID, "game", job.GameID, "error", err)
		return nil, false
	}
	return release, true
}

func recoveryTouchesGame(job Job) bool {
	switch job.GameID {
	case "", itemDownloads, itemScreenshots, itemSettings:
		return false
	}
	switch job.Stage {
	case StagePrepare, StageCommit, StageRepoint, StageCleanup:
		return true
	case StageFailed:
		return job.FailedAt == StageRepoint || job.FailedAt == StageCleanup
	}
	return false
}

// recoverFailed resumes a job that failed after its bytes reached Target: a
// failed repoint is retried, and a failed cleanup retries the removal of the
// old copy, but only while Target is the copy the library points at. Every
// other failed job is just surfaced and dropped.
func (s *Service) recoverFailed(ctx context.Context, job Job) {
	switch job.FailedAt {
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
// by CopyDir/hashdir.Build, so only Staging needs cleaning up.
func (s *Service) recoverCopyOrVerify(job Job) {
	if job.Staging != "" {
		removeStaging(job.Staging)
	}
	if err := s.st.removeManifest(job.ID); err != nil {
		slog.Warn("remove move manifest on recovery", "job", job.ID, "error", err)
	}
	s.markRecoveryFailed(job.ID, fmt.Errorf("перенос прерван на этапе %s", job.Stage))
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
