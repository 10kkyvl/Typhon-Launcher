package relocate

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"typhon/internal/library"
)

func TestStagings(t *testing.T) {
	cases := []struct {
		name       string
		job        Job
		target     string // overrides the expected target when set
		staging    string // overrides the staging path when set; "same" names the source
		makeSource bool
		makeStage  bool
		want       bool
		listed     bool
	}{
		{"cancelled job with source", Job{Stage: StageCancelled}, "", "", true, true, true, true},
		{"failed while copying keeps source", Job{Stage: StageFailed, FailedAt: StageCopy}, "", "", true, true, true, true},
		{"running job owns its staging", Job{Stage: StageCopy}, "", "", true, true, false, true},
		{"commit stage owns its staging", Job{Stage: StageCommit}, "", "", true, true, false, true},
		{"failed after commit is recovered at start", Job{Stage: StageFailed, FailedAt: StageCleanup}, "", "", true, true, false, true},
		{"source already gone: staging may be the only copy", Job{Stage: StageCancelled}, "", "", false, true, false, true},
		{"staging not on disk", Job{Stage: StageCancelled}, "", "", true, false, false, false},
		{"damaged journal: staging is not target plus suffix", Job{Stage: StageCancelled}, "", "other", true, true, false, true},
		{"damaged journal: no target", Job{Stage: StageCancelled}, "none", "", true, true, false, true},
		{"damaged journal: staging is the source", Job{Stage: StageCancelled}, "", "source", true, true, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newBareService(t, newTestLibrary(t))
			base := t.TempDir()
			c.job.ID = "j1"
			c.job.Source = filepath.Join(base, "src")
			c.job.Target = filepath.Join(base, "dst")
			c.job.Staging = c.job.Target + stagingSuffix
			switch c.staging {
			case "other":
				c.job.Staging = filepath.Join(base, "somewhere")
			case "source":
				c.job.Target = c.job.Source[:len(c.job.Source)-len(stagingSuffix)] + "x"
				c.job.Staging = c.job.Source
			}
			if c.target == "none" {
				c.job.Target = ""
			}
			if c.makeSource {
				if err := os.MkdirAll(c.job.Source, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if c.makeStage {
				if err := os.MkdirAll(c.job.Staging, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			s.mu.Lock()
			s.jobs = append(s.jobs, c.job)
			s.mu.Unlock()

			got, err := s.Stagings()
			if err != nil {
				t.Fatal(err)
			}
			if !c.listed {
				if len(got) != 0 {
					t.Fatalf("Stagings = %+v, want none", got)
				}
				return
			}
			if len(got) != 1 || got[0].Path != c.job.Staging || got[0].Deletable != c.want {
				t.Fatalf("Stagings = %+v, want deletable=%v", got, c.want)
			}
			if !c.want && got[0].Reason == "" {
				t.Fatal("a refusal must say why")
			}
		})
	}
}

func TestStagingInsideAGameFolderIsNeverDeletable(t *testing.T) {
	lib := newTestLibrary(t)
	s := newBareService(t, lib)
	_, gameDir := addTestGame(t, lib)
	source := filepath.Join(t.TempDir(), "src")
	job := Job{ID: "j", Stage: StageCancelled, Source: source, Target: filepath.Join(gameDir, "inner")}
	job.Staging = job.Target + stagingSuffix
	for _, dir := range []string{source, job.Staging} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	s.mu.Lock()
	s.jobs = append(s.jobs, job)
	s.mu.Unlock()

	got, err := s.Stagings()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Deletable || got[0].Reason == "" {
		t.Fatalf("a folder inside a game was offered for deletion: %+v", got)
	}
}

func TestReserveStaging(t *testing.T) {
	setup := func(t *testing.T, stage Stage) (*Service, Job) {
		s := newBareService(t, newTestLibrary(t))
		base := t.TempDir()
		job := Job{ID: "j1", Stage: stage, Source: filepath.Join(base, "src"), Target: filepath.Join(base, "dst")}
		job.Staging = job.Target + stagingSuffix
		for _, dir := range []string{job.Source, job.Staging} {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		s.mu.Lock()
		s.jobs = append(s.jobs, job)
		s.mu.Unlock()
		return s, job
	}

	t.Run("refused while the move runs", func(t *testing.T) {
		s, job := setup(t, StageCopy)
		if _, err := s.ReserveStaging(job.Staging); !errors.Is(err, errStagingRefused) {
			t.Fatalf("ReserveStaging = %v, want errStagingRefused", err)
		}
	})
	t.Run("refused for a path the journal does not name", func(t *testing.T) {
		s, _ := setup(t, StageCancelled)
		if _, err := s.ReserveStaging(t.TempDir()); !errors.Is(err, errStagingRefused) {
			t.Fatalf("ReserveStaging = %v, want errStagingRefused", err)
		}
	})
	t.Run("blocks a move that wants the same staging until released", func(t *testing.T) {
		s, job := setup(t, StageCancelled)
		release, err := s.ReserveStaging(job.Staging)
		if err != nil {
			t.Fatal(err)
		}
		if !s.reservedNow(job.Staging) {
			t.Fatal("reservation not visible to a move that wants the folder")
		}
		release()
		if s.reservedNow(job.Staging) {
			t.Fatal("reservation outlived its release")
		}
	})
}

func (s *Service) reservedNow(path string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stagingReservedLocked(path)
}

func TestReserveStagingAndTheCopyNeverBothWin(t *testing.T) {
	for round := 0; round < 200; round++ {
		s := newBareService(t, newTestLibrary(t))
		base := t.TempDir()
		old := Job{ID: "old", Stage: StageCancelled, Source: filepath.Join(base, "src"), Target: filepath.Join(base, "dst")}
		old.Staging = old.Target + stagingSuffix
		fresh := Job{ID: "fresh", Stage: StagePrepare, Source: old.Source, Target: old.Target}
		for _, dir := range []string{old.Source, old.Staging} {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		s.mu.Lock()
		s.jobs = append(s.jobs, old, fresh)
		s.mu.Unlock()

		var reserved, copying bool
		var release func()
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			var err error
			release, err = s.ReserveStaging(old.Staging)
			reserved = err == nil
		}()
		go func() {
			defer wg.Done()
			guard := func() error {
				if s.stagingReservedLocked(fresh.Target + stagingSuffix) {
					return errStagingReserved
				}
				return nil
			}
			_, err := s.guardedTransition("fresh", StageCopy, guard, func(j *Job) { j.Staging = j.Target + stagingSuffix })
			copying = err == nil
		}()
		wg.Wait()
		if reserved && copying {
			t.Fatalf("round %d: the staging was reserved for deletion while a move started copying into it", round)
		}
		if !reserved && !copying {
			t.Fatalf("round %d: both sides were refused", round)
		}
		if release != nil {
			release()
		}
	}
}

// newBareService is a service that was never started: the startup recovery
// would otherwise pick up the journal entries a test puts in by hand.
func newBareService(t *testing.T, lib *library.Service) *Service {
	t.Helper()
	s, err := NewServiceAt(t.TempDir(), nil, lib, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
