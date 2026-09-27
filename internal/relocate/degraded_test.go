package relocate

import (
	"os"
	"path/filepath"
	"testing"
)

// blockJournalDir seeds jobs at dir, then repoints the service's store at a
// path nested under a plain file so any later saveJournal fails: os.CreateTemp
// cannot create a temp file inside a "directory" that is actually a regular
// file.
func blockJournalDir(t *testing.T, s *Service, dir string) {
	t.Helper()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	s.st = newStore(filepath.Join(blocker, "sub"))
}

func TestRemoveJobRollsBackOnPersistFailure(t *testing.T) {
	dir := t.TempDir()
	s, err := NewServiceAt(dir, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	job := Job{ID: "job-1", Scope: ScopeGame, Stage: StageCleanup, Source: "s", Target: "t"}
	s.jobs = []Job{job}
	if err := s.persistJournalLocked(); err != nil {
		t.Fatalf("seed journal: %v", err)
	}

	blockJournalDir(t, s, dir)

	if err := s.removeJob(job.ID); err == nil {
		t.Fatal("persist failure must be returned to the caller")
	}

	if len(s.jobs) != 1 || s.jobs[0].ID != job.ID {
		t.Fatalf("jobs = %+v, want the job kept after a failed persist", s.jobs)
	}
	if !s.status.Degraded || s.status.Message == "" {
		t.Fatalf("status = %+v, want degraded with a message", s.status)
	}

	s.st = newStore(dir)
	if err := s.removeJob(job.ID); err != nil {
		t.Fatalf("remove after recovery: %v", err)
	}
	if len(s.jobs) != 0 {
		t.Fatalf("jobs = %+v, want empty after a successful removal", s.jobs)
	}
	if s.status.Degraded {
		t.Fatalf("status = %+v, want cleared after a successful persist", s.status)
	}
}

func TestTransitionRollsBackOnPersistFailure(t *testing.T) {
	dir := t.TempDir()
	s, err := NewServiceAt(dir, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	job := Job{ID: "job-1", Scope: ScopeGame, Stage: StagePrepare, Source: "s", Target: "t"}
	s.jobs = []Job{job}
	if err := s.persistJournalLocked(); err != nil {
		t.Fatalf("seed journal: %v", err)
	}

	blockJournalDir(t, s, dir)

	_, err = s.transition(job.ID, StageCopy, func(j *Job) {
		j.Staging = "staging"
		j.CurrentFile = "file.bin"
	})
	if err == nil {
		t.Fatal("persist failure must be returned to the caller")
	}

	got := s.jobs[0]
	if got.Stage != StagePrepare || got.Staging != "" || got.CurrentFile != "" {
		t.Fatalf("job = %+v, want rollback to the pre-mutation state", got)
	}
	if !s.status.Degraded || s.status.Message == "" {
		t.Fatalf("status = %+v, want degraded with a message", s.status)
	}

	s.st = newStore(dir)
	if _, err := s.transition(job.ID, StageCopy, func(j *Job) { j.Staging = "staging" }); err != nil {
		t.Fatalf("transition after recovery: %v", err)
	}
	if s.jobs[0].Stage != StageCopy || s.jobs[0].Staging != "staging" {
		t.Fatalf("job = %+v, want the mutation applied once the journal is writable again", s.jobs[0])
	}
	if s.status.Degraded {
		t.Fatalf("status = %+v, want cleared after a successful persist", s.status)
	}
}
