package relocate

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"typhon/internal/hashdir"
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
	switch job.Stage {
	case StageDone, StageFailed, StageCancelled:
		s.surfaceStale(job)
	case StagePrepare:
		s.recoverPrepare(ctx, job)
	case StageCopy, StageVerify:
		s.recoverCopyOrVerify(job)
	case StageCommit:
		s.recoverCommit(ctx, job)
	case StageRepoint:
		s.recoverRepoint(ctx, job)
	case StageCleanup:
		s.recoverCleanup(ctx, job)
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
	s.removeJob(job.ID)
}

func (s *Service) markRecoveryFailed(jobID string, err error) {
	if _, tErr := s.transition(jobID, StageFailed, func(j *Job) {
		j.Error = err.Error()
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
		s.recoverRepoint(ctx, s.mustSnapshot(job.ID))
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
		s.recoverRepoint(ctx, s.mustSnapshot(job.ID))
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
		s.recoverRepoint(ctx, s.mustSnapshot(job.ID))
	default:
		// Both exist, or neither does: which one holds the good copy cannot
		// be decided automatically. Nothing is deleted; the user has to look.
		s.markRecoveryFailed(job.ID, fmt.Errorf("%w: %s и %s", ErrAmbiguousRecovery, job.Staging, job.Target))
	}
}

func (s *Service) recoverRepoint(ctx context.Context, job Job) {
	switch job.GameID {
	case itemSettings:
		next := s.settings.GetSettings()
		next.LibraryPath = job.Target
		if err := s.settings.SaveSettings(next); err != nil {
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
	if err := verifyManifest(ctx, job.Target, manifest); err != nil {
		s.markRecoveryFailed(job.ID, err)
		return
	}
	if err := os.RemoveAll(job.Source); err != nil {
		s.markRecoveryFailed(job.ID, fmt.Errorf("remove old %s: %w", job.Source, err))
		return
	}
	if err := s.st.removeManifest(job.ID); err != nil {
		slog.Warn("remove move manifest on recovery", "job", job.ID, "error", err)
	}
	s.continueAfterItem(job)
}

func verifyManifest(ctx context.Context, target string, manifest hashdir.Manifest) error {
	result, err := hashdir.Verify(ctx, target, manifest, nil)
	if err != nil {
		return fmt.Errorf("повторная проверка не удалась: %w", err)
	}
	if len(result.Issues) > 0 || len(result.Extra) > 0 {
		return fmt.Errorf("%w: %s", ErrVerifyFailed, describeVerify(result))
	}
	return nil
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
