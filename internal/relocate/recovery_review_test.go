package relocate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"typhon/internal/hashdir"
	"typhon/internal/library"
)

type recoveryFixture struct {
	s        *Service
	lib      *library.Service
	game     library.Game
	src      string
	target   string
	job      Job
	manifest hashdir.Manifest
}

func newRecoveryFixture(t *testing.T, stage, failedAt Stage) recoveryFixture {
	t.Helper()
	lib := newTestLibrary(t)
	game, src := addTestGame(t, lib)
	target := filepath.Join(t.TempDir(), "target")
	mustMkdirFile(t, target, "game.exe", "stub")
	manifest, err := hashdir.Build(context.Background(), src, nil)
	if err != nil {
		t.Fatal(err)
	}
	job := Job{ID: "j1", Scope: ScopeGame, Stage: stage, FailedAt: failedAt, Error: "boom", GameID: game.ID, Source: src, Target: target}
	s := newRecoverTestService(t, job)
	s.lib = lib
	if err := s.st.saveManifest(job.ID, manifest); err != nil {
		t.Fatal(err)
	}
	return recoveryFixture{s, lib, game, src, target, job, manifest}
}

func TestInterruptedRecoveryChangesNothing(t *testing.T) {
	cases := []struct {
		name     string
		failedAt Stage
		atNew    bool
	}{
		{"repoint retry", StageRepoint, false},
		{"cleanup retry", StageCleanup, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newRecoveryFixture(t, StageFailed, tc.failedAt)
			if tc.atNew {
				if _, err := f.lib.Relocate(f.game.ID, f.target); err != nil {
					t.Fatal(err)
				}
				manifest, err := hashdir.Build(context.Background(), f.target, nil)
				if err != nil {
					t.Fatal(err)
				}
				if err := f.s.st.saveManifest(f.job.ID, manifest); err != nil {
					t.Fatal(err)
				}
			}
			srcBefore, targetBefore := snapshotTree(t, f.src), snapshotTree(t, f.target)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			f.s.recoverJob(ctx, f.job)

			got := f.s.soleJob(t)
			if got.Stage != StageFailed || got.FailedAt != tc.failedAt || got.RecoveryAttempts != 0 || got.Error != "boom" {
				t.Fatalf("job = %s/%q attempts=%d error=%q, want it unchanged", got.Stage, got.FailedAt, got.RecoveryAttempts, got.Error)
			}
			if !reflect.DeepEqual(srcBefore, snapshotTree(t, f.src)) || !reflect.DeepEqual(targetBefore, snapshotTree(t, f.target)) {
				t.Fatal("an interrupted recovery touched the data")
			}
		})
	}
}

func TestUnreadableSourceWalkStaysRetryable(t *testing.T) {
	f := newRecoveryFixture(t, StageFailed, StageRepoint)
	verifyTree = func(context.Context, string, hashdir.Manifest, func(hashdir.Progress)) (hashdir.Result, error) {
		return hashdir.Result{}, errors.New("walk failed: access denied")
	}
	t.Cleanup(func() { verifyTree = hashdir.Verify })
	targetBefore := snapshotTree(t, f.target)

	f.s.recoverJob(context.Background(), f.job)

	got := f.s.soleJob(t)
	if got.Stage != StageFailed || got.FailedAt != StageRepoint {
		t.Fatalf("job = %s/%q, want a retryable failed job (error %q)", got.Stage, got.FailedAt, got.Error)
	}
	if strings.Contains(got.Error, "relocate.source_changed") {
		t.Fatalf("a walk failure was reported as a changed source: %q", got.Error)
	}
	if !reflect.DeepEqual(targetBefore, snapshotTree(t, f.target)) {
		t.Fatal("target touched")
	}
}

func TestDamagedSourceKeepsBothCopies(t *testing.T) {
	cases := []struct {
		name       string
		damage     func(t *testing.T, src string)
		wantTarget bool
		wantCode   string
	}{
		{"file went missing", func(t *testing.T, src string) {
			if err := os.Remove(filepath.Join(src, "game.exe")); err != nil {
				t.Fatal(err)
			}
		}, true, "relocate.source_damaged"},
		{"content changed", func(t *testing.T, src string) {
			mustMkdirFile(t, src, "game.exe", "player progress")
		}, false, "relocate.source_changed"},
		{"extra file", func(t *testing.T, src string) {
			mustMkdirFile(t, src, "save.dat", "fresh")
		}, false, "relocate.source_changed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newRecoveryFixture(t, StageFailed, StageRepoint)
			tc.damage(t, f.src)
			srcBefore := snapshotTree(t, f.src)

			f.s.recoverJob(context.Background(), f.job)

			if !reflect.DeepEqual(srcBefore, snapshotTree(t, f.src)) {
				t.Fatal("source touched")
			}
			if got := exists(f.target); got != tc.wantTarget {
				t.Fatalf("target exists = %v, want %v", got, tc.wantTarget)
			}
			got := f.s.soleJob(t)
			if got.Stage != StageFailed || got.FailedAt != "" || !strings.Contains(got.Error, tc.wantCode) {
				t.Fatalf("job = %s/%q error %q, want terminal %s", got.Stage, got.FailedAt, got.Error, tc.wantCode)
			}
		})
	}
}

func TestSourceDamageClassification(t *testing.T) {
	cases := []struct {
		name   string
		result hashdir.Result
		want   bool
	}{
		{"clean", hashdir.Result{}, false},
		{"extra files only", hashdir.Result{Extra: []string{"save.dat"}}, false},
		{"content changed", hashdir.Result{Issues: []hashdir.Issue{{Path: "a", Kind: hashdir.IssueCorrupted}}}, false},
		{"size changed", hashdir.Result{Issues: []hashdir.Issue{{Path: "a", Kind: hashdir.IssueSize}}}, false},
		{"missing", hashdir.Result{Issues: []hashdir.Issue{{Path: "a", Kind: hashdir.IssueMissing}}}, true},
		{"unreadable", hashdir.Result{Issues: []hashdir.Issue{{Path: "a", Kind: hashdir.IssueUnreadable}}}, true},
		{"changed and unreadable", hashdir.Result{Issues: []hashdir.Issue{
			{Path: "a", Kind: hashdir.IssueCorrupted}, {Path: "b", Kind: hashdir.IssueUnreadable},
		}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sourceDamaged(tc.result); got != tc.want {
				t.Fatalf("sourceDamaged = %v, want %v", got, tc.want)
			}
		})
	}
}

func breakRelocate(t *testing.T, f recoveryFixture) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(f.target, library.MarkerName), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestInterruptedRepointJobStopsAfterThreeStarts(t *testing.T) {
	f := newRecoveryFixture(t, StageRepoint, "")
	breakRelocate(t, f)
	srcBefore := snapshotTree(t, f.src)

	for start := 1; start <= 2; start++ {
		f.s.recoverJob(context.Background(), f.s.soleJob(t))
		got := f.s.soleJob(t)
		if got.Stage != StageFailed || got.FailedAt != StageRepoint || got.RecoveryAttempts != start {
			t.Fatalf("start %d: job = %s/%q attempts=%d, want failed/repoint with %d attempts", start, got.Stage, got.FailedAt, got.RecoveryAttempts, start)
		}
	}
	f.s.recoverJob(context.Background(), f.s.soleJob(t))

	got := f.s.soleJob(t)
	if got.Stage != StageFailed || got.FailedAt != "" || !strings.Contains(got.Error, "relocate.retries_exhausted") {
		t.Fatalf("job = %s/%q error %q, want terminal after three starts", got.Stage, got.FailedAt, got.Error)
	}
	if !reflect.DeepEqual(srcBefore, snapshotTree(t, f.src)) {
		t.Fatal("source changed")
	}
}

func TestRecoveryAttemptIsPersistedBeforeTheAction(t *testing.T) {
	f := newRecoveryFixture(t, StageCleanup, "")
	if _, err := f.lib.Relocate(f.game.ID, f.target); err != nil {
		t.Fatal(err)
	}
	manifest, err := hashdir.Build(context.Background(), f.target, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.s.st.saveManifest(f.job.ID, manifest); err != nil {
		t.Fatal(err)
	}
	seen := -1
	removeTree = func(path string) error {
		journal, err := f.s.st.loadJournal()
		if err != nil {
			t.Error(err)
			return err
		}
		if len(journal) == 1 {
			seen = journal[0].RecoveryAttempts
		}
		return os.RemoveAll(path)
	}
	t.Cleanup(func() { removeTree = os.RemoveAll })

	f.s.recoverJob(context.Background(), f.job)

	if seen != 1 {
		t.Fatalf("attempts on disk when the cleanup ran = %d, want 1", seen)
	}
}
