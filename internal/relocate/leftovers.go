package relocate

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"typhon/internal/diskreason"
	"typhon/internal/library"
	"typhon/internal/platform"
	"typhon/internal/uierr"
)

var (
	errStagingRefused  = uierr.New("relocate.staging_refused", "эту временную папку переноса удалять нельзя")
	errStagingReserved = uierr.New("relocate.staging_reserved", "временную папку переноса сейчас удаляют")
)

// Staging is a staging folder of a move from the journal that is still on
// disk. Deletable is false whenever the folder may be the only copy of the
// game or the job is not finished with it.
type Staging struct {
	Path      string `json:"path"`
	Deletable bool   `json:"deletable"`
	Reason    string `json:"reason"`
	Code      string `json:"reasonCode"`
}

func terminal(stage Stage) bool {
	return stage == StageDone || stage == StageCancelled || stage == StageFailed
}

// stagingVerdict decides one journaled staging folder. The journal is a file
// on disk and may be damaged, so the folder must also be exactly what a move
// of this target would have made, and clear of the source and of every game.
func stagingVerdict(job Job, sourceExists bool, games []library.Game) (ok bool, code, reason string) {
	switch {
	case !terminal(job.Stage):
		return false, diskreason.RelocateActive, "идёт перенос игры"
	case job.Stage == StageFailed && (job.FailedAt == StageRepoint || job.FailedAt == StageCleanup || job.FailedAt == StageCommit):
		return false, diskreason.RelocateActive, "перенос прервался после копирования и будет доведён при запуске"
	case job.Target == "" || !platform.SamePath(job.Staging, job.Target+stagingSuffix):
		return false, diskreason.RelocateJournal, "путь в журнале переноса не совпадает с ожидаемым"
	case !sourceExists:
		return false, diskreason.OnlyCopy, "исходной папки уже нет, эта копия может быть единственной"
	case job.Source == "" || platform.SamePath(job.Source, job.Staging) ||
		platform.Inside(job.Source, job.Staging) || platform.Inside(job.Staging, job.Source):
		return false, diskreason.RelocateJournal, "папка пересекается с исходной"
	}
	for _, g := range games {
		if g.InstallDir != "" && (platform.SamePath(g.InstallDir, job.Staging) ||
			platform.Inside(g.InstallDir, job.Staging) || platform.Inside(job.Staging, g.InstallDir)) {
			return false, diskreason.HoldsGame, "в этой папке лежит игра из библиотеки"
		}
	}
	return true, "", ""
}

func (s *Service) stagingFacts() ([]Job, []library.Game, error) {
	if s.lib == nil {
		return nil, nil, errors.New("библиотека недоступна, папки переноса не сверить с играми")
	}
	return s.List(), s.lib.GetGames(), nil
}

// Stagings lists the staging folders of the journaled moves that exist, each
// with the verdict on whether it may be deleted.
//
//wails:ignore
func (s *Service) Stagings() ([]Staging, error) {
	jobs, games, err := s.stagingFacts()
	if err != nil {
		return nil, err
	}
	out := make([]Staging, 0, len(jobs))
	for _, job := range jobs {
		if job.Staging == "" {
			continue
		}
		if _, err := os.Lstat(job.Staging); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, err
		}
		_, srcErr := os.Lstat(job.Source)
		if srcErr != nil && !errors.Is(srcErr, fs.ErrNotExist) {
			return nil, srcErr
		}
		ok, code, reason := stagingVerdict(job, srcErr == nil, games)
		out = append(out, Staging{Path: job.Staging, Deletable: ok, Reason: reason, Code: code})
	}
	return out, nil
}

// ReserveStaging checks that the staging folder at path may go and, in the
// same step, bars every move from copying into it until release is called.
//
//wails:ignore
func (s *Service) ReserveStaging(path string) (func(), error) {
	key, err := platform.PathKey(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errStagingRefused, err)
	}
	stagings, err := s.Stagings()
	if err != nil {
		return nil, err
	}
	found := false
	for _, st := range stagings {
		if !platform.SamePath(st.Path, path) {
			continue
		}
		found = true
		if !st.Deletable {
			return nil, fmt.Errorf("%w: %s", errStagingRefused, st.Reason)
		}
	}
	if !found {
		return nil, fmt.Errorf("%w: перенос больше не ссылается на эту папку", errStagingRefused)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, j := range s.jobs {
		if j.Staging != "" && platform.SamePath(j.Staging, path) && !terminal(j.Stage) {
			return nil, fmt.Errorf("%w: идёт перенос игры", errStagingRefused)
		}
	}
	if s.reservedStaging == nil {
		s.reservedStaging = map[string]int{}
	}
	s.reservedStaging[key]++
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.reservedStaging[key]--; s.reservedStaging[key] <= 0 {
			delete(s.reservedStaging, key)
		}
	}, nil
}

func (s *Service) stagingReservedLocked(path string) bool {
	key, err := platform.PathKey(path)
	if err != nil {
		return false
	}
	return s.reservedStaging[key] > 0
}
