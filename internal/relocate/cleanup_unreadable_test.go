package relocate

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"typhon/internal/hashdir"
	"typhon/internal/platform"
)

// unreadableAt makes verifyTree report issues for files of root, as a walk
// does for a file another process holds locked (Windows), on top of what the
// real walk finds.
func unreadableAt(t *testing.T, root string, issues ...hashdir.Issue) {
	t.Helper()
	verifyTree = func(ctx context.Context, dir string, m hashdir.Manifest, p func(hashdir.Progress)) (hashdir.Result, error) {
		r, err := hashdir.Verify(ctx, dir, m, p)
		if err == nil && platform.SamePath(dir, root) {
			r.Issues = append(r.Issues, issues...)
		}
		return r, err
	}
	t.Cleanup(func() { verifyTree = hashdir.Verify })
}

// An old folder with a file the cleanup cannot read is not a changed folder:
// the resumed cleanup deletes nothing and fails as a failed delete does, an
// attempt spent and the job resumable at the cleanup for the next start.
// Only a file it gained or that differs ends the job by relocate.source_changed.
func TestResumedCleanupLeavesAnOldFolderItCannotReadForTheNextStart(t *testing.T) {
	locked := hashdir.Issue{Path: "game.exe", Kind: hashdir.IssueUnreadable}
	for _, tc := range []struct {
		name      string
		issues    []hashdir.Issue
		wantStage Stage
		wantCode  string
	}{
		{"a locked file", []hashdir.Issue{locked}, StageCleanup, ""},
		{"a locked file and a changed one", []hashdir.Issue{locked, {Path: "data.bin", Kind: hashdir.IssueCorrupted}}, "", "relocate.source_changed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRecoveryFixture(t, StageFailed, StageCleanup)
			if _, err := f.lib.Relocate(f.game.ID, f.target); err != nil {
				t.Fatal(err)
			}
			unreadableAt(t, f.src, tc.issues...)
			srcBefore := snapshotTree(t, f.src)

			f.s.recoverAll(context.Background())

			if !reflect.DeepEqual(srcBefore, snapshotTree(t, f.src)) {
				t.Fatal("old folder touched")
			}
			got := f.s.soleJob(t)
			if got.Stage != StageFailed || got.FailedAt != tc.wantStage {
				t.Fatalf("job = %s/%q %q, want failed at %q", got.Stage, got.FailedAt, got.Error, tc.wantStage)
			}
			if tc.wantCode == "" {
				if strings.Contains(got.Error, "relocate.source_changed") || got.RecoveryAttempts != 1 {
					t.Fatalf("job error %q attempts %d, want a retryable failure that spent one attempt", got.Error, got.RecoveryAttempts)
				}
				return
			}
			if !strings.Contains(got.Error, tc.wantCode) {
				t.Fatalf("job error %q, want %s", got.Error, tc.wantCode)
			}
		})
	}
}
