package relocate

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"typhon/internal/download"
	"typhon/internal/hashdir"
	"typhon/internal/history"
	"typhon/internal/install"
	"typhon/internal/library"
	"typhon/internal/platform"
	"typhon/internal/settings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	eventStarted   = "move:started"
	eventProgress  = "move:progress"
	eventCompleted = "move:completed"
	eventFailed    = "move:failed"
	eventCancelled = "move:cancelled"

	progressThrottle = 250 * time.Millisecond
)

// busyChecker matches internal/install.Service.Busy and
// internal/updates.Service.Busy.
type busyChecker interface{ Busy(gameID string) bool }

type Service struct {
	mu      sync.Mutex
	st      *store
	jobs    []Job
	running map[string]context.CancelFunc
	done    map[string]chan struct{}
	lastTx  map[string]time.Time

	settings *settings.Service
	lib      *library.Service
	dl       *download.Manager
	inst     busyChecker
	upd      busyChecker

	historyRecord func(history.Record) error

	// afterItem is a test-only hook, invoked synchronously right after a
	// library-move queue item settles, so tests can call Cancel exactly
	// between two items without a sleep-based poll.
	afterItem func(Job)

	ctx     context.Context
	cancel  context.CancelFunc
	closing bool
	wg      sync.WaitGroup
}

func NewService(set *settings.Service, lib *library.Service, dl *download.Manager, inst, upd busyChecker) (*Service, error) {
	dir, err := settings.ConfigDir()
	if err != nil {
		return nil, fmt.Errorf("resolve config dir: %w", err)
	}
	return NewServiceAt(dir, set, lib, dl, inst, upd)
}

//wails:ignore
func NewServiceAt(dir string, set *settings.Service, lib *library.Service, dl *download.Manager, inst, upd busyChecker) (*Service, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("moves state dir unavailable")
	}
	st := newStore(dir)
	jobs, err := st.loadJournal()
	if err != nil {
		return nil, err
	}
	return &Service{
		st:       st,
		jobs:     jobs,
		running:  map[string]context.CancelFunc{},
		done:     map[string]chan struct{}{},
		lastTx:   map[string]time.Time{},
		settings: set,
		lib:      lib,
		dl:       dl,
		inst:     inst,
		upd:      upd,
	}, nil
}

func (s *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.mu.Lock()
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.recoverAll(s.ctx)
	}()
	return nil
}

func (s *Service) ServiceShutdown() error {
	s.mu.Lock()
	s.closing = true
	cancel := s.cancel
	for _, c := range s.running {
		c()
	}
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	s.wg.Wait()
	return nil
}

//wails:ignore
func (s *Service) SetHistoryRecorder(fn func(history.Record) error) {
	s.mu.Lock()
	s.historyRecord = fn
	s.mu.Unlock()
}

func (s *Service) List() []Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		out = append(out, j.clone())
	}
	return out
}

func (s *Service) SelectTargetFolder() (string, error) {
	app := application.Get()
	if app == nil {
		return "", errors.New("диалог недоступен")
	}
	dialog := app.Dialog.OpenFile().
		SetTitle("Выберите папку назначения").
		CanChooseDirectories(true).
		CanChooseFiles(false)
	path, err := dialog.PromptForSingleSelection()
	if err != nil {
		slog.Warn("select target folder", "error", err)
		return "", err
	}
	return path, nil
}

func (s *Service) Cancel(jobID string) error {
	s.mu.Lock()
	cancel, ok := s.running[jobID]
	s.mu.Unlock()
	if !ok {
		return ErrJobNotFound
	}
	cancel()
	return nil
}

// context returns the service's running context, or an error once the
// service has not started yet or is shutting down. There is no fallback to
// context.Background: a caller before ServiceStartup or after
// ServiceShutdown is a programming error, not a state to paper over.
func (s *Service) context() (context.Context, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing {
		return nil, errors.New("операция переноса недоступна: сервис завершает работу")
	}
	if s.ctx == nil {
		return nil, errors.New("операция переноса недоступна: сервис ещё не запущен")
	}
	return s.ctx, nil
}

func newID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("m%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

// --- journal bookkeeping -----------------------------------------------

func (s *Service) findJobLocked(id string) *Job {
	for i := range s.jobs {
		if s.jobs[i].ID == id {
			return &s.jobs[i]
		}
	}
	return nil
}

func (s *Service) persistJournalLocked() error {
	return s.st.saveJournal(s.jobs)
}

func (s *Service) jobSnapshot(id string) (Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.findJobLocked(id)
	if job == nil {
		return Job{}, ErrJobNotFound
	}
	return job.clone(), nil
}

// registerJob appends job to the journal and persists it. The conflict
// check against every other active job happens inside the same critical
// section as the append: checkBusy (external systems) and this check
// (relocate's own in-flight jobs) run at different times relative to the
// caller, but nothing can observe "no conflict" here and then have a
// second registerJob call slip a colliding job in before this one's append
// — that gap is exactly the TOCTOU invariant 17 forbids.
func (s *Service) registerJob(job Job) error {
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		return errors.New("операция переноса недоступна: сервис завершает работу")
	}
	if err := s.conflictsLocked(job.GameID, job.Source, job.Target); err != nil {
		s.mu.Unlock()
		return err
	}
	if job.Scope == ScopeLibrary {
		if err := s.libraryConflictLocked(); err != nil {
			s.mu.Unlock()
			return err
		}
	}
	s.jobs = append(s.jobs, job)
	if err := s.persistJournalLocked(); err != nil {
		s.jobs = s.jobs[:len(s.jobs)-1]
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()
	emit(eventStarted, job.clone())
	return nil
}

func jobActiveStage(stage Stage) bool {
	return stage != StageFailed && stage != StageCancelled
}

// pathsConflict reports whether the two (source, target) pairs overlap in
// either direction: same path, or one nested inside the other. Two
// different games racing onto the same destination folder is exactly as
// dangerous as the same game racing against itself.
func pathsConflict(aSource, aTarget, bSource, bTarget string) bool {
	pairs := [4][2]string{
		{aSource, bSource}, {aSource, bTarget},
		{aTarget, bSource}, {aTarget, bTarget},
	}
	for _, p := range pairs {
		if p[0] == "" || p[1] == "" {
			continue
		}
		if platform.SamePath(p[0], p[1]) || platform.Inside(p[0], p[1]) || platform.Inside(p[1], p[0]) {
			return true
		}
	}
	return false
}

func (s *Service) conflictsLocked(gameID, source, target string) error {
	for _, j := range s.jobs {
		if !jobActiveStage(j.Stage) {
			continue
		}
		if gameID != "" && j.GameID == gameID {
			return fmt.Errorf("%s: %w", gameID, ErrMoveInProgress)
		}
		if pathsConflict(source, target, j.Source, j.Target) {
			return fmt.Errorf("%s: %w", target, ErrMoveInProgress)
		}
	}
	return nil
}

// gameConflict is conflictsLocked's gameID-only pre-check, taken before any
// filesystem probing. Path collisions still need a real target, so those
// stay caught only by the atomic check inside registerJob.
func (s *Service) gameConflict(gameID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conflictsLocked(gameID, "", "")
}

// libraryConflictLocked refuses a second concurrent MoveLibrary: it would
// walk the same games/downloads/screenshots as the first and race it
// end to end, not just on one path.
func (s *Service) libraryConflictLocked() error {
	for _, j := range s.jobs {
		if jobActiveStage(j.Stage) && j.Scope == ScopeLibrary {
			return ErrMoveInProgress
		}
	}
	return nil
}

func (s *Service) removeJob(id string) {
	s.mu.Lock()
	for i := range s.jobs {
		if s.jobs[i].ID == id {
			s.jobs = append(s.jobs[:i:i], s.jobs[i+1:]...)
			break
		}
	}
	if err := s.persistJournalLocked(); err != nil {
		slog.Error("persist moves journal after cleanup", "job", id, "error", err)
	}
	delete(s.lastTx, id)
	s.mu.Unlock()
}

// transition mutates a job, stamps UpdatedAt, persists the whole journal
// and emits the event matching the new stage. Every stage change goes
// through here so the on-disk journal never lags behind what a crash needs
// to see (invariant 9): progress-only updates use setProgress instead,
// which does not hit disk.
func (s *Service) transition(id string, stage Stage, mutate func(*Job)) (Job, error) {
	s.mu.Lock()
	job := s.findJobLocked(id)
	if job == nil {
		s.mu.Unlock()
		return Job{}, ErrJobNotFound
	}
	if mutate != nil {
		mutate(job)
	}
	job.Stage = stage
	job.UpdatedAt = time.Now()
	snap := job.clone()
	if err := s.persistJournalLocked(); err != nil {
		s.mu.Unlock()
		return Job{}, err
	}
	s.mu.Unlock()
	emit(eventForStage(stage), snap)
	return snap, nil
}

func eventForStage(stage Stage) string {
	switch stage {
	case StageDone:
		return eventCompleted
	case StageFailed:
		return eventFailed
	case StageCancelled:
		return eventCancelled
	default:
		return eventProgress
	}
}

func (s *Service) setProgress(id string, copied int64, currentFile string) {
	s.mu.Lock()
	job := s.findJobLocked(id)
	if job == nil {
		s.mu.Unlock()
		return
	}
	job.CopiedBytes = copied
	job.CurrentFile = currentFile
	job.UpdatedAt = time.Now()
	now := time.Now()
	shouldEmit := now.Sub(s.lastTx[id]) >= progressThrottle
	var snap Job
	if shouldEmit {
		s.lastTx[id] = now
		snap = job.clone()
	}
	s.mu.Unlock()
	if shouldEmit {
		emit(eventProgress, snap)
	}
}

func emit(name string, data any) {
	if app := application.Get(); app != nil {
		app.Event.Emit(name, data)
	}
}

// --- job lifecycle: spawn / cancel / wait -------------------------------

func (s *Service) spawnJob(id string) context.Context {
	s.mu.Lock()
	jobCtx, cancel := context.WithCancel(s.ctx)
	s.running[id] = cancel
	if _, ok := s.done[id]; !ok {
		s.done[id] = make(chan struct{})
	}
	s.mu.Unlock()
	return jobCtx
}

func (s *Service) finishJob(id string) {
	s.mu.Lock()
	if cancel, ok := s.running[id]; ok {
		cancel()
		delete(s.running, id)
	}
	ch := s.done[id]
	s.mu.Unlock()
	if ch != nil {
		close(ch)
	}
}

// wait blocks until the job identified by id reaches a terminal stage. It
// exists for tests, which must not poll with time.Sleep: they synchronize
// on this channel instead.
func (s *Service) wait(id string) Job {
	s.mu.Lock()
	ch := s.done[id]
	s.mu.Unlock()
	if ch != nil {
		<-ch
	}
	job, err := s.jobSnapshot(id)
	if err != nil {
		return Job{}
	}
	return job
}

// --- shared validation ---------------------------------------------------

func downloadUnfinished(status download.Status) bool {
	return status != download.StatusCompleted && status != download.StatusFailed
}

func (s *Service) checkBusy(gameID string) error {
	if s.lib != nil && s.lib.IsRunning(gameID) {
		return fmt.Errorf("%s: %w", gameID, ErrGameRunning)
	}
	if s.upd != nil && s.upd.Busy(gameID) {
		return fmt.Errorf("%s: %w", gameID, ErrUpdateBusy)
	}
	if s.inst != nil && s.inst.Busy(gameID) {
		return fmt.Errorf("%s: %w", gameID, ErrInstallBusy)
	}
	if s.dl != nil {
		purposes := []download.Purpose{download.PurposeRelease, download.PurposeUpdate, download.PurposeRepair}
		for _, purpose := range purposes {
			for _, d := range s.dl.ByOrigin(gameID, purpose) {
				if downloadUnfinished(d.Status) {
					return fmt.Errorf("%s: %w", gameID, ErrDownloadBusy)
				}
			}
		}
	}
	return nil
}

func validateTarget(source, target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", ErrEmptyTarget
	}
	if !filepath.IsAbs(target) {
		return "", fmt.Errorf("%w: %s", ErrRelativeTarget, target)
	}
	target = filepath.Clean(target)
	if isVolumeRoot(target) {
		return "", fmt.Errorf("%w: %s", ErrTargetIsRoot, target)
	}
	if platform.Inside(source, target) {
		return "", ErrTargetInsideSource
	}
	if platform.Inside(target, source) {
		return "", ErrSourceInsideTarget
	}
	if !targetAvailable(target) {
		return "", ErrTargetNotEmpty
	}
	return target, nil
}

func isVolumeRoot(path string) bool {
	vol := filepath.VolumeName(path)
	rest := strings.TrimPrefix(path, vol)
	return rest == string(filepath.Separator) || rest == ""
}

func targetAvailable(path string) bool {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return true
	}
	if err != nil || !info.IsDir() {
		return false
	}
	entries, err := os.ReadDir(path)
	return err == nil && len(entries) == 0
}

func checkFreeSpace(target string, needed int64) error {
	info, err := platform.GetStorageInfo(filepath.Dir(target))
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFreeSpaceUnknown, err)
	}
	if needed < 0 {
		needed = 0
	}
	required := uint64(needed) + uint64(needed)*spaceMarginPercent/100
	if info.FreeBytes < required {
		return fmt.Errorf("%w: нужно %d байт, доступно %d", ErrNotEnoughSpace, required, info.FreeBytes)
	}
	return nil
}

func rebase(oldRoot, newRoot, path string) string {
	rel, err := filepath.Rel(oldRoot, path)
	if err != nil || !filepath.IsLocal(rel) {
		return filepath.Join(newRoot, filepath.Base(path))
	}
	return filepath.Join(newRoot, rel)
}

func removeStaging(path string) {
	if err := os.RemoveAll(path); err != nil {
		slog.Warn("remove move staging", "path", path, "error", err)
	}
}

func describeVerify(r hashdir.Result) string {
	if len(r.Issues) > 0 {
		return fmt.Sprintf("%s: %s", r.Issues[0].Path, r.Issues[0].Kind)
	}
	if len(r.Extra) > 0 {
		return fmt.Sprintf("лишний файл: %s", r.Extra[0])
	}
	return ""
}

// --- MoveGame -------------------------------------------------------------

func (s *Service) MoveGame(gameID, target string) (Job, error) {
	ctx, err := s.context()
	if err != nil {
		return Job{}, err
	}
	game, err := s.lib.Find(gameID)
	if err != nil {
		return Job{}, fmt.Errorf("%s: %w", gameID, ErrGameNotFound)
	}
	source := strings.TrimSpace(game.InstallDir)
	if source == "" {
		return Job{}, fmt.Errorf("%s: %w", gameID, ErrEmptyInstallDir)
	}
	source = filepath.Clean(source)

	if err := s.checkBusy(gameID); err != nil {
		return Job{}, err
	}
	// Cheap pre-check before the filesystem work below: a second MoveGame
	// for a game already being moved would otherwise race DirSize (and
	// hashdir.Build/CopyDir inside the first job's pipeline) against files
	// the first job already has open, surfacing a confusing OS-level I/O
	// error instead of a clean ErrMoveInProgress. registerJob still holds
	// the atomic guarantee (invariant 17); this only makes the common case
	// fail fast and legibly.
	if err := s.gameConflict(gameID); err != nil {
		return Job{}, err
	}
	cleanTarget, err := validateTarget(source, target)
	if err != nil {
		return Job{}, err
	}

	total, err := install.DirSize(ctx, source)
	if err != nil {
		return Job{}, fmt.Errorf("measure %s: %w", source, err)
	}
	if err := checkFreeSpace(cleanTarget, total); err != nil {
		return Job{}, err
	}

	job := Job{
		ID:         newID(),
		Scope:      ScopeGame,
		Stage:      StagePrepare,
		GameID:     gameID,
		Title:      game.Title,
		Source:     source,
		Target:     cleanTarget,
		TotalBytes: total,
		StartedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := s.registerJob(job); err != nil {
		return Job{}, err
	}

	jobCtx := s.spawnJob(job.ID)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.runGameJob(jobCtx, job.ID, source, cleanTarget)
	}()
	return job.clone(), nil
}

func (s *Service) runGameJob(ctx context.Context, jobID, source, target string) {
	defer s.finishJob(jobID)
	if err := s.runPipeline(ctx, jobID, source, target, s.repointGame); err != nil {
		return
	}
	job, err := s.jobSnapshot(jobID)
	if err != nil {
		return
	}
	s.recordHistory(job)
	s.completeJob(jobID)
}

// completeJob emits move:completed once and drops the journal entry: a
// finished move needs no more crash-recovery tracking (per the per-item
// algorithm's own "снять запись журнала, done" ordering), unlike a failed
// or cancelled job, which stays listed so a UI that missed the event can
// still see the error via List().
func (s *Service) completeJob(jobID string) {
	job, err := s.jobSnapshot(jobID)
	if err != nil {
		return
	}
	job.Stage = StageDone
	job.UpdatedAt = time.Now()
	emit(eventCompleted, job.clone())
	s.removeJob(jobID)
}

func (s *Service) repointGame(_ context.Context, job Job) error {
	if _, err := s.lib.Relocate(job.GameID, job.Target); err != nil {
		return fmt.Errorf("relocate %s: %w", job.GameID, err)
	}
	return nil
}

func repointNoop(context.Context, Job) error { return nil }

func (s *Service) recordHistory(job Job) {
	s.mu.Lock()
	rec := s.historyRecord
	s.mu.Unlock()
	if rec == nil {
		return
	}
	if err := rec(history.Record{
		Kind:       history.KindMoved,
		GameID:     job.GameID,
		Title:      job.Title,
		Bytes:      job.TotalBytes,
		BytesKnown: true,
		Detail:     fmt.Sprintf("%s → %s", job.Source, job.Target),
	}); err != nil {
		slog.Error("record history", "kind", history.KindMoved, "job", job.ID, "error", err)
	}
}

// --- shared move pipeline --------------------------------------------------

// runPipeline moves source to target through the staged
// commit-fast/copy/verify/commit/repoint/cleanup pipeline. The caller has
// already persisted the job at StagePrepare with GameID/Title/Source/
// Target/TotalBytes set for this item. On success the job is left at
// whatever state cleanup produced (source removed); it does not set
// StageDone, since a library move still has more queue items to process —
// the caller decides when the whole job is finished.
func (s *Service) runPipeline(ctx context.Context, jobID, source, target string, repoint func(context.Context, Job) error) error {
	if err := ctx.Err(); err != nil {
		return s.cancelJob(jobID)
	}

	if renameErr := os.Rename(source, target); renameErr == nil {
		if _, err := s.transition(jobID, StageRepoint, func(j *Job) {
			j.Renamed = true
			j.CopiedBytes = j.TotalBytes
		}); err != nil {
			return err
		}
	} else {
		if err := s.copyAndVerify(ctx, jobID, source, target); err != nil {
			return err
		}
	}

	current, err := s.jobSnapshot(jobID)
	if err != nil {
		return err
	}
	if err := repoint(ctx, current); err != nil {
		// The bytes already live at Target. Leaving the job at StageRepoint
		// (not Failed) means the next startup's recovery retries this call
		// instead of abandoning already-moved data.
		return err
	}
	return s.cleanupItem(jobID, source)
}

// afterCopyBeforeVerify is a test-only seam: tests use it to corrupt a file
// in staging between the copy and verify steps — the same window a real
// crash could land in — so TestMoveGameVerifyFailureKeepsSource can prove
// the source survives a verify failure without needing an actual crash.
// Nil in production.
var afterCopyBeforeVerify func(staging string)

func (s *Service) copyAndVerify(ctx context.Context, jobID, source, target string) error {
	staging := target + ".staging"
	if _, err := s.transition(jobID, StageCopy, func(j *Job) {
		j.Staging = staging
		j.Phase = phaseCopying
		j.CopiedBytes = 0
		j.CurrentFile = ""
	}); err != nil {
		return err
	}

	manifest, err := hashdir.Build(ctx, source, s.hashProgress(jobID))
	if err != nil {
		return s.failOrCancel(jobID, err)
	}
	if err := s.st.saveManifest(jobID, manifest); err != nil {
		return s.failOrCancel(jobID, err)
	}
	if err := install.CopyDir(ctx, source, staging, s.copyProgress(jobID)); err != nil {
		removeStaging(staging)
		return s.failOrCancel(jobID, err)
	}
	if afterCopyBeforeVerify != nil {
		afterCopyBeforeVerify(staging)
	}

	if _, err := s.transition(jobID, StageVerify, func(j *Job) {
		j.Phase = phaseVerifying
		j.CopiedBytes = 0
		j.CurrentFile = ""
	}); err != nil {
		return err
	}
	result, err := hashdir.Verify(ctx, staging, manifest, s.hashProgress(jobID))
	if err != nil {
		removeStaging(staging)
		return s.failOrCancel(jobID, err)
	}
	if len(result.Issues) > 0 || len(result.Extra) > 0 {
		removeStaging(staging)
		return s.failOrCancel(jobID, fmt.Errorf("%w: %s", ErrVerifyFailed, describeVerify(result)))
	}

	if _, err := s.transition(jobID, StageCommit, nil); err != nil {
		return err
	}
	if err := os.Rename(staging, target); err != nil {
		return s.failOrCancel(jobID, err)
	}
	// The manifest stays on disk past this point: a crash recovery resuming
	// at StageCleanup needs it to re-verify Target before it may delete
	// Source (invariant 10). cleanupItem removes it once cleanup finishes.
	if _, err := s.transition(jobID, StageRepoint, func(j *Job) {
		j.Staging = ""
		j.CopiedBytes = j.TotalBytes
	}); err != nil {
		return err
	}
	return nil
}

// cleanupItem removes source once its data is safely at Target and Target
// has been repointed to. It does not re-verify Target: the copy path just
// verified it by hash before the commit rename a moment ago, and re-hashing
// gigabytes of data again here would be the exact repeated expensive
// operation invariant 35 forbids. A crash-recovery resume at StageCleanup
// is different — it cannot trust that the pre-crash verify actually
// finished — and re-verifies itself in recover.go before calling this.
func (s *Service) cleanupItem(jobID, source string) error {
	if _, err := s.transition(jobID, StageCleanup, nil); err != nil {
		return err
	}
	if err := os.RemoveAll(source); err != nil {
		return s.failOrCancel(jobID, fmt.Errorf("remove old %s: %w", source, err))
	}
	if err := s.st.removeManifest(jobID); err != nil {
		slog.Warn("remove move manifest", "job", jobID, "error", err)
	}
	return nil
}

func (s *Service) hashProgress(jobID string) func(hashdir.Progress) {
	return func(p hashdir.Progress) {
		s.setProgress(jobID, p.ProcessedBytes, p.CurrentFile)
	}
}

func (s *Service) copyProgress(jobID string) func(install.Progress) {
	return func(p install.Progress) {
		s.setProgress(jobID, p.BytesDone, p.CurrentFile)
	}
}

func (s *Service) failOrCancel(jobID string, err error) error {
	stage := StageFailed
	if errors.Is(err, context.Canceled) {
		stage = StageCancelled
	}
	if _, tErr := s.transition(jobID, stage, func(j *Job) {
		j.Error = err.Error()
	}); tErr != nil {
		return tErr
	}
	return err
}

func (s *Service) cancelJob(jobID string) error {
	if _, err := s.transition(jobID, StageCancelled, func(j *Job) {
		j.Error = context.Canceled.Error()
	}); err != nil {
		return err
	}
	return context.Canceled
}

// --- MoveLibrary ------------------------------------------------------------

// MoveLibrary moves the whole library (every installed game, the downloads
// tree and the screenshots tree) to a new root and switches settings over
// to it. settings.derivePaths ties GamesPath/DownloadsPath/ScreenshotsPath
// to one LibraryPath, so this is the only way to relocate downloads too —
// there is no separate "move downloads" setting to drive independently.
func (s *Service) MoveLibrary(parent string) (Job, error) {
	if _, err := s.context(); err != nil {
		return Job{}, err
	}
	root, err := s.settings.ProposeLibraryPath(parent)
	if err != nil {
		return Job{}, err
	}
	cfg := s.settings.GetSettings()
	oldRoot := strings.TrimSpace(cfg.LibraryPath)
	if oldRoot == "" {
		return Job{}, settings.ErrLibraryNotConfigured
	}
	oldRoot = filepath.Clean(oldRoot)
	if platform.SamePath(oldRoot, root) {
		return Job{}, ErrTargetInsideSource
	}
	if platform.Inside(oldRoot, root) {
		return Job{}, ErrTargetInsideSource
	}
	if platform.Inside(root, oldRoot) {
		return Job{}, ErrSourceInsideTarget
	}

	games := s.lib.GetInstalledGames()
	for _, g := range games {
		if strings.TrimSpace(g.InstallDir) == "" {
			continue
		}
		if err := s.checkBusy(g.ID); err != nil {
			return Job{}, err
		}
	}
	if s.dl != nil {
		for _, d := range s.dl.List() {
			if downloadUnfinished(d.Status) {
				return Job{}, fmt.Errorf("%s: %w", d.Name, ErrDownloadBusy)
			}
		}
	}

	queue := make([]string, 0, len(games)+3)
	for _, g := range games {
		if strings.TrimSpace(g.InstallDir) == "" {
			continue
		}
		queue = append(queue, g.ID)
	}
	queue = append(queue, itemDownloads, itemScreenshots, itemSettings)

	job := Job{
		ID:        newID(),
		Scope:     ScopeLibrary,
		Stage:     StagePrepare,
		Title:     "Библиотека",
		Source:    oldRoot,
		Target:    root,
		Queue:     queue,
		StartedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := s.registerJob(job); err != nil {
		return Job{}, err
	}

	jobCtx := s.spawnJob(job.ID)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.runLibraryJob(jobCtx, job.ID, cfg, oldRoot, root)
	}()
	return job.clone(), nil
}

func (s *Service) runLibraryJob(ctx context.Context, jobID string, cfg settings.Settings, oldRoot, root string) {
	defer s.finishJob(jobID)
	if !s.processLibraryQueue(ctx, jobID, cfg, oldRoot, root) {
		return
	}
	s.completeJob(jobID)
}

// processLibraryQueue drains Queue one item at a time. It returns true only
// once every item — including the trailing "@settings" item that saves the
// new LibraryPath — has finished without error or cancellation.
func (s *Service) processLibraryQueue(ctx context.Context, jobID string, cfg settings.Settings, oldRoot, root string) bool {
	for {
		job, err := s.jobSnapshot(jobID)
		if err != nil {
			return false
		}
		if len(job.Queue) == 0 {
			return true
		}
		if err := ctx.Err(); err != nil {
			if cerr := s.cancelJob(jobID); cerr != nil && !errors.Is(cerr, context.Canceled) {
				slog.Error("mark library move cancelled", "job", jobID, "error", cerr)
			}
			return false
		}
		item := job.Queue[0]
		rest := append([]string(nil), job.Queue[1:]...)
		if err := s.runLibraryItem(ctx, jobID, item, rest, cfg, oldRoot, root); err != nil {
			return false
		}
		if s.afterItem != nil {
			if snap, err := s.jobSnapshot(jobID); err == nil {
				s.afterItem(snap)
			}
		}
	}
}

func (s *Service) runLibraryItem(ctx context.Context, jobID, item string, rest []string, cfg settings.Settings, oldRoot, root string) error {
	switch item {
	case itemDownloads:
		return s.runDownloadsItem(ctx, jobID, rest, cfg, oldRoot, root)
	case itemScreenshots:
		newPath := rebase(oldRoot, root, cfg.ScreenshotsPath)
		return s.runDirItem(ctx, jobID, itemScreenshots, "Скриншоты", cfg.ScreenshotsPath, newPath, rest)
	case itemSettings:
		return s.runSettingsItem(jobID, rest, oldRoot, root)
	default:
		return s.runGameLibraryItem(ctx, jobID, item, rest, oldRoot, root)
	}
}

func (s *Service) runGameLibraryItem(ctx context.Context, jobID, gameID string, rest []string, oldRoot, root string) error {
	game, err := s.lib.Find(gameID)
	if err != nil {
		return s.failOrCancel(jobID, fmt.Errorf("%s: %w", gameID, ErrGameNotFound))
	}
	source := strings.TrimSpace(game.InstallDir)
	if source == "" {
		return s.failOrCancel(jobID, fmt.Errorf("%s: %w", gameID, ErrEmptyInstallDir))
	}
	source = filepath.Clean(source)
	if err := s.checkBusy(gameID); err != nil {
		return s.failOrCancel(jobID, err)
	}
	target := rebase(oldRoot, root, source)
	if !targetAvailable(target) {
		return s.failOrCancel(jobID, fmt.Errorf("%s: %w", target, ErrTargetNotEmpty))
	}
	total, err := install.DirSize(ctx, source)
	if err != nil {
		return s.failOrCancel(jobID, fmt.Errorf("measure %s: %w", source, err))
	}
	if err := checkFreeSpace(target, total); err != nil {
		return s.failOrCancel(jobID, err)
	}

	if _, err := s.transition(jobID, StagePrepare, func(j *Job) {
		j.GameID = gameID
		j.Title = game.Title
		j.Source = source
		j.Target = target
		j.TotalBytes = total
		j.Renamed = false
		j.Staging = ""
		j.CopiedBytes = 0
		j.CurrentFile = ""
		j.Phase = ""
		j.Error = ""
		j.Queue = rest
	}); err != nil {
		return err
	}

	if err := s.runPipeline(ctx, jobID, source, target, s.repointGame); err != nil {
		return err
	}
	if current, err := s.jobSnapshot(jobID); err == nil {
		s.recordHistory(current)
	}
	return nil
}

// runDirItem moves one directory that carries no per-entry bookkeeping of
// its own (screenshots today). A missing source is not an error: it means
// the folder was never created, so there is nothing to move.
func (s *Service) runDirItem(ctx context.Context, jobID, itemID, title, source, target string, rest []string) error {
	source = filepath.Clean(strings.TrimSpace(source))
	target = filepath.Clean(strings.TrimSpace(target))
	if source == "" {
		return s.failOrCancel(jobID, fmt.Errorf("%s: %w", itemID, ErrEmptySource))
	}
	if target == "" {
		return s.failOrCancel(jobID, fmt.Errorf("%s: %w", itemID, ErrEmptyTarget))
	}

	if _, err := os.Stat(source); errors.Is(err, fs.ErrNotExist) {
		_, err := s.transition(jobID, StagePrepare, func(j *Job) {
			j.GameID = itemID
			j.Title = title
			j.Source = source
			j.Target = target
			j.TotalBytes = 0
			j.CopiedBytes = 0
			j.Staging = ""
			j.Renamed = false
			j.CurrentFile = ""
			j.Phase = ""
			j.Error = ""
			j.Queue = rest
		})
		return err
	}

	total, err := install.DirSize(ctx, source)
	if err != nil {
		return s.failOrCancel(jobID, fmt.Errorf("measure %s: %w", source, err))
	}
	if _, err := s.transition(jobID, StagePrepare, func(j *Job) {
		j.GameID = itemID
		j.Title = title
		j.Source = source
		j.Target = target
		j.TotalBytes = total
		j.Renamed = false
		j.Staging = ""
		j.CopiedBytes = 0
		j.CurrentFile = ""
		j.Phase = ""
		j.Error = ""
		j.Queue = rest
	}); err != nil {
		return err
	}
	return s.runPipeline(ctx, jobID, source, target, repointNoop)
}

// runDownloadsItem hands the downloads tree to download.Manager.Repoint
// instead of the generic pipeline: only that package can stop the live
// torrent engines holding its files open, and it owns the Destination
// bookkeeping that has to change together with the move.
func (s *Service) runDownloadsItem(ctx context.Context, jobID string, rest []string, cfg settings.Settings, oldRoot, root string) error {
	oldDownloads := strings.TrimSpace(cfg.DownloadsPath)
	if oldDownloads == "" {
		return s.failOrCancel(jobID, fmt.Errorf("%s: %w", itemDownloads, ErrEmptySource))
	}
	oldDownloads = filepath.Clean(oldDownloads)
	newDownloads := rebase(oldRoot, root, oldDownloads)

	// No download.Manager wired up (a degraded configuration, or a test that
	// does not need one): there is nothing to repoint, so just record that
	// this item was seen and move on rather than dereference a nil pointer.
	if s.dl == nil {
		_, err := s.transition(jobID, StagePrepare, func(j *Job) {
			j.GameID = itemDownloads
			j.Title = "Загрузки"
			j.Source = oldDownloads
			j.Target = newDownloads
			j.Queue = rest
		})
		return err
	}

	if _, err := s.transition(jobID, StagePrepare, func(j *Job) {
		j.GameID = itemDownloads
		j.Title = "Загрузки"
		j.Source = oldDownloads
		j.Target = newDownloads
		j.TotalBytes = 0
		j.CopiedBytes = 0
		j.Staging = ""
		j.Renamed = false
		j.CurrentFile = ""
		j.Phase = ""
		j.Error = ""
		j.Queue = rest
	}); err != nil {
		return err
	}
	if _, err := s.transition(jobID, StageRepoint, nil); err != nil {
		return err
	}
	if err := s.dl.Repoint(ctx, oldDownloads, newDownloads); err != nil {
		// Same reasoning as repointGame: the tree may already be at
		// newDownloads by the time Repoint fails on its own bookkeeping, so
		// the job stays at StageRepoint for an idempotent retry rather than
		// being marked failed.
		return err
	}
	return s.cleanupItem(jobID, oldDownloads)
}

// runSettingsItem is the queue's last item: once every directory has moved,
// it points settings.LibraryPath at the new root. Recording it as an
// ordinary queue item (rather than a step taken after the loop) means its
// own Source/Target land in the journal exactly like any other item, so
// recoverAll can find them after a crash without having to remember the
// operation's original arguments.
func (s *Service) runSettingsItem(jobID string, rest []string, oldRoot, root string) error {
	if _, err := s.transition(jobID, StagePrepare, func(j *Job) {
		j.GameID = itemSettings
		j.Title = "Библиотека"
		j.Source = oldRoot
		j.Target = root
		j.Staging = ""
		j.Renamed = false
		j.CopiedBytes = 0
		j.TotalBytes = 0
		j.CurrentFile = ""
		j.Phase = ""
		j.Error = ""
		j.Queue = rest
	}); err != nil {
		return err
	}
	return s.applyLibrarySettings(jobID, root)
}

func (s *Service) applyLibrarySettings(jobID, root string) error {
	if _, err := s.transition(jobID, StageRepoint, nil); err != nil {
		return err
	}
	next := s.settings.GetSettings()
	next.LibraryPath = root
	if err := s.settings.SaveSettings(next); err != nil {
		// Left at StageRepoint: recoverAll retries SaveSettings on next
		// startup, per the task's own note that every directory has already
		// moved by this point.
		return err
	}
	return nil
}
