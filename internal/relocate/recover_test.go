package relocate

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"typhon/internal/hashdir"
)

func newRecoverTestService(t *testing.T, job Job) *Service {
	t.Helper()
	dir := t.TempDir()
	st := newStore(dir)
	if err := st.saveJournal([]Job{job}); err != nil {
		t.Fatal(err)
	}
	return &Service{
		st:      st,
		jobs:    []Job{job},
		running: map[string]context.CancelFunc{},
		done:    map[string]chan struct{}{},
		lastTx:  map[string]time.Time{},
	}
}

func mustMkdirFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func (s *Service) soleJob(t *testing.T) Job {
	t.Helper()
	jobs := s.List()
	if len(jobs) != 1 {
		t.Fatalf("jobs = %+v, want exactly one", jobs)
	}
	return jobs[0]
}

func TestRecoverAtStage(t *testing.T) {
	t.Run("prepare leaves nothing touched", func(t *testing.T) {
		root := t.TempDir()
		src := filepath.Join(root, "src")
		mustMkdirFile(t, src, "game.exe", "data")
		tgt := filepath.Join(root, "tgt")

		job := Job{ID: "j1", Scope: ScopeGame, Stage: StagePrepare, GameID: itemScreenshots, Source: src, Target: tgt, TotalBytes: 4}
		s := newRecoverTestService(t, job)
		s.recoverJob(context.Background(), job)

		got := s.soleJob(t)
		if got.Stage != StageFailed {
			t.Fatalf("stage = %s, want %s", got.Stage, StageFailed)
		}
		if _, err := os.Stat(filepath.Join(src, "game.exe")); err != nil {
			t.Fatalf("source touched: %v", err)
		}
		if _, err := os.Stat(tgt); !os.IsNotExist(err) {
			t.Fatal("target should not exist")
		}
	})

	t.Run("prepare detects an unlogged rename", func(t *testing.T) {
		root := t.TempDir()
		tgt := filepath.Join(root, "tgt")
		mustMkdirFile(t, tgt, "game.exe", "data")
		src := filepath.Join(root, "src") // never created: the rename already moved it

		job := Job{ID: "j1", Scope: ScopeGame, Stage: StagePrepare, GameID: itemScreenshots, Source: src, Target: tgt, TotalBytes: 4}
		s := newRecoverTestService(t, job)
		s.recoverJob(context.Background(), job)

		if got := s.List(); len(got) != 0 {
			t.Fatalf("job should have completed and been dropped, got %+v", got)
		}
		if _, err := os.Stat(filepath.Join(tgt, "game.exe")); err != nil {
			t.Fatalf("target lost: %v", err)
		}
	})

	t.Run("copy removes staging and fails, source intact", func(t *testing.T) {
		root := t.TempDir()
		src := filepath.Join(root, "src")
		mustMkdirFile(t, src, "game.exe", "data")
		tgt := filepath.Join(root, "tgt")
		staging := tgt + ".staging"
		mustMkdirFile(t, staging, "game.exe", "partial")

		job := Job{ID: "j1", Scope: ScopeGame, Stage: StageCopy, GameID: itemScreenshots, Source: src, Target: tgt, Staging: staging, TotalBytes: 4}
		s := newRecoverTestService(t, job)
		s.recoverJob(context.Background(), job)

		got := s.soleJob(t)
		if got.Stage != StageFailed {
			t.Fatalf("stage = %s, want %s", got.Stage, StageFailed)
		}
		if _, err := os.Stat(staging); !os.IsNotExist(err) {
			t.Fatal("staging left behind")
		}
		if _, err := os.Stat(filepath.Join(src, "game.exe")); err != nil {
			t.Fatalf("source touched: %v", err)
		}
	})

	t.Run("verify removes staging and fails, source intact", func(t *testing.T) {
		root := t.TempDir()
		src := filepath.Join(root, "src")
		mustMkdirFile(t, src, "game.exe", "data")
		tgt := filepath.Join(root, "tgt")
		staging := tgt + ".staging"
		mustMkdirFile(t, staging, "game.exe", "data")

		job := Job{ID: "j1", Scope: ScopeGame, Stage: StageVerify, GameID: itemScreenshots, Source: src, Target: tgt, Staging: staging, TotalBytes: 4}
		s := newRecoverTestService(t, job)
		s.recoverJob(context.Background(), job)

		got := s.soleJob(t)
		if got.Stage != StageFailed {
			t.Fatalf("stage = %s, want %s", got.Stage, StageFailed)
		}
		if _, err := os.Stat(staging); !os.IsNotExist(err) {
			t.Fatal("staging left behind")
		}
	})

	t.Run("commit with only target present continues", func(t *testing.T) {
		root := t.TempDir()
		src := filepath.Join(root, "src")
		mustMkdirFile(t, src, "game.exe", "data")
		tgt := filepath.Join(root, "tgt")
		mustMkdirFile(t, tgt, "game.exe", "data")

		job := Job{ID: "j1", Scope: ScopeGame, Stage: StageCommit, GameID: itemScreenshots, Source: src, Target: tgt, Staging: tgt + ".staging", TotalBytes: 4}
		s := newRecoverTestService(t, job)
		s.recoverJob(context.Background(), job)

		if got := s.List(); len(got) != 0 {
			t.Fatalf("job should complete, got %+v", got)
		}
		if _, err := os.Stat(src); !os.IsNotExist(err) {
			t.Fatal("source should have been removed by cleanup")
		}
		if _, err := os.Stat(filepath.Join(tgt, "game.exe")); err != nil {
			t.Fatalf("target lost: %v", err)
		}
	})

	t.Run("commit with only staging present retries the rename", func(t *testing.T) {
		root := t.TempDir()
		src := filepath.Join(root, "src")
		mustMkdirFile(t, src, "game.exe", "data")
		tgt := filepath.Join(root, "tgt")
		staging := tgt + ".staging"
		mustMkdirFile(t, staging, "game.exe", "data")

		job := Job{ID: "j1", Scope: ScopeGame, Stage: StageCommit, GameID: itemScreenshots, Source: src, Target: tgt, Staging: staging, TotalBytes: 4}
		s := newRecoverTestService(t, job)
		s.recoverJob(context.Background(), job)

		if got := s.List(); len(got) != 0 {
			t.Fatalf("job should complete, got %+v", got)
		}
		if _, err := os.Stat(staging); !os.IsNotExist(err) {
			t.Fatal("staging should have been renamed away")
		}
		if _, err := os.Stat(filepath.Join(tgt, "game.exe")); err != nil {
			t.Fatalf("target missing after retried rename: %v", err)
		}
	})

	t.Run("commit with both staging and target present is ambiguous", func(t *testing.T) {
		root := t.TempDir()
		src := filepath.Join(root, "src")
		mustMkdirFile(t, src, "game.exe", "data")
		tgt := filepath.Join(root, "tgt")
		mustMkdirFile(t, tgt, "game.exe", "target-copy")
		staging := tgt + ".staging"
		mustMkdirFile(t, staging, "game.exe", "staging-copy")

		job := Job{ID: "j1", Scope: ScopeGame, Stage: StageCommit, GameID: itemScreenshots, Source: src, Target: tgt, Staging: staging, TotalBytes: 4}
		s := newRecoverTestService(t, job)
		s.recoverJob(context.Background(), job)

		got := s.soleJob(t)
		if got.Stage != StageFailed {
			t.Fatalf("stage = %s, want %s", got.Stage, StageFailed)
		}
		if got.Error == "" {
			t.Fatal("ambiguous recovery must record an error")
		}
		if _, err := os.Stat(tgt); err != nil {
			t.Fatal("target must not be deleted while ambiguous")
		}
		if _, err := os.Stat(staging); err != nil {
			t.Fatal("staging must not be deleted while ambiguous")
		}
		if _, err := os.Stat(src); err != nil {
			t.Fatal("source must not be deleted while ambiguous")
		}
	})

	t.Run("cleanup after a rename has nothing left to verify", func(t *testing.T) {
		root := t.TempDir()
		tgt := filepath.Join(root, "tgt")
		mustMkdirFile(t, tgt, "game.exe", "data")
		src := filepath.Join(root, "src") // already gone: Renamed

		job := Job{ID: "j1", Scope: ScopeGame, Stage: StageCleanup, GameID: itemScreenshots, Source: src, Target: tgt, Renamed: true, TotalBytes: 4}
		s := newRecoverTestService(t, job)
		s.recoverJob(context.Background(), job)

		if got := s.List(); len(got) != 0 {
			t.Fatalf("job should complete, got %+v", got)
		}
	})
}

// TestRecoverCleanupAlreadySucceededReportsSuccess covers KRIT-2: a crash
// after cleanupItem's RemoveAll(Source) succeeded — which only ever runs
// after a clean verify — but before the manifest/journal entry were
// cleared behind it. Before the fix, recoverCleanup unconditionally tried
// to re-verify Target against a manifest that either was already removed
// (open error) or, if it survived, would just re-confirm success no one
// asked for; either way the transfer had already finished. The job must
// resolve as done, not failed.
func TestRecoverCleanupAlreadySucceededReportsSuccess(t *testing.T) {
	root := t.TempDir()
	tgt := filepath.Join(root, "tgt")
	mustMkdirFile(t, tgt, "game.exe", "data")
	src := filepath.Join(root, "src") // RemoveAll(Source) already ran

	// No manifest on disk: cleanupItem removes it right after RemoveAll(Source)
	// succeeds, and that RemoveAll is exactly the step this scenario crashed
	// after — the manifest is already gone along with Source.
	job := Job{ID: "j1", Scope: ScopeGame, Stage: StageCleanup, GameID: itemScreenshots, Source: src, Target: tgt, Renamed: false, TotalBytes: 4}
	s := newRecoverTestService(t, job)

	s.recoverJob(context.Background(), job)

	// completeJob (via continueAfterItem) drops a successfully finished job
	// from the journal rather than leaving a StageDone row behind — the
	// same policy a live, non-recovered completion follows. A job still
	// listed here would mean recovery treated this as unresolved or failed.
	if got := s.List(); len(got) != 0 {
		t.Fatalf("job should have completed and been dropped, got %+v", got)
	}
	if _, err := os.Stat(filepath.Join(tgt, "game.exe")); err != nil {
		t.Fatalf("target lost: %v", err)
	}
	if _, err := os.Stat(s.st.manifestPath(job.ID)); !os.IsNotExist(err) {
		t.Fatal("leftover manifest not cleaned up")
	}
}

func TestRecoverCleanupVerifiesBeforeDeletingSource(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	mustMkdirFile(t, src, "game.exe", "original")
	tgt := filepath.Join(root, "tgt")
	mustMkdirFile(t, tgt, "game.exe", "original")

	manifest, err := hashdir.Build(context.Background(), src, nil)
	if err != nil {
		t.Fatal(err)
	}

	job := Job{ID: "j1", Scope: ScopeGame, Stage: StageCleanup, GameID: itemScreenshots, Source: src, Target: tgt, Renamed: false, TotalBytes: 8}
	s := newRecoverTestService(t, job)
	if err := s.st.saveManifest(job.ID, manifest); err != nil {
		t.Fatal(err)
	}

	// Corrupt the target after the manifest was built: this is exactly the
	// case a crash recovery cannot trust without re-hashing.
	if err := os.WriteFile(filepath.Join(tgt, "game.exe"), []byte("corrupted"), 0o644); err != nil {
		t.Fatal(err)
	}

	s.recoverJob(context.Background(), job)

	got := s.soleJob(t)
	if got.Stage != StageFailed {
		t.Fatalf("stage = %s, want %s", got.Stage, StageFailed)
	}
	if _, err := os.Stat(filepath.Join(src, "game.exe")); err != nil {
		t.Fatalf("source deleted despite failed verification: %v", err)
	}
}

func TestRecoverRepointRetriesSettingsSave(t *testing.T) {
	root := t.TempDir()
	oldRoot := filepath.Join(root, "old")
	newRoot := filepath.Join(root, "new")
	if err := os.MkdirAll(newRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	set := newTestSettings(t)
	// Point LibraryPath back at oldRoot the way an interrupted MoveLibrary
	// would leave it: every directory already moved, only the settings
	// write did not land before the crash.
	cfg := set.GetSettings()
	cfg.LibraryPath = oldRoot
	if err := set.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}

	job := Job{ID: "j1", Scope: ScopeLibrary, Stage: StageRepoint, GameID: itemSettings, Source: oldRoot, Target: newRoot}
	s := newRecoverTestService(t, job)
	s.settings = set

	s.recoverJob(context.Background(), job)

	if got := s.List(); len(got) != 0 {
		t.Fatalf("job should complete, got %+v", got)
	}
	if got := set.GetSettings().LibraryPath; got != newRoot {
		t.Fatalf("libraryPath = %q, want %q", got, newRoot)
	}
}
